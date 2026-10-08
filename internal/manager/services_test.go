package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceCoordinationRequiresInstallationOwnership(t *testing.T) {
	e, r := testEngine(t)
	s, _ := specFor("hermes")
	inst := syntheticInstall(t, e, s, "1.0.0")
	service := filepath.Join(e.cfg.Home, "Library", "LaunchAgents", s.LaunchLabels[0]+".plist")
	if err := os.MkdirAll(filepath.Dir(service), 0700); err != nil {
		t.Fatal(err)
	}
	body := "<plist><dict><key>Label</key><string>" + s.LaunchLabels[0] + "</string><key>ProgramArguments</key><array><string>" + inst.Path + "</string></array></dict></plist>"
	if err := os.WriteFile(service, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	inst.ServicePaths = []string{service}
	record := operationRecord{ID: randomID()}
	if err := e.stopServices(context.Background(), inst, &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Services) != 1 {
		t.Fatal("loaded service was not journaled")
	}
	bootout := false
	for _, call := range r.calls {
		if len(call.Args) > 0 && call.Args[0] == "bootout" {
			bootout = true
		}
	}
	if !bootout {
		t.Fatal("owned service was not stopped")
	}
	if err := os.WriteFile(service, []byte(strings.ReplaceAll(body, inst.Path, "/unowned/program")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.stopServices(context.Background(), inst, &record); err == nil {
		t.Fatal("foreign service was stopped")
	}
}

func TestServiceOwnershipUsesLaunchExecutable(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := specFor("hermes")
	inst := syntheticInstall(t, e, s, "1.0.0")
	path := filepath.Join(e.cfg.Home, "Library", "LaunchAgents", s.LaunchLabels[0]+".plist")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, command string
		owned         bool
	}{
		{"arguments executable", "<key>ProgramArguments</key><array><string>" + inst.Path + "</string><string>gateway</string></array>", true},
		{"program executable", "<key>Program</key><string>" + inst.Path + "</string>", true},
		{"owned runtime", "<key>Program</key><string>" + filepath.Join(inst.Root, "runtime", "python") + "</string>", true},
		{"environment reference", "<key>Program</key><string>/foreign/program</string><key>EnvironmentVariables</key><dict><key>HARNESS</key><string>" + inst.Path + "</string></dict>", false},
		{"working directory", "<key>Program</key><string>/foreign/program</string><key>WorkingDirectory</key><string>" + inst.Root + "</string>", false},
		{"later argument", "<key>ProgramArguments</key><array><string>/foreign/program</string><string>" + inst.Path + "</string></array>", false},
		{"program overrides argv", "<key>Program</key><string>/foreign/program</string><key>ProgramArguments</key><array><string>" + inst.Path + "</string></array>", false},
		{"duplicate program", "<key>Program</key><string>" + inst.Path + "</string><key>Program</key><string>/foreign/program</string>", false},
		{"nested program", "<key>EnvironmentVariables</key><dict><key>Program</key><string>" + inst.Path + "</string></dict>", false},
		{"duplicate label", "<key>Program</key><string>" + inst.Path + "</string><key>Label</key><string>foreign.service</string>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "<plist><dict><key>Label</key><string>" + s.LaunchLabels[0] + "</string>" + tc.command + "</dict></plist>"
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := e.validateService(inst, path); (err == nil) != tc.owned {
				t.Fatalf("ownership = %v, want %v: %v", err == nil, tc.owned, err)
			}
		})
	}
}

func TestManagedServiceBlocksPrefixReplacement(t *testing.T) {
	for _, action := range []string{"update", "reinstall"} {
		t.Run(action, func(t *testing.T) {
			e, r := testEngine(t)
			e.client = fixtureHTTP{body: "#!/bin/sh\nexit 0\n"}
			s, _ := specFor("hermes")
			inst := syntheticInstall(t, e, s, "1.0.0")
			e.reg.Installs = []installation{inst}
			if err := writeJSON(e.statePath, e.reg); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(e.cfg.Home, "Library", "LaunchAgents", s.LaunchLabels[0]+".plist")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			body := "<plist><dict><key>Label</key><string>" + s.LaunchLabels[0] + "</string><key>Program</key><string>" + inst.Path + "</string></dict></plist>"
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			p, err := e.buildPlan(context.Background(), request{Harness: s.ID, InstallID: inst.ID, Action: action, Target: strings.Repeat("a", 40)})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(p.Blockers, " "), "service rebinding") {
				t.Fatal("prefix replacement did not require verified service rebinding")
			}
			if err = e.execute(context.Background(), p, p.ID, nil); err == nil || len(r.calls) != 0 {
				t.Fatal("blocked update invoked native commands", err)
			}
			if _, err = os.Stat(inst.Path); err != nil {
				t.Fatal("blocked update removed the old executable", err)
			}
		})
	}
}

func TestRunningAffectedClientBlocksMutation(t *testing.T) {
	e, r := testEngine(t)
	p := resetPlan(t, e)
	p.Install.Path = "/synthetic/pi"
	r.onRun = func(c command) error { return nil }
	// Use a runner with a deterministic process list, never user processes.
	e.run = processRunner{}
	if err := e.execute(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("running affected client was ignored")
	}
	if _, err := os.Stat(p.Resources[0].Path); err != nil {
		t.Fatal("blocked operation changed user state")
	}
}

func TestIsolatedStateDoesNotCoordinateSharedDesktop(t *testing.T) {
	e, _ := testEngine(t)
	e.run = processRunner{output: "99999 /Applications/Codex.app/Contents/MacOS/Codex\n"}
	s, _ := specFor("codex")
	p := &plan{Spec: s, Request: request{Harness: s.ID, Action: "reset", Model: "isolated"}}
	if err := e.checkProcesses(context.Background(), p); err != nil {
		t.Fatal("isolated file state unnecessarily depended on shared desktop", err)
	}
	p.Request.Model = "tracked"
	if err := e.checkProcesses(context.Background(), p); err == nil {
		t.Fatal("tracked shared state ignored the running desktop")
	}
}

type processRunner struct{ output string }

func (r processRunner) Run(context.Context, command) (string, error) {
	if r.output != "" {
		return r.output, nil
	}
	return "99999 /synthetic/pi --interactive\n", nil
}
