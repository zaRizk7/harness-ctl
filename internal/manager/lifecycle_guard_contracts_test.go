package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRejectsInvalidLifecycleAndOwnershipRequests(t *testing.T) {
	e, _ := testEngine(t)
	for _, req := range []request{{Harness: "missing", Action: "reset"}, {Harness: "pi", Action: "bad"}, {Harness: "pi", Action: "install", Target: "../bad"}, {Harness: "pi", Action: "install", Model: "bad"}, {Harness: "pi", Action: "reset", InstallID: "missing"}, {Harness: "pi", Action: "manage", Permanent: true}, {Harness: "pi", Action: "manage", RemoveOld: true}} {
		if _, err := e.buildPlan(context.Background(), req); err == nil {
			t.Fatal(req)
		}
	}
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	p, err := e.buildPlan(context.Background(), request{Harness: "pi", Action: "migrate", RemoveOld: true})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal(err, p)
	}
	p, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "install"})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal(err, p)
	}
	old := fingerprint
	defer func() { fingerprint = old }()
	calls := 0
	fingerprint = func(path string) (string, error) {
		if path == e.statePath {
			calls++
			if calls > 1 {
				return "concurrent-registry", nil
			}
		}
		return old(path)
	}
	if _, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "reset"}); err == nil || !strings.Contains(err.Error(), "registry changed") {
		t.Fatal(err)
	}
	fingerprint = old
	p = resetPlan(t, e)
	p.RegistryDigest = "changed"
	if err = e.validatePlan(p); err == nil {
		t.Fatal("changed registry accepted")
	}
	p.RegistryDigest, _ = fingerprint(e.statePath)
	p.NativeScript = []byte("changed installer")
	p.Integrity = "sha256:unapproved"
	if err = e.validatePlan(p); err == nil {
		t.Fatal("changed installer accepted")
	}
}

func TestTrackedSharedAuthRequiresOwnerAndPermanentConsent(t *testing.T) {
	for _, id := range []string{"codex", "claude"} {
		t.Run(id, func(t *testing.T) {
			e, _ := testEngine(t)
			s, _ := e.specFor(id)
			root := filepath.Join(e.cfg.Home, ".local/share", id)
			if id == "codex" {
				root = filepath.Join(e.stateRoot(s), "packages/standalone")
			}
			payload := filepath.Join(root, "1.0.0", s.Command)
			_ = atomicWrite(payload, []byte("fixture"), 0700)
			path := filepath.Join(e.cfg.Home, ".local/bin", s.Command)
			_ = os.MkdirAll(filepath.Dir(path), 0700)
			_ = os.Symlink(payload, path)
			_ = atomicWrite(filepath.Join(e.stateRoot(s), "auth.json"), []byte("fixture"), 0600)
			for _, permanent := range []bool{false, true} {
				p, err := e.buildPlan(context.Background(), request{Harness: id, Action: "reset", Preserve: map[category]bool{}, Owners: s.SharedClients, Permanent: permanent})
				if err != nil {
					t.Fatal(err)
				}
				if !permanent && len(p.Blockers) == 0 {
					t.Fatal("OS secret discard without permanent consent")
				}
				if permanent && (len(p.Steps) != 1 || !strings.Contains(p.Steps[0].Description, "logout")) {
					t.Fatal(p.Steps)
				}
			}
			p, err := e.buildPlan(context.Background(), request{Harness: id, Action: "reset", Preserve: map[category]bool{}})
			if err != nil || len(p.Warnings) == 0 {
				t.Fatal(err, p)
			}
			p, err = e.buildPlan(context.Background(), request{Harness: id, Action: "profile"})
			if err != nil || len(p.Blockers) == 0 {
				t.Fatal("tracked profile accepted", err)
			}
		})
	}
}

func TestDiscoveryIdentifiesOnlyOwnedNativePayloads(t *testing.T) {
	e, _ := testEngine(t)
	for _, id := range []string{"codex", "claude", "prime-agent"} {
		s, _ := e.specFor(id)
		root := filepath.Join(e.cfg.Home, ".local/share", id)
		if id == "codex" {
			root = filepath.Join(e.stateRoot(s), "packages/standalone")
		}
		inst := installation{}
		e.identify(s, filepath.Join(root, "1.0.0", s.Command), &inst)
		if !strings.HasPrefix(inst.Method, "native-") || inst.Root != root {
			t.Fatal(id, inst)
		}
	}
	s, _ := e.specFor("pi")
	s.BrewPackages = []string{"pi"}
	inst := installation{}
	e.identify(s, filepath.Join(e.cfg.Home, "Cellar/pi/1.0.0/bin/pi"), &inst)
	if inst.Method != "brew" || inst.Package != "pi" {
		t.Fatal(inst)
	}
	inst = syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	t.Setenv("PATH", filepath.Dir(inst.Path))
	e.cfg.AdditionalCommands = []string{"extra"}
	_ = atomicWrite(filepath.Join(filepath.Dir(inst.Path), "extra"), []byte("fixture"), 0700)
	items, err := e.discover(context.Background())
	if err != nil || len(items) != 2 || items[0].ID != inst.ID || items[1].Method != "unsupported" {
		t.Fatal(items, err)
	}
	old := fileIO
	defer func() { fileIO = old }()
	fileIO.eval = func(string) (string, error) { return "", errors.New("resolution failure") }
	items, err = e.discover(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatal("registered install was lost on probe failure", items, err)
	}
}

func TestInstallationVerificationRejectsEscapesModesAndIdentity(t *testing.T) {
	for _, scenario := range []string{"escape", "mode", "identity", "read-failure"} {
		t.Run(scenario, func(t *testing.T) {
			e, _ := testEngine(t)
			s, _ := e.specFor("pi")
			inst := syntheticInstall(t, e, s, "1")
			p := &plan{Spec: s, Destination: inst.Root, Request: request{Target: "1"}, StateRoot: inst.StateRoot}
			switch scenario {
			case "escape":
				outside := filepath.Join(e.cfg.Home, "foreign")
				_ = atomicWrite(outside, []byte("foreign"), 0700)
				_ = os.Remove(inst.Path)
				_ = os.Symlink(outside, inst.Path)
			case "mode":
				_ = os.Chmod(inst.Path, 0600)
			case "identity":
				_ = writeJSON(filepath.Join(inst.Root, "lib/node_modules", s.Package, "package.json"), map[string]string{"name": "foreign", "version": "1"})
			case "read-failure":
				old := fileIO
				defer func() { fileIO = old }()
				fileIO.stat = func(string) (os.FileInfo, error) { return nil, errors.New("stat failure") }
			}
			if _, err := e.installedResult(p); err == nil {
				t.Fatal("unverified install accepted")
			}
		})
	}
}

func TestNativeUninstallAndLauncherErrorsPreserveOwnership(t *testing.T) {
	for _, boundary := range []string{"success", "validate", "remove-root", "service-validate", "service-remove", "launcher-stat", "launcher-read"} {
		t.Run(boundary, func(t *testing.T) {
			e, _ := testEngine(t)
			root := filepath.Join(e.cfg.Home, ".local/share/claude")
			target := filepath.Join(root, "bin/claude")
			_ = atomicWrite(target, []byte("fixture"), 0700)
			path := filepath.Join(e.cfg.Home, ".local/bin/claude")
			_ = os.MkdirAll(filepath.Dir(path), 0700)
			if strings.HasPrefix(boundary, "launcher") {
				_ = atomicWrite(path, []byte("#!/bin/sh\nexec "+target+"\n"), 0700)
			} else {
				_ = os.Symlink(target, path)
			}
			service := filepath.Join(e.cfg.Home, "Library/LaunchAgents/demo.plist")
			_ = atomicWrite(service, []byte("fixture"), 0600)
			inst := installation{Path: path, Root: root, Method: "native-claude", ServicePaths: []string{service}}
			oldIO, oldValidate := fileIO, validateOwnedPath
			defer func() { fileIO, validateOwnedPath = oldIO, oldValidate }()
			fault := errors.New("uninstall boundary")
			switch boundary {
			case "validate":
				validateOwnedPath = func(string, string) error { return fault }
			case "remove-root":
				fileIO.removeAll = func(string) error { return fault }
			case "service-validate":
				validateOwnedPath = func(r, p string) error {
					if p == service {
						return fault
					}
					return oldValidate(r, p)
				}
			case "service-remove":
				fileIO.remove = func(p string) error {
					if p == service {
						return fault
					}
					return oldIO.remove(p)
				}
			case "launcher-stat":
				fileIO.lstat = func(string) (os.FileInfo, error) { return nil, fault }
			case "launcher-read":
				fileIO.readFile = func(string) ([]byte, error) { return nil, fault }
			}
			err := e.removeInstallation(inst)
			if boundary == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, fault) {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecycleRecordsAndInvalidLockedApproval(t *testing.T) {
	e, _ := testEngine(t)
	if err := e.executeLocked(context.Background(), nil, "", nil); err == nil {
		t.Fatal("locked execution bypassed approval")
	}
	if err := e.saveRecord(operationRecord{ID: "../bad"}); err == nil {
		t.Fatal("unsafe journal accepted")
	}
	path := filepath.Join(e.cfg.Root, "operations", "bad.json")
	_ = writeJSON(path, operationRecord{ID: "../bad"})
	if _, err := e.records(); err == nil {
		t.Fatal("unsafe journal loaded")
	}
	_ = os.Remove(path)
	_ = atomicWrite(filepath.Join(e.cfg.Root, "operations", "notes.txt"), nil, 0600)
	if records, err := e.records(); err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
	if err := e.createProfile(installation{}, nil); err == nil {
		t.Fatal("unmanaged profile accepted")
	}
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1")
	if err := e.createProfile(inst, map[category]bool{"unknown": true}); err == nil {
		t.Fatal("unknown exclusion accepted")
	}
}
