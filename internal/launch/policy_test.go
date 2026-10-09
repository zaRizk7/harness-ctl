package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIndependentPolicyAndShellParity(t *testing.T) {
	p := Policy{Rules: []Rule{{Args: []string{"--sandbox", "workspace"}, Flags: []string{"--sandbox", "-s"}, ConfigKeys: []string{"sandbox"}}, {Args: []string{"--approval", "auto"}, Flags: []string{"--approval", "-a"}, ConfigKeys: []string{"approval"}}}, ConfigFlags: []string{"--config", "-c"}}
	if !slices.Equal(p.DefaultArgs(), []string{"--sandbox", "workspace", "--approval", "auto"}) {
		t.Fatal(p.DefaultArgs())
	}
	path := filepath.Join(t.TempDir(), "launcher")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+p.Shell("native limitation")+"printf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"-c", "model=example"}, {"--config=sandbox=read-only"}, {"-capproval=manual"}, {"-c", " approval =manual"}, {"-sread-only"}, {"-a=manual"}, {"--sandbox=read-only"}, {"--config", "sandbox"}, {"--", "--help"}, {"--help"}, {"-c"}} {
		cmd := exec.Command(path, args...)
		cmd.Env = append(os.Environ(), "HARNESS_CTL_NATIVE=0")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		want := p.Args(args, false)
		text := strings.Join(want, "\n") + "\n"
		if string(out) != text {
			t.Fatalf("%q: shell %q != direct %q", args, out, text)
		}
	}
	if !slices.Equal(p.Args([]string{"hello"}, true), []string{"hello"}) {
		t.Fatal("native launch changed")
	}
	legacy := Policy{LegacyArgs: []string{"--mode", "safe"}, LegacyFlags: []string{"--mode", "-c", "--config"}}
	if !slices.Equal(legacy.Args([]string{"-c", "model=x"}, false), []string{"--mode", "safe", "-c", "model=x"}) {
		t.Fatal("legacy config dropped security")
	}
	if !slices.Equal(legacy.Args([]string{"--mode", "manual"}, false), []string{"--mode", "manual"}) {
		t.Fatal("legacy override lost")
	}
	_ = legacy.Shell("")
	if Quote("a'b") != "'a'\"'\"'b'" {
		t.Fatal("unsafe quote")
	}
}
