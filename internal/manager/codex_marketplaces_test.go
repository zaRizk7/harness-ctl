package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexMarketplaceNativeContractsAndManualInventory(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(inst.StateRoot, "config.toml")
	if err := atomicWrite(config, []byte("[marketplaces.manual]\nsource_type = 'git'\nsource = 'https://example.invalid/catalog.git'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := e.marketplaceItems(inst, "base")
	if err != nil || len(items) != 1 || items[0].Name != "manual" {
		t.Fatal(items, err)
	}
	for operation, want := range map[string]string{"update": "plugin marketplace upgrade manual", "remove": "plugin marketplace remove manual"} {
		p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "manage", Owners: s.SharedClients, Component: &componentRequest{Category: marketplaces, Operation: operation, Native: true, Name: "manual"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Steps) != 1 || strings.Join(p.Steps[0].Args, " ") != want {
			t.Fatal(p.Steps)
		}
	}
}

func TestCodexMarketplaceExecutionAndRollback(t *testing.T) {
	e, r, inst := nativeComponentFixture(t, "codex")
	s, _ := e.specFor("codex")
	config := filepath.Join(inst.StateRoot, "config.toml")
	cache := filepath.Join(inst.StateRoot, ".tmp", "marketplaces", "demo")
	r.onRun = func(c command) error {
		if len(c.Args) < 4 || c.Args[0] != "plugin" || c.Args[1] != "marketplace" {
			return nil
		}
		if c.Args[2] == "remove" {
			if err := atomicWrite(config, []byte("model = 'preserved'\n"), 0600); err != nil {
				return err
			}
			return os.RemoveAll(cache)
		}
		if err := atomicWrite(filepath.Join(cache, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"demo"}`), 0600); err != nil {
			return err
		}
		return atomicWrite(config, []byte("model = 'preserved'\n[marketplaces.demo]\nsource_type = 'git'\nsource = 'https://example.invalid/catalog.git'\n"), 0600)
	}
	if err := atomicWrite(config, []byte("model = 'preserved'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"add", "update", "edit", "remove"} {
		p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "manage", Owners: s.SharedClients, Component: &componentRequest{Category: marketplaces, Operation: op, Native: true, Name: "demo", Source: "owner/repo"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.execute(context.Background(), p, p.ID, nil); err != nil {
			t.Fatal(op, err)
		}
	}
	// Missing expected manifest must fail and roll back both config and new cache.
	original := r.onRun
	r.onRun = func(c command) error {
		if err := original(c); err != nil {
			return err
		}
		if strings.HasPrefix(c.Description, "Native marketplace") {
			return os.Remove(filepath.Join(cache, ".agents", "plugins", "marketplace.json"))
		}
		return nil
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "manage", Owners: s.SharedClients, Component: &componentRequest{Category: marketplaces, Operation: "add", Native: true, Name: "demo", Source: "owner/repo"}})
	if err != nil {
		t.Fatal(err)
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("invalid native success accepted")
	}
	records, err := readCodexMarketplaces(inst.StateRoot)
	if err != nil || len(records) != 0 {
		t.Fatal("failed add survived rollback", err)
	}
}
