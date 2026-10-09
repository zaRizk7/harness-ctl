package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedReplacementRetainsRollbackWhenBothRenamesFail(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	if err := atomicWrite(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	old := fileIO
	defer func() { fileIO = old }()
	fault := errors.New("rename denied")
	fileIO.rename = func(a, b string) error {
		if b == target {
			return fault
		}
		return old.rename(a, b)
	}
	err := replaceTree(source, target)
	if !errors.Is(err, fault) {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(root, ".harness-ctl-replace-*", "old"))
	if err != nil || len(backups) != 1 {
		t.Fatal("rollback payload lost after failed restoration", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != "old" {
		t.Fatal("original payload changed", string(data), err)
	}
}

func TestStagingFailuresAreJournaledAndNeverRunInstallers(t *testing.T) {
	for _, scenario := range []string{"dependency", "package", "script-path", "script-write", "permanent-purge", "cleanup"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, _ := archiveFaultFixture(t)
			p.Request.Action = "install"
			p.Destination = filepath.Join(e.cfg.Root, "installs", "pi", "stage")
			p.Steps = []command{{Path: "synthetic", Description: "must not run"}}
			fault := errors.New("staging failure")
			oldIO, oldAtomic, oldValidate := fileIO, atomicWrite, validateOwnedPath
			defer func() { fileIO, atomicWrite, validateOwnedPath = oldIO, oldAtomic, oldValidate }()
			if scenario == "dependency" || scenario == "cleanup" {
				cleanupCalls := 0
				p.DependencyCommands = []command{{Path: "synthetic", Description: "dependency"}}
				e.run = commandRunner(func(_ context.Context, c command) (string, error) {
					if c.Description != "dependency" && filepath.Base(c.Path) != "ps" {
						t.Fatal("installer started", c)
					}
					if filepath.Base(c.Path) == "ps" {
						return "", nil
					}
					if scenario == "cleanup" {
						fileIO.removeAll = func(path string) error {
							if path == p.Destination {
								cleanupCalls++
							}
							if path == p.Destination && cleanupCalls > 1 {
								return fault
							}
							return oldIO.removeAll(path)
						}
					}
					return "", fault
				})
			} else {
				e.run = commandRunner(func(_ context.Context, c command) (string, error) {
					if filepath.Base(c.Path) != "ps" {
						t.Fatal("installer started", c)
					}
					return "", nil
				})
			}
			switch scenario {
			case "package":
				p.PackageURL = "https://fixture.test/payload"
				p.Artifact = filepath.Join(e.cfg.Root, "downloads", "payload")
				e.client = accountHTTP(func(*http.Request) (*http.Response, error) { return nil, fault })
			case "script-path", "script-write":
				p.NativeScript = []byte("#!/bin/sh\n")
				p.Artifact = filepath.Join(e.cfg.Root, "downloads", "installer")
				sum := sha256.Sum256(p.NativeScript)
				p.Integrity = "sha256:" + hex.EncodeToString(sum[:])
				if scenario == "script-path" {
					validateOwnedPath = func(root, path string) error {
						if path == p.Artifact {
							return fault
						}
						return oldValidate(root, path)
					}
				} else {
					atomicWrite = func(path string, data []byte, mode fs.FileMode) error {
						if path == p.Artifact {
							return fault
						}
						return oldAtomic(path, data, mode)
					}
				}
			case "permanent-purge":
				p.Request.Permanent = true
				p.Request.Preserve = map[category]bool{}
				if _, err := e.snapshot(context.Background(), p); err != nil {
					t.Fatal(err)
				}
				fileIO.remove = func(path string) error {
					if strings.HasSuffix(path, ".age") {
						return fault
					}
					return oldIO.remove(path)
				}
			}
			if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
				t.Fatal("staging failure ignored")
			}
			fileIO, atomicWrite, validateOwnedPath = oldIO, oldAtomic, oldValidate
			records, err := e.records()
			if err != nil || len(records) != 1 || records[0].Status == "complete" {
				t.Fatal(records, err)
			}
			if scenario == "cleanup" && records[0].Status != "recovery-required" {
				t.Fatal(records[0])
			}
		})
	}
}

func TestServiceFailuresStopExecutionAndRemainRecoverable(t *testing.T) {
	for _, scenario := range []string{"stop", "restart", "restore-stop", "restore-process", "restore-journal"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, _ := serviceFixture(t)
			p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, Action: "reset", Preserve: keepAll()})
			if err != nil {
				t.Fatal(err)
			}
			fault := errors.New("service failure")
			prints := 0
			e.run = commandRunner(func(_ context.Context, c command) (string, error) {
				if filepath.Base(c.Path) == "ps" {
					if scenario == "restore-process" {
						return "", fault
					}
					return "", nil
				}
				if c.Args[0] == "print" {
					prints++
					if scenario == "restart" && prints > 1 {
						return "Could not find service", fault
					}
					return "loaded", nil
				}
				if c.Args[0] == "bootout" && (scenario == "stop" || scenario == "restore-stop") {
					return "", fault
				}
				if c.Args[0] == "bootstrap" {
					return "", fault
				}
				return "", nil
			})
			if strings.HasPrefix(scenario, "restore-") {
				meta, err := e.snapshot(context.Background(), p)
				if err != nil {
					t.Fatal(err)
				}
				old := writeJSON
				defer func() { writeJSON = old }()
				if scenario == "restore-journal" {
					writeJSON = func(string, any) error { return fault }
				}
				err = e.restoreApproved(context.Background(), meta.ID, nil)
				if !errors.Is(err, fault) {
					t.Fatal(err)
				}
			} else {
				if err = e.execute(context.Background(), p, p.ID, nil); !errors.Is(err, fault) {
					t.Fatal(err)
				}
				records, err := e.records()
				if err != nil || len(records) != 1 || records[0].Status == "complete" {
					t.Fatal(records, err)
				}
			}
		})
	}
}

func TestRecoveryCancellationAndChangedStagedMembersStopPublication(t *testing.T) {
	for _, scenario := range []string{"cancel", "disappeared", "copy-cancel", "manifest-id"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, _ := archiveFaultFixture(t)
			meta, err := e.snapshot(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "manifest-id" {
				_ = writeJSON(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".json"), snapshotMeta{ID: randomID()})
				items, err := e.snapshots()
				if err != nil || items[0].Harness != "unindexed" {
					t.Fatal(items, err)
				}
				return
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			old := fileIO
			defer func() { fileIO = old }()
			if scenario == "copy-cancel" {
				cancel()
				if err := copyTreeContext(ctx, p.StateRoot, t.TempDir()); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
			if scenario == "cancel" {
				fileIO.mkdirTemp = func(a, b string) (string, error) { result, err := old.mkdirTemp(a, b); cancel(); return result, err }
			} else {
				seen := map[string]int{}
				fileIO.lstat = func(path string) (os.FileInfo, error) {
					if strings.Contains(path, "/restore-") && filepath.Base(path) == "0" {
						seen[path]++
						if seen[path] == 2 {
							return nil, os.ErrNotExist
						}
					}
					return old.lstat(path)
				}
			}
			if err = e.restoreSnapshot(ctx, meta.ID); err == nil {
				t.Fatal("changed restore accepted")
			}
		})
	}
}

func TestPrivateRootProfileAndSelfKeyFailures(t *testing.T) {
	oldAncestors, oldIO := rejectLinkedAncestors, fileIO
	defer func() { rejectLinkedAncestors, fileIO = oldAncestors, oldIO }()
	fault := errors.New("private storage failure")
	e, _ := testEngine(t)
	rejectLinkedAncestors = func(string) error { return fault }
	if _, err := e.lock(); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	rejectLinkedAncestors = oldAncestors
	e, inst, _ := componentFixture(t)
	stage, stageCalls := "", 0
	fileIO.mkdirTemp = func(parent, pattern string) (string, error) {
		var err error
		stage, err = oldIO.mkdirTemp(parent, pattern)
		return stage, err
	}
	fileIO.mkdir = func(path string, mode fs.FileMode) error {
		if path == stage {
			stageCalls++
		}
		if path == stage && stageCalls > 1 {
			return fault
		}
		return oldIO.mkdir(path, mode)
	}
	if err := e.createProfile(inst, nil); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fileIO = oldIO
	binary := filepath.Join(e.cfg.Home, "bin/harness-ctl")
	_ = atomicWrite(binary, []byte("synthetic"), 0700)
	p, err := e.buildSelfPlan(context.Background(), binary, false, true, request{Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	e.keys = undeletableKeys{}
	if err = e.executeSelf(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("key deletion failure ignored")
	}
	if _, err = os.Stat(binary); err != nil {
		t.Fatal("binary removed after key deletion failed", err)
	}
}

func TestBatchRejectsRegistryChangesBetweenPreviewItems(t *testing.T) {
	e, inst, _ := componentFixture(t)
	s, _ := e.specFor("gemini")
	second := syntheticInstall(t, e, s, "1")
	e.reg.Installs = append(e.reg.Installs, second)
	_ = writeJSON(e.statePath, e.reg)
	old := fingerprint
	defer func() { fingerprint = old }()
	calls := 0
	fingerprint = func(path string) (string, error) {
		if path == e.statePath {
			calls++
			if calls > 2 {
				return "next-registry", nil
			}
		}
		return old(path)
	}
	if _, err := e.buildBatch(context.Background(), []request{{Harness: inst.Harness, Action: "reset"}, {Harness: second.Harness, Action: "reset"}}); err == nil || !strings.Contains(err.Error(), "registry changed while building batch") {
		t.Fatal(err)
	}
	fingerprint = old
	b, err := e.buildBatch(context.Background(), []request{{Harness: inst.Harness, Action: "reset"}})
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	fingerprint = func(path string) (string, error) {
		if path == e.statePath {
			calls++
			if calls == 2 {
				return "", io.ErrClosedPipe
			}
		}
		return old(path)
	}
	if err = e.executeBatch(context.Background(), b, b.ID, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
