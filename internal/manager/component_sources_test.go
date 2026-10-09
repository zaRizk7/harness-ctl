package manager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitComponentSourcesRescanWithoutClaimingOwnership(t *testing.T) {
	e, _ := testEngine(t)
	root := filepath.Join(e.cfg.Home, "project", ".claude")
	if err := atomicWrite(filepath.Join(root, "skills", "external", "SKILL.md"), []byte("external skill"), 0600); err != nil {
		t.Fatal(err)
	}
	e.cfg.ComponentSources = map[string]map[string]string{"claude": {"project": root}}
	items, err := e.components(installation{Harness: "claude"}, "source:project", skills)
	if err != nil || len(items) != 1 || items[0].Name != "external" {
		t.Fatal(items, err)
	}
	var out bytes.Buffer
	if err := e.cli(context.Background(), []string{"components", "list", "--scope", "source:project", "claude", "skills"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "external") {
		t.Fatal(out.String())
	}
	p := &plan{Spec: e.cfg.Harnesses[1], Install: installation{Harness: "claude"}, Request: request{Component: &componentRequest{Scope: "source:project", Category: skills, Operation: "remove", Path: items[0].Path}}}
	if e.planComponent(p) == nil {
		t.Fatal("unowned source mutation accepted")
	}
}

func TestManualProjectPluginLedgerIsVisibleWithoutPayloadOwnership(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1")
	payload := filepath.Join(e.cfg.Home, "project", ".claude", "external")
	ledger := map[string]any{"plugins": map[string]any{"demo@market": []any{map[string]any{"scope": "project", "installPath": payload, "projectPath": filepath.Dir(payload)}}}}
	if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "installed_plugins.json"), ledger); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "base", plugins)
	if err != nil || len(items) != 1 {
		t.Fatal("manual project registration omitted", items, err)
	}
}

func TestExplicitComponentSourceSafetyAndTUI(t *testing.T) {
	e, _ := testEngine(t)
	root := filepath.Join(e.cfg.Home, "project", ".codex")
	if err := atomicWrite(filepath.Join(root, "skills", "project", "SKILL.md"), []byte("project"), 0600); err != nil {
		t.Fatal(err)
	}
	e.cfg.ComponentSources = map[string]map[string]string{"codex": {"project": root, "system": filepath.Join(e.cfg.Home, "system")}}
	if err := e.cfg.validateComponentSources(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]map[string]string{{"unknown": {"project": root}}, {"codex": {"../bad": root}}, {"codex": {"project": "/"}}, {"codex": {"project": e.cfg.Home}}, {"codex": {"project": "relative"}}} {
		c := e.cfg
		c.ComponentSources = bad
		if c.validate() == nil {
			t.Fatal("unsafe component source")
		}
	}
	if _, _, err := e.componentEngine(installation{Harness: "codex"}, "source:missing"); err == nil {
		t.Fatal("unknown source")
	}
	linked := filepath.Join(e.cfg.Home, "linked-source")
	if err := os.Symlink(root, linked); err != nil {
		t.Fatal(err)
	}
	e.cfg.ComponentSources["codex"]["linked"] = linked
	if _, _, err := e.componentEngine(installation{Harness: "codex"}, "source:linked"); err == nil {
		t.Fatal("linked source")
	}
	delete(e.cfg.ComponentSources["codex"], "linked")
	m := newModel(e)
	m.screen = "components"
	m.componentScope = "base"
	m.componentCat = skills
	if len(m.componentScopes()) != 3 {
		t.Fatal(m.componentScopes())
	}
	model, cmd := m.componentKey("p")
	m = model.(tuiModel)
	if cmd == nil || m.componentScope != "source:project" {
		t.Fatal("source not selectable")
	}
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if len(m.componentItems) != 1 {
		t.Fatal(m.componentItems)
	}
	if !strings.Contains(m.View().Content, "Read-only") {
		t.Fatal(m.View().Content)
	}
	for _, key := range []string{"a", "e", "enter", "d", "u", "space", "x", "t"} {
		if _, cmd := m.componentKey(key); cmd != nil {
			t.Fatal("read-only source dispatched mutation", key)
		}
	}
	m.componentItems[0].ReadOnly = true
	m.componentItems[0].Note = "external payload"
	m.componentScope = "base"
	if !strings.Contains(m.View().Content, "external read-only") {
		t.Fatal(m.View().Content)
	}
	if _, cmd := m.componentKey("x"); cmd != nil {
		t.Fatal("unowned payload removed")
	}
	e.cfg.ComponentSources = nil
	m.componentItems = nil
	_, _ = m.componentKey("p")
	inst := syntheticInstall(t, e, e.cfg.Harnesses[0], "1")
	m.installs = []installation{inst}
	e.reg.Profiles[inst.ID] = profile{Root: filepath.Join(e.cfg.Root, "profiles", inst.ID)}
	if len(m.componentScopes()) != 2 {
		t.Fatal("profile source omitted")
	}
}
