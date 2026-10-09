package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexMarketplaceRejectsInvalidRegistrations(t *testing.T) {
	for _, body := range []string{"invalid = [", "marketplaces = false", "[marketplaces]\nbad = []", "[marketplaces.'../bad']\nsource = 'repo'"} {
		t.Run(body, func(t *testing.T) {
			e, _, inst := nativeComponentFixture(t, "codex")
			if err := atomicWrite(filepath.Join(inst.StateRoot, "config.toml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := codexMarketplaceItems(inst.StateRoot, nil); err == nil {
				t.Fatal("invalid marketplace inventory accepted")
			}
			p := &plan{Spec: e.cfg.Harnesses[0], Install: inst, StateRoot: inst.StateRoot, Component: &componentMutation{Request: componentRequest{Name: "demo"}}}
			if err := e.planCodexMarketplace(p, componentRequest{Name: "demo", Operation: "add", Source: "owner/repo"}); err == nil {
				t.Fatal("invalid registry planned")
			}
			if verifyCodexMarketplace(p) == nil {
				t.Fatal("invalid registry verified")
			}
		})
	}
}

func TestCodexMarketplacePlanningRejectsUnsafeSourcesAndNativeFailures(t *testing.T) {
	for _, scenario := range []string{"installation", "operation", "existence", "cache-path", "cache-io", "cache-tree", "source", "overlap", "source-tree", "source-digest", "native-command", "local"} {
		t.Run(scenario, func(t *testing.T) {
			e, _, inst := nativeComponentFixture(t, "codex")
			change := componentRequest{Name: "demo", Operation: "add", Source: "owner/repo"}
			p := &plan{Spec: e.cfg.Harnesses[0], Install: inst, StateRoot: inst.StateRoot, RootDigests: map[string]string{}, Component: &componentMutation{Request: change}}
			cache := filepath.Join(inst.StateRoot, ".tmp", "marketplaces")
			local := filepath.Join(e.cfg.Home, "local-marketplace")
			oldValidate, oldFingerprint, oldIO := validateOwnedPath, fingerprint, fileIO
			t.Cleanup(func() { validateOwnedPath, fingerprint, fileIO = oldValidate, oldFingerprint, oldIO })
			fault := errors.New("synthetic marketplace boundary")
			switch scenario {
			case "installation":
				p.Install.ID = ""
			case "operation":
				change.Operation = "disable"
			case "existence":
				change.Operation = "remove"
			case "cache-path":
				validateOwnedPath = func(root, path string) error {
					if path == cache {
						return fault
					}
					return oldValidate(root, path)
				}
			case "cache-io":
				fileIO.lstat = func(path string) (os.FileInfo, error) {
					if path == cache {
						return nil, fault
					}
					return oldIO.lstat(path)
				}
			case "cache-tree":
				if err := os.MkdirAll(cache, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(e.cfg.Home, filepath.Join(cache, "linked")); err != nil {
					t.Fatal(err)
				}
			case "source":
				change.Source = "--unsafe"
			case "overlap":
				change.Source = inst.StateRoot
			case "source-tree":
				change.Source = local
			case "source-digest", "local":
				change.Source = local
				if err := atomicWrite(filepath.Join(local, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"demo"}`), 0600); err != nil {
					t.Fatal(err)
				}
				if scenario == "source-digest" {
					fingerprint = func(path string) (string, error) {
						if path == local {
							return "", fault
						}
						return oldFingerprint(path)
					}
				}
			case "native-command":
				e.cfg.BinDir = inst.Root
			}
			err := e.planCodexMarketplace(p, change)
			if scenario == "local" {
				if err != nil || p.RootDigests[local] == "" || len(p.Blockers) == 0 {
					t.Fatal("local source fingerprint/owners missing", err)
				}
				if err := atomicWrite(filepath.Join(inst.StateRoot, "config.toml"), []byte("[marketplaces.demo]\nsource_type = 'local'\nsource = '"+local+"'\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := verifyCodexMarketplace(p); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsafe marketplace planned")
			}
		})
	}
}

func TestCodexMarketplaceResultRejectsMissingUnsafeAndIncorrectPayloads(t *testing.T) {
	for _, scenario := range []string{"absent", "still-registered", "relative-local", "linked-local", "owned-path", "missing-tree"} {
		t.Run(scenario, func(t *testing.T) {
			e, _, inst := nativeComponentFixture(t, "codex")
			p := &plan{StateRoot: inst.StateRoot, Component: &componentMutation{Request: componentRequest{Name: "demo", Operation: "add"}}}
			body := "[marketplaces.demo]\nsource_type = 'git'\nsource = 'owner/repo'\n"
			oldValidate := validateOwnedPath
			t.Cleanup(func() { validateOwnedPath = oldValidate })
			switch scenario {
			case "absent":
				body = "model = 'preserved'\n"
			case "still-registered":
				p.Component.Request.Operation = "remove"
			case "relative-local":
				body = "[marketplaces.demo]\nsource_type = 'local'\nsource = 'relative'\n"
			case "linked-local":
				path := filepath.Join(e.cfg.Home, "linked")
				if err := os.Symlink(e.cfg.Home, path); err != nil {
					t.Fatal(err)
				}
				body = "[marketplaces.demo]\nsource_type = 'local'\nsource = '" + path + "'\n"
			case "owned-path":
				validateOwnedPath = func(root, path string) error {
					if filepath.Base(path) == "demo" {
						return errors.New("synthetic owned cache failure")
					}
					return oldValidate(root, path)
				}
			}
			if err := atomicWrite(filepath.Join(inst.StateRoot, "config.toml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if verifyCodexMarketplace(p) == nil {
				t.Fatal("unsafe result accepted")
			}
		})
	}
	e, _, inst := nativeComponentFixture(t, "codex")
	old := validateOwnedPath
	t.Cleanup(func() { validateOwnedPath = old })
	validateOwnedPath = func(string, string) error { return errors.New("synthetic registry path") }
	if _, err := readCodexMarketplaces(inst.StateRoot); err == nil {
		t.Fatal("unsafe registry path accepted")
	}
	validateOwnedPath = old
	// A local source fingerprint invalidates approval when another tool edits it.
	local := filepath.Join(e.cfg.Home, "local")
	if err := atomicWrite(filepath.Join(local, ".agents", "plugins", "marketplace.json"), []byte(`{"name":"demo"}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "manage", Owners: e.cfg.Harnesses[0].SharedClients, Component: &componentRequest{Category: marketplaces, Operation: "add", Name: "demo", Source: local, Native: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(local, "changed"), []byte("outside change"), 0600); err != nil {
		t.Fatal(err)
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("changed local marketplace source accepted")
	}
}
