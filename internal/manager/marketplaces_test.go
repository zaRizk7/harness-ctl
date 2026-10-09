package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNativeLedgerFindsManuallyInstalledDisabledPlugin(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(inst.StateRoot, "plugins", "cache", "market", "demo", "1")
	if err := writeJSON(filepath.Join(payload, ".claude-plugin", "plugin.json"), map[string]any{"name": "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "installed_plugins.json"), claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"demo@market": {{Scope: "user", InstallPath: payload}}}}); err != nil {
		t.Fatal(err)
	}
	entries, err := e.components(inst, "base", plugins)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "demo@market" || !entries[0].Native || !entries[0].Disabled {
		t.Fatal(entries)
	}
}
func TestMarketplacePlansCaptureDependentPlugins(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "known_marketplaces.json"), map[string]any{"market": map[string]any{"source": map[string]any{"source": "github", "repo": "org/repo"}, "installLocation": filepath.Join(inst.StateRoot, "plugins", "marketplaces", "market")}}); err != nil {
		t.Fatal(err)
	}
	entries, err := e.components(inst, "base", marketplaces)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "claude", InstallID: inst.ID, Action: "manage", Component: &componentRequest{Category: marketplaces, Operation: "remove", Name: "market", Native: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Steps[0].Args, []string{"plugin", "marketplace", "remove", "market", "--scope", "user"}) {
		t.Fatal(p.Steps)
	}
	found := false
	for _, r := range p.Resources {
		found = found || r.Path == filepath.Join(inst.StateRoot, "plugins")
	}
	if !found {
		t.Fatal("dependent plugin state not captured")
	}
	if len(p.Warnings) == 0 {
		t.Fatal("missing removal effect preview")
	}
}

func TestMarketplaceNativeExecutionAndRollback(t *testing.T) {
	e, r, inst := nativeComponentFixture(t, "claude")
	registryPath := filepath.Join(inst.StateRoot, "plugins", "known_marketplaces.json")
	cachePath := filepath.Join(inst.StateRoot, "plugins", "marketplaces", "demo")
	r.onRun = func(c command) error {
		if len(c.Args) < 4 || c.Args[0] != "plugin" || c.Args[1] != "marketplace" {
			return nil
		}
		entries, err := readMarketplaces(inst.StateRoot)
		if err != nil {
			return err
		}
		if c.Args[2] == "remove" {
			delete(entries, "demo")
			if err = os.RemoveAll(cachePath); err != nil {
				return err
			}
		} else {
			if err = atomicWrite(filepath.Join(cachePath, ".claude-plugin", "marketplace.json"), []byte(`{"name":"demo"}`), 0600); err != nil {
				return err
			}
			entries["demo"] = marketplaceRecord{InstallLocation: cachePath, Source: map[string]any{"source": "github", "repo": "org/repo"}}
		}
		return writeJSON(registryPath, entries)
	}
	for _, operation := range []string{"add", "update", "edit", "remove"} {
		change := componentRequest{Category: marketplaces, Operation: operation, Name: "demo", Source: "org/repo", Native: true}
		p, err := e.buildPlan(context.Background(), request{Harness: "claude", InstallID: inst.ID, Action: "manage", Component: &change})
		if err != nil {
			t.Fatal(operation, err)
		}
		if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
			t.Fatal(operation, err)
		}
	}
	change := componentRequest{Category: marketplaces, Operation: "add", Name: "demo", Source: "org/repo", Native: true}
	p, err := e.buildPlan(context.Background(), request{Harness: "claude", InstallID: inst.ID, Action: "manage", Component: &change})
	if err != nil {
		t.Fatal(err)
	}
	oldRun := r.onRun
	r.onRun = func(c command) error {
		if err := oldRun(c); err != nil {
			return err
		}
		if strings.HasPrefix(c.Description, "Native marketplace") {
			return errors.New("fixture failure")
		}
		return nil
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("native failure accepted")
	}
	entries, err := readMarketplaces(inst.StateRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("rollback retained failed marketplace", entries, err)
	}
}
