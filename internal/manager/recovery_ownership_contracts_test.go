package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"filippo.io/age"
)

func TestSnapshotRestoreOwnershipIncludesNativeAndSharedScopes(t *testing.T) {
	e, _ := testEngine(t)
	for _, id := range []string{"claude", "codex", "prime-agent", "hermes", "pi"} {
		s, _ := e.specFor(id)
		paths := []string{}
		inst := installation{Method: "native-" + id, Path: filepath.Join(e.cfg.Home, ".local/bin", s.Command)}
		paths = append(paths, inst.Path)
		if id == "claude" {
			paths = append(paths, filepath.Join(e.cfg.Home, ".claude.json"), filepath.Join(e.cfg.Home, ".local/share/claude"))
		}
		if id == "codex" {
			paths = append(paths, filepath.Join(e.codexSkillsRoot(), "skills", "demo"), filepath.Join(e.stateRoot(s), "packages/standalone"))
		}
		if id == "prime-agent" {
			paths = append(paths, filepath.Join(e.cfg.Home, ".local/share/prime-agent"))
		}
		if id == "hermes" {
			paths = append(paths, filepath.Join(e.stateRoot(s), "hermes-agent"))
		}
		for _, label := range s.LaunchLabels {
			paths = append(paths, filepath.Join(e.cfg.Home, "Library/LaunchAgents", label+".plist"))
		}
		for _, path := range paths {
			inst.Root = path
			item := snapshotItem{Path: path, Root: filepath.Dir(path), Launcher: path == inst.Path}
			if err := e.validRestoreItem(s, item, inst); err != nil {
				t.Fatal(id, path, err)
			}
		}
		if id == "pi" {
			inst.Method = "npm"
			inst.Package = s.Package
			inst.Root = filepath.Join(e.cfg.Home, "native/lib/node_modules", s.Package)
			if err := e.validRestoreItem(s, snapshotItem{Path: inst.Root, Root: filepath.Dir(inst.Root)}, inst); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.validRestoreItem(s, snapshotItem{Path: e.stateRoot(s) + "/bad", Root: e.stateRoot(s), Launcher: true}, inst); err == nil {
			t.Fatal("invalid launcher accepted")
		}
	}
}

func TestSnapshotsIncludeNativeLauncherServicesAndSharedAbsentState(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	s.LaunchLabels = []string{"fixture.claude"}
	root := filepath.Join(e.cfg.Home, ".local/share/claude")
	payload := filepath.Join(root, "bin/claude")
	_ = atomicWrite(payload, []byte("fixture"), 0700)
	path := filepath.Join(e.cfg.Home, ".local/bin/claude")
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.Symlink(payload, path)
	service := filepath.Join(e.cfg.Home, "Library/LaunchAgents", s.LaunchLabels[0]+".plist")
	_ = atomicWrite(service, []byte("fixture"), 0600)
	p := &plan{Spec: s, Install: installation{ID: installID(s.ID, path), Harness: s.ID, Path: path, Root: root, Method: "native-claude", ServicePaths: []string{service}}, StateRoot: e.stateRoot(s), Request: request{Model: "tracked", Action: "reset", Harness: s.ID}}
	p.Component = &componentMutation{Native: true, Request: componentRequest{Scope: "base"}}
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	launcher, shared, plist := false, false, false
	for _, item := range meta.Items {
		launcher = launcher || item.Launcher
		plist = plist || item.Path == service
		shared = shared || item.Absent && len(item.Owners) > 1
	}
	if !launcher || !shared || !plist {
		t.Fatal(meta.Items)
	}
	p.Component.Request.Scope = "profile"
	if _, err = e.snapshot(context.Background(), p); err == nil {
		t.Fatal("failed root check ignored")
	}
}

func TestRecoveryIndexesAndAuthenticatedManifestFailures(t *testing.T) {
	e, p, _ := archiveFaultFixture(t)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(e.cfg.Root, "snapshots", meta.ID+".json")
	_ = atomicWrite(index, []byte("malformed"), 0600)
	items, err := e.snapshots()
	if err != nil || len(items) != 1 || items[0].Harness != "unindexed" {
		t.Fatal(items, err)
	}
	_ = atomicWrite(filepath.Join(e.cfg.Root, "snapshots", "bad.age"), nil, 0600)
	if _, err = e.snapshots(); err == nil {
		t.Fatal("invalid archive identity accepted")
	}
	_ = os.Remove(filepath.Join(e.cfg.Root, "snapshots", "bad.age"))
	if err = e.purgeSnapshot("../bad"); err == nil {
		t.Fatal("unsafe purge accepted")
	}
	for _, boundary := range []string{"validate", "remove"} {
		oldIO, oldValidate := fileIO, validateOwnedPath
		fault := errors.New("purge boundary")
		if boundary == "remove" {
			fileIO.remove = func(string) error { return fault }
		} else {
			validateOwnedPath = func(string, string) error { return fault }
		}
		err = e.purgeSnapshot(meta.ID)
		fileIO, validateOwnedPath = oldIO, oldValidate
		if !errors.Is(err, fault) {
			t.Fatal(err)
		}
	}
	oldKeys := e.keys
	e.keys = &memoryKeys{}
	_, err = e.authenticatedSnapshot(meta.ID)
	e.keys = oldKeys
	if err == nil {
		t.Fatal("missing identity accepted")
	}
	// The authenticated container can still contain an invalid or mismatched manifest.
	for _, scenario := range []string{"wrong-id", "truncated", "invalid-registry", "missing-member"} {
		id := randomID()
		bad := meta
		bad.ID = id
		if scenario == "wrong-id" {
			bad.ID = randomID()
		}
		if scenario == "invalid-registry" {
			bad.Registry.Installs = []installation{{ID: "invalid", Managed: true}}
		}
		payload := archivePayload(t, bad)
		if scenario == "truncated" {
			payload = payload[:30]
		}
		signedArchive(t, e, id, payload)
		if scenario == "missing-member" || scenario == "invalid-registry" {
			if err = e.restoreSnapshot(context.Background(), id); err == nil {
				t.Fatal("missing archive member accepted")
			}
		} else {
			if _, err = e.authenticatedSnapshot(id); err == nil {
				t.Fatal(scenario)
			}
		}
	}
}

func TestSnapshotSpecialFilesAndAuthenticatedStreamTruncation(t *testing.T) {
	e, p, _ := archiveFaultFixture(t)
	path := filepath.Join(p.StateRoot, "fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	p.Resources = append(p.Resources, resource{Path: path, Root: p.StateRoot, Category: other})
	if _, err := e.snapshot(context.Background(), p); err == nil {
		t.Fatal("special state file archived")
	}
	_ = os.Remove(path)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	// Fail after manifest parsing, while authenticating the rest of the stream.
	old := archiveIO.copy
	defer func() { archiveIO.copy = old }()
	copyCalls := 0
	archiveIO.copy = func(w io.Writer, r io.Reader) (int64, error) {
		copyCalls++
		if copyCalls == 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return old(w, r)
	}
	if _, err = e.authenticatedSnapshot(meta.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
}

func TestRetentionAndAffectedPurgeReturnDeletionFailures(t *testing.T) {
	for _, scenario := range []string{"expired", "affected", "malformed", "authentication"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, _ := archiveFaultFixture(t)
			meta, err := e.snapshot(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "expired" {
				meta.Expires = time.Now().Add(-time.Hour)
				signedArchive(t, e, meta.ID, archivePayload(t, meta))
			}
			if scenario == "malformed" {
				_ = atomicWrite(filepath.Join(e.cfg.Root, "snapshots", "invalid.age"), nil, 0600)
			}
			if scenario == "authentication" {
				_ = atomicWrite(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".mac"), nil, 0600)
			}
			old := fileIO
			defer func() { fileIO = old }()
			fileIO.remove = func(string) error { return errors.New("retention delete") }
			if scenario == "expired" {
				err = e.expireSnapshots()
			} else {
				p.Request.Preserve = map[category]bool{}
				err = e.purgeAffected(p)
			}
			if err == nil {
				t.Fatal("purge failure ignored")
			}
		})
	}
}

func TestRollbackErrorsKeepRecoveryRequiredJournal(t *testing.T) {
	for _, scenario := range []string{"decrypt", "stage-cleanup", "purge"} {
		t.Run(scenario, func(t *testing.T) {
			e, operation, _ := nativeFaultFixture(t, "install")
			r := e.run.(*fakeRunner)
			oldIO, oldArchive := fileIO, archiveIO
			if scenario == "purge" {
				var p *plan
				e, p, _ = mutationFaultFixture(t, "reset")
				p.Request.Permanent = true
				operation = func() error { return e.execute(context.Background(), p, p.ID, nil) }
			}
			defer func() { fileIO, archiveIO = oldIO, oldArchive }()
			r.onRun = func(c command) error {
				if c.Description != "synthetic install" {
					return nil
				}
				if scenario == "decrypt" {
					archiveIO.decrypt = func(io.Reader, ...age.Identity) (io.Reader, error) { return nil, errors.New("rollback decrypt") }
				}
				if scenario == "stage-cleanup" {
					fileIO.removeAll = func(path string) error {
						if strings.HasSuffix(path, "/installs/pi/2") {
							return errors.New("stage cleanup")
						}
						return oldIO.removeAll(path)
					}
				}
				return errors.New("fixture installer")
			}
			if scenario == "purge" {
				fileIO.remove = func(path string) error {
					if strings.HasSuffix(path, ".age") {
						return errors.New("purge failure")
					}
					return oldIO.remove(path)
				}
			}
			if err := operation(); err == nil {
				t.Fatal("failed operation succeeded")
			}
			fileIO, archiveIO = oldIO, oldArchive
			records, err := e.records()
			if err != nil || len(records) != 1 {
				t.Fatal(records, err)
			}
			if scenario != "purge" && records[0].Status != "recovery-required" {
				t.Fatal(records[0])
			}
		})
	}
}
