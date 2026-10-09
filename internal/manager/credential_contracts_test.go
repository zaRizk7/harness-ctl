package manager

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

// credentialFixture uses two declared files to prove partial-write rollback.
func credentialFixture(t *testing.T) (*engine, installation, credentials.Profile) {
	t.Helper()
	e, _ := testEngine(t)
	e.cfg.Harnesses[0].Auth.CredentialFiles = []credentials.Location{{Root: 0, Path: "auth.json"}, {Root: 0, Path: "extra.json"}}
	s := e.cfg.Harnesses[0]
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	p := credentials.Profile{ID: "work", Harness: s.ID, Files: []credentials.File{{Location: s.Auth.CredentialFiles[0], Data: []byte("first-secret")}, {Location: s.Auth.CredentialFiles[1], Data: []byte("second-secret")}}}
	path, err := e.credentialPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Write(path, e.cfg.MetadataBytes, e.identity, []credentials.Profile{p}); err != nil {
		t.Fatal(err)
	}
	return e, inst, p
}

func credentialPlan(t *testing.T, e *engine, op string) *plan {
	t.Helper()
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "credentials", Target: op, CredentialID: "work", Owners: e.cfg.Harnesses[0].SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCredentialRestoreRollsBackPartialWritesAndAbsentFiles(t *testing.T) {
	e, inst, _ := credentialFixture(t)
	p := credentialPlan(t, e, "apply")
	old := atomicWrite
	fault := errors.New("synthetic second credential write")
	atomicWrite = func(path string, data []byte, mode os.FileMode) error {
		if path == filepath.Join(inst.StateRoot, "extra.json") {
			return fault
		}
		return old(path, data, mode)
	}
	t.Cleanup(func() { atomicWrite = old })
	if err := e.execute(context.Background(), p, p.ID, nil); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	atomicWrite = old
	for _, name := range []string{"auth.json", "extra.json"} {
		if _, err := os.Stat(filepath.Join(inst.StateRoot, name)); !os.IsNotExist(err) {
			t.Fatal("partial credential restore survived rollback", name, err)
		}
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatal("rollback damaged encrypted source", err)
	}
	p = credentialPlan(t, e, "apply")
	if err := e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	for _, file := range profiles[0].Files {
		path := filepath.Join(inst.StateRoot, file.Location.Path)
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, file.Data) {
			t.Fatal("compatible restore failed", err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatal("restored secret is not private")
		}
	}
}

func TestCredentialProfilesRejectUnapprovedStaleAndIncompatibleActions(t *testing.T) {
	e, inst, profile := credentialFixture(t)
	p := credentialPlan(t, e, "apply")
	if e.execute(context.Background(), p, "", nil) == nil {
		t.Fatal("unapproved restore accepted")
	}
	path := filepath.Join(inst.StateRoot, "auth.json")
	if err := atomicWrite(path, []byte("new-native-refresh"), 0600); err != nil {
		t.Fatal(err)
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("stale restore overwrote native refresh")
	}
	p = credentialPlan(t, e, "capture")
	p.Request.Owners = nil
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("edited ownership accepted")
	}
	for _, args := range [][]string{nil, {"capture", "--unknown"}, {"apply"}, {"remove", "--profile", "missing", "codex"}, {"capture", "--profile", "../bad", "codex"}, {"unknown", "--profile", "work", "codex"}, {"apply", "--profile", "work", "claude"}, {"capture", "--profile", "new", "pi"}} {
		if err := e.credentialCLI(context.Background(), args, strings.NewReader("cancel\n"), io.Discard); err == nil {
			t.Fatal("invalid credential request", args)
		}
	}
	for _, args := range [][]string{{"list"}, {"capture", "--preview", "--profile", "work", "codex"}, {"remove", "--yes", "--profile", "work", "codex"}} {
		if err := e.credentialCLI(context.Background(), args, strings.NewReader(""), io.Discard); err != nil {
			t.Fatal(args, err)
		}
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatal("remove retained profile", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "new-native-refresh" {
		t.Fatal("removing captured copy mutated native credentials")
	}
	profile.Harness = "missing"
	if err := vault.Write(filepath.Join(e.cfg.Root, "credentials.age"), e.cfg.MetadataBytes, e.identity, []credentials.Profile{profile}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.loadCredentialProfiles(); err == nil {
		t.Fatal("unknown harness profile accepted")
	}
}

func TestCredentialVaultValidationAndOwnerScope(t *testing.T) {
	e, _, profile := credentialFixture(t)
	path := filepath.Join(e.cfg.Root, "credentials.age")
	for _, profiles := range [][]credentials.Profile{{profile, profile}, {{ID: "invalid", Harness: "codex"}}} {
		if err := vault.Write(path, e.cfg.MetadataBytes, e.identity, profiles); err != nil {
			t.Fatal(err)
		}
		if _, err := e.loadCredentialProfiles(); err == nil {
			t.Fatal("invalid encrypted profile accepted")
		}
	}
	other := credentials.Profile{ID: "personal", Harness: "pi", Files: []credentials.File{{Location: credentials.Location{Root: 0, Path: "auth.json"}, Data: []byte("other-secret")}}}
	if err := vault.Write(path, e.cfg.MetadataBytes, e.identity, []credentials.Profile{profile, other}); err != nil {
		t.Fatal(err)
	}
	p := credentialPlan(t, e, "apply")
	if len(p.Blockers) == 0 {
		t.Fatal("shared credential vault owners omitted")
	}
	if err := e.applyCredentials(&plan{Spec: e.cfg.Harnesses[0], Credential: &credentialMutation{Action: "purge", Path: path}}); err != nil {
		t.Fatal(err)
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].ID != other.ID {
		t.Fatal("purge removed a different harness profile", err)
	}
	if err := atomicWrite(path, []byte("corrupted ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.loadCredentialProfiles(); err == nil {
		t.Fatal("corrupted credentials accepted")
	}
}

func TestCredentialCaptureRejectsFilesystemFailuresAndConcurrentRefresh(t *testing.T) {
	for _, scenario := range []string{"missing", "directory", "oversize", "lstat", "open", "read", "close", "growth", "stale", "escape"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, _ := credentialFixture(t)
			path := filepath.Join(inst.StateRoot, "auth.json")
			if err := atomicWrite(path, []byte("native-secret"), 0600); err != nil {
				t.Fatal(err)
			}
			p := credentialPlan(t, e, "capture")
			old := fileIO
			t.Cleanup(func() { fileIO = old })
			fault := errors.New("synthetic credential IO failure")
			switch scenario {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				e.cfg.MetadataBytes = 4096
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 4097), 0600); err != nil {
					t.Fatal(err)
				}
			case "lstat":
				fileIO.lstat = func(name string) (os.FileInfo, error) {
					if name == path {
						return nil, fault
					}
					return old.lstat(name)
				}
			case "open":
				fileIO.open = func(name string) (*os.File, error) {
					if name == path {
						return nil, fault
					}
					return old.open(name)
				}
			case "read":
				fileIO.open = func(name string) (*os.File, error) {
					f, err := old.open(name)
					if err == nil && name == path {
						_ = f.Close()
					}
					return f, err
				}
				fileIO.close = func(f *os.File) error { _ = f.Close(); return nil }
			case "close":
				fileIO.close = func(f *os.File) error {
					err := old.close(f)
					if f.Name() == path {
						return fault
					}
					return err
				}
			case "growth":
				e.cfg.MetadataBytes = 4096
				fileIO.open = func(name string) (*os.File, error) {
					if name == path {
						if err := os.WriteFile(name, bytes.Repeat([]byte("x"), 4097), 0600); err != nil {
							return nil, err
						}
					}
					return old.open(name)
				}
			case "stale":
				if err := os.WriteFile(path, []byte("rotated-by-native"), 0600); err != nil {
					t.Fatal(err)
				}
			case "escape":
				p.Credential.Locations = append([]credentials.Location{}, p.Credential.Locations...)
				p.Credential.Locations[0].Root = 99
			}
			if err := e.applyCredentials(p); err == nil {
				t.Fatal("unsafe/incomplete capture accepted")
			}
			fileIO = old
			profiles, err := e.loadCredentialProfiles()
			if err != nil || len(profiles) != 1 || string(profiles[0].Files[0].Data) != "first-secret" {
				t.Fatal("failed capture changed encrypted profile", err)
			}
		})
	}
}

func TestCredentialPlanningTrustBoundaries(t *testing.T) {
	for _, boundary := range []string{"vault-path", "vault-read", "vault-digest", "native-path", "native-digest"} {
		t.Run(boundary, func(t *testing.T) {
			e, inst, _ := credentialFixture(t)
			p := &plan{Spec: e.cfg.Harnesses[0], StateRoot: inst.StateRoot, Request: request{Harness: "codex", CredentialID: "work", Target: "capture", Owners: e.cfg.Harnesses[0].SharedClients}, RootDigests: map[string]string{}}
			oldValidate, oldFingerprint := validateOwnedPath, fingerprint
			t.Cleanup(func() { validateOwnedPath, fingerprint = oldValidate, oldFingerprint })
			fault := errors.New("synthetic credential boundary failure")
			vaultPath := filepath.Join(e.cfg.Root, "credentials.age")
			if boundary == "vault-read" {
				if err := os.WriteFile(vaultPath, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			validateOwnedPath = func(root, path string) error {
				if (boundary == "vault-path" && path == vaultPath) || (boundary == "native-path" && filepath.Base(path) == "auth.json") {
					return fault
				}
				return oldValidate(root, path)
			}
			fingerprint = func(path string) (string, error) {
				if (boundary == "vault-digest" && path == vaultPath) || (boundary == "native-digest" && filepath.Base(path) == "auth.json") {
					return "", fault
				}
				return oldFingerprint(path)
			}
			if e.planCredentials(p) == nil {
				t.Fatal("unsafe credential preview accepted")
			}
			p.Request.Permanent = true
			p.Request.Preserve = map[category]bool{}
			if boundary != "native-path" && boundary != "native-digest" && e.planCredentialPurge(p) == nil {
				t.Fatal("unsafe purge preview accepted")
			}
		})
	}
	e, inst, _ := credentialFixture(t)
	p := &plan{Spec: e.cfg.Harnesses[0], StateRoot: inst.StateRoot, Credential: &credentialMutation{Action: "apply", Profile: credentials.Profile{Files: []credentials.File{{Location: credentials.Location{Root: 99, Path: "auth.json"}}}}}}
	if e.applyCredentials(p) == nil {
		t.Fatal("invalid restore location")
	}
	e.cfg.Harnesses[0].Auth.CredentialFiles = nil
	p.Request = request{CredentialID: "new", Target: "capture"}
	p.Spec = e.cfg.Harnesses[0]
	// Remove the existing profile first so validation reaches the missing contract.
	if err := os.Remove(filepath.Join(e.cfg.Root, "credentials.age")); err != nil {
		t.Fatal(err)
	}
	if e.planCredentials(p) == nil {
		t.Fatal("capture without a declared credential contract")
	}
	p.Request.Permanent = true
	if err := e.planCredentialPurge(p); err != nil {
		t.Fatal(err)
	}
}

func TestObsoleteCredentialProfilesRemainManageableWithoutUnsafeRestore(t *testing.T) {
	e, _, _ := credentialFixture(t)
	e.cfg.Harnesses[0].Auth.CredentialFiles = []credentials.Location{{Root: 0, Path: "new-auth.json"}}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatal("catalog edit made a saved credential copy unmanageable", err)
	}
	if _, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "credentials", Target: "apply", CredentialID: "work", Owners: e.cfg.Harnesses[0].SharedClients}); err == nil {
		t.Fatal("obsolete native credential format could be restored")
	}
	if err := e.credentialCLI(context.Background(), []string{"remove", "--yes", "--profile", "work", "codex"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal("obsolete credential copy could not be removed", err)
	}
	profiles, err = e.loadCredentialProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatal("obsolete credential copy retained", err)
	}
}
