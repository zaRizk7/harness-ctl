package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// componentModel returns a model scoped to a synthetic component installation.
func componentModel(t *testing.T) tuiModel {
	t.Helper()
	e, inst, _ := componentFixture(t)
	m := newModel(e)
	m.installs = []installation{inst}
	for i, s := range e.cfg.Harnesses {
		if s.ID == inst.Harness {
			m.harness = i
		}
	}
	m.req = request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Preserve: keepAll()}
	m.componentScope = "base"
	m.componentCat = mcp
	return m
}

func TestComponentScreenControlsKeepOperationsInPreview(t *testing.T) {
	m := componentModel(t)
	m.screen = "component-groups"
	m.cursor = 0
	model, cmd := m.componentKey("enter")
	m = model.(tuiModel)
	if cmd == nil || m.screen != "loading" {
		t.Fatal("category did not load")
	}
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "components" {
		t.Fatal(m.screen)
	}
	m.componentCat = mcp
	model, _ = m.Update(m.loadComponents()())
	m = model.(tuiModel)
	for _, k := range []string{"o", "space", "space", "enter"} {
		model, _ = m.componentKey(k)
		m = model.(tuiModel)
	}
	if m.screen != "components" || len(m.req.Owners) != 0 {
		t.Fatal("owner toggle not reversible")
	}
	model, _ = m.componentKey("p")
	m = model.(tuiModel)
	if !strings.Contains(m.status, "No active") {
		t.Fatal(m.status)
	}
	inst := m.componentInstallation()
	if err := m.e.createProfile(inst, map[category]bool{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"p", "p", "r"} {
		model, cmd = m.componentKey(k)
		m = model.(tuiModel)
		if cmd == nil {
			t.Fatal("missing refresh")
		}
		model, _ = m.Update(cmd())
		m = model.(tuiModel)
	}
	if m.componentScope != "base" {
		t.Fatal("scope toggle not reversible")
	}
	for _, k := range []string{"space", "d", "u", "x", "t"} {
		m.screen = "components"
		model, cmd = m.componentKey(k)
		m = model.(tuiModel)
		if cmd == nil || m.req.Component == nil || m.screen != "loading" {
			t.Fatal(k, "operation bypassed preview")
		}
	}
	m.screen = "components"
	m.componentItems[0].Disabled = true
	model, _ = m.componentKey("space")
	m = model.(tuiModel)
	if m.req.Component.Operation != "enable" {
		t.Fatal("disabled entry not enabled")
	}
	m.screen = "components"
	m.componentItems[0].Subpath = "child"
	model, cmd = m.componentKey("x")
	m = model.(tuiModel)
	if cmd != nil || !strings.Contains(m.status, "child files") {
		t.Fatal("child removal not blocked")
	}
	m.componentItems = nil
	if _, cmd = m.componentKey("x"); cmd != nil {
		t.Fatal("empty inventory mutated")
	}
	m.screen = "component-owners"
	m.owners = nil
	if !strings.Contains(strings.Join(m.componentView(), "\n"), "No additional") {
		t.Fatal("missing empty owners label")
	}
}

func TestComponentTemplatesMatchNativeHarnessContracts(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, id := range []string{"codex", "claude", "gemini", "pi", "opencode"} {
			t.Run(id+map[bool]string{false: "/retained", true: "/managed"}[managed], func(t *testing.T) {
				e, _ := testEngine(t)
				s, _ := e.specFor(id)
				inst := syntheticInstall(t, e, s, "1")
				if !managed {
					inst.Managed = false
					inst.StateRoot = ""
				}
				m := newModel(e)
				m.installs = []installation{inst}
				m.componentScope = "base"
				for i, spec := range e.cfg.Harnesses {
					if spec.ID == id {
						m.harness = i
					}
				}
				for _, cat := range []category{skills, mcp, plugins, marketplaces, connectors, hooks} {
					m.componentCat = cat
					change := m.addComponentTemplate()
					if change.Category != cat || change.Operation == "" {
						t.Fatal("invalid category template", change)
					}
					if cat == skills && (!filepath.IsAbs(change.Path) || change.Operation != "install") {
						t.Fatal("invalid skill import", change)
					}
					if cat == marketplaces && !change.Native {
						t.Fatal("marketplace lost native contract")
					}
				}
				m.componentScope = "profile"
				if change := m.addComponentTemplate(); change.Operation != "add" {
					t.Fatal("invalid profile fallback")
				}
			})
		}
	}
	m := componentModel(t)
	m.componentItems = []componentItem{{Path: filepath.Join(m.componentInstallation().StateRoot, "settings.json"), Field: "/mcpServers/one", Category: mcp}}
	if change := m.addComponentTemplate(); change.Field != "/mcpServers/my-entry" {
		t.Fatal(change)
	}
	_ = writeJSON(m.componentItems[0].Path, map[string]any{"hooks": []any{map[string]any{"name": "a"}}})
	m.componentCat = hooks
	m.componentItems[0].Category = hooks
	m.componentItems[0].Field = "/hooks/0"
	if change := m.addComponentTemplate(); change.Field != "/hooks/-" {
		t.Fatal(change)
	}
}

func TestComponentEditorValidatesEveryRequestBeforePreview(t *testing.T) {
	for _, scenario := range []string{"editor-error", "missing", "directory", "large", "invalid", "trailing", "operation", "category", "same", "content", "value", "stale", "add"} {
		t.Run(scenario, func(t *testing.T) {
			m := componentModel(t)
			path := filepath.Join(t.TempDir(), "request")
			data := []byte(`{"operation":"add","category":"mcp"}`)
			var item *componentItem
			before := ""
			var editErr error
			switch scenario {
			case "editor-error":
				editErr = errors.New("fixture")
			case "large":
				m.e.cfg.MetadataBytes = 2
			case "invalid":
				data = []byte("invalid")
			case "trailing":
				data = append(data, []byte(" {}")...)
			case "operation":
				data = []byte(`{"operation":"remove","category":"mcp"}`)
			case "category":
				data = []byte(`{"operation":"add","category":"hooks"}`)
			case "content", "value", "stale":
				source := filepath.Join(t.TempDir(), "source")
				_ = atomicWrite(source, []byte("original"), 0600)
				item = &componentItem{Path: source}
				before, _ = fingerprint(source)
				if scenario == "value" {
					item.Field = "/entry"
				}
				if scenario == "stale" {
					_ = atomicWrite(source, []byte("changed"), 0600)
				}
			}
			if scenario == "directory" {
				_ = os.Mkdir(path, 0700)
			} else if scenario != "missing" {
				_ = atomicWrite(path, data, 0600)
			}
			original := []byte("before")
			if scenario == "same" {
				original = data
			}
			msg := m.finishComponentEdit(path, original, componentRequest{}, item, before, editErr)
			valid := scenario == "content" || scenario == "value" || scenario == "add" || scenario == "same"
			if (msg.err == nil) != valid {
				t.Fatal(scenario, msg.err)
			}
			if scenario == "same" && !msg.cancelled {
				t.Fatal("unchanged editor not cancelled")
			}
			if scenario == "content" && len(msg.change.Content) == 0 {
				t.Fatal("asset edit lost payload")
			}
			if scenario == "value" && len(msg.change.Value) == 0 {
				t.Fatal("field edit lost value")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("temporary editor request leaked")
			}
		})
	}
}

func TestComponentDirectoryNavigationAndEditorPreparation(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m := componentModel(t)
	dir := filepath.Join(m.componentInstallation().StateRoot, "skills", "demo")
	_ = atomicWrite(filepath.Join(dir, "SKILL.md"), []byte("skill"), 0600)
	m.componentItems = []componentItem{{Name: "demo", Path: dir, Category: skills, Directory: true}}
	m.screen = "components"
	model, cmd := m.componentKey("enter")
	m = model.(tuiModel)
	if cmd != nil || len(m.componentItems) != 1 || m.componentItems[0].Directory {
		t.Fatal("directory not browsed")
	}
	if cmd = m.editComponent(&m.componentItems[0]); cmd == nil {
		t.Fatal("asset editor not prepared")
	}
	msg := m.editComponent(&componentItem{Path: filepath.Join(dir, "absent")})().(componentEditorMsg)
	if msg.err == nil {
		t.Fatal("absent editor source accepted")
	}
	if _, cmd = m.componentKey("e"); cmd == nil {
		t.Fatal("file edit command missing")
	}
	if _, cmd = m.componentKey("a"); cmd == nil {
		t.Fatal("add editor missing")
	}
	for _, msg := range []tea.Msg{componentsMsg{err: errors.New("fixture")}, componentEditorMsg{cancelled: true}, componentEditorMsg{err: errors.New("fixture")}, componentEditorMsg{change: componentRequest{Operation: "add", Category: skills}}} {
		model, cmd = m.Update(msg)
		m = model.(tuiModel)
		if edit, ok := msg.(componentEditorMsg); ok && edit.change.Operation != "" && cmd == nil {
			t.Fatal("edited request not previewed")
		}
	}
}
