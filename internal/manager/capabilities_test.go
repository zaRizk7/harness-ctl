package manager

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCapabilitiesHideMissingInstallAndState(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	if ids := m.actions(); !slices.Equal(ids, []string{"install"}) {
		t.Fatal(ids)
	}
	s := e.cfg.Harnesses[0]
	if err := atomicWrite(filepath.Join(e.stateRoot(s), "skills", "demo", "SKILL.md"), []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	if ids := m.actions(); !slices.Equal(ids, []string{"install", "manage"}) {
		t.Fatal(ids)
	}
	m.installs = []installation{syntheticInstall(t, e, s, "1")}
	if ids := m.actions(); slices.Contains(ids, "migrate") || slices.Contains(ids, "install") || !slices.Contains(ids, "uninstall") {
		t.Fatal(ids)
	}
}

func TestRetainedHomeSkillsKeepManagementVisible(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	for i, s := range e.cfg.Harnesses {
		if s.ID == "codex" {
			m.harness = i
		}
	}
	for _, dir := range []string{"skills", filepath.Join(disabledComponentsDir, "skills")} {
		path := filepath.Join(e.cfg.Home, ".agents", dir, "demo", "SKILL.md")
		if err := atomicWrite(path, []byte("retained"), 0600); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(m.actions(), "manage") {
			t.Fatal("retained HOME skills hidden", m.actions())
		}
		if err := fileIO.removeAll(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPreservationOptionsContainOnlyPresentCategories(t *testing.T) {
	e, inst, _ := componentFixture(t)
	m := newModel(e)
	m.installs = []installation{inst}
	for i, s := range e.cfg.Harnesses {
		if s.ID == inst.Harness {
			m.harness = i
		}
	}
	msg := m.loadOptions()().(optionsMsg)
	if !slices.Contains(msg.categories, mcp) || slices.Contains(msg.categories, history) {
		t.Fatal(msg.categories)
	}
}

func actionIndex(m tuiModel, id string) int { return slices.Index(m.actions(), id) }

func TestProfileControlsOnlyPresentStateCategories(t *testing.T) {
	e, inst, _ := componentFixture(t)
	m := newModel(e)
	m.installs = []installation{inst}
	for i, s := range e.cfg.Harnesses {
		if s.ID == inst.Harness {
			m.harness = i
		}
	}
	m.screen = "actions"
	m.cursor = actionIndex(m, "profile")
	model, cmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if cmd != nil {
		model, _ = m.Update(cmd())
		m = model.(tuiModel)
	}
	if !strings.Contains(m.View().Content, "Exclude mcp") || strings.Contains(m.View().Content, "Exclude history") {
		t.Fatal(m.View().Content)
	}
}
