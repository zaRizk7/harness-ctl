package manager

import (
	"archive/tar"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
)

func malformedSnapshot(t *testing.T, e *engine, meta snapshotMeta, member string, kind byte) {
	t.Helper()
	identity, err := e.identity(true)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.cfg.Root, "snapshots")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, meta.ID+".age")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := age.Encrypt(f, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewWriter(encrypted)
	manifest, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err = archive.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err = archive.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err = archive.WriteHeader(&tar.Header{Name: member, Typeflag: kind, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	mac, err := snapshotMAC(path, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = atomicWrite(filepath.Join(dir, meta.ID+".mac"), []byte(hex.EncodeToString(mac)), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatedMalformedArchiveNeverChangesLiveState(t *testing.T) {
	for _, member := range []string{"0/../../escape", "99/.", "0/child"} {
		t.Run(member, func(t *testing.T) {
			e, _ := testEngine(t)
			p := resetPlan(t, e)
			before, err := os.ReadFile(p.Resources[0].Path)
			if err != nil {
				t.Fatal(err)
			}
			meta := snapshotMeta{ID: randomID(), Harness: "pi", Registry: e.reg, Items: []snapshotItem{{Path: p.Resources[0].Path, Root: p.StateRoot}}}
			malformedSnapshot(t, e, meta, member, tar.TypeDir)
			if err = e.restoreSnapshot(context.Background(), meta.ID); err == nil {
				t.Fatal("malformed archive was accepted")
			}
			after, err := os.ReadFile(p.Resources[0].Path)
			if err != nil || string(after) != string(before) {
				t.Fatal("malformed archive changed live state")
			}
		})
	}
}

func TestEditableSnapshotIndexCannotTriggerRetentionDeletion(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	meta.Expires = time.Now().Add(-time.Hour)
	if err = writeJSON(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".json"), meta); err != nil {
		t.Fatal(err)
	}
	if err = e.expireSnapshots(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".age")); err != nil {
		t.Fatal("untrusted expiration index deleted a valid snapshot")
	}
}

func TestRecoveryKeepsOtherHarnessRegistrations(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := specFor("claude")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err = writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err = e.restoreSnapshot(context.Background(), meta.ID); err != nil {
		t.Fatal(err)
	}
	if len(e.reg.Installs) != 1 || e.reg.Installs[0].ID != inst.ID {
		t.Fatal("PI recovery dropped another harness installation")
	}
}

func TestRecoveryOfSharedStateRequiresEveryOwner(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	p.Resources[0].Owners = []string{"pi", "claude"}
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.restoreApproved(context.Background(), meta.ID, nil); err == nil {
		t.Fatal("shared recovery proceeded without explicit owner selection")
	}
}

func TestPermanentDiscardErasesOlderAuthEvenWhenLiveFileIsAbsent(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	if _, err := e.snapshot(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.Resources[0].Path); err != nil {
		t.Fatal(err)
	}
	p.Resources = nil
	p.Request.Permanent = true
	p.Request.Preserve = keepAll()
	p.Request.Preserve[auth] = false
	if err := e.purgeAffected(p); err != nil {
		t.Fatal(err)
	}
	backups, err := e.snapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatal("permanent auth discard kept recoverable authentication after the live file disappeared")
	}
}

func TestMissingDisplayIndexDoesNotHideEncryptedRecovery(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".json")); err != nil {
		t.Fatal(err)
	}
	backups, err := e.snapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].ID != meta.ID {
		t.Fatal("an orphaned payload was hidden from recovery and purge")
	}
}
