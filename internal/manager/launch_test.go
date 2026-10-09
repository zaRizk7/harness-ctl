package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLaunchOptionsAndDirectShim(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	c, err := e.launchCommand(inst, []string{"hello"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Args, append(launchPolicy(s).DefaultArgs(), "hello")) {
		t.Fatal(c.Args)
	}
	for _, args := range [][]string{{"--help"}} {
		c, err = e.launchCommand(inst, args, false)
		if err != nil || !reflect.DeepEqual(c.Args, args) {
			t.Fatal("explicit options overridden", c, err)
		}
	}
	c, err = e.launchCommand(inst, []string{"hello"}, true)
	if err != nil || !reflect.DeepEqual(c.Args, []string{"hello"}) {
		t.Fatal(c, err)
	}
	if err = atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = e.writeShim(inst); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(e.cfg.BinDir, s.Command)
	got, err := exec.Command(shim, "hello").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != strings.Join(append(launchPolicy(s).DefaultArgs(), "hello"), "\n")+"\n" {
		t.Fatal(string(got))
	}
	got, err = exec.Command(shim, "--help").Output()
	if err != nil || string(got) != "--help\n" {
		t.Fatal(string(got), err)
	}
	if c.Env[s.HomeEnv] != inst.StateRoot {
		t.Fatal("launch state scope missing")
	}
	if _, err = os.Stat(shim); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchOverridesPreserveIndependentSecurityDefaults(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.writeShim(inst); err != nil {
		t.Fatal(err)
	}
	sandbox := []string{"--sandbox", "workspace-write"}
	approval := []string{"--ask-for-approval", "on-request"}
	reviewer := []string{"--config", "approvals_reviewer=\"auto_review\""}
	for _, tc := range []struct {
		name           string
		args, defaults []string
	}{
		{"model", []string{"-c", "model=\"example\""}, launchPolicy(s).DefaultArgs()},
		{"model-inline", []string{"--config=model=\"example\""}, launchPolicy(s).DefaultArgs()},
		{"sandbox", []string{"-s", "read-only"}, append(append([]string{}, approval...), reviewer...)},
		{"sandbox-config", []string{"-c", "sandbox_mode=\"read-only\""}, append(append([]string{}, approval...), reviewer...)},
		{"sandbox-config-spaces", []string{"-c", " sandbox_mode =\"read-only\""}, append(append([]string{}, approval...), reviewer...)},
		{"sandbox-attached", []string{"-sread-only"}, append(append([]string{}, approval...), reviewer...)},
		{"invalid-config", []string{"-c", "sandbox_mode"}, launchPolicy(s).DefaultArgs()},
		{"approval", []string{"--ask-for-approval=never"}, append(append([]string{}, sandbox...), reviewer...)},
		{"reviewer", []string{"-capprovals_reviewer=\"user\""}, append(append([]string{}, sandbox...), approval...)},
		{"terminator", []string{"--", "--sandbox", "--help"}, launchPolicy(s).DefaultArgs()},
		{"explicit-bypass", []string{"--yolo"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := append(append([]string{}, tc.defaults...), tc.args...)
			got := defaultLaunchArgs(s, tc.args, false)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
			data, err := exec.Command(filepath.Join(e.cfg.BinDir, s.Command), tc.args...).Output()
			if err != nil || string(data) != strings.Join(want, "\n")+"\n" {
				t.Fatalf("shim: %q, %v", data, err)
			}
		})
	}
}

func TestTrackedOpenCodeLaunchUsesNativeXDGRoots(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("opencode")
	inst := installation{Harness: "opencode", Path: filepath.Join(e.cfg.Home, "tools", "opencode")}
	c, err := e.launchCommand(inst, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Env["XDG_CONFIG_HOME"] != filepath.Join(e.cfg.Home, ".config") || c.Env["XDG_DATA_HOME"] != filepath.Join(e.cfg.Home, ".local", "share") {
		t.Fatal("tracked launch redirected native state", c.Env, s)
	}
}
