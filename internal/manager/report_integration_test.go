package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaRizk7/harness-ctl/internal/providers"
)

func TestSDKSubscriptionUsesApprovedSourceAndRefusesOldVersions(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "personal", Provider: "anthropic", Kind: "subscription", ReportHarness: "claude", Enabled: true}
	s, _ := e.specFor(a.ReportHarness)
	inst := syntheticInstall(t, e, s, "unknown")
	a.ReportInstallID = inst.ID
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
test "$CLAUDE_CONFIG_DIR" = "$EXPECTED_NATIVE_STATE" || exit 8
read -r request
printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{"claude_code_version":"2.1.296"}}}'
read -r request
case "$request" in *'"skip_behaviors":true'*) ;; *) exit 7 ;; esac
printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"usage","response":{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":12.5}}}}}'
read -r wait
`
	if err := atomicWrite(inst.Path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXPECTED_NATIVE_STATE", inst.StateRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := e.monitorAccount(ctx, a, time.Now())
	if len(m.Errors) > 0 || len(m.Limits) != 1 || !strings.Contains(m.Limits[0], "12.5%") {
		t.Fatal(m)
	}
	model := newModel(e)
	model.accountMetrics = []accountMetric{m}
	if !strings.Contains(strings.Join(model.monitorView(), "\n"), "12.5%") || !strings.Contains(strings.Join(metricLines(m), "\n"), "experimental") {
		t.Fatal(model.monitorView(), metricLines(m))
	}
	for _, version := range []string{"unknown", "2.1.295", "2.1.296-beta"} {
		outdated := strings.ReplaceAll(script, `"claude_code_version":"2.1.296"`, `"claude_code_version":"`+version+`"`)
		if err := atomicWrite(inst.Path, []byte(outdated), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := e.nativeSubscriptionReport(ctx, a); err == nil || !strings.Contains(err.Error(), "initialization") {
			t.Fatal(version, err)
		}
	}
	if _, err := os.Stat(filepath.Join(e.cfg.Root, "accounts.age")); !os.IsNotExist(err) {
		t.Fatal("report wrote credentials", err)
	}
}

func TestNativeSubscriptionSourceUsesSelectedSyntheticInstallation(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "personal", Provider: "openai", Kind: "subscription", Enabled: true, ReportHarness: "codex"}
	if _, err := e.nativeSubscriptionReport(context.Background(), a); err == nil {
		t.Fatal("missing report source")
	}
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nread -r request\nprintf '%s\\n' '{\"id\":1,\"result\":{}}'\nread -r notification\nread -r request\nprintf '%s\\n' '{\"id\":2,\"result\":{\"rateLimitsByLimitId\":{\"subscription\":{\"primary\":{\"usedPercent\":12.5,\"windowDurationMins\":300}}}}}'\nread -r wait\n"
	if err := atomicWrite(inst.Path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a.ReportInstallID = inst.ID
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	m := e.monitorAccount(ctx, a, time.Now())
	if len(m.Errors) > 0 || len(m.Limits) != 1 || !strings.Contains(m.Limits[0], "12.5%") {
		t.Fatal(m)
	}
	a.Kind = "api"
	if e.validateAccount(a) == nil {
		t.Fatal("API record used subscription source")
	}
	a.Kind = "subscription"
	a.ReportHarness = "missing"
	if e.validateAccount(a) == nil {
		t.Fatal("unknown report harness")
	}
	a.ReportHarness = ""
	a.ReportInstallID = ""
	a.BillingExport = &providers.BillingExport{Project: "../invalid"}
	if err := e.validateAccount(a); err == nil || !strings.Contains(err.Error(), "billing export") {
		t.Fatal("invalid export did not reach export validation", err)
	}
	a.BillingExport = nil
	a.ReportHarness = "codex"
	oldBin := e.cfg.BinDir
	e.cfg.BinDir = filepath.Dir(inst.Path)
	if _, err := e.nativeSubscriptionReport(ctx, a); err == nil {
		t.Fatal("recursive reporting launcher")
	}
	e.cfg.BinDir = oldBin
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := e.nativeSubscriptionReport(cancelled, a); err == nil {
		t.Fatal("cancelled reporting continued")
	}
	old := validateOwnedPath
	t.Cleanup(func() { validateOwnedPath = old })
	validateOwnedPath = func(string, string) error { return errors.New("synthetic registry boundary") }
	if _, err := e.nativeSubscriptionReport(ctx, a); err == nil {
		t.Fatal("invalid registry bypassed")
	}
}

func TestGlobalMonitorLabelsCachedAndScopedCosts(t *testing.T) {
	e, _ := testEngine(t)
	now := time.Now()
	cost := 2.0
	m := newModel(e)
	m.accountMetrics = []accountMetric{{ID: "cloud", CostUSD: &cost, CostScope: "project export", DataThrough: &now, NextFetch: &now, Cached: true, Fetched: now}}
	lines := strings.Join(m.monitorView(), "\n")
	if !strings.Contains(lines, "cached") {
		t.Fatal(lines)
	}
	lines = strings.Join(metricLines(m.accountMetrics[0]), "\n")
	for _, value := range []string{"Cost scope", "Next provider fetch", "Latest matching billing export"} {
		if !strings.Contains(lines, value) {
			t.Fatal(lines)
		}
	}
}

func TestNativeSubscriptionRejectsUnrelatedHarnessSources(t *testing.T) {
	e, _ := testEngine(t)
	for _, a := range []account{
		{ID: "source", Provider: "openai", Kind: "subscription", ReportHarness: "pi"},
		{ID: "source", Provider: "google", Kind: "subscription", ReportHarness: "codex"},
		{ID: "source", Provider: "openai", Kind: "subscription", ReportInstallID: "orphan"},
	} {
		if err := e.validateAccount(a); err == nil {
			t.Fatal("unrelated native source accepted", a)
		}
	}
}
