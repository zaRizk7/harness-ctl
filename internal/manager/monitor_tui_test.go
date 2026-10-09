package manager

import (
	"strings"
	"testing"
	"time"
)

func TestGlobalMonitoringDoesNotChangePreviewFingerprint(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.screen = "account-preview"
	m.accountBefore = "approved-before"
	m.typed = "ap"
	cost := 2.5
	model, cmd := m.Update(globalMonitorMsg{metrics: []accountMetric{{ID: "api", Provider: "openai", CostUSD: &cost, Fetched: time.Now()}}})
	m = model.(tuiModel)
	if cmd == nil || m.accountBefore != "approved-before" || m.screen != "account-preview" || m.typed != "ap" {
		t.Fatal(m)
	}
	if !strings.Contains(m.View().Content, "2.5000") {
		t.Fatal(m.View().Content)
	}
	model, _ = m.key(keyMessage("f2"))
	m = model.(tuiModel)
	if !strings.Contains(m.View().Content, "Fetched:") {
		t.Fatal("details toggle absent")
	}
	model, _ = m.key(keyMessage("f2"))
	m = model.(tuiModel)
	if m.screen != "account-preview" || m.typed != "ap" {
		t.Fatal("details changed approval state")
	}
}

func TestMonitorFooterRemainsVisibleInSmallTerminal(t *testing.T) {
	e, _ := testEngine(t)
	m := newModel(e)
	m.width = 48
	m.height = 10
	m.screen = "home"
	m.accountMetrics = []accountMetric{{ID: "team", Limits: []string{"requests: 1000", "tokens: 10000"}}}
	view := m.View().Content
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("monitor clipped below terminal: %d rows for %d-row terminal", len(lines), m.height)
	}
	if !strings.Contains(view, "Monitor 5s") || !strings.Contains(view, "team") {
		t.Fatal("monitor absent", view)
	}
}
