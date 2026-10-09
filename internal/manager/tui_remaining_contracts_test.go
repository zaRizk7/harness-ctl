package manager

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestMonitorOverlayPreservesNavigationAndReportsErrors(t *testing.T) {
	m := componentModel(t)
	m.screen = "components"
	model, cmd := m.Update(globalMonitorMsg{err: errors.New("report failure")})
	m = model.(tuiModel)
	if cmd == nil || !strings.Contains(strings.Join(m.monitorView(), "\n"), "report failure") {
		t.Fatal("monitor error lost")
	}
	m.monitorExpanded = true
	m.monitorCursor = 2
	for _, key := range []string{"up", "down", "x"} {
		model, _ = m.key(keyMessage(key))
		m = model.(tuiModel)
	}
	if m.monitorCursor != 2 || m.screen != "components" {
		t.Fatal("overlay changed underlying screen", m.monitorCursor, m.screen)
	}
	m.accountMetrics = nil
	if !strings.Contains(strings.Join(m.monitorView(), "\n"), "No enabled") {
		t.Fatal("empty overlay omitted capability")
	}
	metric := accountMetric{ID: "a", UsageAvailable: true, WindowStart: time.Now(), Requests: 3, InputTokens: 5, OutputTokens: 8}
	if !strings.Contains(strings.Join(metricLines(metric), "\n"), "3 requests") {
		t.Fatal("usage metrics omitted")
	}
	model, _, _ = m.managementUpdate(accountsMsg{})
	m = model.(tuiModel)
	if m.screen != "components" {
		t.Fatal("background account inventory stole screen")
	}
}

func TestHomeLifecycleAndBatchControlsReachApprovedExecution(t *testing.T) {
	m := componentModel(t)
	m.screen = "home"
	for _, key := range []string{"r", "d"} {
		model, cmd := m.key(keyMessage(key))
		if cmd == nil {
			t.Fatal(key)
		}
		updated, _ := model.(tuiModel).Update(cmd())
		m = updated.(tuiModel)
		m.screen = "home"
	}
	m.screen = "actions"
	m.cursor = actionIndex(m, "reset")
	model, cmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "options" || len(m.req.Preserve) != 0 {
		t.Fatal("reset preservation default wrong", m.screen, m.req)
	}
	model, _ = m.key(keyMessage("v"))
	m = model.(tuiModel)
	if !m.editing || !strings.Contains(m.View().Content, "Editing version") {
		t.Fatal("version editing inaccessible")
	}
	m.editing = false
	m.screen = "batch-select"
	m.batchSelected = map[string]bool{}
	model, _, _ = m.managementKey("enter")
	m = model.(tuiModel)
	if !strings.Contains(m.status, "Select at least") {
		t.Fatal(m.status)
	}
	m.screen = "batch-actions"
	m.cursor = 100
	if _, cmd, _ = m.managementKey("enter"); cmd != nil {
		t.Fatal("invalid batch action executed")
	}
	m.batchSelected = map[string]bool{m.e.cfg.Harnesses[m.harness].ID: true}
	m.cursor = 0
	for i, id := range m.batchActions() {
		if id == "reset" {
			m.cursor = i
		}
	}
	model, cmd, _ = m.managementKey("enter")
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.req.Action != "reset" || len(m.req.Preserve) != 0 {
		t.Fatal(m.req)
	}
	requests := m.batchRequests()
	b, err := m.e.buildBatch(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	m.screen = "preview"
	m.batchMode = true
	m.batch = b
	m.req.Permanent = true
	m.typed = "apply"
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("permanent approval accepted apply")
	}
	m.req.Permanent = false
	m.typed = "apply"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	if !strings.Contains(m.status, "verified") {
		t.Fatal(m.status)
	}
}

func TestSelfRemovalTUIUsesPreviewAndReportsExecutableFailures(t *testing.T) {
	m := componentModel(t)
	m.screen = "self-options"
	m.cursor = 0
	model, _, _ := m.managementKey("space")
	m = model.(tuiModel)
	if !m.selfRemoveHarnesses {
		t.Fatal("harness choice unavailable")
	}
	m.selfRemoveHarnesses = false
	old := executablePath
	defer func() { executablePath = old }()
	executablePath = func() (string, error) { return "", errors.New("executable failure") }
	_, cmd, _ := m.managementKey("enter")
	if msg := cmd().(selfPlanMsg); msg.err == nil {
		t.Fatal("executable failure lost")
	}
	path := filepath.Join(m.e.cfg.Home, "tools", "harness-ctl")
	_ = atomicWrite(path, []byte("fixture"), 0700)
	executablePath = func() (string, error) { return path, nil }
	m.screen = "self-options"
	model, cmd, _ = m.managementKey("enter")
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "self-preview" || m.self == nil {
		t.Fatal(m.screen, m.status)
	}
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("manager removal skipped typed approval")
	}
	m.typed = "uninstall"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("approved binary retained", err)
	}
	m.screen = "self-preview"
	m.self = nil
	if lines, _ := m.managementView(); lines != nil {
		t.Fatal(lines)
	}
}

func TestManagementEditorsAndAccountOpenFailure(t *testing.T) {
	m := componentModel(t)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "/usr/bin/true")
	t.Setenv("VISUAL", "")
	_ = m.e.saveAccount(account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true})
	_ = m.e.saveLibraryItem(reusableSkill("demo"), "")
	for _, screen := range []string{"accounts", "library"} {
		m.screen = screen
		m.cursor = 0
		if screen == "accounts" {
			model, _ := m.Update(m.loadAccountsView()())
			m = model.(tuiModel)
		} else {
			model, _ := m.Update(m.loadLibraryView()())
			m = model.(tuiModel)
		}
		for _, key := range []string{"a", "e"} {
			model, cmd := m.key(keyMessage(key))
			if cmd == nil {
				t.Fatal(screen, key)
			}
			p := tea.NewProgram(editorProgramModel{model.(tuiModel), cmd}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
			if _, err := p.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	m.screen = "accounts"
	m.accountItems = nil
	if _, cmd, _ := m.managementKey("e"); cmd != nil {
		t.Fatal("empty account edit")
	}
	m.accountItems = []accountView{{ID: "a"}}
	m.e.run = commandRunner(func(context.Context, command) (string, error) { return "", errors.New("open failure") })
	_, cmd, _ := m.managementKey("b")
	if msg := cmd().(accountsMsg); msg.err == nil {
		t.Fatal("page open failure lost")
	}
	if _, cmd, _ := m.managementKey("unknown"); cmd != nil {
		t.Fatal("unknown account key executed")
	}
	m.screen = "account-preview"
	m.accountAction = "save"
	m.accountEdit = accountEditMsg{item: account{ID: "a", Provider: "openai", Kind: "subscription", Credential: "hidden"}}
	if view := m.View().Content; !strings.Contains(view, "Provider: openai") || strings.Contains(view, "hidden") {
		t.Fatal(view)
	}
}

func TestComponentOwnersDirectoryErrorsAndPreviewLabels(t *testing.T) {
	m := componentModel(t)
	inst := m.componentInstallation()
	m.e.cfg.StateRoots[inst.Harness] = inst.StateRoot
	s := m.e.cfg.Harnesses[m.harness]
	s.SharedClients = []string{"shared", "second"}
	m.e.cfg.Harnesses[m.harness] = s
	inst.Managed = false
	m.installs = []installation{inst}
	m.componentCat = mcp
	msg := m.loadComponents()().(componentsMsg)
	if msg.err != nil || len(msg.owners) < 2 {
		t.Fatal(msg)
	}
	m.owners = []string{"shared", "second"}
	m.req.Owners = []string{"shared", "second"}
	m.screen = "component-owners"
	m.cursor = 0
	model, _ := m.componentKey("space")
	m = model.(tuiModel)
	if len(m.req.Owners) != 1 || m.req.Owners[0] != "second" {
		t.Fatal(m.req.Owners)
	}
	asset := filepath.Join(inst.StateRoot, "skills", "demo")
	_ = atomicWrite(filepath.Join(asset, "SKILL.md"), []byte("fixture"), 0600)
	m.screen = "components"
	m.componentItems = []componentItem{{Path: asset, Directory: true}}
	m.cursor = 0
	oldIO, oldValidate := fileIO, validateOwnedPath
	defer func() { fileIO, validateOwnedPath = oldIO, oldValidate }()
	fault := errors.New("directory boundary")
	validateOwnedPath = func(string, string) error { return fault }
	model, _ = m.componentKey("enter")
	m = model.(tuiModel)
	if m.status != fault.Error() {
		t.Fatal(m.status)
	}
	validateOwnedPath = oldValidate
	count := 0
	fileIO.walk = func(path string, fn fs.WalkDirFunc) error {
		count++
		if count == 1 {
			return oldIO.walk(path, fn)
		}
		return fn(path, nil, fault)
	}
	model, _ = m.componentKey("enter")
	m = model.(tuiModel)
	if m.status != fault.Error() {
		t.Fatal(m.status)
	}
	fileIO = oldIO
	m.componentItems = []componentItem{{Name: "opaque", Category: plugins}}
	if !strings.Contains(strings.Join(m.componentView(), "\n"), "unverified local asset") {
		t.Fatal("missing compatibility label")
	}
	inst.ServicePaths = []string{"/service"}
	m.p = &plan{Component: &componentMutation{Native: true, Request: componentRequest{Category: marketplaces}, Writes: []componentWrite{{Path: "/removed", Remove: true}, {Path: "/imported", Source: "/source", SourceDigest: "digest"}}}, Install: inst, Resources: []resource{{Path: "/native", Category: plugins}}}
	if lines := strings.Join(m.previewLines(), "\n"); !strings.Contains(lines, "REMOVE") || !strings.Contains(lines, "IMPORT") || !strings.Contains(lines, "NATIVE CHANGE") {
		t.Fatal(lines)
	}
}
