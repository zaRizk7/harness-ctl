package manager

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zaRizk7/harness-ctl/internal/library"
)

func TestLibraryScreensOwnershipAndRecordApproval(t *testing.T) {
	e, inst, _ := componentFixture(t)
	item := reusableSkill("demo")
	item.Targets = map[string]library.Target{inst.Harness: {Path: "skills/demo"}, "codex": {Path: "skills/demo", HomeSkills: true}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	m.installs = []installation{inst}
	codexSpec, _ := e.specFor("codex")
	if err := atomicWrite(filepath.Join(e.managedStateRoot(codexSpec), "skills", "present"), []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(m.loadLibraryView()())
	m = updated.(tuiModel)
	for _, key := range []string{"r", "d", "u", "x"} {
		updated, cmd := m.key(keyMessage(key))
		m = updated.(tuiModel)
		if key == "r" {
			updated, _ = m.Update(cmd())
			m = updated.(tuiModel)
			continue
		}
		if _, cmd, _ = m.libraryConfirm(); cmd != nil {
			t.Fatal("record change skipped approval")
		}
		m.typed = "apply"
		updated, cmd, _ = m.libraryConfirm()
		m = completeUIOperation(t, updated.(tuiModel), cmd)
		updated, _ = m.Update(m.loadLibraryView()())
		m = updated.(tuiModel)
	}
	if len(m.libraryItems) != 0 || !strings.Contains(m.View().Content, "No reusable entries") {
		t.Fatal(m.libraryItems)
	}
	if _, _, handled := m.libraryKey("enter"); !handled {
		t.Fatal("empty library key not handled")
	}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(m.loadLibraryView()())
	m = updated.(tuiModel)
	m.libraryItems[0].Enabled = false
	updated, _, _ = m.libraryKey("enter")
	m = updated.(tuiModel)
	if !strings.Contains(m.status, "Enable") {
		t.Fatal(m.status)
	}
	m.libraryItems[0].Enabled = true
	updated, _, _ = m.libraryKey("enter")
	m = updated.(tuiModel)
	m.librarySelected = map[string]bool{inst.Harness: true, "codex": true}
	updated, _, _ = m.libraryKey("o")
	m = updated.(tuiModel)
	if m.screen != "library-owners" || len(m.owners) == 0 || !strings.Contains(m.View().Content, "affected owner") {
		t.Fatal(m.screen, m.owners)
	}
	for _, key := range []string{"space", "space", "space", "enter"} {
		updated, _, _ = m.libraryKey(key)
		m = updated.(tuiModel)
	}
	if m.screen != "library-select" || len(m.req.Owners) != 1 {
		t.Fatal(m.screen, m.req.Owners)
	}
	m.screen = "library-record-preview"
	m.libraryAction = "save"
	path, _ := e.libraryPath()
	before, _ := fingerprint(path)
	m.libraryEdit = libraryEditMsg{item: item, before: before}
	m.typed = "apply"
	if !strings.Contains(m.View().Content, "demo") {
		t.Fatal("save preview omitted entry")
	}
	updated, cmd, _ := m.libraryConfirm()
	completeUIOperation(t, updated.(tuiModel), cmd)
	for _, msg := range []tea.Msg{libraryMsg{err: errors.New("fixture")}, libraryEditMsg{err: errors.New("fixture")}, libraryApplyMsg{err: errors.New("fixture")}, libraryEditMsg{cancelled: true}, libraryEditMsg{item: item, before: before}} {
		updated, _ = m.Update(msg)
		m = updated.(tuiModel)
		if edit, ok := msg.(libraryEditMsg); ok && edit.item.ID != "" && m.screen != "library-record-preview" {
			t.Fatal("save preview not created")
		}
	}
}

// libraryEditorResultModel exits only after the editor callback is processed.
type libraryEditorResultModel struct {
	tuiModel
	first tea.Cmd
}

func (m libraryEditorResultModel) Init() tea.Cmd { return m.first }
func (m libraryEditorResultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.tuiModel.Update(msg)
	m.tuiModel = model.(tuiModel)
	if _, done := msg.(libraryEditMsg); done {
		return m, tea.Quit
	}
	return m, cmd
}

func TestLibraryEditorRejectsInvalidChangedRequests(t *testing.T) {
	for _, scenario := range []string{"failure", "missing", "invalid", "trailing", "oversize", "invalid-recipe", "valid"} {
		t.Run(scenario, func(t *testing.T) {
			e, _ := testEngine(t)
			m := newModel(e)
			if err := e.saveLibraryItem(reusableSkill("demo"), ""); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			t.Setenv("VISUAL", "")
			script := filepath.Join(dir, "edit")
			body := "#!/bin/sh\n"
			switch scenario {
			case "failure":
				body += "exit 1\n"
			case "missing":
				body += "/bin/rm \"$1\"\n"
			case "invalid":
				body += "printf bad > \"$1\"\n"
			case "trailing":
				body += "printf '{} {}' > \"$1\"\n"
			case "oversize":
				e.cfg.MetadataBytes = 1024
				body += "printf '%2048s' x > \"$1\"\n"
			case "invalid-recipe":
				body += "printf '%s' '{\"id\":\"../bad\",\"category\":\"skills\",\"targets\":{\"pi\":{\"path\":\"skills/a\"}}}' > \"$1\"\n"
			case "valid":
				body += "printf '%s' '{\"id\":\"edited\",\"category\":\"skills\",\"enabled\":true,\"targets\":{\"pi\":{\"path\":\"skills/a\"}},\"files\":{\"SKILL.md\":\"YQ==\"}}' > \"$1\"\n"
			}
			if err := os.WriteFile(script, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("EDITOR", shellQuote(script))
			cmd := m.editLibrary("demo")
			program := tea.NewProgram(libraryEditorResultModel{m, cmd}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
			model, err := program.Run()
			if err != nil {
				t.Fatal(err)
			}
			result := model.(libraryEditorResultModel).tuiModel
			if scenario == "valid" {
				if result.screen != "library-record-preview" || result.libraryEdit.item.ID != "edited" {
					t.Fatal(result.screen, result.status)
				}
			} else if result.status == "" {
				t.Fatal("invalid editor result accepted", result.screen)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 {
				t.Fatal("private request leaked", files, err)
			}
		})
	}
}

func TestLibraryEditorReadFailuresDoNotLeakRequests(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	for _, kind := range []string{"absent", "directory"} {
		path := filepath.Join(t.TempDir(), "request")
		if kind == "directory" {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
		msg := m.finishLibraryEdit(path, nil, library.Item{}, "", nil)
		if msg.err == nil {
			t.Fatal("unreadable request accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("request retained", err)
		}
	}
}
