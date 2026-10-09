package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTUICommonNavigationAndApprovalEditing(t *testing.T) {
	m := componentModel(t)
	for _, screen := range []string{"home", "actions", "options", "components", "component-owners", "preview", "restore-preview", "profile-preview", "result"} {
		m.screen = screen
		m.cursor = 1
		m.typed = "abc"
		model, _ := m.key(keyMessage("up"))
		m = model.(tuiModel)
		if m.cursor != 0 {
			t.Fatal(screen, "up did not move cursor")
		}
		model, _ = m.key(keyMessage("down"))
		m = model.(tuiModel)
		model, cmd := m.key(keyMessage("ctrl+c"))
		m = model.(tuiModel)
		if cmd == nil {
			t.Fatal(screen, "quit command missing")
		}
		if strings.Contains(screen, "preview") {
			model, _ = m.key(keyMessage("backspace"))
			m = model.(tuiModel)
			if m.typed != "ab" {
				t.Fatal("approval backspace failed")
			}
			model, _ = m.key(keyPress("\x01z"))
			m = model.(tuiModel)
			if m.typed != "abz" {
				t.Fatal("approval input not sanitized", m.typed)
			}
		}
		model, _ = m.key(keyMessage("esc"))
		m = model.(tuiModel)
		want := "home"
		if screen == "components" {
			want = "component-groups"
		}
		if screen == "component-owners" {
			want = "components"
		}
		if m.screen != want {
			t.Fatal(screen, m.screen)
		}
	}
	m.screen = "options"
	m.editing = true
	m.req.Target = "1.2"
	model, _ := m.key(keyMessage("backspace"))
	m = model.(tuiModel)
	model, _ = m.key(keyPress("3;$"))
	m = model.(tuiModel)
	if m.req.Target != "1.3" {
		t.Fatal("version editor accepted shell text", m.req.Target)
	}
	model, _ = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if m.editing {
		t.Fatal("version editing did not finish")
	}
	m.screen = "loading"
	if _, cmd := m.key(keyMessage("enter")); cmd != nil {
		t.Fatal("loading accepted operation")
	}
	if _, cmd := m.key(keyMessage("ctrl+c")); cmd == nil {
		t.Fatal("loading did not quit")
	}
	m.screen = "busy"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.cancel = cancel
	model, _ = m.key(keyMessage("ctrl+c"))
	m = model.(tuiModel)
	if ctx.Err() == nil || !strings.Contains(m.status, "rollback") {
		t.Fatal("busy cancellation skipped rollback")
	}
	m.screen = "home"
	if _, cmd := m.Update(interruptMsg{}); cmd == nil {
		t.Fatal("idle interrupt did not quit")
	}
}

func TestTUIOwnerApprovalTogglesAndProfileBounds(t *testing.T) {
	m := componentModel(t)
	m.screen = "restore-preview"
	m.owners = []string{"other", "second"}
	m.req.Owners = []string{"other", "second"}
	model, _ := m.key(keyMessage("space"))
	m = model.(tuiModel)
	if len(m.req.Owners) != 1 || m.req.Owners[0] != "second" {
		t.Fatal(m.req.Owners)
	}
	model, _ = m.key(keyMessage("space"))
	m = model.(tuiModel)
	if len(m.req.Owners) != 2 {
		t.Fatal(m.req.Owners)
	}
	m.screen = "profile"
	m.optionCategories = []category{skills}
	m.cursor = 1
	if _, cmd := m.key(keyMessage("space")); cmd != nil {
		t.Fatal("out-of-range category mutated")
	}
	m.screen = "actions"
	m.cursor = 100
	if _, cmd := m.key(keyMessage("enter")); cmd != nil {
		t.Fatal("out-of-range action executed")
	}
	model, _ = m.key(keyMessage("tab"))
	m = model.(tuiModel)
	if m.installCursor != 1 || m.cursor != 0 {
		t.Fatal("installation cycle failed")
	}
	m.screen = "result"
	model, _ = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if m.screen != "home" {
		t.Fatal("result did not return home")
	}
	m.screen = "preview"
	m.typed = "apply"
	m.p = nil
	if _, cmd := m.confirm(); cmd != nil {
		t.Fatal("nil preview executed")
	}
	m.p = &plan{Blockers: []string{"blocked"}}
	if _, cmd := m.confirm(); cmd != nil {
		t.Fatal("blocked preview executed")
	}
	model, _ = m.Update(snapshotMsg{err: errors.New("fixture")})
	m = model.(tuiModel)
	if m.screen != "result" || m.status != "fixture" {
		t.Fatal(m.screen, m.status)
	}
	model, _ = m.Update(snapshotMsg{meta: snapshotMeta{Harness: "pi", Items: []snapshotItem{{Owners: []string{"pi", "other", "other"}}}}})
	m = model.(tuiModel)
	if m.screen != "restore-preview" || len(m.owners) != 1 {
		t.Fatal(m.screen, m.owners)
	}
}

func TestTUIEveryScreenRetainsMonitoringAndHidesValues(t *testing.T) {
	m := componentModel(t)
	inst := m.componentInstallation()
	m.req.Preserve = keepAll()
	m.optionCategories = []category{skills}
	m.owners = []string{"shared"}
	m.profileDisabled = map[category]bool{skills: true}
	m.accountItems = []accountView{{ID: "account", Provider: "openai", Kind: "api", Enabled: true}}
	cost := 2.5
	credit := 3.0
	m.accountMetrics = []accountMetric{{ID: "account", Provider: "openai", Kind: "api", CostUSD: &cost, CreditUsage: &credit, CreditUnit: "credits", Limits: []string{"limit 10"}, Errors: []string{"report error"}}}
	m.componentItems = []componentItem{{Name: "native", Category: plugins, Native: true, BuiltFor: []string{"pi"}}, {Name: "disabled", Disabled: true, Category: plugins}}
	m.componentCat = plugins
	m.selectedBackup = snapshotMeta{ID: randomID(), Harness: "pi", Items: []snapshotItem{{Path: "/owned", Absent: true}}}
	m.selectedRecord = operationRecord{ID: randomID(), Harness: "pi", Action: "reset"}
	m.backups = []snapshotMeta{m.selectedBackup}
	m.records = []operationRecord{m.selectedRecord}
	m.p = &plan{Spec: m.e.cfg.Harnesses[m.harness], Install: inst, Request: request{Action: "profile", Disabled: map[category]bool{skills: true}}, StateRoot: inst.StateRoot, Destination: inst.Root, Integrity: "fixture", Warnings: []string{"notice"}, Blockers: []string{"block"}, DependencyCommands: []command{{Path: "/tool", Description: "dependency", Env: map[string]string{"HOME": "/private"}}}, Resources: []resource{{Path: "/owned", Category: skills}}}
	m.self = &selfPlan{Binary: "/manager", Receipt: "/receipt", Links: map[string]string{"/link": "digest"}, Paths: []string{"/metadata"}, Batch: &batchPlan{Plans: []*plan{m.p}}}
	m.typed = "apply"
	m.status = "status"
	for _, screen := range []string{"home", "actions", "options", "preview", "busy", "result", "loading", "recovery", "restore-preview", "purge-preview", "journal-preview", "profile", "component-groups", "component-owners", "components", "batch-select", "batch-actions", "self-options", "self-preview", "accounts-loading", "account-preview", "accounts", "library", "library-select", "library-owners", "library-record-preview"} {
		t.Run(screen, func(t *testing.T) {
			m.screen = screen
			m.cursor = 0
			view := m.View().Content
			if !strings.Contains(view, "Monitor") || strings.Contains(view, "synthetic-secret") {
				t.Fatal("monitor missing or secret visible", view)
			}
			if m.itemCount() < 0 {
				t.Fatal("negative navigation count")
			}
		})
	}
	m.screen = "preview"
	m.req.Action = "profile"
	m.req.Permanent = true
	m.batchMode = true
	m.batch = &batchPlan{Plans: []*plan{m.p}}
	if view := m.View().Content; !strings.Contains(view, "discard") {
		t.Fatal(view)
	}
	m.monitorExpanded = true
	m.monitorError = "unavailable"
	if view := m.View().Content; !strings.Contains(view, "Account reports") {
		t.Fatal(view)
	}
	m.height = 1
	m.width = 0
	if got := m.terminalContent([]string{"body"}, []string{"one", "two"}); got != "two" {
		t.Fatal(got)
	}
}

func TestAccountEditorsAndMessagesRequireApproval(t *testing.T) {
	m := componentModel(t)
	for _, scenario := range []string{"cancelled", "error", "missing", "large", "invalid", "trailing", "invalid-account", "valid"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "request")
			data := []byte(`{"id":"a","provider":"openai","kind":"subscription"}`)
			var err error
			before := []byte("original")
			switch scenario {
			case "cancelled":
				before = data
			case "error":
				err = errors.New("fixture")
			case "large":
				data = make([]byte, m.e.cfg.MetadataBytes+1)
			case "invalid":
				data = []byte("bad")
			case "trailing":
				data = append(data, []byte(" {}")...)
			case "invalid-account":
				data = []byte(`{"id":"../bad"}`)
			}
			if scenario != "missing" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			msg := m.finishAccountEdit(path, before, "before", err)
			if (msg.err == nil) != (scenario == "valid" || scenario == "cancelled") {
				t.Fatal(scenario, msg.err)
			}
			model, _ := m.Update(msg)
			result := model.(tuiModel)
			if scenario == "valid" && result.screen != "account-preview" {
				t.Fatal("editor skipped preview")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("private request leaked")
			}
		})
	}
}
