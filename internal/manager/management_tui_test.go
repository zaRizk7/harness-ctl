package manager

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSelfRemovalStateControlsAndOwners(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.screen = "self-options"
	m.selfRemoveHarnesses = true
	for _, row := range []int{2, 3} {
		m.cursor = row
		model, _, _ := m.managementKey("space")
		m = model.(tuiModel)
	}
	if !strings.Contains(m.View().Content, "Discard harness user state") || !strings.Contains(m.View().Content, "erase harness recovery") {
		t.Fatal(m.View().Content)
	}
	model, _, _ := m.managementKey("o")
	m = model.(tuiModel)
	if m.screen != "self-owners" {
		t.Fatal(m.screen)
	}
}

func TestTUIBatchSelectionAndPreview(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	for _, id := range []string{"codex", "claude"} {
		s, _ := e.specFor(id)
		e.reg.Installs = append(e.reg.Installs, syntheticInstall(t, e, s, "1.0.0"))
	}
	_ = writeJSON(e.statePath, e.reg)
	m.installs = e.reg.Installs
	updated, _ := m.key(keyMessage("b"))
	m = updated.(tuiModel)
	if m.screen != "batch-select" {
		t.Fatal(m.screen)
	}
	updated, _ = m.key(keyMessage("space"))
	m = updated.(tuiModel)
	m.cursor = 1
	updated, _ = m.key(keyMessage("space"))
	m = updated.(tuiModel)
	updated, _ = m.key(keyMessage("enter"))
	m = updated.(tuiModel)
	if m.screen != "batch-actions" {
		t.Fatal(m.screen)
	}
	m.cursor = slices.Index(m.batchActions(), "uninstall")
	updated, cmd := m.key(keyMessage("enter"))
	m = updated.(tuiModel)
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	updated, cmd = m.key(keyMessage("enter"))
	m = updated.(tuiModel)
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	if m.screen != "preview" || m.batch == nil || len(m.batch.Plans) != 2 {
		t.Fatal(m.screen, m.batch)
	}
	if !strings.Contains(m.View().Content, "2 harnesses") {
		t.Fatal(m.View().Content)
	}
}

func TestTUIAccountsAndSelfControls(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	updated, cmd := m.key(keyMessage("a"))
	m = updated.(tuiModel)
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	if m.screen != "accounts" {
		t.Fatal(m.screen)
	}
	m.screen = "home"
	updated, _ = m.key(keyMessage("u"))
	m = updated.(tuiModel)
	if m.screen != "self-options" {
		t.Fatal(m.screen)
	}
	m.cursor = 1
	updated, _ = m.key(keyMessage("space"))
	m = updated.(tuiModel)
	if !m.selfRemoveState {
		t.Fatal("state toggle ignored")
	}
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	_ = atomicWrite(binary, []byte("fixture"), 0700)
	p, err := e.buildSelfPlan(context.Background(), binary, false, true, request{Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(selfPlanMsg{plan: p})
	m = updated.(tuiModel)
	if m.screen != "self-preview" || m.self.Binary != binary || !strings.Contains(m.View().Content, "REMOVE executable:") {
		t.Fatal(m.screen, m.View().Content)
	}
}

func keyMessage(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return keyPress(k)
}
