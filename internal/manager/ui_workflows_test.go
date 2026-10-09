package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// completeUIOperation exercises the asynchronous command/message boundary.
func completeUIOperation(t *testing.T, m tuiModel, cmd tea.Cmd) tuiModel {
	t.Helper()
	for i := 0; m.screen == "busy" && i < 64; i++ {
		if cmd == nil {
			t.Fatal("busy operation has no event receiver")
		}
		messages := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { messages <- c() }(cmd)
		var msg tea.Msg
		select {
		case msg = <-messages:
		case <-time.After(5 * time.Second):
			t.Fatal("operation did not finish")
		}
		if done, ok := msg.(doneMsg); ok && done.err != nil {
			t.Fatal(done.err)
		}
		model, next := m.Update(msg)
		m, cmd = model.(tuiModel), next
	}
	if m.screen != "result" {
		t.Fatal(m.screen)
	}
	return m
}

func TestTUIRecoveryRestorePurgeAndInterruptedJournal(t *testing.T) {
	e, _ := testEngine(t)
	p := resetPlan(t, e)
	path := p.Resources[0].Path
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	model, _ := m.Update(m.loadRecovery()())
	m = model.(tuiModel)
	if len(m.backups) != 1 || len(m.records) != 0 {
		t.Fatal(m.backups, m.records)
	}
	model, cmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "restore-preview" {
		t.Fatal(m.screen)
	}
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("restore skipped approval")
	}
	m.typed = "restore"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatal(string(data), err)
	}
	m.screen, m.typed = "purge-preview", ""
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("purge skipped approval")
	}
	m.typed = "purge"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	if snapshots, err := e.snapshots(); err != nil || len(snapshots) != 0 {
		t.Fatal(snapshots, err)
	}
	record := operationRecord{ID: randomID(), Harness: "pi", Action: "reset", Status: "preparing", Started: time.Now()}
	if err := e.saveRecord(record); err != nil {
		t.Fatal(err)
	}
	model, _ = m.Update(m.loadRecovery()())
	m = model.(tuiModel)
	model, _ = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if m.screen != "journal-preview" {
		t.Fatal(m.screen)
	}
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("journal recovery skipped approval")
	}
	m.typed = "recover"
	model, cmd = m.confirm()
	completeUIOperation(t, model.(tuiModel), cmd)
	records, err := e.records()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if r.ID == record.ID && r.Status != "rolled-back" {
			t.Fatal(r)
		}
	}
}

func TestTUIProfileCreationAndBaseLaunch(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(inst.StateRoot, "settings.json"), map[string]any{"ordinary": true}); err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	m.installs = e.reg.Installs
	for i, spec := range e.cfg.Harnesses {
		if spec.ID == "pi" {
			m.harness, m.cursor = i, i
		}
	}
	model, _ := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	m.cursor = actionIndex(m, "profile")
	model, profileCmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.Update(profileCmd())
	m = model.(tuiModel)
	if m.screen != "profile" {
		t.Fatal(m.screen)
	}
	model, _ = m.key(keyMessage("space"))
	m = model.(tuiModel)
	model, cmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "preview" {
		t.Fatal(m.screen, m.status)
	}
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("profile skipped approval")
	}
	m.typed = "profile"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	prof, ok := e.reg.Profiles[inst.ID]
	if !ok {
		t.Fatal("profile not activated")
	}
	m.screen = "profile"
	model, cmd = m.key(keyMessage("b"))
	completeUIOperation(t, model.(tuiModel), cmd)
	if _, ok = e.reg.Profiles[inst.ID]; ok {
		t.Fatal("profile remained active")
	}
	if _, err := os.Stat(prof.Root); err != nil {
		t.Fatal("base launch removed retained profile", err)
	}
}

func TestTUIAccountApprovalAndStaleMutation(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	m := newModel(e)
	m.screen = "accounts"
	model, _ := m.Update(m.loadAccountsView()())
	m = model.(tuiModel)
	model, _ = m.key(keyMessage("d"))
	m = model.(tuiModel)
	if _, cmd, _ := m.managementConfirm(); cmd != nil {
		t.Fatal("account mutation skipped approval")
	}
	a.Label = "Concurrent edit"
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	m.typed = "apply"
	model, cmd, _ := m.managementConfirm()
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if !strings.Contains(m.status, "changed") || len(m.accountItems) != 1 {
		t.Fatal(m.status, m.accountItems)
	}
	items, err := e.loadAccounts()
	if err != nil || !items[0].Enabled {
		t.Fatal(items, err)
	}
	model, _ = m.Update(m.loadAccountsView()())
	m = model.(tuiModel)
	for _, key := range []string{"d", "u", "x"} {
		model, _ = m.key(keyMessage(key))
		m = model.(tuiModel)
		m.typed = "apply"
		model, cmd, _ = m.managementConfirm()
		m = model.(tuiModel)
		model, _ = m.Update(cmd())
		m = model.(tuiModel)
		if m.status != "" || m.screen != "accounts" {
			t.Fatal(m.screen, m.status)
		}
	}
	if len(m.accountItems) != 0 {
		t.Fatal(m.accountItems)
	}
}
