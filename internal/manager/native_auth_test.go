package manager

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAuthUsesApprovedSelectedInstallation(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "claude", InstallID: inst.ID, Action: "auth", Target: "login", Owners: s.SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || strings.Join(p.Steps[0].Args, " ") != "auth login" || p.Steps[0].Env["CLAUDE_CONFIG_DIR"] != inst.StateRoot {
		t.Fatalf("wrong native auth scope: %+v", p)
	}
	var out bytes.Buffer
	if err := e.cli(context.Background(), []string{"auth", "--preview", "login", "claude"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "auth login") || !strings.Contains(out.String(), "credential") {
		t.Fatal(out.String())
	}
}

func TestNativeAuthRoutingExecutionAndSafety(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" \"$CODEX_HOME\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, args := range [][]string{{"auth", "--preview", "status", "codex"}, {"auth", "status", "codex"}, {"auth", "route", "codex", "login", "status"}, {"codex", "login", "status"}} {
		out.Reset()
		if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
			t.Fatal(args, err)
		}
		if !strings.Contains(out.String(), "status") {
			t.Fatal(out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(e.cfg.Root, "accounts.age")); !os.IsNotExist(err) {
		t.Fatal("native status wrote vault")
	}
	owners := strings.Join(s.SharedClients, ",")
	out.Reset()
	if err := e.cli(context.Background(), []string{"auth", "--yes", "--owners", owners, "login", "codex", "--device-auth"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--device-auth") || !strings.Contains(out.String(), inst.StateRoot) {
		t.Fatal(out.String())
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "logout", Owners: s.SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	p.Steps[0].Args = []string{"unexpected"}
	if e.executeNativeAuth(context.Background(), p, p.ID, strings.NewReader(""), &out) == nil {
		t.Fatal("edited preview executed")
	}
	p, err = e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "logout", Owners: s.SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	if e.execute(context.Background(), p, p.ID, nil) == nil {
		t.Fatal("native mutation without streams")
	}
	// Failures are sanitized before being journaled and captured local files roll back.
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\necho secret-token\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err = e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "logout", Owners: s.SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.executeNativeAuth(context.Background(), p, p.ID, strings.NewReader(""), &out); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"auth"}, {"auth", "--bad"}, {"auth", "status", "missing"}, {"auth", "route", "codex", "other"}, {"auth", "login", "codex"}, {"auth", "--preview", "logout", "pi"}, {"auth", "status", "pi"}, {"auth", "--install-id", "missing", "status", "codex"}} {
		if err := e.cli(context.Background(), args, strings.NewReader("cancel\n"), io.Discard); err == nil {
			t.Fatal("invalid native request", args)
		}
	}
}

func TestNativeAuthShimDelegatesBeforeScopedEnvironment(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	manager := filepath.Join(e.cfg.Home, "harness-ctl")
	if err := atomicWrite(manager, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	t.Cleanup(func() { executablePath = old })
	executablePath = func() (string, error) { return manager, nil }
	if err := e.writeShim(inst); err != nil {
		t.Fatal(err)
	}
	data, err := exec.Command(filepath.Join(e.cfg.BinDir, s.Command), "auth", "login", "--console").Output()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"--root", e.cfg.Root, "auth", "--install-id", inst.ID, "route", "claude", "auth", "login", "--console"}
	if string(data) != strings.Join(expected, "\n")+"\n" {
		t.Fatal(string(data))
	}
	executablePath = func() (string, error) { return "", errors.New("synthetic executable error") }
	if e.writeShim(inst) == nil {
		t.Fatal("missing router executable")
	}
}

func TestNativeCredentialProfilesCaptureRestoreWithoutExposingSecrets(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(inst.StateRoot, "auth.json")
	original := []byte(`{"tokens":{"refresh_token":"native-secret"}}`)
	if err := atomicWrite(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	owners := strings.Join(s.SharedClients, ",")
	var out bytes.Buffer
	if err := e.cli(context.Background(), []string{"auth", "profiles", "capture", "--yes", "--owners", owners, "--profile", "work", "codex"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "native-secret") {
		t.Fatal("credential leaked in preview")
	}
	vaultData, err := os.ReadFile(filepath.Join(e.cfg.Root, "credentials.age"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(vaultData, []byte("native-secret")) {
		t.Fatal("unencrypted credential profile")
	}
	if err := atomicWrite(file, []byte(`{"tokens":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := e.cli(context.Background(), []string{"auth", "profiles", "apply", "--yes", "--owners", owners, "--profile", "work", "codex"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatal("credential restoration failed", err)
	}
	if strings.Contains(out.String(), "native-secret") {
		t.Fatal("credential leaked during restore")
	}
}

func TestPermanentDiscardAlsoErasesCapturedNativeCredentials(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(inst.StateRoot, "auth.json"), []byte("native-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	owners := strings.Join(s.SharedClients, ",")
	if err := e.cli(context.Background(), []string{"auth", "profiles", "capture", "--yes", "--owners", owners, "--profile", "work", "codex"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := e.cli(context.Background(), []string{"reset", "--permanent", "--preserve", "none", "--owners", owners, "--yes", "codex"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatal("permanent auth discard retained a credential copy", err)
	}
}

func TestNativeAuthCannotExecuteAnUnreadablePreview(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(inst.StateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(inst.StateRoot, "started")
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\n: > \"$CODEX_HOME/started\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	err := e.cli(context.Background(), []string{"auth", "--yes", "--owners", strings.Join(s.SharedClients, ","), "login", "codex"}, strings.NewReader(""), brokenOutput{})
	t.Log("preview boundary:", err)
	if err == nil {
		t.Fatal("unreadable preview accepted")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("native authentication started before a readable preview")
	}
}
