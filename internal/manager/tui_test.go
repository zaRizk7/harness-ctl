package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyPress(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: text, Code: []rune(text)[0]} }

func TestTUIRequiresTypedApprovalAndShowsBusy(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	m := newModel(e)
	m.screen = "preview"
	m.req = p.Request
	m.p = p
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter executed an unconfirmed preview")
	}
	m = model.(tuiModel)
	m.typed = "apply"
	model, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("typed confirmation did not execute")
	}
	m = model.(tuiModel)
	if m.screen != "busy" {
		t.Fatalf("operation did not show busy screen: %s", m.screen)
	}
	for m.screen == "busy" {
		model, next := m.Update(cmd())
		m = model.(tuiModel)
		cmd = next
	}
	if m.screen != "result" {
		t.Fatalf("operation did not finish: %s", m.screen)
	}
}

func TestInventoryRefreshDoesNotHideOperationError(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.screen = "busy"
	model, _ := m.Update(doneMsg{err: errors.New("synthetic operation failure")})
	m = model.(tuiModel)
	model, _ = m.Update(inventoryMsg{})
	m = model.(tuiModel)
	if !strings.Contains(m.View().Content, "synthetic operation failure") {
		t.Fatal("inventory refresh hid the operation failure")
	}
}

func TestSharedOwnerIsAnIndividualOption(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.screen = "options"
	m.req = request{Harness: "codex", Action: "update", Preserve: keepAll()}
	m.optionCategories = categories
	m.owners = []string{"Claude Code", "Codex desktop / IDE"}
	m.cursor = 3 + len(categories)
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = model.(tuiModel)
	if !contains(m.req.Owners, "Claude Code") || contains(m.req.Owners, "Codex desktop / IDE") {
		t.Fatal("owner toggle approved additional owners")
	}
}

func TestTUIOffersDedicatedComponentManagement(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.screen = "actions"
	m.installs = []installation{syntheticInstall(t, e, e.cfg.Harnesses[0], "1")}
	if !strings.Contains(m.View().Content, "Manage components") {
		t.Fatal("there is no dedicated component management action")
	}
}

func TestTUIComponentNavigationOwnersAndApproval(t *testing.T) {
	e, inst, path := componentFixture(t)
	m := newModel(e)
	for i, s := range catalog {
		if s.ID == inst.Harness {
			m.harness = i
		}
	}
	m.installs = []installation{inst}
	m.screen, m.cursor = "actions", actionIndex(m, "manage")
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(tuiModel)
	if m.screen != "component-groups" || !strings.Contains(m.View().Content, "mcp") {
		t.Fatal("component categories are absent")
	}
	m.cursor = 1
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "components" || len(m.componentItems) != 2 {
		t.Fatal("category did not list entries")
	}
	m.owners = []string{"one", "two"}
	model, _ = m.Update(keyPress("o"))
	m = model.(tuiModel)
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = model.(tuiModel)
	if !contains(m.req.Owners, "one") || contains(m.req.Owners, "two") {
		t.Fatal("owner selection approved another owner")
	}
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(tuiModel)
	model, cmd = m.Update(keyPress("d"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "preview" {
		t.Fatal("disable did not create a preview", m.status)
	}
	if strings.Contains(m.View().Content, "KEEP "+path) {
		t.Fatal("preview incorrectly labels changed configuration KEEP")
	}
	model, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("component mutation did not require typed approval")
	}
	_ = model
}

func TestTUIBrowsesAndEditsDisabledDirectory(t *testing.T) {
	e, inst, _ := componentFixture(t)
	path := filepath.Join(inst.StateRoot, "skills", "demo")
	if err := atomicWrite(filepath.Join(path, "SKILL.md"), []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	applyComponentRequest(t, e, inst, componentRequest{Category: skills, Path: path, Operation: "disable"})
	items, err := e.components(inst, "base", skills)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	for i, s := range catalog {
		if s.ID == inst.Harness {
			m.harness = i
		}
	}
	m.installs, m.req = []installation{inst}, request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage"}
	m.screen, m.componentScope, m.componentCat, m.componentItems = "components", "base", skills, items
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(tuiModel)
	if cmd != nil || len(m.componentItems) != 1 || m.componentItems[0].Subpath != "SKILL.md" {
		t.Fatal("disabled directory did not expose editable files", m.componentItems)
	}
	item := m.componentItems[0]
	applyComponentRequest(t, e, inst, componentRequest{Category: skills, Path: item.Path, Parked: item.Parked, Subpath: item.Subpath, Operation: "edit", Content: []byte("edited")})
	applyComponentRequest(t, e, inst, componentRequest{Category: skills, Path: item.Path, Parked: item.Parked, Operation: "enable"})
	data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil || string(data) != "edited" {
		t.Fatal("disabled file edit was lost", err)
	}
}

func TestTUIComponentErrorsStayInManagement(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.req.Action = "manage"
	model, _ := m.Update(planMsg{err: errors.New("invalid registration")})
	m = model.(tuiModel)
	if m.screen != "components" || !strings.Contains(m.View().Content, "invalid registration") {
		t.Fatal("failed preview discarded management context")
	}
}

func TestComponentEditorValidationCancellationAndCleanup(t *testing.T) {
	e, _, _ := componentFixture(t)
	m := newModel(e)
	m.componentCat = mcp
	for _, mode := range []string{"valid", "unknown", "cancel", "failed", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			initial := []byte(`{"operation":"add","category":"mcp","field":"/mcpServers/new","value":{}}`)
			edited := []byte(`{"operation":"add","category":"mcp","field":"/mcpServers/edited","value":{}}`)
			var editorErr error
			switch mode {
			case "unknown":
				edited = []byte(`{"operation":"add","category":"mcp","typo":true}`)
			case "cancel":
				edited = initial
			case "failed":
				editorErr = errors.New("synthetic editor failure")
			case "oversize":
				edited = make([]byte, e.cfg.MetadataBytes+1)
			}
			path := filepath.Join(t.TempDir(), "edit.json")
			if err := atomicWrite(path, edited, 0600); err != nil {
				t.Fatal(err)
			}
			msg := m.finishComponentEdit(path, initial, componentRequest{}, nil, "", editorErr)
			if mode == "valid" && (msg.err != nil || msg.change.Field != "/mcpServers/edited") {
				t.Fatal(msg.err)
			}
			if mode == "cancel" && !msg.cancelled {
				t.Fatal("unchanged editor result created a mutation")
			}
			if mode != "valid" && mode != "cancel" && msg.err == nil {
				t.Fatal("invalid editor result accepted", mode)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("editor temporary file was retained")
			}
		})
	}
}

func TestComponentEditorRejectsChangedDisabledSource(t *testing.T) {
	e, inst, path := componentFixture(t)
	applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "disable"})
	item := disabledItem(t, e, inst, mcp, "/mcpServers/one")
	before, err := componentEditFingerprint(item)
	if err != nil {
		t.Fatal(err)
	}
	data, err := e.componentEditData(inst, "base", item)
	if err != nil {
		t.Fatal(err)
	}
	var record parkedComponent
	if err := readJSON(filepath.Join(item.Parked, "meta.json"), &record); err != nil {
		t.Fatal(err)
	}
	record.Value = []byte(`{"command":"concurrent-change"}`)
	if err := writeJSON(filepath.Join(item.Parked, "meta.json"), record); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(t.TempDir(), "edit.json")
	if err := atomicWrite(tmp, []byte(`{"command":"edited"}`), 0600); err != nil {
		t.Fatal(err)
	}
	msg := newModel(e).finishComponentEdit(tmp, data, componentRequest{}, &item, before, nil)
	if msg.err == nil {
		t.Fatal("editor overwrote a concurrent disabled-state change")
	}
}
