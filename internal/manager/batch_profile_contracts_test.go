package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchRejectsConcurrentRegistryAndOverlappingState(t *testing.T) {
	e, _, inst := nativeComponentFixture(t, "pi")
	if _, err := e.buildBatch(context.Background(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	if err := e.executeBatchLocked(context.Background(), nil, "", nil); err == nil {
		t.Fatal("unapproved locked batch accepted")
	}
	old := fingerprint
	defer func() { fingerprint = old }()
	calls := 0
	fingerprint = func(path string) (string, error) {
		if path == e.statePath {
			calls++
			if calls > 2 {
				return "concurrent", nil
			}
		}
		return old(path)
	}
	if _, err := e.buildBatch(context.Background(), []request{{Harness: "pi", Action: "reset"}, {Harness: "gemini", Action: "install"}}); err == nil {
		t.Fatal("changed registry accepted")
	}
	fingerprint = old
	b, err := e.buildBatch(context.Background(), []request{{Harness: "pi", Action: "reset"}})
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := e.lock()
	if err != nil {
		t.Fatal(err)
	}
	err = e.executeBatch(context.Background(), b, b.ID, nil)
	unlock()
	if err == nil {
		t.Fatal("concurrent batch accepted")
	}
	for _, boundary := range []string{"read-failure", "changed"} {
		calls = 0
		fingerprint = func(path string) (string, error) {
			if path == e.statePath {
				calls++
				if calls > 1 {
					if boundary == "changed" {
						return "changed", nil
					}
					return "", errors.New("batch registry")
				}
			}
			return old(path)
		}
		if err = e.executeBatch(context.Background(), b, b.ID, nil); err == nil {
			t.Fatal(boundary)
		}
	}
	fingerprint = old
	_ = inst
	// Two tracked clients can have the same configured state root. Select every
	// owner, then reject overlapping destructive transactions before either runs.
	e.reg = registry{Profiles: map[string]profile{}}
	_ = os.Remove(e.statePath)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	root := filepath.Join(e.cfg.Home, "shared")
	_ = atomicWrite(filepath.Join(root, "auth.json"), []byte("fixture"), 0600)
	e.cfg.StateRoots = map[string]string{"pi": root, "gemini": root}
	for _, id := range []string{"pi", "gemini"} {
		s, _ := e.specFor(id)
		_ = atomicWrite(filepath.Join(bin, s.Command), []byte("fixture"), 0700)
	}
	_, err = e.buildBatch(context.Background(), []request{{Harness: "pi", Action: "reset", Owners: []string{"gemini"}, Preserve: map[category]bool{}}, {Harness: "gemini", Action: "reset", Owners: []string{"pi"}, Preserve: map[category]bool{}}})
	if err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatal(err)
	}
}

func TestProfilesAndMigrationPreserveLinkedAndUnmappedSources(t *testing.T) {
	for _, scenario := range []string{"linked", "unmapped", "parked-error", "copy-error", "mkdir-error", "stage-error", "final-mkdir", "replace-error"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, path := componentFixture(t)
			if scenario == "linked" {
				_ = os.Symlink(path, filepath.Join(inst.StateRoot, "linked"))
			}
			if scenario == "parked-error" {
				_ = writeJSON(filepath.Join(inst.StateRoot, disabledComponentsDir, string(skills), "a", "meta.json"), parkedComponent{Path: filepath.Join(e.cfg.Home, "foreign"), Category: skills})
			}
			old := fileIO
			defer func() { fileIO = old }()
			fault := errors.New("profile boundary")
			switch scenario {
			case "unmapped":
				fileIO.rel = func(a, b string) (string, error) {
					if a == inst.StateRoot {
						return "", fault
					}
					return old.rel(a, b)
				}
			case "copy-error":
				fileIO.open = func(string) (*os.File, error) { return nil, fault }
			case "mkdir-error":
				fileIO.mkdir = func(string, os.FileMode) error { return fault }
			case "stage-error":
				fileIO.mkdirTemp = func(string, string) (string, error) { return "", fault }
			case "final-mkdir":
				fileIO.mkdir = func(path string, mode os.FileMode) error {
					if strings.Contains(path, ".profile-") {
						return fault
					}
					return old.mkdir(path, mode)
				}
			case "replace-error":
				fileIO.rename = func(string, string) error { return fault }
			}
			err := e.createProfile(inst, nil)
			if scenario == "linked" || scenario == "unmapped" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("profile failure ignored")
			}
		})
	}
	e, p, _ := archiveFaultFixture(t)
	p.StateRoot = e.stateRoot(p.Spec)
	_ = atomicWrite(filepath.Join(e.managedStateRoot(p.Spec), "existing"), nil, 0600)
	if err := e.migrateState(p); err == nil {
		t.Fatal("migration overwrote retained state")
	}
	_ = os.RemoveAll(e.managedStateRoot(p.Spec))
	p.Resources = []resource{{Linked: true}, {Path: filepath.Join(e.cfg.Home, "foreign"), Root: e.cfg.Home}}
	if err := e.migrateState(p); err != nil {
		t.Fatal("protected migration source was copied", err)
	}
	_ = os.RemoveAll(e.managedStateRoot(p.Spec))
	park := filepath.Join(p.StateRoot, disabledComponentsDir, string(skills))
	_ = writeJSON(filepath.Join(park, "a/meta.json"), parkedComponent{Path: e.cfg.Home, Category: skills})
	p.Resources = []resource{{Path: park, Root: p.StateRoot, Category: skills}}
	if err := e.migrateState(p); err == nil {
		t.Fatal("unsafe copied parking metadata accepted")
	}
}

func TestTrackedPlanBlockersAndRegistryBootstrapFailures(t *testing.T) {
	e, _ := testEngine(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	s, _ := e.specFor("pi")
	_ = atomicWrite(filepath.Join(bin, s.Command), []byte("fixture"), 0700)
	p, err := e.buildPlan(context.Background(), request{Harness: "pi", Action: "update"})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal("unknown ownership permitted", err)
	}
	e.client = contractHTTP{}
	_ = atomicWrite(filepath.Join(bin, "npm"), []byte("fixture"), 0700)
	e.run = commandRunner(func(context.Context, command) (string, error) { return filepath.Join(e.cfg.Home, "native"), nil })
	p, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "install", Model: "tracked", Target: "1.2.3"})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal(err, p)
	}
	for _, boundary := range []string{"lstat", "catalog-parent", "catalog-read", "registry-path"} {
		c := e.cfg
		oldIO, oldAncestors, oldValidate := fileIO, rejectLinkedAncestors, validateOwnedPath
		fault := errors.New("bootstrap boundary")
		switch boundary {
		case "lstat":
			fileIO.lstat = func(string) (os.FileInfo, error) { return nil, fault }
		case "catalog-parent":
			c.CatalogFile = filepath.Join(e.cfg.Root, "catalog.json")
			rejectLinkedAncestors = func(string) error { return fault }
		case "catalog-read":
			c.CatalogFile = filepath.Join(e.cfg.Root, "missing.json")
		case "registry-path":
			validateOwnedPath = func(root, path string) error {
				if filepath.Base(path) == "registry.json" {
					return fault
				}
				return oldValidate(root, path)
			}
		}
		_, err = newEngine(c, &fakeRunner{})
		fileIO, rejectLinkedAncestors, validateOwnedPath = oldIO, oldAncestors, oldValidate
		if err == nil {
			t.Fatal(boundary)
		}
	}
	old := rejectLinkedAncestors
	defer func() { rejectLinkedAncestors = old }()
	rejectLinkedAncestors = func(string) error { return errors.New("configuration ancestor") }
	if err = e.cfg.validate(); err == nil {
		t.Fatal("unsafe configuration ancestor accepted")
	}
}
