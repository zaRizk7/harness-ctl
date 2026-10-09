package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// nativePlanFixture supplies a concrete native contract without executing it.
func nativePlanFixture(t *testing.T, id string, change componentRequest) (*engine, *plan) {
	t.Helper()
	e, _, inst := nativeComponentFixture(t, id)
	s, _ := e.specFor(id)
	return e, &plan{ID: randomID(), Spec: s, Install: inst, StateRoot: inst.StateRoot, Request: request{Harness: id}, RootDigests: map[string]string{}, Component: &componentMutation{Request: change}}
}

func TestNativePluginPlanningRejectsUnsupportedRequests(t *testing.T) {
	for _, scenario := range []struct {
		id     string
		change componentRequest
	}{
		{"claude", componentRequest{Category: mcp, Operation: "install", Name: "demo@market"}},
		{"claude", componentRequest{Category: plugins, Operation: "install"}},
		{"claude", componentRequest{Category: plugins, Operation: "edit", Name: "demo@market"}},
		{"claude", componentRequest{Category: plugins, Operation: "install", Name: "demo"}},
		{"pi", componentRequest{Category: plugins, Operation: "disable", Name: "npm:demo"}},
		{"pi", componentRequest{Category: plugins, Operation: "remove", Name: "npm:demo"}},
		{"pi", componentRequest{Category: plugins, Operation: "install", Name: "git:demo"}},
		{"gemini", componentRequest{Category: plugins, Operation: "install", Name: "demo"}},
		{"gemini", componentRequest{Category: plugins, Operation: "enable", Name: "../demo"}},
		{"gemini", componentRequest{Category: plugins, Operation: "enable", Name: "demo"}},
		{"codex", componentRequest{Category: plugins, Operation: "install", Name: "demo"}},
	} {
		e, p := nativePlanFixture(t, scenario.id, scenario.change)
		if e.planNativeComponent(p, scenario.change) == nil {
			t.Fatal("unverified native request accepted", scenario)
		}
	}
	e, p := nativePlanFixture(t, "gemini", componentRequest{Category: plugins, Operation: "install", Name: "demo", Source: "https://example.test"})
	p.StateRoot = filepath.Dir(p.StateRoot)
	if e.planNativeComponent(p, p.Component.Request) == nil {
		t.Fatal("wrong Gemini home accepted")
	}
}

func TestNativePluginSourcesAndScopedPayloads(t *testing.T) {
	change := componentRequest{Category: plugins, Operation: "install", Name: "demo", Source: t.TempDir()}
	e, p := nativePlanFixture(t, "gemini", change)
	if e.planNativeComponent(p, change) == nil {
		t.Fatal("missing extension manifest accepted")
	}
	_ = writeJSON(filepath.Join(change.Source, "gemini-extension.json"), map[string]string{"name": "demo", "version": "1"})
	if err := e.planNativeComponent(p, change); err != nil {
		t.Fatal(err)
	}
	if p.RootDigests[change.Source] == "" {
		t.Fatal("source was not bound")
	}
	change.Source = p.StateRoot
	if e.planNativeComponent(p, change) == nil {
		t.Fatal("source overlaps state")
	}
	change.Source = filepath.Join(t.TempDir(), "missing")
	if e.planNativeComponent(p, change) == nil {
		t.Fatal("missing local source accepted")
	}
	change = componentRequest{Category: plugins, Operation: "remove", Source: "npm:demo@1"}
	e, p = nativePlanFixture(t, "pi", change)
	payload := filepath.Join(p.StateRoot, "npm", "node_modules", "demo")
	_ = writeJSON(filepath.Join(payload, "package.json"), map[string]any{"name": "demo", "version": "1", "pi": map[string]any{}})
	if err := e.planNativeComponent(p, change); err != nil || p.Steps[0].Args[0] != "remove" {
		t.Fatal(p.Steps, err)
	}
	change = componentRequest{Category: plugins, Operation: "enable", Name: "demo@market"}
	e, p = nativePlanFixture(t, "claude", change)
	payload = filepath.Join(p.StateRoot, "plugins", "cache", "demo")
	_ = writeJSON(filepath.Join(payload, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo"})
	_ = writeJSON(filepath.Join(p.StateRoot, "plugins", "installed_plugins.json"), claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"demo@market": {{Scope: "project", InstallPath: "/foreign"}, {Scope: "user", InstallPath: payload}}}})
	if err := e.planNativeComponent(p, change); err != nil {
		t.Fatal(err)
	}
	change.Operation = "install"
	if e.planNativeComponent(p, change) == nil {
		t.Fatal("installed native plugin overwritten")
	}
}

func TestNativePluginResultVerificationRejectsIncompleteState(t *testing.T) {
	for _, id := range []string{"claude", "gemini", "pi"} {
		t.Run(id, func(t *testing.T) {
			change := componentRequest{Category: plugins, Operation: "install", Name: "demo"}
			if id == "claude" {
				change.Name = "demo@market"
			}
			if id == "pi" {
				change.Name = "npm:demo@1"
			}
			e, p := nativePlanFixture(t, id, change)
			if e.verifyNativeComponent(p) == nil {
				t.Fatal("missing native registration accepted")
			}
			switch id {
			case "claude":
				ledger := filepath.Join(p.StateRoot, "plugins", "installed_plugins.json")
				payload := filepath.Join(p.StateRoot, "plugins", "cache", "demo")
				settings := filepath.Join(p.StateRoot, "settings.json")
				_ = writeJSON(ledger, claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"demo@market": {{Scope: "project", InstallPath: "/foreign"}, {Scope: "user", InstallPath: payload}}}})
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("missing payload accepted")
				}
				_ = writeJSON(filepath.Join(payload, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo"})
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("missing settings accepted")
				}
				_ = writeJSON(settings, map[string]any{"enabledPlugins": map[string]any{"demo@market": true}})
				if err := e.verifyNativeComponent(p); err != nil {
					t.Fatal(err)
				}
				p.Component.Request.Operation = "disable"
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("wrong enablement accepted")
				}
				p.Component.Request.Operation = "remove"
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("remaining registration accepted")
				}
				_ = os.Remove(ledger)
				_ = writeJSON(settings, map[string]any{})
				if err := e.verifyNativeComponent(p); err != nil {
					t.Fatal("verified removal rejected", err)
				}
			case "gemini":
				payload := filepath.Join(p.StateRoot, "extensions", "demo")
				_ = writeJSON(filepath.Join(payload, "gemini-extension.json"), map[string]string{"name": "demo", "version": "1"})
				if err := e.verifyNativeComponent(p); err != nil {
					t.Fatal(err)
				}
				p.Component.Request.Operation = "disable"
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("wrong enablement accepted")
				}
				p.Component.Request.Operation = "remove"
				if e.verifyNativeComponent(p) == nil {
					t.Fatal("remaining extension accepted")
				}
				_ = os.RemoveAll(payload)
				if err := e.verifyNativeComponent(p); err != nil {
					t.Fatal(err)
				}
			case "pi":
				_ = writeJSON(filepath.Join(p.StateRoot, "settings.json"), map[string]any{"packages": []string{"npm:demo@1"}})
				_ = writeJSON(filepath.Join(p.StateRoot, "npm", "node_modules", "demo", "package.json"), map[string]any{"name": "demo", "version": "1", "pi": map[string]any{}})
				if err := e.verifyNativeComponent(p); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestMarketplacePlanningRejectsUnsafeOperationAndSource(t *testing.T) {
	for _, change := range []componentRequest{{Name: "../bad", Operation: "add", Source: "org/repo"}, {Name: "demo", Operation: "disable"}, {Name: "demo", Operation: "remove"}, {Name: "demo", Operation: "add"}, {Name: "demo", Operation: "add", Source: "-bad"}} {
		e, p := nativePlanFixture(t, "claude", change)
		if e.planMarketplace(p, change) == nil {
			t.Fatal("invalid marketplace request accepted", change)
		}
	}
	change := componentRequest{Name: "demo", Operation: "add", Source: t.TempDir()}
	e, p := nativePlanFixture(t, "claude", change)
	p.Component.Request = change
	if err := e.planMarketplace(p, change); err != nil || p.RootDigests[change.Source] == "" {
		t.Fatal("local source unbound", err)
	}
	change.Source = p.StateRoot
	if e.planMarketplace(p, change) == nil {
		t.Fatal("overlapping source accepted")
	}
	change.Source = filepath.Join(t.TempDir(), "absent")
	if e.planMarketplace(p, change) == nil {
		t.Fatal("missing source accepted")
	}
	e, p = nativePlanFixture(t, "claude", change)
	ledger := filepath.Join(p.StateRoot, "plugins", "known_marketplaces.json")
	_ = writeJSON(ledger, map[string]marketplaceRecord{"demo": {InstallLocation: "/foreign/cache"}})
	change.Operation = "remove"
	if e.planMarketplace(p, change) == nil {
		t.Fatal("external marketplace cache accepted")
	}
	// Native category restrictions apply even when no filesystem payload exists.
	_, err := e.buildPlan(context.Background(), request{Harness: "claude", Action: "manage", Component: &componentRequest{Category: marketplaces, Operation: "enable", Name: "demo", Native: true}})
	if err == nil {
		t.Fatal("invented marketplace enable contract")
	}
}
