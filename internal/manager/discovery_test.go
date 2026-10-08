package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHomebrewRuntimeCannotBeOwnedAsHarness(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := specFor("hermes")
	path := filepath.Join(e.cfg.Home, "brew", "Cellar", "python@3.12", "3.12.1", "bin", "hermes")
	inst := installation{Harness: s.ID, Path: path, Method: "unknown", Version: "unknown"}
	e.identify(s, path, &inst)
	if inst.Method != "unknown" {
		t.Fatalf("shared Python runtime was claimed as a harness package: %s %s", inst.Method, inst.Package)
	}
}

func TestHomebrewReinstallAcceptsBlankTarget(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := specFor("codex")
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p := &plan{Spec: s, Install: installation{Method: "brew", Package: "codex", Version: "1.2.3"}, Request: request{Action: "reinstall", Model: "tracked"}}
	if err := e.installRecipe(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if len(p.Blockers) > 0 {
		t.Fatal("blank target was rejected after inferring an unsupported exact Homebrew version", p.Blockers)
	}
	if len(p.Steps) != 1 || p.Steps[0].Args[0] != "reinstall" {
		t.Fatal("native Homebrew reinstall was not selected")
	}
}
