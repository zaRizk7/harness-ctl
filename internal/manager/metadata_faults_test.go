package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// metadataFaultFixture exercises read-only inventory and editor preparation.
func metadataFaultFixture(t *testing.T, scenario string) (*engine, func() error, string) {
	t.Helper()
	m := componentModel(t)
	e := m.e
	inst := m.componentInstallation()
	marker := filepath.Join(e.cfg.Home, "unrelated", "keep")
	_ = atomicWrite(marker, []byte("keep"), 0600)
	_ = e.saveAccount(account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true})
	_ = e.saveLibraryItem(reusableSkill("demo"), "")
	t.Setenv("TMPDIR", t.TempDir())
	asset := filepath.Join(inst.StateRoot, "skills", "demo")
	_ = atomicWrite(filepath.Join(asset, "SKILL.md"), []byte("skill"), 0600)
	field := componentItem{Path: filepath.Join(inst.StateRoot, "settings.json"), Field: "/mcpServers/one", Category: mcp}
	var cmd tea.Cmd
	operation := func() error {
		switch scenario {
		case "skill-inventory":
			_, err := e.components(inst, "base", skills)
			return err
		case "plugin-inventory":
			_, err := e.components(inst, "base", plugins)
			return err
		case "edit-field":
			_, err := e.componentEditData(inst, "base", field)
			return err
		case "edit-asset":
			_, err := e.componentEditData(inst, "base", componentItem{Path: filepath.Join(asset, "SKILL.md"), Category: skills})
			return err
		case "editor-component":
			cmd = m.editComponent(&field)
		case "editor-account":
			cmd = m.editAccount("a")
		case "editor-library":
			cmd = m.editLibrary("demo")
		case "library-view":
			cmd = m.loadLibraryView()
		case "account-view":
			cmd = m.loadAccountsView()
		case "monitor":
			cmd = m.loadGlobalMonitor()
		case "options":
			cmd = m.loadOptions()
		case "inventory":
			cmd = m.inventory()
		case "recovery-view":
			cmd = m.loadRecovery()
		case "batch-options":
			m.batchSelected = map[string]bool{inst.Harness: true}
			cmd = m.batchOptions()
		case "component-view":
			cmd = m.loadComponents()
		}
		if cmd == nil {
			t.Fatal("missing metadata command")
		}
		switch msg := cmd().(type) {
		case componentEditorMsg:
			return msg.err
		case accountEditMsg:
			return msg.err
		case libraryEditMsg:
			return msg.err
		case libraryMsg:
			return msg.err
		case accountsMsg:
			return msg.err
		case globalMonitorMsg:
			return msg.err
		case optionsMsg:
			return msg.err
		case inventoryMsg:
			return msg.err
		case recoveryMsg:
			return msg.err
		case componentsMsg:
			return msg.err
		}
		return nil
	}
	return e, operation, marker
}

func TestMetadataFailuresRemainReadOnlyAndDoNotExposeCredentials(t *testing.T) {
	for _, scenario := range []string{"skill-inventory", "plugin-inventory", "edit-field", "edit-asset", "editor-component", "editor-account", "editor-library", "library-view", "account-view", "monitor", "options", "inventory", "recovery-view", "batch-options", "component-view"} {
		for _, boundary := range []string{"validate", "fingerprint", "read", "readDir", "lstat", "stat", "walk", "info", "readFile", "open"} {
			t.Run(scenario+"/"+boundary, func(t *testing.T) {
				inject := func(n int, c *int) func() {
					if strings.Contains(" validate fingerprint read ", " "+boundary+" ") {
						return installMutationFault(boundary, n, c)
					}
					return injectArchiveFailure(boundary, n, c)
				}
				_, operation, _ := metadataFaultFixture(t, scenario)
				count := 0
				restore := inject(0, &count)
				err := operation()
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, operation, marker := metadataFaultFixture(t, scenario)
						before, _ := fingerprint(e.cfg.Root)
						count := 0
						restore := inject(nth, &count)
						_ = operation()
						restore()
						if count < nth {
							t.Fatal("failure boundary not reached")
						}
						after, _ := fingerprint(e.cfg.Root)
						data, err := os.ReadFile(marker)
						if err != nil || string(data) != "keep" || before != after {
							t.Fatal("read-only workflow mutated manager state", err)
						}
					})
				}
			})
		}
	}
}

func TestNativeInventoriesIncludeManualPluginPayloadsAndErrors(t *testing.T) {
	for _, id := range []string{"claude", "gemini", "codex"} {
		t.Run(id, func(t *testing.T) {
			e, _, inst := nativeComponentFixture(t, id)
			root := inst.StateRoot
			if id == "claude" {
				_ = writeJSON(filepath.Join(root, "plugins", "installed_plugins.json"), claudePluginLedger{Plugins: map[string][]claudePluginRegistration{"demo@market": {{Scope: "user", InstallPath: filepath.Join(root, "plugins", "cache", "demo")}}}})
			}
			if id == "gemini" {
				_ = writeJSON(filepath.Join(root, "extensions", "demo", "gemini-extension.json"), map[string]string{"name": "demo", "version": "1"})
				_ = writeJSON(filepath.Join(root, "extensions", "extension-enablement.json"), map[string]any{"demo": map[string]any{"overrides": []string{filepath.Dir(root) + "/*"}}})
			}
			if id == "codex" {
				_ = atomicWrite(filepath.Join(e.codexSkillsRoot(), "skills", "demo", "SKILL.md"), []byte("skill"), 0600)
			}
			for _, boundary := range []string{"validate", "fingerprint", "read", "stat", "lstat", "readDir", "readFile", "info"} {
				t.Run(boundary, func(t *testing.T) {
					cat := plugins
					if id == "codex" {
						cat = skills
					}
					count := 0
					inject := func(n int) func() {
						if strings.Contains(" validate fingerprint read ", " "+boundary+" ") {
							return installMutationFault(boundary, n, &count)
						}
						return injectArchiveFailure(boundary, n, &count)
					}
					restore := inject(0)
					_, err := e.components(inst, "base", cat)
					restore()
					if err != nil {
						t.Fatal(err)
					}
					total := count
					for nth := 1; nth <= total; nth++ {
						count = 0
						restore := inject(nth)
						_, _ = e.components(inst, "base", cat)
						restore()
						if count < nth {
							t.Fatal("failure point not reached")
						}
					}
				})
			}
			if _, _, err := e.componentEngine(inst, "profile"); err == nil {
				t.Fatal("missing profile accepted")
			}
			if _, err := e.components(installation{Harness: "unknown"}, "base", plugins); err == nil {
				t.Fatal("unknown harness accepted")
			}
		})
	}
}
