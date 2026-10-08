package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

type memoryKeys struct{ value string }

func (k *memoryKeys) Get(string) (string, error) {
	if k.value == "" {
		return "", keyring.ErrNotFound
	}
	return k.value, nil
}
func (k *memoryKeys) Set(_ string, value string) error { k.value = value; return nil }

type fakeRunner struct {
	calls []command
	fail  bool
	onRun func(command) error
}

func (r *fakeRunner) Run(_ context.Context, c command) (string, error) {
	r.calls = append(r.calls, c)
	if r.onRun != nil {
		return "", r.onRun(c)
	}
	if r.fail && c.Description == "synthetic install" {
		return "", errors.New("synthetic command failure")
	}
	return "", nil
}

func testEngine(t *testing.T) (*engine, *fakeRunner) {
	t.Helper()
	for _, s := range catalog {
		if s.HomeEnv != "" {
			t.Setenv(s.HomeEnv, "")
		}
	}
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, "")
	}
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	cfg := defaultConfig(home)
	cfg.Root = filepath.Join(home, "manager")
	cfg.BinDir = filepath.Join(cfg.Root, "bin")
	r := &fakeRunner{}
	e, err := newEngine(cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	e.keys = &memoryKeys{}
	return e, r
}
func resetPlan(t *testing.T, e *engine) *plan {
	t.Helper()
	s, _ := specFor("pi")
	root := e.stateRoot(s)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "auth.json")
	if err := os.WriteFile(path, []byte(`{"token":"synthetic"}`), 0600); err != nil {
		t.Fatal(err)
	}
	rs, err := e.resources(s)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := fingerprint(e.statePath)
	if err != nil {
		t.Fatal(err)
	}
	p := &plan{ID: randomID(), Created: time.Now(), Spec: s, Request: request{Harness: "pi", Action: "reset", Preserve: map[category]bool{}}, Resources: rs, StateRoot: root, RegistryDigest: digest}
	return p
}

func TestExecutionRequiresApprovalAndFreshPreview(t *testing.T) {
	e, r := testEngine(t)
	p := resetPlan(t, e)
	if err := e.execute(context.Background(), p, "", nil); err == nil {
		t.Fatal("unapproved plan was executed")
	}
	if err := os.WriteFile(p.Resources[0].Path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("stale preview was executed")
	}
	if len(r.calls) != 0 {
		t.Fatal("stale operation invoked a command")
	}
}

func TestResetEncryptedRecoveryRoundTrip(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	before, err := os.ReadFile(p.Resources[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(p.Resources[0].Path); !os.IsNotExist(err) {
		t.Fatal("reset did not discard selected state")
	}
	backups, err := e.snapshots()
	if err != nil || len(backups) != 1 {
		t.Fatalf("recovery missing: %v %v", backups, err)
	}
	data, err := os.ReadFile(filepath.Join(e.cfg.Root, "snapshots", backups[0].ID+".age"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == string(before) {
		t.Fatal("snapshot is plaintext")
	}
	if err = e.restoreSnapshot(context.Background(), backups[0].ID); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p.Resources[0].Path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("state not restored: %q %v", after, err)
	}
}

func TestCommandFailureRestoresDiscardedState(t *testing.T) {
	e, r := testEngine(t)
	p := resetPlan(t, e)
	p.Steps = []command{{Path: "fake", Description: "synthetic install"}}
	r.fail = true
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("command failure was ignored")
	}
	invoked := false
	for _, call := range r.calls {
		if call.Description == "synthetic install" {
			invoked = true
		}
	}
	if !invoked {
		t.Fatal("failure proof never reached the failing command")
	}
	if _, err := os.Stat(p.Resources[0].Path); err != nil {
		t.Fatal("failure lost original state")
	}
}

func TestPermanentDiscardPurgesRecovery(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	if _, err := e.snapshot(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.Request.Permanent = true
	if err := e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	backups, err := e.snapshots()
	if err != nil || len(backups) != 0 {
		t.Fatalf("permanent discard retained recovery: %v %v", backups, err)
	}
}

func TestMutationLockExcludesOtherOperations(t *testing.T) {
	e, _ := testEngine(t)
	release, err := e.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if second, err := e.lock(); err == nil {
		second()
		t.Fatal("concurrent mutation lock accepted")
	}
}
