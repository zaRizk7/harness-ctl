package manager

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractHTTP supplies actual adapter-shaped metadata and package bytes without
// accessing vendor services or executing upstream installers.
type contractHTTP struct{}

func (contractHTTP) Do(r *http.Request) (*http.Response, error) {
	body := `{"dist-tags":{"latest":"1.2.3"}}`
	if strings.Contains(r.URL.Path, "/1.2.3") {
		sum := sha512.Sum512([]byte("fixture-package"))
		value := map[string]any{"version": "1.2.3", "dist": map[string]string{"integrity": "sha512-" + base64.StdEncoding.EncodeToString(sum[:]), "tarball": "https://fixture.test/package.tgz"}}
		data, _ := json.Marshal(value)
		body = string(data)
	}
	if r.URL.Host == "fixture.test" {
		body = "fixture-package"
	}
	if strings.HasSuffix(r.URL.Path, ".sh") {
		body = "#!/bin/sh\nexit 0\n"
	}
	if strings.HasSuffix(r.URL.Path, "/stable") {
		body = "1.2.3"
	}
	if strings.HasSuffix(r.URL.Path, "/commits/main") {
		body = `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func installFixtureAt(t *testing.T, e *engine, p *plan) {
	t.Helper()
	inst := syntheticInstall(t, e, p.Spec, filepath.Base(p.Destination))
	if p.Spec.Kind == "npm" {
		if err := writeJSON(filepath.Join(inst.Root, "lib", "node_modules", p.Spec.Package, "package.json"), map[string]string{"name": p.Spec.Package, "version": p.Request.Target}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLifecyclePlanToVerifiedResultForEveryAdapter(t *testing.T) {
	for _, s := range catalog {
		t.Run(s.ID, func(t *testing.T) {
			e, r := testEngine(t)
			e.client = contractHTTP{}
			bin := t.TempDir()
			_ = atomicWrite(filepath.Join(bin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
			_ = atomicWrite(filepath.Join(bin, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0700)
			t.Setenv("PATH", bin)
			for _, action := range []string{"install", "update", "reinstall"} {
				p, err := e.buildPlan(context.Background(), request{Harness: s.ID, Action: action})
				if err != nil {
					t.Fatal(action, err)
				}
				if len(p.Blockers) != 0 {
					t.Fatal(action, p.Blockers)
				}
				r.onRun = func(c command) error {
					if c.Description == "Run reviewed native installer" || strings.HasPrefix(c.Description, "Install verified npm package") || c.Description == "Hermes native python-deps stage" {
						installFixtureAt(t, e, p)
					}
					return nil
				}
				if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
					t.Fatal(action, err)
				}
				if len(e.reg.Installs) != 1 {
					t.Fatal("missing verified installation")
				}
				if action == "install" {
					_ = atomicWrite(filepath.Join(e.reg.Installs[0].StateRoot, "auth.json"), []byte("fixture auth"), 0600)
				}
			}
			inst := e.reg.Installs[0]
			p, err := e.buildPlan(context.Background(), request{Harness: s.ID, Action: "reset", Preserve: map[category]bool{auth: true}})
			if err != nil {
				t.Fatal(err)
			}
			if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(inst.StateRoot, "auth.json")); err != nil {
				t.Fatal("preserved auth missing", err)
			}
			p, err = e.buildPlan(context.Background(), request{Harness: s.ID, Action: "uninstall"})
			if err != nil {
				t.Fatal(err)
			}
			if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMigrationCopiesTrackedStateAndRetainsOriginal(t *testing.T) {
	e, r := testEngine(t)
	e.client = contractHTTP{}
	s, _ := e.specFor("pi")
	oldRoot := filepath.Join(e.cfg.Home, "prefix", "lib", "node_modules", s.Package)
	oldBin := filepath.Join(oldRoot, "bin", "pi.js")
	_ = atomicWrite(oldBin, []byte("#!/bin/sh\nexit 0\n"), 0700)
	_ = writeJSON(filepath.Join(oldRoot, "package.json"), map[string]string{"name": s.Package, "version": "1.0.0"})
	bin := filepath.Join(e.cfg.Home, "prefix", "bin")
	_ = os.MkdirAll(bin, 0700)
	_ = os.Symlink(oldBin, filepath.Join(bin, s.Command))
	_ = atomicWrite(filepath.Join(bin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", bin)
	oldState := e.stateRoot(s)
	_ = atomicWrite(filepath.Join(oldState, "auth.json"), []byte("fixture"), 0600)
	p, err := e.buildPlan(context.Background(), request{Harness: "pi", Action: "migrate"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Method != "npm" {
		t.Fatal(p.Install)
	}
	r.onRun = func(c command) error {
		if strings.HasPrefix(c.Description, "Install verified npm package") {
			installFixtureAt(t, e, p)
		}
		return nil
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldBin, filepath.Join(oldState, "auth.json"), filepath.Join(e.reg.Installs[0].StateRoot, "auth.json")} {
		if _, err = os.Stat(path); err != nil {
			t.Fatal(path, err)
		}
	}
}

func TestProfileBaseLaunchKeepsRetainedCopy(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	_ = atomicWrite(filepath.Join(inst.StateRoot, "settings.json"), []byte(`{"ordinary":true}`), 0600)
	if err := e.createProfile(inst, map[category]bool{proxies: true}); err != nil {
		t.Fatal(err)
	}
	_ = writeJSON(e.statePath, e.reg)
	c, err := e.launchCommand(inst, nil, false)
	if err != nil || c.Env["HTTP_PROXY"] != "" || c.Env["HOME"] == "" {
		t.Fatal(c, err)
	}
	profile := e.reg.Profiles[inst.ID].Root
	if err = e.disableProfile(inst); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.reg.Profiles[inst.ID]; ok {
		t.Fatal("profile remains active")
	}
	if _, err = os.Stat(profile); err != nil {
		t.Fatal("base switch deleted retained state", err)
	}
	if err = e.disableProfile(installation{ID: "missing"}); err == nil {
		t.Fatal("unknown install switched")
	}
}
