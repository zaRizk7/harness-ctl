package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func componentFixture(t *testing.T) (*engine, installation, string) {
	t.Helper()
	e, _ := testEngine(t)
	s, _ := specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inst.StateRoot, "settings.json")
	if err := atomicWrite(path, []byte(`{"ordinary":"keep","mcpServers":{"one":{"command":"demo","token":"synthetic-secret"},"two":{"command":"other"}},"plugins":{"one":{}},"connectors":{"one":{}},"proxy":"http://proxy.test","hooks":{"Stop":[{"command":"demo"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	return e, inst, path
}

func applyComponentRequest(t *testing.T, e *engine, inst installation, change componentRequest) *plan {
	t.Helper()
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestComponentInventoryListsNamedEntriesWithoutValues(t *testing.T) {
	e, inst, _ := componentFixture(t)
	items, err := e.components(inst, "base", mcp)
	if err != nil || len(items) != 2 || items[0].Name != "one" || items[1].Name != "two" {
		t.Fatal("MCP entries were not individually listed", items, err)
	}
	for _, cat := range []category{plugins, connectors, proxies, hooks} {
		items, err := e.components(inst, "base", cat)
		if err != nil || len(items) != 1 {
			t.Fatal("category is missing", cat, items, err)
		}
	}
}

func TestComponentFieldCRUDAndDisablePreservesSiblings(t *testing.T) {
	e, inst, path := componentFixture(t)
	change := componentRequest{Category: mcp, Path: path, Field: "/mcpServers/three", Operation: "add", Value: json.RawMessage(`{"command":"three"}`)}
	applyComponentRequest(t, e, inst, change)
	change.Operation, change.Value = "edit", json.RawMessage(`{"command":"edited","port":123}`)
	applyComponentRequest(t, e, inst, change)
	change.Operation = "disable"
	applyComponentRequest(t, e, inst, change)
	items, err := e.components(inst, "base", mcp)
	if err != nil || len(items) != 3 {
		t.Fatal("disabled entry disappeared", items, err)
	}
	value, _ := readConfig(path, "json")
	if _, exists := value["mcpServers"].(map[string]any)["three"]; exists || value["ordinary"] != "keep" {
		t.Fatal("disable retained the active registration or changed a sibling")
	}
	change.Operation = "enable"
	applyComponentRequest(t, e, inst, change)
	value, _ = readConfig(path, "json")
	entry := value["mcpServers"].(map[string]any)["three"].(map[string]any)
	if entry["command"] != "edited" || value["ordinary"] != "keep" {
		t.Fatal("enable lost edited state")
	}
	change.Operation = "remove"
	p := applyComponentRequest(t, e, inst, change)
	metas, err := e.snapshots()
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range metas {
		if meta.Action == "manage" && meta.Created.After(p.Created) {
			if err := e.restoreSnapshot(context.Background(), meta.ID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	value, _ = readConfig(path, "json")
	if _, exists := value["mcpServers"].(map[string]any)["three"]; !exists {
		t.Fatal("component removal was not recoverable")
	}
}

func TestComponentSkillImportDisableEnableAndExecutableEdit(t *testing.T) {
	e, inst, _ := componentFixture(t)
	source := t.TempDir()
	if err := atomicWrite(filepath.Join(source, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\nDemo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inst.StateRoot, "skills", "demo")
	change := componentRequest{Category: skills, Path: path, Operation: "install", Source: source}
	applyComponentRequest(t, e, inst, change)
	change.Operation = "disable"
	applyComponentRequest(t, e, inst, change)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("disabled skill remains in its native loader directory")
	}
	change.Operation = "enable"
	applyComponentRequest(t, e, inst, change)
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatal("enabled skill payload missing", err)
	}
	file := filepath.Join(inst.StateRoot, "hooks", "demo.sh")
	if err := atomicWrite(file, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	applyComponentRequest(t, e, inst, componentRequest{Category: hooks, Path: file, Operation: "edit", Content: []byte("#!/bin/sh\nexit 1\n")})
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("editing removed the executable mode", err)
	}
}

func TestComponentArrayDisableEnableDoesNotOverwriteAnotherEntry(t *testing.T) {
	e, inst, path := componentFixture(t)
	if err := atomicWrite(path, []byte(`{"plugin":["one","two"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	change := componentRequest{Category: plugins, Path: path, Field: "/plugin/0", Operation: "disable"}
	applyComponentRequest(t, e, inst, change)
	items, err := e.components(inst, "base", plugins)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Disabled {
			change.Parked = item.Parked
		}
	}
	change.Operation = "enable"
	applyComponentRequest(t, e, inst, change)
	value, _ := readConfig(path, "json")
	plugins := value["plugin"].([]any)
	if len(plugins) != 2 || plugins[0] != "one" || plugins[1] != "two" {
		t.Fatal("enable overwrote an array sibling", plugins)
	}
}

func disabledItem(t *testing.T, e *engine, inst installation, cat category, field string) componentItem {
	t.Helper()
	items, err := e.components(inst, "base", cat)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Disabled && item.Field == field {
			return item
		}
	}
	t.Fatal("disabled item missing", field, items)
	return componentItem{}
}

func TestComponentParkedArrayEditAndRemovePreserveActiveSibling(t *testing.T) {
	for _, operation := range []string{"edit", "remove"} {
		t.Run(operation, func(t *testing.T) {
			e, inst, path := componentFixture(t)
			if err := atomicWrite(path, []byte(`{"plugin":["one","two","three"]}`), 0600); err != nil {
				t.Fatal(err)
			}
			applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: "/plugin/0", Operation: "disable"})
			item := disabledItem(t, e, inst, plugins, "/plugin/0")
			applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: item.Field, Parked: item.Parked, Operation: operation, Value: json.RawMessage(`"edited"`)})
			value, _ := readConfig(path, "json")
			if !reflect.DeepEqual(value["plugin"], []any{"two", "three"}) {
				t.Fatal("parked operation changed active siblings", value)
			}
			if operation == "edit" {
				applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: item.Field, Parked: item.Parked, Operation: "enable"})
				value, _ = readConfig(path, "json")
				if !reflect.DeepEqual(value["plugin"], []any{"edited", "two", "three"}) {
					t.Fatal("disabled edit was lost", value)
				}
			}
		})
	}
}

func TestComponentArrayRestorationOrdersAndRemoval(t *testing.T) {
	for _, mode := range []string{"forward", "reverse", "remove-disabled", "remove-active"} {
		t.Run(mode, func(t *testing.T) {
			e, inst, path := componentFixture(t)
			if err := atomicWrite(path, []byte(`{"plugin":["one","two","three","four"]}`), 0600); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"/plugin/0", "/plugin/1"} {
				applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: field, Operation: "disable"})
			}
			first := disabledItem(t, e, inst, plugins, "/plugin/0")
			last := disabledItem(t, e, inst, plugins, "/plugin/2")
			want := []any{"one", "two", "three", "four"}
			if mode == "remove-disabled" {
				applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: first.Field, Parked: first.Parked, Operation: "remove"})
				last = disabledItem(t, e, inst, plugins, "/plugin/1")
				want = []any{"two", "three", "four"}
			} else if mode == "remove-active" {
				applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: "/plugin/0", Operation: "remove"})
				last = disabledItem(t, e, inst, plugins, "/plugin/1")
				want = []any{"one", "three", "four"}
			}
			order := []componentItem{first, last}
			if mode == "reverse" {
				order = []componentItem{last, first}
			}
			if mode == "remove-disabled" {
				order = []componentItem{last}
			}
			for _, item := range order {
				applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Path: path, Field: item.Field, Parked: item.Parked, Operation: "enable"})
			}
			value, _ := readConfig(path, "json")
			if !reflect.DeepEqual(value["plugin"], want) {
				t.Fatal("array order or removal was lost", value, want)
			}
		})
	}
}

func TestComponentRejectsInvalidStructuredAssetEdits(t *testing.T) {
	e, inst, _ := componentFixture(t)
	for _, name := range []string{"hook.json", "hook.toml", "hook.yaml"} {
		path := filepath.Join(inst.StateRoot, "hooks", name)
		if err := atomicWrite(path, []byte("valid"), 0600); err != nil {
			t.Fatal(err)
		}
		change := componentRequest{Category: hooks, Path: path, Operation: "edit", Content: []byte("{broken[")}
		if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
			t.Fatal("invalid structured edit accepted", name)
		}
	}
}

func TestComponentCodexUserSkillsRequireSharedOwnersAndRecover(t *testing.T) {
	e, _ := testEngine(t)
	inst := installation{Harness: "codex", Method: "state-only"}
	path := filepath.Join(e.cfg.Home, ".agents", "skills", "demo")
	if err := atomicWrite(filepath.Join(path, "SKILL.md"), []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "base", skills)
	if err != nil || len(items) != 1 {
		t.Fatal("Codex user skills are absent", items, err)
	}
	change := componentRequest{Category: skills, Path: path, Operation: "disable"}
	req := request{Harness: "codex", Action: "manage", Component: &change}
	p, err := e.buildPlan(context.Background(), req)
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal("shared skills need explicit owners", err)
	}
	req.Owners = items[0].Owners
	p, err = e.buildPlan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, err = e.components(inst, "base", skills)
	if err != nil || len(items) != 1 || !items[0].Disabled {
		t.Fatal("disabled shared skill is missing", items, err)
	}
	change.Operation, change.Parked = "enable", items[0].Parked
	p, err = e.buildPlan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func nativeComponentFixture(t *testing.T, id string) (*engine, *fakeRunner, installation) {
	t.Helper()
	e, runner := testEngine(t)
	s, _ := specFor(id)
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	return e, runner, inst
}

func TestComponentNativeGeminiUsesSourceAndVerifiesInstall(t *testing.T) {
	e, runner, inst := nativeComponentFixture(t, "gemini")
	change := componentRequest{Operation: "install", Category: plugins, Native: true, Source: "https://github.com/example/extension", Name: "demo"}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Steps[0].Args, []string{"extensions", "install", change.Source, "--consent", "--skip-settings"}) {
		t.Fatal("install did not use its source", p.Steps[0].Args)
	}
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("native success without installed manifest was accepted")
	}
	runner.onRun = func(c command) error {
		if c.Description == "Native plugin install" {
			return writeJSON(filepath.Join(inst.StateRoot, "extensions", "demo", "gemini-extension.json"), map[string]string{"name": "demo", "version": "1.0.0"})
		}
		return nil
	}
	applyComponentRequest(t, e, inst, change)
}

func TestComponentNativeFailureRollsBackAndPreservesUnrelatedState(t *testing.T) {
	for _, mode := range []string{"failure", "unrelated-change"} {
		t.Run(mode, func(t *testing.T) {
			e, runner, inst := nativeComponentFixture(t, "gemini")
			path := filepath.Join(inst.StateRoot, "settings.json")
			if err := atomicWrite(path, []byte(`{"ordinary":"keep"}`), 0600); err != nil {
				t.Fatal(err)
			}
			runner.onRun = func(c command) error {
				if c.Description != "Native plugin install" {
					return nil
				}
				if err := writeJSON(filepath.Join(inst.StateRoot, "extensions", "demo", "gemini-extension.json"), map[string]string{"name": "demo", "version": "1.0.0"}); err != nil {
					return err
				}
				if mode == "failure" {
					return errors.New("synthetic native failure")
				}
				return atomicWrite(path, []byte(`{"ordinary":"changed"}`), 0600)
			}
			change := componentRequest{Operation: "install", Category: plugins, Native: true, Source: "https://github.com/example/demo", Name: "demo"}
			p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
				t.Fatal("native failure or unrelated mutation was accepted")
			}
			value, err := readConfig(path, "json")
			if err != nil || value["ordinary"] != "keep" {
				t.Fatal("rollback did not restore unrelated settings", value, err)
			}
			if _, err := os.Stat(filepath.Join(inst.StateRoot, "extensions", "demo")); !os.IsNotExist(err) {
				t.Fatal("native rollback retained new extension")
			}
		})
	}
}

func TestComponentRejectsStaleSourceAndSharedOwnerOmission(t *testing.T) {
	e, inst, path := componentFixture(t)
	change := componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "remove"}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte(`{"mcpServers":{"one":{"command":"changed"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("stale component edit was accepted")
	}
	s, _ := specFor("codex")
	tracked := installation{Harness: s.ID, Method: "state-only"}
	path = filepath.Join(e.stateRoot(s), "config.toml")
	if err := atomicWrite(path, []byte("[mcp_servers.demo]\ncommand='demo'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	change = componentRequest{Category: mcp, Path: path, Field: "/mcp_servers/demo", Operation: "remove"}
	p, err = e.buildPlan(context.Background(), request{Harness: tracked.Harness, Action: "manage", Component: &change})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal("shared component removal did not require affected owners", err)
	}
}

func TestComponentRefusesLinkedImportsAndRuntimeTargets(t *testing.T) {
	e, inst, _ := componentFixture(t)
	source := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(source, "external")); err != nil {
		t.Fatal(err)
	}
	for _, change := range []componentRequest{
		{Category: skills, Path: filepath.Join(inst.StateRoot, "skills", "linked"), Source: source, Operation: "install"},
		{Category: other, Path: filepath.Join(inst.StateRoot, "hermes-agent", "runtime"), Operation: "remove"},
	} {
		if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
			t.Fatal("unsafe component operation was accepted")
		}
	}
}

func TestComponentRejectsCategoryAndConfigPathBypasses(t *testing.T) {
	e, inst, path := componentFixture(t)
	if err := atomicWrite(path, []byte(`{"auth":{"plugin":"secret"},"group":{"hooks":{"Stop":[]},"normal":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, change := range []componentRequest{
		{Category: plugins, Path: path, Field: "/auth/plugin", Operation: "remove"},
		{Category: settings, Path: path, Field: "/group", Operation: "remove"},
		{Category: mcp, Path: filepath.Join(inst.StateRoot, "hermes-agent", "settings.json"), Field: "/mcpServers/demo", Value: json.RawMessage(`{}`), Operation: "add"},
	} {
		if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
			t.Fatal("category or runtime boundary bypass accepted", change.Field)
		}
	}
}

func TestComponentCodexNativeEnablementRetainsRegistration(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := specFor("codex")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inst.StateRoot, "config.toml")
	if err := atomicWrite(path, []byte("[mcp_servers.demo]\ncommand='demo'\nenabled=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "base", mcp)
	if err != nil || len(items) != 1 || !items[0].Disabled {
		t.Fatal("native disabled registration not identified", items, err)
	}
	for _, operation := range []string{"enable", "disable"} {
		applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Path: path, Field: "/mcp_servers/demo", Operation: operation})
		value, err := readConfig(path, "toml")
		if err != nil {
			t.Fatal(err)
		}
		entry := value["mcp_servers"].(map[string]any)["demo"].(map[string]any)
		if entry["command"] != "demo" || entry["enabled"] != (operation == "enable") {
			t.Fatal("native enablement lost the registration", entry)
		}
	}
}

func TestComponentNativeClaudeCommandContracts(t *testing.T) {
	e, runner, inst := nativeComponentFixture(t, "claude")
	path := filepath.Join(inst.StateRoot, "settings.json")
	if err := atomicWrite(path, []byte(`{"ordinary":"keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	name := "demo@market"
	payload := filepath.Join(inst.StateRoot, "plugins", "cache", "market", "demo", "1.0.0")
	runner.onRun = func(c command) error {
		if !strings.HasPrefix(c.Description, "Native plugin") {
			return nil
		}
		if !reflect.DeepEqual(c.Args, []string{"plugin", c.Args[1], name, "--scope", "user"}) || c.Env["CLAUDE_CONFIG_DIR"] != inst.StateRoot {
			return errors.New("incorrect Claude command contract")
		}
		config := map[string]any{"ordinary": "keep", "enabledPlugins": map[string]bool{name: c.Args[1] != "disable"}}
		ledger := map[string]any{"version": 2, "plugins": map[string]any{name: []any{map[string]string{"scope": "user", "installPath": payload}}}}
		if c.Args[1] == "uninstall" {
			config["enabledPlugins"] = map[string]bool{}
			ledger["plugins"] = map[string]any{}
			if err := os.RemoveAll(payload); err != nil {
				return err
			}
		} else if err := writeJSON(filepath.Join(payload, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo"}); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "installed_plugins.json"), ledger); err != nil {
			return err
		}
		return writeJSON(path, config)
	}
	for _, operation := range []string{"install", "disable", "enable", "remove"} {
		applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Native: true, Name: name, Operation: operation})
		items, err := e.components(inst, "base", plugins)
		if err != nil {
			t.Fatal(err)
		}
		if operation == "remove" {
			if len(items) != 0 {
				t.Fatal("removed native plugin still listed", items)
			}
			continue
		}
		if len(items) != 1 || items[0].Name != name || items[0].Disabled != (operation == "disable") {
			t.Fatal("native plugin status incorrect", items)
		}
	}
}

func TestComponentPlanCannotChangeAfterApproval(t *testing.T) {
	e, inst, path := componentFixture(t)
	change := componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "remove"}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	p.Request.Preserve[auth] = false
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("mutated approved component plan was accepted")
	}
}

func TestComponentCancelledApplyLeavesLocalState(t *testing.T) {
	e, inst, path := componentFixture(t)
	change := componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "remove"}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.applyComponent(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled component apply did not stop", err)
	}
	value, _ := readConfig(path, "json")
	if _, exists := value["mcpServers"].(map[string]any)["one"]; !exists {
		t.Fatal("cancelled component apply removed state")
	}
}

func TestComponentProfileRetainsDisabledEntriesInItsOwnScope(t *testing.T) {
	e, inst, path := componentFixture(t)
	applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "disable"})
	if err := e.createProfile(inst, nil); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	items, err := e.components(inst, "profile", mcp)
	if err != nil {
		t.Fatal("copied disabled state still points at the base scope", err)
	}
	for _, item := range items {
		if !item.Disabled {
			continue
		}
		applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Path: item.Path, Field: item.Field, Parked: item.Parked, Scope: "profile", Operation: "enable"})
	}
	value, _ := readConfig(path, "json")
	if _, exists := value["mcpServers"].(map[string]any)["one"]; exists {
		t.Fatal("profile management enabled the base entry")
	}
}

func TestComponentNativeRejectsExternalClaudePayloadBeforeCommand(t *testing.T) {
	e, runner, inst := nativeComponentFixture(t, "claude")
	if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "installed_plugins.json"), map[string]any{"version": 2, "plugins": map[string]any{"demo@market": []any{map[string]string{"scope": "user", "installPath": t.TempDir()}}}}); err != nil {
		t.Fatal(err)
	}
	change := componentRequest{Category: plugins, Native: true, Name: "demo@market", Operation: "remove"}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err == nil && len(p.Blockers) == 0 {
		t.Fatal("native removal can delete uncaptured external payload")
	}
	if len(runner.calls) > 0 {
		t.Fatal("planning invoked a native command")
	}
}

func TestComponentNativeGeminiEnablementAndRemovalContracts(t *testing.T) {
	e, runner, inst := nativeComponentFixture(t, "gemini")
	path := filepath.Join(inst.StateRoot, "extensions", "demo")
	if err := writeJSON(filepath.Join(path, "gemini-extension.json"), map[string]string{"name": "demo", "version": "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	runner.onRun = func(c command) error {
		if !strings.HasPrefix(c.Description, "Native plugin") {
			return nil
		}
		if c.Env["HOME"] != filepath.Dir(inst.StateRoot) {
			return errors.New("incorrect native HOME")
		}
		if c.Args[1] == "uninstall" {
			if !reflect.DeepEqual(c.Args, []string{"extensions", "uninstall", "demo"}) {
				return errors.New("incorrect uninstall contract")
			}
			return os.RemoveAll(path)
		}
		if !reflect.DeepEqual(c.Args, []string{"extensions", c.Args[1], "demo", "--scope", "user"}) {
			return errors.New("incorrect enablement contract")
		}
		rule := filepath.ToSlash(filepath.Dir(inst.StateRoot)) + "/*"
		if c.Args[1] == "disable" {
			rule = "!" + rule
		}
		return writeJSON(filepath.Join(inst.StateRoot, "extensions", "extension-enablement.json"), map[string]any{"demo": map[string]any{"overrides": []string{rule}}})
	}
	for _, operation := range []string{"disable", "enable", "remove"} {
		applyComponentRequest(t, e, inst, componentRequest{Category: plugins, Native: true, Name: "demo", Operation: operation})
		items, err := e.components(inst, "base", plugins)
		if err != nil {
			t.Fatal(err)
		}
		if operation == "remove" {
			if len(items) != 0 {
				t.Fatal("removed extension is listed", items)
			}
			continue
		}
		if len(items) != 1 || items[0].Disabled != (operation == "disable") {
			t.Fatal("extension status incorrect", items)
		}
	}
}

func TestComponentValuesRejectTrailingInput(t *testing.T) {
	if _, err := decodeComponentValue([]byte(`{"command":"demo"} garbage`)); err == nil {
		t.Fatal("invalid trailing editor data was ignored")
	}
	if _, err := decodeComponentValue([]byte(`{"command":"demo"} {}`)); err == nil {
		t.Fatal("second editor document was ignored")
	}
}

func TestComponentPreviewDoesNotExposeSecretValues(t *testing.T) {
	e, inst, path := componentFixture(t)
	change := componentRequest{Category: mcp, Path: path, Field: "/mcpServers/one", Operation: "edit", Value: json.RawMessage(`{"token":"synthetic-secret"}`)}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	m.screen, m.req, m.p = "preview", p.Request, p
	if strings.Contains(m.View().Content, "synthetic-secret") {
		t.Fatal("preview displays a component secret")
	}
}
