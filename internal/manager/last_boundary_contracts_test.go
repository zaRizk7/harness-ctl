package manager

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestBoundaryFailuresStopRecoveryBatchAndNativeUninstall(t *testing.T) {
	fault := errors.New("final boundary")
	oldIO, oldFingerprint := fileIO, fingerprint
	defer func() { fileIO, fingerprint = oldIO, oldFingerprint }()
	e, p, _ := archiveFaultFixture(t)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = e.restoreApproved(ctx, meta.ID, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	fileIO.walk = func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, fault) }
	if err = copyTree(p.StateRoot, t.TempDir()); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fileIO = oldIO
	// Authenticated encryption does not imply a valid internal tar structure.
	id := randomID()
	signedArchive(t, e, id, []byte("invalid tar"))
	if _, err = e.authenticatedSnapshot(id); err == nil {
		t.Fatal("invalid authenticated manifest accepted")
	}
	id = randomID()
	meta.ID = id
	payload := archivePayload(t, meta)
	payload = append(payload[:len(payload)-1024], []byte("truncated member header")...)
	signedArchive(t, e, id, payload)
	if err = e.restoreSnapshot(context.Background(), id); err == nil {
		t.Fatal("truncated authenticated archive restored")
	}
	fileIO = oldIO
	e, inst, _ := componentFixture(t)
	b, err := e.buildBatch(context.Background(), []request{{Harness: inst.Harness, Action: "reset"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeBatch(ctx, b, b.ID, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled batch executed", err)
	}
	count := 0
	fingerprint = func(path string) (string, error) {
		if path == e.statePath {
			count++
			if count > 1 {
				return "", fault
			}
		}
		return oldFingerprint(path)
	}
	b.Plans[0].RegistryDigest, _ = oldFingerprint(e.statePath)
	b.Digest = valueDigest(b.Plans)
	if err = e.executeBatch(context.Background(), b, b.ID, nil); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fingerprint = oldFingerprint
	e, _ = testEngine(t)
	s, _ := e.specFor("pi")
	root := filepath.Join(e.cfg.Home, "native/lib/node_modules", s.Package)
	path := filepath.Join(root, "bin/pi")
	_ = atomicWrite(path, []byte("fixture"), 0700)
	_ = writeJSON(filepath.Join(root, "package.json"), map[string]string{"name": s.Package, "version": "1"})
	t.Setenv("PATH", filepath.Dir(path))
	if _, err = e.buildPlan(context.Background(), request{Harness: "pi", Action: "uninstall"}); err == nil {
		t.Fatal("uninstall planned without native package manager")
	}
}

func TestArrayRemovalRejectsFailedSiblingRebase(t *testing.T) {
	e, inst, path := componentFixture(t)
	_ = writeJSON(path, map[string]any{"hooks": []any{map[string]any{"name": "one"}, map[string]any{"name": "two"}, map[string]any{"name": "three"}}})
	applyComponentRequest(t, e, inst, componentRequest{Category: hooks, Operation: "disable", Path: path, Field: "/hooks/2"})
	items, err := e.components(inst, "base", hooks)
	if err != nil {
		t.Fatal(err)
	}
	var park string
	for _, item := range items {
		if item.Parked != "" {
			park = item.Parked
		}
	}
	old := marshalJSON
	defer func() { marshalJSON = old }()
	fault := errors.New("sibling metadata serialization")
	marshalJSON = func(any) ([]byte, error) { return nil, fault }
	p := &plan{ID: randomID(), Install: inst, StateRoot: inst.StateRoot, Spec: e.cfg.Harnesses[0], Request: request{Harness: inst.Harness, Component: &componentRequest{Category: hooks, Operation: "remove", Path: path, Field: "/hooks/0"}}, RootDigests: map[string]string{}}
	p.Spec, _ = e.specFor(inst.Harness)
	if err = e.planComponent(p); !errors.Is(err, fault) {
		t.Fatal(park, err)
	}
}
