package manager

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// nativeContractFixture builds registrations without invoking a vendor process.
func nativeContractFixture(t *testing.T, id, operation string, marketplace bool) (*engine, *plan, componentRequest) {
	t.Helper()
	e, _, inst := nativeComponentFixture(t, id)
	s, _ := e.specFor(id)
	change := componentRequest{Category: plugins, Native: true, Operation: operation, Name: "demo", Source: "https://example.test/demo"}
	if id == "claude" {
		change.Name = "demo@market"
	}
	if id == "pi" {
		change.Name, change.Source = "npm:demo", "npm:demo"
	}
	if marketplace {
		change.Category, change.Name = marketplaces, "market"
	}
	p := &plan{ID: randomID(), Spec: s, Install: inst, StateRoot: inst.StateRoot, RootDigests: map[string]string{}, Request: request{Harness: id, Preserve: keepAll()}, Component: &componentMutation{Request: change}}
	if operation != "install" && operation != "add" {
		switch id {
		case "claude":
			payload := filepath.Join(p.StateRoot, "plugins", "cache", "demo")
			_ = writeJSON(filepath.Join(payload, ".claude-plugin", "plugin.json"), map[string]string{"name": "demo"})
			_ = writeJSON(filepath.Join(p.StateRoot, "plugins", "installed_plugins.json"), claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"demo@market": {{Scope: "project", InstallPath: "ignored"}, {Scope: "user", InstallPath: payload}}}})
			_ = writeJSON(filepath.Join(p.StateRoot, "settings.json"), map[string]any{"enabledPlugins": map[string]any{"demo@market": true}})
			if marketplace {
				cache := filepath.Join(p.StateRoot, "plugins", "marketplaces", "market")
				_ = writeJSON(filepath.Join(cache, ".claude-plugin", "marketplace.json"), map[string]string{"name": "market"})
				_ = writeJSON(filepath.Join(p.StateRoot, "plugins", "known_marketplaces.json"), map[string]marketplaceRecord{"market": {InstallLocation: cache}})
			}
		case "gemini":
			_ = writeJSON(filepath.Join(p.StateRoot, "extensions", "demo", "gemini-extension.json"), map[string]string{"name": "demo", "version": "1"})
		case "pi":
			_ = writeJSON(filepath.Join(p.StateRoot, "npm", "node_modules", "demo", "package.json"), map[string]string{"name": "demo", "version": "1"})
			_ = writeJSON(filepath.Join(p.StateRoot, "settings.json"), map[string]any{"packages": []any{"npm:demo"}})
		}
	}
	return e, p, change
}

func TestNativePlanningPropagatesRegistrationAndPayloadFailures(t *testing.T) {
	for _, scenario := range []struct {
		id, op string
		market bool
	}{{"claude", "enable", false}, {"claude", "remove", true}, {"gemini", "enable", false}, {"gemini", "install", false}, {"pi", "remove", false}} {
		for _, boundary := range []string{"validate", "fingerprint", "read", "lstat", "stat", "readFile", "walk", "info"} {
			t.Run(fmt.Sprintf("%s/%s/%t/%s", scenario.id, scenario.op, scenario.market, boundary), func(t *testing.T) {
				inject := func(n int, count *int) func() {
					if strings.Contains(" validate fingerprint read ", " "+boundary+" ") {
						return installMutationFault(boundary, n, count)
					}
					return injectArchiveFailure(boundary, n, count)
				}
				e, p, change := nativeContractFixture(t, scenario.id, scenario.op, scenario.market)
				operation := func(e *engine, p *plan, change componentRequest) error {
					if scenario.market {
						return e.planMarketplace(p, change)
					}
					return e.planNativeComponent(p, change)
				}
				count := 0
				restore := inject(0, &count)
				err := operation(e, p, change)
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, p, change := nativeContractFixture(t, scenario.id, scenario.op, scenario.market)
						before, _ := fingerprint(e.cfg.Home)
						count := 0
						restore := inject(nth, &count)
						_ = operation(e, p, change)
						restore()
						after, _ := fingerprint(e.cfg.Home)
						if count < nth || before != after {
							t.Fatal("planning boundary skipped or mutated state")
						}
					})
				}
			})
		}
	}
}

func TestMarketplaceOwnershipScopeAndVerifiedResults(t *testing.T) {
	e, p, change := nativeContractFixture(t, "claude", "remove", true)
	if _, err := e.marketplaceItems(installation{Harness: "unknown"}, "base"); err == nil {
		t.Fatal("unknown adapter accepted")
	}
	if _, err := e.marketplaceItems(installation{Harness: "pi"}, "base"); err == nil {
		t.Fatal("unsupported marketplace accepted")
	}
	if err := e.planMarketplace(&plan{Spec: harnessSpec{ID: "pi"}}, change); err == nil {
		t.Fatal("unsupported plan accepted")
	}
	ledger := filepath.Join(p.StateRoot, "plugins", "known_marketplaces.json")
	change.Operation = "add"
	if err := e.planMarketplace(p, change); err == nil {
		t.Fatal("duplicate marketplace accepted")
	}
	change.Operation = "remove"
	p.Component.Request = change
	if err := verifyMarketplace(p); err == nil {
		t.Fatal("retained marketplace reported removed")
	}
	for _, record := range []marketplaceRecord{{}, {InstallLocation: filepath.Join(e.cfg.Home, "foreign")}, {InstallLocation: filepath.Join(p.StateRoot, "plugins", "missing")}} {
		_ = writeJSON(ledger, map[string]marketplaceRecord{"market": record})
		p.Component.Request.Operation = "update"
		if err := verifyMarketplace(p); err == nil {
			t.Fatal("unverified payload accepted")
		}
	}
	_ = writeJSON(ledger, map[string]marketplaceRecord{})
	if err := verifyMarketplace(p); err == nil {
		t.Fatal("missing marketplace accepted")
	}
	_ = atomicWrite(ledger, []byte("invalid"), 0600)
	if err := verifyMarketplace(p); err == nil {
		t.Fatal("invalid ledger accepted")
	}
	if _, err := e.marketplaceItems(p.Install, "base"); err == nil {
		t.Fatal("invalid inventory accepted")
	}
	_ = writeJSON(ledger, map[string]marketplaceRecord{})
	old := validateOwnedPath
	defer func() { validateOwnedPath = old }()
	validateOwnedPath = func(string, string) error { return errors.New("marketplace validation") }
	if _, err := readMarketplaces(p.StateRoot); err == nil {
		t.Fatal("validation failure ignored")
	}
	validateOwnedPath = old
	_ = writeJSON(filepath.Join(p.StateRoot, "plugins", "installed_plugins.json"), claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"invalid": {{Scope: "user"}}}})
	if _, err := addLedgerPlugins(p.StateRoot, nil, nil); err == nil {
		t.Fatal("invalid qualified identity accepted")
	}
	// A native state scope has product-shared owners. Its approval must include them.
	p.StateRoot = e.stateRoot(p.Spec)
	p.Install.Managed = false
	_ = writeJSON(filepath.Join(p.StateRoot, "plugins", "known_marketplaces.json"), map[string]marketplaceRecord{"market": {}})
	items, err := e.marketplaceItems(p.Install, "base")
	if err != nil || len(items) != 1 || len(items[0].Owners) < 2 {
		t.Fatal(items, err)
	}
	if err = e.planMarketplace(p, change); err != nil || len(p.Blockers) == 0 {
		t.Fatal("shared owner requirement lost", err, p.Blockers)
	}
	p.Request.Owners = p.Spec.SharedClients
	p.Blockers = nil
	p.Install.Managed = true
	p.Component.Request.Scope = "profile"
	change.Scope = "profile"
	e.reg.Profiles[p.Install.ID] = profile{Root: filepath.Join(e.cfg.Root, "profiles", p.Install.ID)}
	if err = e.planMarketplace(p, change); err != nil || p.Steps[len(p.Steps)-1].Env["HOME"] == "" {
		t.Fatal(err, p.Steps)
	}
}

func TestNativeVerificationProtectsPayloadsAndUnrelatedResources(t *testing.T) {
	for _, id := range []string{"claude", "gemini"} {
		t.Run(id, func(t *testing.T) {
			e, p, change := nativeContractFixture(t, id, "enable", false)
			for _, boundary := range []string{"validate", "read", "readFile", "fingerprint", "readDir"} {
				count := 0
				inject := func(n int) func() {
					if strings.Contains(" validate read fingerprint ", " "+boundary+" ") {
						return installMutationFault(boundary, n, &count)
					}
					return injectArchiveFailure(boundary, n, &count)
				}
				restore := inject(0)
				_ = e.verifyNativeComponent(p)
				restore()
				total := count
				for nth := 1; nth <= total; nth++ {
					count = 0
					restore := inject(nth)
					_ = e.verifyNativeComponent(p)
					restore()
					if count < nth {
						t.Fatal("boundary skipped")
					}
				}
			}
			p.Resources = nil
			_ = atomicWrite(filepath.Join(p.StateRoot, "auth.json"), []byte("retained"), 0600)
			state, _, _ := e.componentEngine(p.Install, "base")
			p.Resources, _ = state.resources(p.Spec)
			p.Resources = append(p.Resources, resource{Path: filepath.Join(e.cfg.Root, "profiles", "retained", "auth.json"), Digest: "retained", Category: auth})
			if err := e.verifyNativeComponent(p); err != nil {
				t.Fatal("unchanged sibling or retained profile rejected", err)
			}
			if id == "claude" {
				file := filepath.Join(p.StateRoot, "plugins", "installed_plugins.json")
				_ = writeJSON(file, claudePluginLedger{Plugins: map[string][]claudePluginRegistration{change.Name: {{Scope: "user", InstallPath: filepath.Join(e.cfg.Home, "foreign")}}}})
				if err := e.verifyNativeComponent(p); err == nil {
					t.Fatal("external native payload accepted")
				}
			}
			p.Component.Request.Scope = "profile"
			if err := e.verifyNativeComponent(p); err == nil {
				t.Fatal("missing profile accepted")
			}
		})
	}
	_, p, change := nativeContractFixture(t, "gemini", "install", false)
	change.Name = ""
	change.Source = "demo"
	p.Component.Request = change
	p.Install.Harness = "unknown"
	e, _ := testEngine(t)
	if err := e.verifyNativeComponent(p); err == nil {
		t.Fatal("unknown installation verified")
	}
}
