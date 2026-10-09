package manager

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginInventoryDoesNotInventCompatibility(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	root := filepath.Join(inst.StateRoot, "extensions")
	if err := writeJSON(filepath.Join(root, "foreign", ".claude-plugin", "plugin.json"), map[string]any{"name": "foreign"}); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(root, "unknown.ts"), []byte("// local extension"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "base", plugins)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range items {
		switch item.Name {
		case "foreign":
			found++
			if strings.Join(item.BuiltFor, ",") != "claude" {
				t.Fatal("inventory claimed wrong compatibility", item)
			}
		case "unknown.ts":
			found++
			if len(item.BuiltFor) != 0 {
				t.Fatal("unknown compatibility presented as verified", item)
			}
		}
	}
	if found != 2 {
		t.Fatal(items)
	}
}

func TestPluginCompatibilityRejectsWrongHarness(t *testing.T) {
	e, _ := testEngine(t)
	root := t.TempDir()
	_ = writeJSON(filepath.Join(root, "package.json"), map[string]any{"name": "example", "pi": map[string]any{"extensions": []string{"extension.ts"}}})
	_ = atomicWrite(filepath.Join(root, "extension.ts"), []byte("export default function() {}"), 0600)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	change := componentRequest{Operation: "install", Category: plugins, Path: filepath.Join(inst.StateRoot, "plugins", "example"), Source: root}
	if _, err := e.buildPlan(context.Background(), request{Harness: "claude", Action: "manage", Component: &change}); err == nil {
		t.Fatal("Pi package allowed into another harness")
	}
	change.Source = ""
	change.Operation = "add"
	change.Text = "{}"
	change.BuiltFor = []string{"pi"}
	if _, err := e.buildPlan(context.Background(), request{Harness: "claude", Action: "manage", Component: &change}); err == nil {
		t.Fatal("explicit compatibility ignored")
	}
}

func TestPiNativePackageContractAndLocalEnablement(t *testing.T) {
	e, r := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	source := "npm:@example/pi-tools@1.0.0"
	manifest := filepath.Join(inst.StateRoot, "npm", "node_modules", "@example", "pi-tools", "package.json")
	r.onRun = func(c command) error {
		if len(c.Args) > 0 && c.Args[0] == "install" {
			if len(c.Args) != 2 || c.Args[1] != source || c.Env["PI_CODING_AGENT_DIR"] != inst.StateRoot {
				t.Fatal(c)
			}
			if err := writeJSON(filepath.Join(inst.StateRoot, "settings.json"), map[string]any{"packages": []string{source}, "ordinary": true}); err != nil {
				return err
			}
			return writeJSON(manifest, map[string]any{"name": "@example/pi-tools", "version": "1.0.0", "pi": map[string]any{"extensions": []string{"extension.ts"}}})
		}
		return nil
	}
	_ = writeJSON(filepath.Join(inst.StateRoot, "settings.json"), map[string]any{"ordinary": true})
	p, err := e.buildPlan(context.Background(), request{Harness: "pi", Action: "manage", Component: &componentRequest{Operation: "install", Category: plugins, Native: true, Source: source, Name: source}})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "base", plugins)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Name == source {
			found = true
			if len(item.BuiltFor) != 1 || item.BuiltFor[0] != "pi" {
				t.Fatal(item)
			}
		}
	}
	if !found {
		t.Fatal("native package not listed", items)
	}
	p, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "manage", Component: &componentRequest{Operation: "disable", Category: plugins, Path: filepath.Join(inst.StateRoot, "settings.json"), Field: "/packages/0"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "manage", Component: &componentRequest{Operation: "install", Category: plugins, Native: true, Source: "git:example"}}); err == nil {
		t.Fatal("unverified native source allowed")
	}
}
