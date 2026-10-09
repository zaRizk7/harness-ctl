package manager

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMainCLIConfigurationAndExitCodes(t *testing.T) {
	e, _ := testEngine(t)
	t.Setenv("HOME", e.cfg.Home)
	file := filepath.Join(e.cfg.Home, "config.json")
	_ = writeJSON(file, e.cfg)
	for _, args := range [][]string{{"version"}, {"--help"}, {"--config", file, "config"}, {"--config", file, "catalog"}, {"--config", file, "list"}, {"--config", file, "path"}, {"--root", filepath.Join(e.cfg.Home, "alternate"), "catalog"}} {
		var out, errout bytes.Buffer
		if status := mainWithIO(args, strings.NewReader(""), &out, &errout); status != 0 {
			t.Fatal(args, status, errout.String())
		}
	}
	for _, args := range [][]string{{"--bad-flag"}, {"--config", "missing", "config"}, {"--root", "relative", "list"}, {"--config", file, "unknown"}} {
		var out, errout bytes.Buffer
		if status := mainWithIO(args, strings.NewReader(""), &out, &errout); status == 0 {
			t.Fatal(args)
		}
	}
	old := hostOS
	hostOS = "linux"
	defer func() { hostOS = old }()
	var out, errout bytes.Buffer
	if status := mainWithIO([]string{"catalog"}, strings.NewReader(""), &out, &errout); status != 1 {
		t.Fatal(status)
	}
	hostOS = old
	t.Setenv("HOME", "")
	if status := mainWithIO([]string{"config"}, strings.NewReader(""), &out, &errout); status != 1 {
		t.Fatal(status)
	}
	if status := Main([]string{"version"}); status != 0 {
		t.Fatal(status)
	}
}

func TestCLIComponentApplyAndNativeLaunch(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	change := componentRequest{Operation: "add", Category: mcp, Path: filepath.Join(inst.StateRoot, "settings.json"), Field: "/mcpServers/demo", Value: []byte(`{"command":"fake-server"}`)}
	file := filepath.Join(e.cfg.Home, "request.json")
	_ = writeJSON(file, change)
	var out bytes.Buffer
	for _, args := range [][]string{{"components", "apply", "--preview", "pi", file}, {"components", "apply", "--yes", "pi", file}, {"components", "list", "pi", "mcp"}} {
		if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
			t.Fatal(args, err)
		}
	}
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := e.cli(context.Background(), []string{"pi", "--model", "example"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "--model\nexample\n" {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := e.cli(context.Background(), []string{"launch", "--native", "--install-id", inst.ID, "pi", "--", "--help"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"components", "unknown", "pi", "mcp"}, {"components", "apply", "pi", "missing"}, {"launch"}, {"launch", "missing"}, {"launch", "--unknown"}, {"launch", "--install-id", "missing", "pi"}} {
		if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err == nil {
			t.Fatal(args)
		}
	}
	if err := e.cli(context.Background(), nil, strings.NewReader(""), &out); err == nil {
		t.Fatal("empty command accepted")
	}
	if err := e.cli(context.Background(), []string{"install", "--install-id", "id", "pi", "codex"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("ambiguous install selector accepted")
	}
	if err := confirmCLI(brokenInput{}, &out, "id", false); err == nil {
		t.Fatal("unreadable approval accepted")
	}
	if err := outputJSON(brokenOutput{}, map[string]int{"x": 1}); err == nil {
		t.Fatal("output failure lost")
	}
	if err := e.cli(context.Background(), []string{"install", "--help"}, strings.NewReader(""), &out); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
}

type brokenInput struct{}

func (brokenInput) Read([]byte) (int, error) { return 0, errors.New("fixture read failure") }

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestNativeKeyStoreBoundaryUsesRootScopedAccount(t *testing.T) {
	k := newKeychainStore()
	accountID := "fixture-account"
	getCalls, setCalls, deleteCalls := 0, 0, 0
	k.get = func(service, id string) (string, error) {
		getCalls++
		if service != keychainService || id != accountID {
			t.Fatal(service, id)
		}
		return "fixture", nil
	}
	k.set = func(service, id, value string) error {
		setCalls++
		if service != keychainService || id != accountID || value != "fixture" {
			t.Fatal(service, id)
		}
		return nil
	}
	k.remove = func(service, id string) error {
		deleteCalls++
		if service != keychainService || id != accountID {
			t.Fatal(service, id)
		}
		return keyring.ErrNotFound
	}
	if value, err := k.Get(accountID); err != nil || value != "fixture" {
		t.Fatal(value, err)
	}
	if err := k.Set(accountID, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete(accountID); err != keyring.ErrNotFound {
		t.Fatal(err)
	}
	if getCalls != 1 || setCalls != 1 || deleteCalls != 1 {
		t.Fatal("native boundary not exercised")
	}
}
