package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// componentFaultFixture prepares active or parked entries before fault injection.
func componentFaultFixture(t *testing.T, scenario string) (*engine, func() error, string) {
	t.Helper()
	e, inst, configPath := componentFixture(t)
	marker := filepath.Join(e.cfg.Home, "unrelated", "keep")
	if err := atomicWrite(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	change := componentRequest{Operation: "disable", Category: mcp, Path: configPath, Field: "/mcpServers/one"}
	asset := filepath.Join(inst.StateRoot, "skills", "demo")
	if strings.HasPrefix(scenario, "asset") {
		if err := atomicWrite(filepath.Join(asset, "SKILL.md"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
		change = componentRequest{Operation: "disable", Category: skills, Path: asset}
	}
	if strings.HasPrefix(scenario, "array") {
		if err := writeJSON(configPath, map[string]any{"hooks": []any{map[string]any{"name": "one"}, map[string]any{"name": "two"}, map[string]any{"name": "three"}}}); err != nil {
			t.Fatal(err)
		}
		change = componentRequest{Operation: "disable", Category: hooks, Path: configPath, Field: "/hooks/1"}
	}
	if strings.Contains(scenario, "parked") {
		applyComponentRequest(t, e, inst, change)
		items, err := e.components(inst, "base", change.Category)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.Parked != "" {
				change.Parked = item.Parked
				change.Field = item.Field
				break
			}
		}
		if change.Parked == "" {
			t.Fatal("parked fixture missing")
		}
		change.Operation = "enable"
		if strings.HasSuffix(scenario, "edit") {
			change.Operation = "edit"
			change.Value = json.RawMessage(`{"command":"replacement"}`)
			if change.Field == "" {
				change.Subpath = "SKILL.md"
				change.Content = []byte("replacement")
			}
		}
		if strings.HasSuffix(scenario, "remove") {
			change.Operation = "remove"
		}
	}
	if scenario == "asset-import" {
		change.Operation = "install"
		change.Path = filepath.Join(inst.StateRoot, "skills", "imported")
		change.Source = asset
	}
	if scenario == "asset-files" {
		change.Operation = "install"
		change.Path = filepath.Join(inst.StateRoot, "skills", "new")
		change.Files = map[string][]byte{"SKILL.md": []byte("new"), "script.sh": []byte("script")}
	}
	operation := func() error {
		if strings.HasSuffix(scenario, "inventory") {
			_, err := e.components(inst, "base", change.Category)
			return err
		}
		p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
		if err != nil {
			return err
		}
		return e.execute(context.Background(), p, p.ID, nil)
	}
	return e, operation, marker
}

func TestComponentFailuresKeepUnrelatedStateAndAuthenticatedRecovery(t *testing.T) {
	for _, scenario := range []string{"field-disable", "field-parked-enable", "field-parked-edit", "field-parked-remove", "asset-import", "asset-files", "asset-disable", "asset-parked-enable", "asset-parked-edit", "asset-parked-remove", "asset-inventory", "asset-parked-inventory", "array-disable", "array-parked-enable", "array-parked-edit", "array-parked-remove"} {
		for _, boundary := range []string{"validate", "fingerprint", "read", "atomic", "json", "lstat", "stat", "readDir", "readFile", "walk", "open", "mkdir", "removeAll"} {
			t.Run(scenario+"/"+boundary, func(t *testing.T) {
				_, operation, _ := componentFaultFixture(t, scenario)
				count := 0
				inject := func(n int, c *int) func() {
					if strings.Contains(" validate fingerprint read atomic json ", " "+boundary+" ") {
						return installMutationFault(boundary, n, c)
					}
					return injectArchiveFailure(boundary, n, c)
				}
				restore := inject(0, &count)
				err := operation()
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, operation, marker := componentFaultFixture(t, scenario)
						count := 0
						restore := inject(nth, &count)
						_ = operation()
						restore()
						if count < nth {
							t.Fatal("fault boundary not reached")
						}
						data, err := os.ReadFile(marker)
						if err != nil || string(data) != "keep" {
							t.Fatal("unrelated state changed", err)
						}
						if err = e.refreshRegistry(); err != nil {
							t.Fatal("invalid registry", err)
						}
						metas, err := e.snapshots()
						if err != nil {
							t.Fatal("invalid recovery index", err)
						}
						for _, meta := range metas {
							if _, err = e.authenticatedSnapshot(meta.ID); err != nil {
								t.Fatal("recovery lost integrity", err)
							}
						}
					})
				}
			})
		}
	}
}
