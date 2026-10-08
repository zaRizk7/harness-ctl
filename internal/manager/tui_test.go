package manager

import (
	"errors"
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
	m.req = request{Harness: "codex", Preserve: keepAll()}
	m.owners = []string{"Claude Code", "Codex desktop / IDE"}
	m.cursor = 3 + len(categories)
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = model.(tuiModel)
	if !contains(m.req.Owners, "Claude Code") || contains(m.req.Owners, "Codex desktop / IDE") {
		t.Fatal("owner toggle approved additional owners")
	}
}
