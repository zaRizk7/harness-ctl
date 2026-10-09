package manager

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNativeComponentAliasesScopeAndSourceFailure(t *testing.T) {
	change := componentRequest{Category: plugins, Operation: "add", Source: "demo@market", Scope: "profile"}
	e, p := nativePlanFixture(t, "claude", change)
	profileRoot := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
	e.reg.Profiles[p.Install.ID] = profile{Root: profileRoot}
	if err := e.planNativeComponent(p, change); err != nil || p.Steps[0].Env["HOME"] != filepath.Join(profileRoot, "home") {
		t.Fatal(p.Steps, err)
	}
	// Native base state outside manager storage requires every shared owner.
	p.StateRoot = filepath.Join(e.cfg.Home, ".claude")
	change.Scope = "base"
	if err := e.planNativeComponent(p, change); err != nil || len(p.Blockers) == 0 {
		t.Fatal(p.Blockers, err)
	}
	change = componentRequest{Category: plugins, Operation: "install", Name: "demo", Source: t.TempDir()}
	e, p = nativePlanFixture(t, "gemini", change)
	_ = writeJSON(filepath.Join(change.Source, "gemini-extension.json"), map[string]string{"name": "demo"})
	old := fingerprint
	defer func() { fingerprint = old }()
	fault := errors.New("source fingerprint")
	fingerprint = func(path string) (string, error) {
		if path == change.Source {
			return "", fault
		}
		return old(path)
	}
	if err := e.planNativeComponent(p, change); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fingerprint = old
	change = componentRequest{Category: plugins, Operation: "remove", Source: "npm:demo@1"}
	e, p = nativePlanFixture(t, "pi", change)
	_ = writeJSON(filepath.Join(p.StateRoot, "settings.json"), map[string]any{"packages": []any{}})
	if err := e.verifyNativeComponent(p); err != nil {
		t.Fatal(err)
	}
	p.Component.Request = componentRequest{Category: marketplaces, Operation: "remove", Name: "still-present"}
	p.Spec.ID = "claude"
	_ = writeJSON(filepath.Join(p.StateRoot, "plugins/known_marketplaces.json"), map[string]any{"still-present": map[string]any{}})
	if err := e.verifyNativeComponent(p); err == nil {
		t.Fatal("marketplace removal not verified")
	}
	change = componentRequest{Category: marketplaces, Operation: "add", Name: "demo", Source: t.TempDir()}
	e, p = nativePlanFixture(t, "claude", change)
	fingerprint = func(path string) (string, error) {
		if path == change.Source {
			return "", fault
		}
		return old(path)
	}
	if err := e.planMarketplace(p, change); !errors.Is(err, fault) {
		t.Fatal(err)
	}
}

func TestComponentPreservesActiveAndParkedRegistrationIdentity(t *testing.T) {
	e, inst, path := componentFixture(t)
	applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Operation: "disable", Path: path, Field: "/mcpServers/one"})
	value, err := readConfig(path, "json")
	if err != nil {
		t.Fatal(err)
	}
	value["mcpServers"].(map[string]any)["one"] = map[string]any{"command": "externally-restored"}
	if err = writeJSON(path, value); err != nil {
		t.Fatal(err)
	}
	applyComponentRequest(t, e, inst, componentRequest{Category: mcp, Operation: "remove", Path: path, Field: "/mcpServers/one"})
	items, err := e.components(inst, "base", mcp)
	if err != nil || len(items) != 1 || items[0].Name != "two" {
		t.Fatal(items, err)
	}
	root := t.TempDir()
	_ = writeJSON(filepath.Join(root, "meta.json"), parkedComponent{Path: "irrelevant"})
	if err = relocateParkedComponents(root, t.TempDir(), root); err != nil {
		t.Fatal("unrelated metadata relocated", err)
	}
	// Native enablement serialization fails before publishing any writes.
	s, _ := e.specFor("opencode")
	p := &plan{ID: randomID(), Spec: s, Install: installation{Harness: s.ID}, StateRoot: e.stateRoot(s), Request: request{Harness: s.ID, Component: &componentRequest{Category: mcp, Operation: "disable", Path: filepath.Join(e.stateRoot(s), "opencode.json"), Field: "/mcp/demo"}}, RootDigests: map[string]string{}}
	_ = writeJSON(p.Request.Component.Path, map[string]any{"mcp": map[string]any{"demo": map[string]any{"enabled": true}}})
	old := encodeConfig
	defer func() { encodeConfig = old }()
	fault := errors.New("native flag encoding")
	encodeConfig = func(string, map[string]any) ([]byte, error) { return nil, fault }
	if err = e.planComponent(p); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	encodeConfig = old
	asset := filepath.Join(inst.StateRoot, "hooks/demo")
	if err = syscall.Mkfifo(asset, 0600); err != nil {
		_ = os.MkdirAll(filepath.Dir(asset), 0700)
		if err = syscall.Mkfifo(asset, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p.Request = request{Harness: inst.Harness, InstallID: inst.ID, Component: &componentRequest{Category: hooks, Operation: "remove", Path: asset}}
	p.Install = inst
	p.Spec, _ = e.specFor(inst.Harness)
	p.StateRoot = inst.StateRoot
	if err = e.planComponent(p); err == nil {
		t.Fatal("special asset removed")
	}
}

func TestPlanProfileAndOwnershipBoundaryFailures(t *testing.T) {
	e, inst, path := componentFixture(t)
	if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, Action: "manage", Component: &componentRequest{Scope: "profile"}}); err == nil {
		t.Fatal("missing profile accepted")
	}
	if err := os.Symlink(path, filepath.Join(inst.StateRoot, "linked")); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, Action: "reset"})
	if err != nil || !strings.Contains(strings.Join(p.Warnings, " "), "linked source") {
		t.Fatal(p, err)
	}
	old := validateOwnedPath
	defer func() { validateOwnedPath = old }()
	calls := 0
	fault := errors.New("late ownership failure")
	validateOwnedPath = func(root, target string) error {
		if target == path {
			calls++
			if calls > 0 {
				return fault
			}
		}
		return old(root, target)
	}
	p, err = e.buildPlan(context.Background(), request{Harness: inst.Harness, Action: "reset", Preserve: map[category]bool{}})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal(p, err)
	}
	validateOwnedPath = old
	// A tracked native command may be discovered before its service changes.
	e, inst, service := serviceFixture(t)
	t.Setenv("PATH", filepath.Dir(inst.Path))
	count := 0
	oldRead := fileIO.open
	defer func() { fileIO.open = oldRead }()
	fileIO.open = func(file string) (*os.File, error) {
		if file == service {
			count++
			if count == 1 {
				return nil, fault
			}
		}
		return oldRead(file)
	}
	p, err = e.buildPlan(context.Background(), request{Harness: inst.Harness, Action: "reset"})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal(p, err)
	}
}

func TestTrackedExecutionAndMigratedDiscardFailures(t *testing.T) {
	for _, scenario := range []string{"success", "missing-command"} {
		t.Run(scenario, func(t *testing.T) {
			e, _ := testEngine(t)
			s, _ := e.specFor("pi")
			root := filepath.Join(e.cfg.Home, "prefix/lib/node_modules", s.Package)
			path := filepath.Join(root, "bin/pi")
			_ = atomicWrite(path, []byte("fixture"), 0700)
			_ = writeJSON(filepath.Join(root, "package.json"), map[string]string{"name": s.Package, "version": "1"})
			p := &plan{ID: randomID(), Spec: s, Install: installation{ID: installID(s.ID, path), Harness: s.ID, Path: path, Root: root, Method: "npm", Package: s.Package}, StateRoot: e.stateRoot(s), Request: request{Harness: s.ID, Action: "update", Model: "tracked", Target: "1", Preserve: keepAll()}, Steps: []command{{Path: "synthetic", Description: "fixture update"}}}
			p.RegistryDigest, _ = fingerprint(e.statePath)
			p.InstallDigest, _ = fingerprint(root)
			e.run = commandRunner(func(_ context.Context, c command) (string, error) {
				if c.Description == "fixture update" && scenario == "missing-command" {
					_ = os.Remove(path)
				}
				return "", nil
			})
			err := e.execute(context.Background(), p, p.ID, nil)
			if (err != nil) != (scenario == "missing-command") {
				t.Fatal(err)
			}
		})
	}
	e, inst, _ := componentFixture(t)
	path := filepath.Join(inst.StateRoot, "auth.json")
	_ = atomicWrite(path, []byte("fixture"), 0600)
	s, _ := e.specFor(inst.Harness)
	p := &plan{Spec: s, Request: request{Harness: s.ID, Preserve: map[category]bool{}}}
	old := fileIO
	defer func() { fileIO = old }()
	fault := errors.New("discard failure")
	fileIO.removeAll = func(target string) error {
		if target == path {
			return fault
		}
		return old.removeAll(target)
	}
	if err := e.applyMigratedState(p); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	r := resource{Path: filepath.Join(e.cfg.Root, "profiles/any"), Category: skills}
	if err := e.verifyState(&plan{Request: request{Action: "profile"}, Resources: []resource{r}}); err != nil {
		t.Fatal(err)
	}
}

func TestShimOwnershipEnvironmentAndNativeLaunchValidation(t *testing.T) {
	e, inst, _ := componentFixture(t)
	s, _ := e.specFor(inst.Harness)
	prof := profile{Root: filepath.Join(e.cfg.Root, "profiles", inst.ID), Disabled: map[category]bool{proxies: true}}
	e.reg.Profiles[inst.ID] = prof
	if err := e.writeShim(inst); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(e.cfg.BinDir, s.Command)
	data, _ := os.ReadFile(shim)
	if !strings.Contains(string(data), "unset HTTP_PROXY") {
		t.Fatal("proxy exclusions missing")
	}
	_ = atomicWrite(shim, []byte("foreign"), 0700)
	if err := e.writeShim(inst); err == nil {
		t.Fatal("foreign launcher replaced")
	}
	if err := e.removeShim(inst); err != nil {
		t.Fatal("foreign launcher not retained", err)
	}
	e.cfg.BinDir = filepath.Dir(inst.Path)
	if err := e.launchCLI(context.Background(), []string{inst.Harness}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("recursive native launch accepted")
	}
	e.cfg.BinDir = filepath.Join(e.cfg.Root, "bin")
	_ = atomicWrite(inst.Path, []byte("#!/bin/sh\nexit 0\n"), 0700)
	for i := range e.cfg.Providers {
		if e.cfg.Providers[i].ID == "openai" {
			e.cfg.Providers[i].KeyEnv = s.HomeEnv
		}
	}
	if err := e.saveAccount(account{ID: "a", Provider: "openai", Kind: "api", Enabled: true, Credential: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := e.launchCLI(context.Background(), []string{"--account", "a", inst.Harness}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("provider override changed owned state")
	}
	// OpenCode's separate XDG directories fail independently before publication.
	s, _ = e.specFor("opencode")
	inst = syntheticInstall(t, e, s, "1")
	old := fileIO.mkdir
	defer func() { fileIO.mkdir = old }()
	fault := errors.New("xdg directory")
	fileIO.mkdir = func(path string, mode fs.FileMode) error {
		if strings.Contains(path, "/xdg/data/") {
			return fault
		}
		return old(path, mode)
	}
	if err := e.writeShim(inst); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if _, err := (systemRunner{}).Run(context.Background(), command{Path: "/usr/bin/true", Env: map[string]string{"FIXTURE": "value"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCompatiblePayloadSetupAndActiveCLIContracts(t *testing.T) {
	source := t.TempDir()
	_ = writeJSON(filepath.Join(source, ".claude-plugin/plugin.json"), map[string]string{"name": "demo"})
	if err := validateCompatibility("claude", componentRequest{Category: plugins, Source: source}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	_ = writeJSON(filepath.Join(root, "settings.json"), map[string]any{"packages": []any{"npm:demo@1"}})
	_ = writeJSON(filepath.Join(root, "npm/node_modules/demo/package.json"), map[string]any{"name": "demo", "version": "2", "pi": map[string]any{}})
	if err := verifyPiPackage(root, "npm:demo@1", "install"); err == nil {
		t.Fatal("different native package version accepted")
	}
	e, inst, _ := componentFixture(t)
	t.Setenv("PATH", filepath.Dir(inst.Path))
	if err := e.componentsCLI(context.Background(), []string{"list", inst.Harness, "mcp"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := e.setupCLI(context.Background(), []string{"--binary", filepath.Join(e.cfg.Home, "missing"), "--sha256", strings.Repeat("a", 64), "--preview"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing setup binary accepted")
	}
	_ = e.saveRecord(operationRecord{ID: randomID(), Harness: inst.Harness, Status: "executing"})
	if err := e.setupCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("setup overlapped pending recovery")
	}
}
