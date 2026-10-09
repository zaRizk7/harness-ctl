package manager

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestNpmRecipeDependencyAndTrackedOwnership(t *testing.T) {
	e, _ := testEngine(t)
	e.client = contractHTTP{}
	s, _ := e.specFor("pi")
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	p := &plan{ID: randomID(), Spec: s, Request: request{Model: "isolated", Action: "install", Target: "1.2.3"}}
	if e.installRecipe(context.Background(), p) == nil {
		t.Fatal("missing prerequisite accepted")
	}
	_ = atomicWrite(filepath.Join(bin, "brew"), []byte("#!/bin/sh\n"), 0700)
	if err := e.installRecipe(context.Background(), p); err != nil || len(p.DependencyCommands) != 1 {
		t.Fatal(p.DependencyCommands, err)
	}
	_ = atomicWrite(filepath.Join(bin, "npm"), []byte("#!/bin/sh\n"), 0700)
	for _, mode := range []string{"npm", "prefix", "bad-prefix", "prefix-error"} {
		p = &plan{ID: randomID(), Spec: s, Request: request{Model: "tracked", Action: "update", Target: "1.2.3"}, Install: installation{Method: "unknown"}}
		if mode == "npm" {
			p.Install.Method = "npm"
			p.Install.Package = s.Package
			p.Install.Root = filepath.Join(e.cfg.Home, "native", "lib", "node_modules", s.Package)
		}
		e.run = commandRunner(func(context.Context, command) (string, error) {
			if mode == "prefix-error" {
				return "", errors.New("fixture prefix failure")
			}
			if mode == "bad-prefix" {
				return "relative", nil
			}
			return filepath.Join(e.cfg.Home, "native"), nil
		})
		err := e.installRecipe(context.Background(), p)
		if (err == nil) != (mode == "npm" || mode == "prefix") {
			t.Fatal(mode, err)
		}
	}
	for _, action := range []string{"update", "reinstall"} {
		p = &plan{Spec: s, Request: request{Model: "tracked", Action: action, Target: "explicit"}, Install: installation{Method: "brew", Package: "pi", Root: "/verified"}}
		if err := e.installRecipe(context.Background(), p); err != nil || len(p.Blockers) != 1 {
			t.Fatal(err, p.Blockers)
		}
	}
	p = &plan{Spec: s, Request: request{Action: "reinstall"}, Install: installation{Version: "unknown"}}
	if e.installRecipe(context.Background(), p) == nil {
		t.Fatal("unknown version reinstalled")
	}
	t.Setenv("PATH", t.TempDir())
	p = &plan{Spec: s, Request: request{Model: "tracked"}, Install: installation{Method: "brew"}}
	if e.installRecipe(context.Background(), p) == nil {
		t.Fatal("missing brew accepted")
	}
}

func TestRecipeMetadataFailuresAreReadOnly(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	for _, body := range []string{"bad", `{"dist-tags":{"latest":"../bad"}}`, `{"dist-tags":{"latest":"1.2.3"}}`} {
		e.client = fixtureHTTP{body: body}
		p := &plan{Spec: s, Request: request{Model: "isolated"}}
		if e.installRecipe(context.Background(), p) == nil {
			t.Fatal("incomplete metadata accepted", body)
		}
	}
	for _, body := range []string{"bad", `{"version":"wrong","dist":{"integrity":"sha512-X","tarball":"https://example.test"}}`} {
		e.client = fixtureHTTP{body: body}
		p := &plan{Spec: s, Request: request{Model: "isolated", Target: "1.2.3"}}
		if e.installRecipe(context.Background(), p) == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	for _, id := range []string{"prime-agent", "hermes", "codex"} {
		s, _ := e.specFor(id)
		e.client = fixtureHTTP{status: 403}
		p := &plan{ID: randomID(), Spec: s, Request: request{Model: "isolated"}}
		if e.nativeRecipe(context.Background(), p, "") == nil {
			t.Fatal("upstream error ignored", id)
		}
	}
	for _, test := range []struct{ id, target string }{{"prime-agent", "../unsafe"}, {"hermes", "short"}, {"pi", "1"}, {"claude", ""}} {
		s, _ := e.specFor(test.id)
		e.client = contractHTTP{}
		p := &plan{ID: randomID(), Spec: s, Request: request{Model: "isolated"}}
		if e.nativeRecipe(context.Background(), p, test.target) == nil {
			t.Fatal("unsupported recipe accepted", test)
		}
	}
}

func TestNativeTrackedRecipesAndUninstallOwnership(t *testing.T) {
	e, _ := testEngine(t)
	e.client = contractHTTP{}
	ctx := context.Background()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, name := range []string{"npm", "brew"} {
		_ = atomicWrite(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0700)
	}
	for _, id := range []string{"codex", "claude", "prime-agent", "hermes"} {
		s, _ := e.specFor(id)
		p := &plan{ID: randomID(), Spec: s, Request: request{Model: "tracked", Action: "migrate"}, StateRoot: e.stateRoot(s), Install: installation{Path: filepath.Join(e.cfg.Home, "native", s.Command), Method: "native-" + id}}
		if err := e.installRecipe(ctx, p); err != nil {
			t.Fatal(id, err)
		}
		if id == "hermes" || id == "prime-agent" {
			if p.Destination == "" {
				t.Fatal("tracked destination missing")
			}
		}
	}
	s, _ := e.specFor("hermes")
	for _, mode := range []string{"dirty", "failure"} {
		e.run = commandRunner(func(context.Context, command) (string, error) {
			if mode == "failure" {
				return "", errors.New("fixture git failure")
			}
			return "modified", nil
		})
		p := &plan{ID: randomID(), Spec: s, Install: installation{Root: filepath.Join(e.cfg.Home, "checkout")}, Request: request{Model: "isolated"}}
		err := e.nativeRecipe(ctx, p, strings.Repeat("a", 40))
		if mode == "dirty" {
			if err != nil || len(p.Blockers) != 1 {
				t.Fatal(err, p.Blockers)
			}
		} else if err == nil {
			t.Fatal("git failure ignored")
		}
	}
	for _, method := range []string{"npm", "brew", "native-codex", "native-claude", "native-hermes", "native-prime"} {
		p := &plan{}
		inst := installation{Method: method, Root: filepath.Join(e.cfg.Home, "native"), Path: filepath.Join(e.cfg.Home, ".local", "bin", "native"), Package: "demo"}
		if err := e.uninstallRecipe(p, inst); err != nil {
			t.Fatal(method, err)
		}
	}
	for _, inst := range []installation{{Method: "unknown"}, {Method: "native-codex"}, {Method: "native-codex", Path: "/foreign", Root: "/"}} {
		if e.uninstallRecipe(&plan{}, inst) == nil {
			t.Fatal("unsafe uninstall accepted")
		}
	}
	t.Setenv("PATH", t.TempDir())
	for _, method := range []string{"npm", "brew"} {
		if e.uninstallRecipe(&plan{}, installation{Method: method}) == nil {
			t.Fatal("missing manager accepted")
		}
	}
}
