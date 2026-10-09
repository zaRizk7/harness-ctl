package manager

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/credentials"
)

func TestNativeAuthTUIContracts(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(inst.Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	m.installs = []installation{inst}
	m.screen = "actions"
	m.cursor = actionIndex(m, "auth")
	model, _ := m.Update(keyMessage("enter"))
	m = model.(tuiModel)
	if m.screen != "native-auth" || !strings.Contains(m.View().Content, "authentication") {
		t.Fatal(m.View().Content)
	}
	if m.itemCount() != len(m.authOperations()) {
		t.Fatal("auth navigation")
	}
	for _, key := range []string{"tab", "o", "space", "space", "enter"} {
		model, cmd := m.Update(keyMessage(key))
		if key == "o" && cmd != nil {
			model, _ = model.Update(cmd())
		}
		m = model.(tuiModel)
	}
	m.cursor = slices.Index(m.authOperations(), "status")
	model, cmd := m.Update(keyMessage("enter"))
	if cmd == nil || model.(tuiModel).screen != "native-auth-running" {
		t.Fatal("native status not dispatched")
	}
	_ = model.(tuiModel).View()
	m.screen = "native-auth"
	m.cursor = slices.Index(m.authOperations(), "login")
	model, cmd = m.Update(keyMessage("enter"))
	if cmd == nil {
		t.Fatal("login preview missing")
	}
	_ = cmd()
	for _, err := range []error{nil, errors.New("synthetic auth")} {
		model, _ = m.Update(nativeAuthFinished(err))
		m = model.(tuiModel)
	}
	m.monitorExpanded = true
	if _, _, handled := m.nativeAuthUpdate(keyMessage("enter")); handled {
		t.Fatal("monitor overlay triggered auth")
	}
	m.monitorExpanded = false
	m.screen = "native-auth"
	model, cmd = m.Update(keyMessage("p"))
	if cmd == nil {
		t.Fatal("missing profile inventory")
	}
	model, _ = model.Update(cmd())
	m = model.(tuiModel)
	_ = m.View()
	if m.itemCount() != 0 {
		t.Fatal("unexpected profiles")
	}
	model, _ = m.Update(keyMessage("enter"))
	m = model.(tuiModel)
	m.credentialProfiles = []credentials.View{{ID: "work", Harness: "codex"}}
	for _, key := range []string{"enter", "x", "c"} {
		m.screen = "native-auth-profiles"
		m.cursor = 0
		model, cmd = m.Update(keyMessage(key))
		if cmd == nil || model.(tuiModel).req.Action != "credentials" {
			t.Fatal(key)
		}
	}
	m.screen = "native-auth-profiles"
	model, cmd = m.Update(keyMessage("o"))
	if cmd != nil {
		model, _ = model.Update(cmd())
	}
	m = model.(tuiModel)
	_ = m.View()
	if m.itemCount() != len(m.owners) {
		t.Fatal("owner count")
	}
	m.screen = "native-auth-owners"
	m.owners = nil
	_, _ = m.Update(keyMessage("space"))
	_, _ = m.Update(keyMessage("esc"))
	_, _ = m.Update(keyMessage("down"))
	_, _ = m.Update(nativeCredentialMsg{err: errors.New("synthetic vault")})
	m.screen = "native-auth"
	model, cmd = m.Update(keyMessage("c"))
	if cmd == nil || model.(tuiModel).req.CredentialID == "" {
		t.Fatal("capture unavailable")
	}
	// Terminal streams and both execution routes are testable independently of UI.
	c := command{Path: inst.Path, Args: []string{"login", "status"}}
	terminal := &nativeAuthTerminal{e: e, command: &c}
	var out bytes.Buffer
	terminal.SetStdin(strings.NewReader(""))
	terminal.SetStdout(io.Discard)
	terminal.SetStderr(&out)
	if err := terminal.Run(); err != nil || !strings.Contains(out.String(), "status") {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "codex", Action: "auth", Target: "login", Owners: s.SharedClients, Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	terminal.command = nil
	terminal.p = p
	if err := terminal.Run(); err != nil {
		t.Fatal(err)
	}
	m.screen, m.p, m.req, m.typed = "preview", p, p.Request, "apply"
	model, cmd = m.confirm()
	if cmd == nil || model.(tuiModel).screen != "native-auth-running" {
		t.Fatal("approved terminal command not dispatched")
	}
	// A stale/recursive selected executable cannot reach native status.
	m.screen = "native-auth"
	m.cursor = slices.Index(m.authOperations(), "status")
	m.installs[0].Path = filepath.Join(e.cfg.BinDir, "codex")
	_, cmd = m.Update(keyMessage("enter"))
	if cmd == nil {
		t.Fatal("missing failure message")
	}
	if cmd().(nativeAuthDone).err == nil {
		t.Fatal("unsafe native status")
	}
}

func TestNativeAuthProviderArgumentTUI(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("hermes")
	inst := syntheticInstall(t, e, s, "1")
	m := newModel(e)
	m.harness = slices.IndexFunc(e.cfg.Harnesses, func(s harnessSpec) bool { return s.ID == "hermes" })
	m.installs = []installation{inst}
	m.screen = "native-auth"
	m.cursor = slices.Index(m.authOperations(), "login")
	model, _ := m.Update(keyMessage("enter"))
	m = model.(tuiModel)
	if m.screen != "native-auth-argument" || !strings.Contains(m.View().Content, "provider ID") {
		t.Fatal(m.View().Content)
	}
	_, _ = m.Update(keyMessage("enter"))
	for _, ch := range "openrouter" {
		model, _ = m.Update(keyPress(string(ch)))
		m = model.(tuiModel)
	}
	model, _ = m.Update(keyMessage("backspace"))
	m = model.(tuiModel)
	model, _ = m.Update(keyPress("r"))
	m = model.(tuiModel)
	model, cmd := m.Update(keyMessage("enter"))
	if cmd == nil || !slices.Equal(model.(tuiModel).req.AuthArgs, []string{"openrouter"}) {
		t.Fatal("native argument lost")
	}
	m.screen = "native-auth-argument"
	_, _ = m.Update(keyMessage("esc"))
}
