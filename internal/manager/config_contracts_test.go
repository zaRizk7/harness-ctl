package manager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigurationRejectsUnsafeAndUnavailableContracts(t *testing.T) {
	for _, change := range []func(*config){func(c *config) { c.Providers[0].ID = "" }, func(c *config) { c.MonitorWindowDays = 0 }, func(c *config) { c.MonitorMaxPages = 0 }, func(c *config) { c.Harnesses = nil }, func(c *config) { c.Harnesses[0].ID = "" }, func(c *config) { c.RefreshSeconds = 0 }, func(c *config) { c.AdditionalCommands = []string{"bad/path"} }, func(c *config) { c.Root = "relative" }, func(c *config) { c.Root = c.Home }, func(c *config) { c.BackupDays = 0 }, func(c *config) { c.ReleaseChannel = "nightly" }, func(c *config) { c.PackageBytes = 0 }, func(c *config) { c.BinDir = filepath.Join(c.Home, "foreign-bin") }, func(c *config) { c.StateRoots = map[string]string{"pi": c.Home} }} {
		c := defaultConfig(t.TempDir())
		change(&c)
		if c.validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	c := defaultConfig(t.TempDir())
	c.Root = filepath.Join(c.Home, "linked")
	c.BinDir = filepath.Join(c.Root, "bin")
	if err := os.Symlink(t.TempDir(), c.Root); err != nil {
		t.Fatal(err)
	}
	if c.validate() == nil {
		t.Fatal("linked manager root accepted")
	}
}

func TestRegistryOwnershipValidationAndReload(t *testing.T) {
	for _, change := range []func(*installation){func(i *installation) { i.Harness = "missing" }, func(i *installation) { i.ID = "wrong" }, func(i *installation) { i.Managed = false }, func(i *installation) { i.Root = "/foreign" }, func(i *installation) { i.Path = i.Root; i.ID = installID(i.Harness, i.Path) }, func(i *installation) { i.StateRoot = "/foreign" }} {
		e, _ := testEngine(t)
		s, _ := e.specFor("pi")
		inst := syntheticInstall(t, e, s, "1")
		change(&inst)
		if e.validateRegistry(registry{Installs: []installation{inst}}) == nil {
			t.Fatal("foreign registry accepted", inst)
		}
	}
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1")
	if e.validateRegistry(registry{Installs: []installation{inst, inst}}) == nil {
		t.Fatal("duplicate identity accepted")
	}
	for _, prof := range []profile{{Root: "/foreign"}, {Root: filepath.Join(e.cfg.Root, "profiles", inst.ID), Disabled: map[category]bool{"invalid": true}}} {
		if e.validateRegistry(registry{Installs: []installation{inst}, Profiles: map[string]profile{inst.ID: prof}}) == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	if e.validateRegistry(registry{Profiles: map[string]profile{"missing": {}}}) == nil {
		t.Fatal("unknown profile owner accepted")
	}
	if err := writeJSON(e.statePath, registry{Installs: []installation{inst}}); err != nil {
		t.Fatal(err)
	}
	if err := e.refreshRegistry(); err != nil || e.reg.Profiles == nil {
		t.Fatal(err)
	}
	if err := atomicWrite(e.statePath, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if e.refreshRegistry() == nil {
		t.Fatal("corrupt registry accepted")
	}
	c := e.cfg
	if _, err := newEngine(c, e.run); err == nil {
		t.Fatal("startup accepted corrupt registry")
	}
	_ = writeJSON(e.statePath, registry{Profiles: map[string]profile{"missing": {}}})
	if _, err := newEngine(c, e.run); err == nil {
		t.Fatal("startup accepted foreign profile")
	}
	_ = writeJSON(e.statePath, registry{})
	if loaded, err := newEngine(c, e.run); err != nil || loaded.reg.Profiles == nil {
		t.Fatal(err)
	}
}

func TestNativeStateEnvironmentAndUnsupportedResources(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	root := filepath.Join(e.cfg.Home, "custom")
	t.Setenv(s.HomeEnv, root)
	if e.stateRoot(s) != root {
		t.Fatal("native environment ignored")
	}
	s, _ = e.specFor("opencode")
	t.Setenv("XDG_CONFIG_HOME", root)
	for _, key := range []string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, filepath.Join(root, key))
	}
	roots := e.rootsFor(s)
	if len(roots) != 4 || roots[0] != filepath.Join(root, "opencode") || roots[1] != filepath.Join(root, "XDG_DATA_HOME", "opencode") {
		t.Fatal(roots)
	}
	if _, ok := e.resourceDestination(s, roots[0], e.managedStateRoot(s), resource{Root: "/foreign", Path: "/foreign/value"}); ok {
		t.Fatal("foreign state relocated")
	}
	if actionName("custom") != "custom" {
		t.Fatal("custom label lost")
	}
	m := newModel(e)
	m.cursor = 99
	m.toggleOption()
	got := presentCategories([]resource{{Fields: map[string]category{"/auth": auth, "/mcp": mcp}}})
	if !reflect.DeepEqual(got, []category{auth, mcp}) {
		t.Fatal(got)
	}
	if e.retainedInstallation("missing").Harness != "missing" {
		t.Fatal("missing identity lost")
	}
	t.Setenv("PATH", "relative:")
	if len(commandPaths("pi")) != 0 {
		t.Fatal("relative PATH trusted")
	}
	if _, err := lookPath("missing-executable"); err == nil {
		t.Fatal("missing dependency accepted")
	}
}
