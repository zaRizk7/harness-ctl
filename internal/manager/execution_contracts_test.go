package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackedVerificationRequiresExecutableAndPinnedManifest(t *testing.T) {
	e, _ := testEngine(t)
	root := filepath.Join(e.cfg.Home, "native")
	path := filepath.Join(root, "pi")
	p := &plan{Install: installation{Root: root, Path: path, Method: "npm"}, Request: request{Target: "2.0.0"}}
	if err := e.verifyTracked(p); err == nil {
		t.Fatal("missing executable accepted")
	}
	if err := atomicWrite(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.verifyTracked(p); err == nil {
		t.Fatal("missing manifest accepted")
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if err := writeJSON(filepath.Join(root, "package.json"), map[string]any{"version": version}); err != nil {
			t.Fatal(err)
		}
		if err := e.verifyTracked(p); (err == nil) != (version == "2.0.0") {
			t.Fatal(version, err)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.verifyTracked(p); err == nil {
		t.Fatal("installer output without executable permission accepted")
	}
}

func TestVerifyStateEnforcesBothPreservationAndErasure(t *testing.T) {
	for _, after := range []struct {
		name, data string
		ok, remove bool
	}{
		{"selected erasure", `{"api_key":"key"}`, true, false},
		{"discard remains", `{"api_key":"key","mcp_servers":{}}`, false, false},
		{"preserved disappears", `{}`, false, false},
		{"preserved changed", `{"api_key":"different"}`, false, false},
		{"invalid format", `not-json`, false, false},
		{"file disappears", "", false, true},
	} {
		t.Run(after.name, func(t *testing.T) {
			e, _ := testEngine(t)
			path := filepath.Join(e.cfg.Home, "state", "settings.json")
			r := resource{Path: path, Root: filepath.Dir(path), Owners: []string{"pi"}, Format: "json", Fields: map[string]category{"/api_key": auth, "/mcp_servers": mcp}, FieldDigests: map[string]string{"/api_key": valueDigest("key")}}
			p := &plan{Request: request{Harness: "pi", Action: "reset", Preserve: map[category]bool{auth: true}}, Resources: []resource{r}}
			if !after.remove {
				if err := atomicWrite(path, []byte(after.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.verifyState(p); (err == nil) != after.ok {
				t.Fatal(after, err)
			}
			if after.remove {
				p.Request.Preserve = map[category]bool{}
				if err := e.verifyState(p); err != nil {
					t.Fatal("complete erasure rejected", err)
				}
			}
		})
	}
	e, _ := testEngine(t)
	path := filepath.Join(e.cfg.Home, "state", "auth.json")
	if err := atomicWrite(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	r := resource{Path: path, Root: filepath.Dir(path), Owners: []string{"pi"}, Category: auth, Digest: digest}
	p := &plan{Request: request{Harness: "pi", Action: "reset", Preserve: keepAll()}, Resources: []resource{r}}
	if err = e.verifyState(p); err != nil {
		t.Fatal(err)
	}
	if err = atomicWrite(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.verifyState(p); err == nil {
		t.Fatal("preserved whole resource changed")
	}
	p.Request.Preserve = map[category]bool{}
	if err = e.verifyState(p); err == nil {
		t.Fatal("discarded whole resource remains")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = e.verifyState(p); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedNativeLauncherRemovalAndForeignProtection(t *testing.T) {
	for _, mode := range []string{"symlink", "script", "foreign", "missing", "outside"} {
		t.Run(mode, func(t *testing.T) {
			e, _ := testEngine(t)
			root := filepath.Join(e.cfg.Home, ".local", "share", "native")
			target := filepath.Join(root, "bin", "harness")
			path := filepath.Join(e.cfg.Home, ".local", "bin", "harness")
			if err := atomicWrite(target, []byte("native payload"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "script":
				if err := atomicWrite(path, []byte("#!/bin/sh\nexec "+shellQuote(target)+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				if err := atomicWrite(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
					t.Fatal(err)
				}
			case "outside":
				path = target
			}
			err := e.removeOwnedLauncher(installation{Root: root, Path: path})
			if (err != nil) != (mode == "foreign" || mode == "outside") {
				t.Fatal(mode, err)
			}
			if mode == "foreign" {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("foreign launcher removed", err)
				}
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatal("launcher removal erased payload", err)
			}
		})
	}
}

type commandRunner func(context.Context, command) (string, error)

func (r commandRunner) Run(ctx context.Context, c command) (string, error) { return r(ctx, c) }

func TestServiceRestorationRevalidatesOwnershipAndNativeStatus(t *testing.T) {
	for _, mode := range []string{"loaded", "missing", "print-error", "bootstrap-error", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			e, _ := testEngine(t)
			s, _ := e.specFor("hermes")
			inst := syntheticInstall(t, e, s, "1.0.0")
			e.reg.Installs = []installation{inst}
			path := filepath.Join(e.cfg.Home, "Library", "LaunchAgents", s.LaunchLabels[0]+".plist")
			program := inst.Path
			if mode == "foreign" {
				program = "/foreign/program"
			}
			body := "<plist><dict><key>Label</key><string>" + s.LaunchLabels[0] + "</string><key>Program</key><string>" + program + "</string></dict></plist>"
			if err := atomicWrite(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			bootstrap, calls := 0, 0
			e.run = commandRunner(func(_ context.Context, c command) (string, error) {
				calls++
				if c.Args[0] == "bootstrap" {
					bootstrap++
					if mode == "bootstrap-error" {
						return "", errors.New("bootstrap failed")
					}
					return "", nil
				}
				if mode == "loaded" {
					return "loaded", nil
				}
				if mode == "print-error" {
					return "unrelated error", errors.New("print failed")
				}
				return "Could not find service", errors.New("service absent")
			})
			err := e.restartServices(context.Background(), []string{path})
			wantError := strings.HasSuffix(mode, "error") || mode == "foreign"
			if (err != nil) != wantError {
				t.Fatal(mode, err)
			}
			if mode == "missing" && bootstrap != 1 || mode == "loaded" && bootstrap != 0 || mode == "foreign" && calls != 0 {
				t.Fatal(mode, calls, bootstrap)
			}
		})
	}
}
