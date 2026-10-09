package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

func TestNativeAuthBoundaryErrorsAndCancellation(t *testing.T) {
	e, inst, _ := credentialFixture(t)
	s := e.cfg.Harnesses[0]
	p := &plan{Spec: s, Install: inst, Request: request{Target: "missing"}}
	if e.planNativeAuth(p) == nil {
		t.Fatal("unknown operation accepted")
	}
	p.Request.Target = "login"
	p.Install.Harness = "missing"
	if e.planNativeAuth(p) == nil {
		t.Fatal("unknown native installation accepted")
	}
	if err := atomicWrite(filepath.Join(inst.StateRoot, "auth.json"), []byte("native-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "login", Preserve: keepAll()})
	if err != nil || len(p.Blockers) == 0 {
		t.Fatal("native owner guard missing", err)
	}
	if e.authCLI(context.Background(), []string{"status", "codex", "--token", "secret"}, strings.NewReader(""), io.Discard) == nil {
		t.Fatal("inline secret reached native status")
	}
	oldBin := e.cfg.BinDir
	e.cfg.BinDir = inst.Root
	for _, op := range []string{"status", "login"} {
		if e.authCLI(context.Background(), []string{op, "codex"}, strings.NewReader(""), io.Discard) == nil {
			t.Fatal("recursive native command accepted")
		}
	}
	e.cfg.BinDir = oldBin
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if e.authCLI(cancelled, []string{"status", "codex"}, strings.NewReader(""), io.Discard) == nil {
		t.Fatal("cancelled native inventory ignored")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := interactiveCommand(ctx, command{Path: "/bin/sleep", Args: []string{"30"}}, strings.NewReader(""), io.Discard); err == nil || ctx.Err() == nil {
		t.Fatal("native child did not cancel", err)
	}
	var body strings.Builder
	e.sourceConfig = filepath.Join(e.cfg.Home, "custom.json")
	if err := e.authShim(&body, inst, s); err != nil || !strings.Contains(body.String(), "--config "+shellQuote(e.sourceConfig)) {
		t.Fatal("explicit router configuration lost", err)
	}
	if _, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "missing"}); err == nil {
		t.Fatal("missing native command contract planned")
	}
}

func TestCredentialCLIAndPurgeFailureBoundaries(t *testing.T) {
	e, inst, _ := credentialFixture(t)
	path := filepath.Join(e.cfg.Root, "credentials.age")
	if e.credentialCLI(context.Background(), []string{"apply", "--yes", "--profile", "work", "codex"}, strings.NewReader(""), brokenOutput{}) == nil {
		t.Fatal("unreadable credential preview accepted")
	}
	old := validateOwnedPath
	t.Cleanup(func() { validateOwnedPath = old })
	calls := 0
	validateOwnedPath = func(root, name string) error {
		if name == path {
			calls++
			if calls == 2 {
				return errors.New("synthetic purge path change")
			}
		}
		return old(root, name)
	}
	p := &plan{Spec: e.cfg.Harnesses[0], StateRoot: inst.StateRoot, Request: request{Permanent: true, Preserve: map[category]bool{}}, RootDigests: map[string]string{}}
	if e.planCredentialPurge(p) == nil {
		t.Fatal("changed purge path accepted")
	}
	validateOwnedPath = old
	if err := os.WriteFile(path, []byte("invalid ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if e.applyCredentials(p) == nil {
		t.Fatal("corrupted profiles applied")
	}
	if e.credentialCLI(context.Background(), []string{"list"}, strings.NewReader(""), io.Discard) == nil {
		t.Fatal("corrupted profiles listed")
	}
	if _, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "reset", Permanent: true, Preserve: map[category]bool{}}); err == nil {
		t.Fatal("corrupted credential purge planned")
	}
}

func TestNativeAuthTUICredentialInventoryAndOwnerErrors(t *testing.T) {
	e, inst, stored := credentialFixture(t)
	m := newModel(e)
	m.installs = []installation{inst}
	m.screen = "native-auth"
	if _, _, handled := m.nativeAuthUpdate(keyMessage("unused")); handled {
		t.Fatal("unknown native key consumed")
	}
	_, _, _ = m.nativeAuthUpdate(nativeOwnersMsg{err: errors.New("synthetic owners")})
	msg := m.loadNativeCredentials()().(nativeCredentialMsg)
	if msg.err != nil || len(msg.items) != 1 || msg.items[0].ID != stored.ID {
		t.Fatal("native profiles unavailable", msg.err)
	}
	m.credentialProfiles = msg.items
	m.screen = "native-auth-profiles"
	if !strings.Contains(m.View().Content, stored.ID) {
		t.Fatal("native profile hidden")
	}
	if err := atomicWrite(filepath.Join(e.cfg.Home, ".agents", "skills", "shared", "SKILL.md"), []byte("shared"), 0600); err != nil {
		t.Fatal(err)
	}
	other := credentials.Profile{ID: "other", Harness: "pi", Files: []credentials.File{{Location: credentials.Location{Root: 0, Path: "auth.json"}, Data: []byte("other-secret")}}}
	path := filepath.Join(e.cfg.Root, "credentials.age")
	if err := vault.Write(path, e.cfg.MetadataBytes, e.identity, []credentials.Profile{stored, other}); err != nil {
		t.Fatal(err)
	}
	owners := m.loadNativeOwners()().(nativeOwnersMsg)
	if owners.err != nil || !contains(owners.items, "pi") || !contains(owners.items, "Other agents using ~/.agents/skills") {
		t.Fatal("shared credential owners omitted", owners)
	}
	prof := profile{Root: filepath.Join(e.cfg.Root, "profiles", inst.ID)}
	e.reg.Profiles[inst.ID] = prof
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "login", Owners: e.cfg.Harnesses[0].SharedClients, Preserve: keepAll()})
	if err != nil || p.StateRoot != prof.Root {
		t.Fatal("native profile scope lost", err)
	}
	owners = m.loadNativeOwners()().(nativeOwnersMsg)
	if owners.err != nil {
		t.Fatal(owners.err)
	}
	if err := os.WriteFile(path, []byte("invalid ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if m.loadNativeOwners()().(nativeOwnersMsg).err == nil {
		t.Fatal("invalid shared vault ignored")
	}
	if err := atomicWrite(filepath.Join(prof.Root, "config.toml"), []byte("invalid= ["), 0600); err != nil {
		t.Fatal(err)
	}
	if m.loadNativeOwners()().(nativeOwnersMsg).err == nil {
		t.Fatal("invalid owner resources ignored")
	}
	m.installs[0].Harness = "missing"
	if m.loadNativeOwners()().(nativeOwnersMsg).err == nil {
		t.Fatal("missing owner contract ignored")
	}
}

func TestPermanentCredentialPurgeFailureRestoresEncryptedVault(t *testing.T) {
	e, _, _ := credentialFixture(t)
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "reset", Permanent: true, Owners: e.cfg.Harnesses[0].SharedClients, Preserve: map[category]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	old := writeJSON
	t.Cleanup(func() { writeJSON = old })
	tampered := false
	writeJSON = func(path string, value any) error {
		if err := old(path, value); err != nil {
			return err
		}
		if record, ok := value.(operationRecord); ok && record.Status == "executing" && !tampered {
			tampered = true
			return os.WriteFile(filepath.Join(e.cfg.Root, "credentials.age"), []byte("invalid ciphertext"), 0600)
		}
		return nil
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil || !tampered {
		t.Fatal("failed credential purge did not roll back")
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].ID != "work" {
		t.Fatal("failed purge damaged retained vault", err)
	}
}
