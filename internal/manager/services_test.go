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
	body := "<plist><dict><key>ProgramArguments</key><array><string>" + inst.Path + "</string></array></dict></plist>"
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
