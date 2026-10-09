package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/library"
	"github.com/zalando/go-keyring"
)

type contractWriter func([]byte) (int, error)

func (w contractWriter) Write(data []byte) (int, error) { return w(data) }

type undeletableKeys struct{}

func (undeletableKeys) Get(string) (string, error) { return "", keyring.ErrNotFound }
func (undeletableKeys) Set(string, string) error   { return nil }

func TestSelfRemovalOwnershipAndStaleLauncherGuards(t *testing.T) {
	for _, scenario := range []string{"mode", "receipt-outside", "receipt-parent", "cancelled-discovery", "duplicate", "unsupported", "changed-binary", "changed-link-digest", "changed-link-parent", "batch-error", "state-validation"} {
		t.Run(scenario, func(t *testing.T) {
			e, _ := testEngine(t)
			binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
			_ = atomicWrite(binary, []byte("fixture"), 0700)
			link := filepath.Join(e.cfg.Home, "bin/harness-ctl")
			_ = os.MkdirAll(filepath.Dir(link), 0700)
			_ = os.Symlink(binary, link)
			_ = writeJSON(filepath.Join(e.cfg.Root, "installation.json"), map[string]string{"Binary": binary, "Link": link})
			ctx := context.Background()
			removeHarnesses := scenario == "duplicate" || scenario == "unsupported" || scenario == "cancelled-discovery" || scenario == "batch-error"
			s, _ := e.specFor("pi")
			if scenario == "duplicate" || scenario == "batch-error" {
				e.reg.Installs = []installation{syntheticInstall(t, e, s, "1")}
				if scenario == "duplicate" {
					e.reg.Installs = append(e.reg.Installs, syntheticInstall(t, e, s, "2"))
				}
				_ = writeJSON(e.statePath, e.reg)
			}
			switch scenario {
			case "mode":
				_ = os.Chmod(binary, 0600)
			case "receipt-outside":
				_ = writeJSON(filepath.Join(e.cfg.Root, "installation.json"), map[string]string{"Binary": binary, "Link": "/foreign/harness-ctl"})
			case "receipt-parent":
				_ = os.Remove(link)
				_ = os.Remove(filepath.Dir(link))
				_ = os.Symlink(filepath.Join(e.cfg.Home, "tools"), filepath.Dir(link))
			case "cancelled-discovery":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unsupported":
				e.cfg.AdditionalCommands = []string{"extra"}
				_ = atomicWrite(filepath.Join(os.Getenv("PATH"), "extra"), []byte("fixture"), 0700)
			}
			p, err := e.buildSelfPlan(ctx, binary, removeHarnesses, scenario == "state-validation", request{Preserve: keepAll()})
			if scenario == "mode" || scenario == "receipt-outside" || scenario == "receipt-parent" || scenario == "cancelled-discovery" || scenario == "duplicate" {
				if err == nil {
					t.Fatal("invalid preview accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "unsupported" {
				if p.Batch != nil {
					t.Fatal("unsupported harness scheduled for deletion")
				}
				return
			}
			oldDigest, oldAncestors, oldValidate := fingerprint, rejectLinkedAncestors, validateOwnedPath
			defer func() { fingerprint, rejectLinkedAncestors, validateOwnedPath = oldDigest, oldAncestors, oldValidate }()
			fault := errors.New("self-removal boundary")
			switch scenario {
			case "changed-binary":
				_ = atomicWrite(binary, []byte("changed"), 0700)
			case "changed-link-digest":
				count := 0
				fingerprint = func(path string) (string, error) {
					if path == link {
						count++
						if count > 0 {
							return "changed", nil
						}
					}
					return oldDigest(path)
				}
			case "changed-link-parent":
				rejectLinkedAncestors = func(path string) error {
					if path == filepath.Dir(link) {
						return fault
					}
					return oldAncestors(path)
				}
			case "batch-error":
				p.Batch.Plans[0].Blockers = []string{"changed"}
				p.Batch.Digest = valueDigest(p.Batch.Plans)
				p.Digest = selfPlanDigest(p)
			case "state-validation":
				count := 0
				validateOwnedPath = func(root, path string) error {
					if path == p.Paths[0] {
						count++
						if count > 0 {
							return fault
						}
					}
					return oldValidate(root, path)
				}
			}
			if err = e.executeSelf(ctx, p, p.ID, nil); err == nil {
				t.Fatal("stale or failed removal succeeded")
			}
			if _, err = os.Stat(binary); err != nil {
				t.Fatal("failed removal lost manager executable", err)
			}
		})
	}
	e, _ := testEngine(t)
	e.keys = undeletableKeys{}
	if err := e.deleteStorageKey(); err == nil {
		t.Fatal("unsupported key deletion ignored")
	}
	k := newKeychainStore()
	k.remove = func(string, string) error { return keyring.ErrNotFound }
	e.keys = k
	if err := e.deleteStorageKey(); err != nil {
		t.Fatal("missing key not idempotent", err)
	}
	old := rejectLinkedAncestors
	defer func() { rejectLinkedAncestors = old }()
	rejectLinkedAncestors = func(string) error { return errors.New("unsafe manager parent") }
	if _, err := e.selfStateDigest(); err == nil {
		t.Fatal("unsafe root accepted")
	}
}

func TestSelfCLIRecordedPathsBatchOutputAndProgress(t *testing.T) {
	e, _, inst := nativeComponentFixture(t, "pi")
	binary := filepath.Join(e.cfg.Home, "tools/harness-ctl")
	_ = atomicWrite(binary, []byte("fixture"), 0700)
	link := filepath.Join(e.cfg.Home, "bin/harness-ctl")
	_ = os.MkdirAll(filepath.Dir(link), 0700)
	_ = os.Symlink(binary, link)
	_ = writeJSON(filepath.Join(e.cfg.Root, "installation.json"), map[string]string{"Binary": binary, "Link": link})
	old := executablePath
	defer func() { executablePath = old }()
	executablePath = func() (string, error) { return binary, nil }
	calls := 0
	out := contractWriter(func(data []byte) (int, error) {
		calls++
		if calls > 1 {
			return 0, io.ErrClosedPipe
		}
		return len(data), nil
	})
	if err := e.cli(context.Background(), []string{"self-uninstall", "--harnesses", "--yes"}, strings.NewReader(""), out); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if _, err := os.Stat(inst.Path); err != nil {
		t.Fatal("unseen batch mutated harness", err)
	}
	if err := e.selfCLI(context.Background(), []string{"--harnesses", "--yes"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := e.selfCLI(context.Background(), []string{"--preview"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing executable preview accepted")
	}
}

func TestAccountLaunchAndCLIReportBoundaryFailures(t *testing.T) {
	e, _, inst := nativeComponentFixture(t, "pi")
	_ = atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s' \"$OPENAI_API_KEY\"\n"), 0700)
	a := account{ID: "a", Provider: "openai", Kind: "api", Credential: "fixture-inference", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := e.launchCLI(context.Background(), []string{"--account", "a", "pi"}, strings.NewReader(""), &out); err != nil || out.String() != a.Credential {
		t.Fatal(out.String(), err)
	}
	if err := e.launchCLI(context.Background(), []string{"--account", "missing", "pi"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing account launched")
	}
	if _, err := e.launchCommand(installation{Harness: "unknown"}, nil, false); err == nil {
		t.Fatal("unknown launch accepted")
	}
	if _, err := e.launchCommand(installation{Harness: "pi", Path: filepath.Join(e.cfg.BinDir, "pi")}, nil, false); err == nil {
		t.Fatal("recursive launch accepted")
	}
	if err := e.cli(context.Background(), []string{"accounts", "monitor"}, strings.NewReader(""), brokenOutput{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	for _, scenario := range []string{"launch-vault", "watch-vault"} {
		if scenario == "watch-vault" {
			_ = e.saveAccount(a)
			writer := contractWriter(func(data []byte) (int, error) {
				_ = atomicWrite(filepath.Join(e.cfg.Root, "accounts.age"), []byte("invalid"), 0600)
				return len(data), nil
			})
			if err := e.accountsCLI(context.Background(), []string{"monitor", "--watch", "--refresh", "1"}, strings.NewReader(""), writer); err == nil {
				t.Fatal("changed malformed vault ignored")
			}
		} else {
			_ = atomicWrite(filepath.Join(e.cfg.Root, "accounts.age"), []byte("invalid"), 0600)
			if _, err := e.accountLaunchEnvironment("a"); err == nil {
				t.Fatal("invalid vault launched")
			}
		}
		_ = os.Remove(filepath.Join(e.cfg.Root, "accounts.age"))
	}
}

func TestLibraryApplicationCompatibilityScopeAndStorageFailures(t *testing.T) {
	e, _, inst := nativeComponentFixture(t, "codex")
	t.Setenv("PATH", filepath.Dir(inst.Path))
	item := reusableSkill("a")
	item.Targets = map[string]library.Target{"codex": {Path: "skills/a", HomeSkills: true}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.saveLibraryItem(reusableSkill("b"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil); err == nil {
		t.Fatal("incompatible recipe selected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.buildLibraryApply(ctx, "a", []string{"codex"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	p, err := e.buildLibraryApply(context.Background(), "a", []string{"codex"}, []string{"Codex desktop / IDE", "Other agents using ~/.agents/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.libraryCLI(context.Background(), []string{"apply", "--yes", "a", "codex"}, strings.NewReader(""), brokenOutput{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	unlock, err := e.lock()
	if err != nil {
		t.Fatal(err)
	}
	err = e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil)
	unlock()
	if err == nil {
		t.Fatal("concurrent library application")
	}
	oldValidate, oldDigest := validateOwnedPath, fingerprint
	defer func() { validateOwnedPath, fingerprint = oldValidate, oldDigest }()
	fault := errors.New("library storage boundary")
	validateOwnedPath = func(string, string) error { return fault }
	if err = e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if err = e.libraryCLI(context.Background(), []string{"remove", "a"}, strings.NewReader(""), io.Discard); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	validateOwnedPath = oldValidate
	fingerprint = func(string) (string, error) { return "", fault }
	if err = e.libraryCLI(context.Background(), []string{"remove", "a"}, strings.NewReader(""), io.Discard); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fingerprint = oldDigest
	bad := reusableSkill("bad")
	bad.Source = "relative"
	bad.Files = nil
	if err = e.saveLibraryItem(bad, ""); err == nil {
		t.Fatal("unsafe capture accepted")
	}
	bad = reusableSkill("bad")
	bad.Category = "unknown"
	if err = e.saveLibraryItem(bad, ""); err == nil {
		t.Fatal("invalid category accepted")
	}
}
