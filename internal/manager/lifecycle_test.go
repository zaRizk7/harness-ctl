package manager

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureHTTP struct {
	body   string
	status int
}

func (c fixtureHTTP) Do(*http.Request) (*http.Response, error) {
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(c.body))}, nil
}

func syntheticInstall(t *testing.T, e *engine, s harnessSpec, tag string) installation {
	t.Helper()
	root := filepath.Join(e.cfg.Root, "installs", s.ID, tag)
	path := filepath.Join(root, "bin", s.Command)
	if s.ID == "hermes" {
		path = filepath.Join(root, ".hermes", "bin", s.Command)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if s.Kind == "npm" {
		manifest := filepath.Join(root, "lib", "node_modules", s.Package, "package.json")
		if err := writeJSON(manifest, map[string]string{"name": s.Package, "version": tag}); err != nil {
			t.Fatal(err)
		}
	}
	return installation{ID: installID(s.ID, path), Harness: s.ID, Method: "managed-" + s.Kind, Path: path, Root: root, Version: tag, Managed: true, StateRoot: e.managedStateRoot(s), Package: s.Package}
}

func TestEveryAdapterManagedInstallAndUninstallKeepsState(t *testing.T) {
	for _, s := range catalog {
		t.Run(s.ID, func(t *testing.T) {
			e, r := testEngine(t)
			id := randomID()
			dest := filepath.Join(e.cfg.Root, "installs", s.ID, "1.2.3")
			digest, err := fingerprint(e.statePath)
			if err != nil {
				t.Fatal(err)
			}
			p := &plan{ID: id, Spec: s, Request: request{Harness: s.ID, Action: "install", Model: "isolated", Target: "1.2.3", Preserve: keepAll()}, StateRoot: e.managedStateRoot(s), Destination: dest, RegistryDigest: digest, Steps: []command{{Path: "synthetic", Description: "synthetic install"}}}
			r.onRun = func(c command) error {
				if c.Description == "synthetic install" {
					syntheticInstall(t, e, s, "1.2.3")
				}
				return nil
			}
			if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
				t.Fatal(err)
			}
			if len(e.reg.Installs) != 1 {
				t.Fatal("install did not register a managed executable")
			}
			inst := e.reg.Installs[0]
			state := filepath.Join(inst.StateRoot, "auth.json")
			if err = os.MkdirAll(inst.StateRoot, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(state, []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
			p, err = e.buildPlan(context.Background(), request{Harness: s.ID, InstallID: inst.ID, Action: "uninstall", Preserve: keepAll()})
			if err != nil {
				t.Fatal(err)
			}
			if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(inst.Path); !os.IsNotExist(err) {
				t.Fatal("uninstall retained executable")
			}
			if _, err = os.Stat(state); err != nil {
				t.Fatal("uninstall discarded preserved auth")
			}
			if len(e.reg.Installs) != 0 {
				t.Fatal("uninstall retained registry entry")
			}
		})
	}
}

func TestProfileExclusionAndUpgradePreserveProfileWrites(t *testing.T) {
	e, r := testEngine(t)
	s, _ := specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(inst.StateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inst.StateRoot, "settings.json"), []byte(`{"mcpServers":{"demo":{}},"ordinary":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: s.ID, InstallID: inst.ID, Action: "profile", Preserve: keepAll(), Disabled: map[category]bool{mcp: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	profileRoot := e.reg.Profiles[inst.ID].Root
	value, err := readConfig(filepath.Join(profileRoot, "settings.json"), "json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value["mcpServers"]; ok {
		t.Fatal("excluded MCP source reached profile")
	}
	if err = os.WriteFile(filepath.Join(profileRoot, "history.json"), []byte("profile-written"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := fingerprint(e.statePath)
	if err != nil {
		t.Fatal(err)
	}
	installDigest, err := fingerprint(inst.Root)
	if err != nil {
		t.Fatal(err)
	}
	p = &plan{ID: randomID(), Spec: s, Install: inst, Request: request{Harness: s.ID, Action: "update", Model: "isolated", Target: "2.0.0", Preserve: keepAll()}, StateRoot: inst.StateRoot, Destination: filepath.Join(e.cfg.Root, "installs", s.ID, "2.0.0"), RegistryDigest: digest, InstallDigest: installDigest, Steps: []command{{Path: "synthetic", Description: "synthetic install"}}}
	r.onRun = func(c command) error {
		if c.Description == "synthetic install" {
			syntheticInstall(t, e, s, "2.0.0")
		}
		return nil
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	newProfile := e.reg.Profiles[e.reg.Installs[0].ID]
	data, err := os.ReadFile(filepath.Join(newProfile.Root, "history.json"))
	if err != nil || string(data) != "profile-written" {
		t.Fatal("upgrade lost profile-written state", err)
	}
}

func TestOpenCodeProfilePreservesAuthFromDataDirectory(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := specFor("opencode")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	base := filepath.Join(e.cfg.Root, "states", s.ID)
	path := filepath.Join(base, "xdg", "data", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic-auth"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.createProfile(inst, map[category]bool{hooks: true}); err != nil {
		t.Fatal(err)
	}
	profileRoot := e.reg.Profiles[inst.ID].Root
	data, err := os.ReadFile(filepath.Join(profileRoot, "xdg", "data", "opencode", "auth.json"))
	if err != nil || string(data) != "synthetic-auth" {
		t.Fatal("preserved OpenCode data-root authentication was omitted", err)
	}
}

func TestPackagePinRejectsChangedBytes(t *testing.T) {
	e, _ := testEngine(t)
	sum := sha512.Sum512([]byte("approved"))
	p := &plan{ID: randomID(), PackageURL: "https://example.test/package.tgz", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:])}
	p.Artifact = filepath.Join(e.cfg.Root, "downloads", p.ID+".tgz")
	e.client = fixtureHTTP{body: "changed"}
	if err := e.stagePackage(context.Background(), p); err == nil {
		t.Fatal("changed package bytes passed preview integrity")
	}
	if _, err := os.Stat(p.Artifact); !os.IsNotExist(err) {
		t.Fatal("invalid package was published")
	}
	e.client = fixtureHTTP{body: "approved"}
	if err := e.stagePackage(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestNewStateAfterPreviewBlocksExecution(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	digest, err := fingerprint(p.StateRoot)
	if err != nil {
		t.Fatal(err)
	}
	p.RootDigests = map[string]string{p.StateRoot: digest}
	if err = os.WriteFile(filepath.Join(p.StateRoot, "new.json"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("new state created after preview was ignored")
	}
}

func TestInterruptedJournalBlocksMutationAndCanRecover(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	record := operationRecord{ID: randomID(), Harness: p.Spec.ID, Action: "reset", Status: "executing", Snapshot: meta.ID, Started: time.Now()}
	if err = e.saveRecord(record); err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("pending operation did not block a new mutation")
	}
	if err = os.Remove(p.Resources[0].Path); err != nil {
		t.Fatal(err)
	}
	if err = e.recoverOperation(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(p.Resources[0].Path); err != nil {
		t.Fatal("recovery did not restore original state")
	}
}

func TestTamperedSnapshotCannotModifyLiveState(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	meta, err := e.snapshot(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.cfg.Root, "snapshots", meta.ID+".age")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.restoreSnapshot(context.Background(), meta.ID); err == nil {
		t.Fatal("tampered archive was restored")
	}
	if _, err = os.Stat(p.Resources[0].Path); err != nil {
		t.Fatal("tampered archive changed live state")
	}
}

func TestNativeRecipesUseVerifiedInstallerContracts(t *testing.T) {
	for _, id := range []string{"prime-agent", "hermes"} {
		t.Run(id, func(t *testing.T) {
			e, _ := testEngine(t)
			e.client = fixtureHTTP{body: "#!/bin/sh\nexit 0\n"}
			s, _ := specFor(id)
			target := "1.2.3"
			if id == "hermes" {
				target = strings.Repeat("a", 40)
			}
			p := &plan{ID: randomID(), Spec: s, StateRoot: e.managedStateRoot(s), Request: request{Harness: id, Action: "install", Model: "isolated"}}
			if err := e.nativeRecipe(context.Background(), p, target); err != nil {
				t.Fatal(err)
			}
			if id == "prime-agent" && p.Steps[0].Env["PRIME_AGENT_INSTALL_METHOD"] != "binary" {
				t.Fatal("Prime installer method is invalid")
			}
			if id == "hermes" {
				if len(p.Steps) != 2 {
					t.Fatal("Hermes did not select explicit stages")
				}
				for _, step := range p.Steps {
					if strings.Contains(strings.Join(step.Args, " "), "skip-browser") {
						t.Fatal("Hermes recipe invented an unsupported option")
					}
				}
			}
			if p.Steps[0].Env["HOME"] == e.cfg.Home {
				t.Fatal("native installer writes escaped its scoped HOME")
			}
		})
	}
}
