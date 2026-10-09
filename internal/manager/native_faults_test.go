package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nativeFaultFixture isolates one complete manager workflow before an IO failure.
func nativeFaultFixture(t *testing.T, scenario string) (*engine, func() error, string) {
	t.Helper()
	e, p, marker := archiveFaultFixture(t)
	ctx := context.Background()
	var operation func() error
	switch scenario {
	case "restore", "recover":
		meta, err := e.snapshot(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		_ = atomicWrite(filepath.Join(p.StateRoot, "nested", "auth.json"), []byte("changed"), 0600)
		if scenario == "restore" {
			operation = func() error { return e.restoreApproved(ctx, meta.ID, nil) }
		} else {
			r := operationRecord{ID: randomID(), Harness: p.Spec.ID, Action: "reset", Snapshot: meta.ID, Status: "executing"}
			if err = e.saveRecord(r); err != nil {
				t.Fatal(err)
			}
			operation = func() error { return e.recoverOperation(ctx, r.ID) }
		}
	case "profile-disable":
		if err := e.createProfile(p.Install, map[category]bool{}); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(e.statePath, e.reg); err != nil {
			t.Fatal(err)
		}
		operation = func() error { return e.disableProfile(p.Install) }
	case "setup":
		operation = func() error {
			return e.setupCLI(ctx, []string{"--headless", "--yes"}, strings.NewReader(""), io.Discard)
		}
	case "self-plan", "self-execute":
		binary := filepath.Join(e.cfg.Root, "app", "harness-ctl")
		_ = atomicWrite(binary, []byte("fixture binary"), 0700)
		link := filepath.Join(e.cfg.Home, "bin", "harness-ctl")
		_ = os.MkdirAll(filepath.Dir(link), 0700)
		_ = os.Symlink(binary, link)
		_ = writeJSON(filepath.Join(e.cfg.Root, "installation.json"), map[string]string{"Binary": binary, "Link": link})
		if scenario == "self-plan" {
			operation = func() error {
				_, err := e.buildSelfPlan(ctx, binary, true, true, request{Preserve: keepAll()})
				return err
			}
		} else {
			self, err := e.buildSelfPlan(ctx, binary, true, true, request{Preserve: keepAll()})
			if err != nil {
				t.Fatal(err)
			}
			operation = func() error { return e.executeSelf(ctx, self, self.ID, nil) }
		}
	case "install", "update-profile", "uninstall", "migrate":
		inst := p.Install
		if scenario == "update-profile" {
			if err := e.createProfile(inst, map[category]bool{}); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(e.statePath, e.reg); err != nil {
				t.Fatal(err)
			}
		}
		action := scenario
		if action == "update-profile" {
			action = "update"
		}
		digest, err := fingerprint(e.statePath)
		if err != nil {
			t.Fatal(err)
		}
		p = &plan{ID: randomID(), Spec: p.Spec, Install: inst, Request: request{Harness: inst.Harness, InstallID: inst.ID, Action: action, Model: "isolated", Target: "2", Preserve: keepAll()}, StateRoot: inst.StateRoot, RegistryDigest: digest}
		p.InstallDigest, err = fingerprint(inst.Root)
		if err != nil {
			t.Fatal(err)
		}
		if action == "migrate" {
			if err = os.RemoveAll(inst.StateRoot); err != nil {
				t.Fatal(err)
			}
			p.StateRoot = e.stateRoot(p.Spec)
			_ = atomicWrite(filepath.Join(p.StateRoot, "auth.json"), []byte("migrated"), 0600)
			p.Resources, err = e.resources(p.Spec)
			if err != nil {
				t.Fatal(err)
			}
		}
		if action != "uninstall" {
			p.Destination = filepath.Join(e.cfg.Root, "installs", p.Spec.ID, "2")
			p.Steps = []command{{Path: "synthetic", Description: "synthetic install"}}
			r := e.run.(*fakeRunner)
			r.onRun = func(c command) error {
				if c.Description == "synthetic install" {
					return publishFaultInstall(p)
				}
				return nil
			}
		}
		operation = func() error { return e.execute(ctx, p, p.ID, nil) }
	}
	return e, operation, marker
}

// publishFaultInstall simulates an installer using its own IO, independently of
// the manager boundaries being faulted. Native publication failures return errors.
func publishFaultInstall(p *plan) error {
	path := filepath.Join(p.Destination, "bin", p.Spec.Command)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		return err
	}
	manifest := filepath.Join(p.Destination, "lib", "node_modules", p.Spec.Package, "package.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"name": p.Spec.Package, "version": p.Request.Target})
	if err != nil {
		return err
	}
	return os.WriteFile(manifest, data, 0600)
}

func TestNativeFailuresPreserveUnrelatedStateAndRecoverableMetadata(t *testing.T) {
	for _, scenario := range []string{"restore", "recover", "profile-disable", "setup", "self-plan", "self-execute", "install", "update-profile", "uninstall", "migrate"} {
		for _, boundary := range []string{"validate", "fingerprint", "read", "atomic", "json", "lstat", "stat", "readDir", "readFile", "open", "openFile", "mkdir", "mkdirTemp", "rename", "remove", "removeAll", "readlink", "eval"} {
			t.Run(scenario+"/"+boundary, func(t *testing.T) {
				inject := func(n int, c *int) func() {
					if strings.Contains(" validate fingerprint read atomic json ", " "+boundary+" ") {
						return installMutationFault(boundary, n, c)
					}
					return injectArchiveFailure(boundary, n, c)
				}
				_, operation, _ := nativeFaultFixture(t, scenario)
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
						e, operation, marker := nativeFaultFixture(t, scenario)
						count := 0
						restore := inject(nth, &count)
						_ = operation()
						restore()
						if count < nth {
							t.Fatal("failure boundary not reached")
						}
						data, err := os.ReadFile(marker)
						if err != nil || string(data) != "keep" {
							t.Fatal("unrelated state changed", err)
						}
						if err = e.refreshRegistry(); err != nil {
							t.Fatal("malformed registry retained", err)
						}
					})
				}
			})
		}
	}
}
