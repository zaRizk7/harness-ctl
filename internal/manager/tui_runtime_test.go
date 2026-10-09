package manager

import (
	"bytes"
	"context"
	"errors"
	"github.com/zaRizk7/harness-ctl/internal/library"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type editorProgramModel struct {
	tuiModel
	first tea.Cmd
}

func (m editorProgramModel) Init() tea.Cmd { return tea.Sequence(m.first, tea.Quit) }

func TestNativeEditorCommandsUsePrivateRequests(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	t.Setenv("EDITOR", "/usr/bin/true")
	t.Setenv("VISUAL", "")
	t.Setenv("TMPDIR", t.TempDir())
	m.componentScope = "base"
	m.componentCat = mcp
	m.req = request{Harness: "codex", Action: "manage"}
	for _, command := range []tea.Cmd{m.editComponent(nil), m.editAccount("")} {
		program := tea.NewProgram(editorProgramModel{m, command}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
		if _, err := program.Run(); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil || len(files) != 0 {
		t.Fatal("editor requests leaked", files, err)
	}
}

func TestTUIScreensAndNavigationStayReadOnly(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	for _, s := range []string{"home", "actions", "options", "recovery", "profile", "component-groups", "components", "component-owners", "batch-select", "batch-actions", "self-options", "accounts", "result", "loading", "busy", "purge-preview", "restore-preview", "journal-preview", "account-preview"} {
		m.screen = s
		m.cursor = 0
		m.req = request{Harness: "codex", Action: "reset", Preserve: keepAll()}
		m.profileDisabled = map[category]bool{mcp: true}
		m.batchSelected = map[string]bool{"codex": true}
		m.selectedRecord = operationRecord{Action: "reset", Harness: "codex"}
		m.selectedBackup = snapshotMeta{ID: randomID()}
		_ = m.itemCount()
		_ = m.View()
		if s != "busy" && s != "loading" {
			model, _ := m.key(keyMessage("down"))
			m = model.(tuiModel)
			model, _ = m.key(keyMessage("up"))
			m = model.(tuiModel)
			model, _ = m.key(keyMessage("esc"))
			m = model.(tuiModel)
		}
	}
	if _, err := os.Stat(e.cfg.Root); !os.IsNotExist(err) {
		t.Fatal("navigation wrote manager state", err)
	}
}

func TestTUIMessageErrorsAndCancellation(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	bad := errors.New("fixture failure")
	for _, message := range []tea.Msg{inventoryMsg{err: bad}, optionsMsg{err: bad}, planMsg{err: bad}, recoveryMsg{err: bad}, snapshotMsg{err: bad}, batchPlanMsg{err: bad}, selfPlanMsg{err: bad}, accountEditMsg{err: bad}, componentsMsg{err: bad}, componentEditorMsg{err: bad}} {
		m.screen = "home"
		updated, _ := m.Update(message)
		m = updated.(tuiModel)
		if !strings.Contains(m.status, "fixture") {
			t.Fatal(message, m.status)
		}
	}
	for _, message := range []tea.Msg{componentEditorMsg{cancelled: true}, accountEditMsg{cancelled: true}, tea.WindowSizeMsg{Width: 40, Height: 10}, accountTick{}, accountsMsg{}} {
		updated, _ := m.Update(message)
		m = updated.(tuiModel)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.screen = "busy"
	updated, _ := m.Update(interruptMsg{})
	m = updated.(tuiModel)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("interrupt did not cancel")
	}
	m.screen = "home"
	_, cmd := m.Update(interruptMsg{})
	if cmd == nil {
		t.Fatal("idle interrupt did not quit")
	}
	m.screen = "home"
	updated, _ = m.Update(doneMsg{err: bad})
	m = updated.(tuiModel)
	if !strings.Contains(m.status, "fixture") {
		t.Fatal(m.status)
	}
	if msg := m.inventory()(); msg.(inventoryMsg).err != nil {
		t.Fatal(msg)
	}
	if msg := m.loadRecovery()(); msg.(recoveryMsg).err != nil {
		t.Fatal(msg)
	}
	m.screen = "loading"
	_, cmd = m.key(keyMessage("ctrl+c"))
	if cmd == nil {
		t.Fatal("loading cancellation ignored")
	}
}

func TestTUIPreservationAndScrollControls(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.req = request{Action: "update", Preserve: keepAll()}
	m.optionCategories = categories
	m.owners = []string{"owner"}
	for i := 0; i < 3; i++ {
		m.cursor = 0
		m.toggleOption()
	}
	if len(m.req.Preserve) != len(categories) {
		t.Fatal("preset cycle did not preserve all")
	}
	m.cursor = 1
	m.toggleOption()
	if !m.editing {
		t.Fatal("version editor not active")
	}
	m.req.Target = "1"
	updated, _ := m.key(keyMessage("backspace"))
	m = updated.(tuiModel)
	updated, _ = m.key(keyPress("2"))
	m = updated.(tuiModel)
	updated, _ = m.key(keyMessage("enter"))
	m = updated.(tuiModel)
	if m.req.Target != "2" || m.editing {
		t.Fatal(m.req.Target)
	}
	for _, i := range []int{2, 3, 3 + len(categories), 3 + len(categories)} {
		m.cursor = i
		m.toggleOption()
	}
	if !m.req.Permanent || len(m.req.Owners) != 0 {
		t.Fatal(m.req)
	}
	if cleanText("a\n\x1bb") != "ab" {
		t.Fatal("terminal controls retained")
	}
	if mark(true) != "[x]" || mark(false) != "[ ]" {
		t.Fatal("checkbox labels wrong")
	}
	if shortID("small") != "small" {
		t.Fatal("short identity damaged")
	}
	m.height = 12
	m.cursor = 9
	if len(m.listWindow([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10"})) == 0 {
		t.Fatal("scrolled list empty")
	}
	m.cursor = 0
	if len(m.listWindow([]string{"0", "1", "2", "3", "4", "5"})) != 5 {
		t.Fatal("continuation hint missing")
	}
	if len(m.scroll([]string{"0", "1", "2", "3", "4", "5"})) != 4 {
		t.Fatal("scroll height wrong")
	}
}

func TestAccountEditorApprovalAndStaleRequest(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	path := filepath.Join(t.TempDir(), "account.json")
	before, _ := fingerprint(filepath.Join(e.cfg.Root, "accounts.age"))
	raw := []byte(`{"id":"team","provider":"openai","kind":"subscription","enabled":true}`)
	_ = atomicWrite(path, raw, 0600)
	msg := m.finishAccountEdit(path, nil, before, nil)
	if msg.err != nil || msg.item.ID != "team" {
		t.Fatal(msg)
	}
	updated, _ := m.Update(msg)
	m = updated.(tuiModel)
	if _, cmd := m.confirm(); cmd != nil {
		t.Fatal("unapproved account edit executed")
	}
	m.typed = "apply"
	updated, cmd := m.confirm()
	m = updated.(tuiModel)
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	if m.screen != "accounts" || len(m.accountItems) != 1 {
		t.Fatal("approved account edit did not refresh", m.screen, m.status)
	}
	if err := e.saveAccountChecked(account{ID: "new", Provider: "openai", Kind: "subscription"}, before); err == nil {
		t.Fatal("stale account edit saved")
	}
	for _, test := range []struct {
		data      []byte
		editor    error
		unchanged bool
	}{{raw, errors.New("editor failure"), false}, {raw, nil, true}, {[]byte("invalid"), nil, false}, {[]byte(`{"id":"a","provider":"openai","kind":"subscription"} {}`), nil, false}, {[]byte(`{"id":"../bad","provider":"openai","kind":"subscription"}`), nil, false}} {
		path = filepath.Join(t.TempDir(), "account.json")
		_ = atomicWrite(path, test.data, 0600)
		beforeData := []byte(nil)
		if test.unchanged {
			beforeData = test.data
		}
		result := m.finishAccountEdit(path, beforeData, before, test.editor)
		if test.unchanged {
			if !result.cancelled {
				t.Fatal(result)
			}
		} else if result.err == nil {
			t.Fatal("bad editor request accepted", result)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("editor request retained", err)
		}
	}
}

func TestAccountScreenActionsAndRefresh(t *testing.T) {
	e, r := testEngine(t)
	_ = e.saveAccount(account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true})
	m := newModel(e)
	m.screen = "accounts"
	updated, _ := m.Update(m.loadAccountsView()())
	m = updated.(tuiModel)
	updated, _ = m.Update(m.loadGlobalMonitor()())
	m = updated.(tuiModel)
	if !strings.Contains(m.View().Content, "unavailable") {
		t.Fatal(m.View().Content)
	}
	for _, key := range []string{"b", "l", "k", "s", "r"} {
		updated, cmd := m.key(keyMessage(key))
		m = updated.(tuiModel)
		updated, _ = m.Update(cmd())
		m = updated.(tuiModel)
	}
	if len(r.calls) != 4 {
		t.Fatal("native account links not dispatched", r.calls)
	}
	for _, key := range []string{"d", "u", "x"} {
		updated, _ = m.key(keyMessage(key))
		m = updated.(tuiModel)
		m.typed = "apply"
		updated, cmd := m.confirm()
		m = updated.(tuiModel)
		updated, _ = m.Update(cmd())
		m = updated.(tuiModel)
	}
	if len(m.accountItems) != 0 {
		t.Fatal("account removal did not refresh")
	}
	m.e.cfg.RefreshSeconds = 1
	start := time.Now()
	msg := m.accountTimer()()
	if _, ok := msg.(accountTick); !ok || time.Since(start) < time.Second {
		t.Fatal("refresh interval not honored")
	}
	var out bytes.Buffer
	_ = outputJSON(&out, m.accountMetrics)
	if strings.Contains(out.String(), "credential") {
		t.Fatal("monitoring output exposed a credential")
	}
}

func TestLibraryEditorValidatesPrivateExistingRecords(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	item := library.Item{ID: "entry", Category: "skills", Enabled: true, Files: map[string][]byte{"SKILL.md": []byte("contents")}, Targets: map[string]library.Target{"pi": {Path: "skills/entry"}}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "/usr/bin/true")
	for _, id := range []string{"", "entry"} {
		program := tea.NewProgram(editorProgramModel{m, m.editLibrary(id)}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
		if _, err := program.Run(); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil || len(files) != 0 {
		t.Fatal("library editor request leaked", files, err)
	}
}
