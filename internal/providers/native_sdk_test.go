package providers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSDKSubscriptionReportKeepsNativeQuotaSeparateFromEstimates(t *testing.T) {
	r := testReader()
	r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) {
		return json.RawMessage(`{"session":{"total_cost_usd":99},"subscription_type":"max","rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":25,"resets_at":"2026-10-14T08:00:00Z"},"seven_day":{"utilization":0},"extra_usage":{"is_enabled":true,"monthly_limit":2000,"used_credits":150,"currency":"USD"}}}`), nil
	}
	m := r.monitorAccount(context.Background(), Account{ID: "personal", Provider: "anthropic", Kind: "subscription", Enabled: true, ReportHarness: "claude"}, time.Now())
	lines := strings.Join(append(m.Limits, m.Notes...), "\n")
	for _, want := range []string{"five_hour: 25.0% used", "seven_day: 0.0% used", "Plan: max", "Usage credits spent: 150 USD minor units", "Usage credit cap: 2000 USD minor units"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("missing %q: %s", want, lines)
		}
	}
	if m.CostUSD != nil || m.CreditUsage != nil || m.UsageAvailable || len(m.Errors) != 0 {
		t.Fatal("session estimate treated as billed API cost", m)
	}
}

func TestNativeAccountActivityIsNotAPIBilling(t *testing.T) {
	r := testReader()
	r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) {
		return json.RawMessage(`{"rateLimits":{"limitId":"account","primary":{"usedPercent":12}},"usage":{"summary":{"lifetimeTokens":123,"peakDailyTokens":45},"dailyUsageBuckets":[{"startDate":"2026-10-14","tokens":4}]}}`), nil
	}
	m := r.monitorAccount(context.Background(), Account{ID: "personal", Provider: "openai", Kind: "subscription", Enabled: true, ReportHarness: "codex"}, time.Now())
	if !strings.Contains(strings.Join(m.Notes, "\n"), "Lifetime account tokens: 123") || m.CostUSD != nil || m.UsageAvailable {
		t.Fatal(m)
	}
}

func TestSDKQuotaFieldsStayUnavailableWhenMissingOrInvalid(t *testing.T) {
	for _, tc := range []struct {
		body    string
		limits  int
		failure bool
	}{
		{`bad`, 0, true}, {`{}`, 0, false}, {`{"rate_limits_available":true}`, 0, false},
		{`{"rate_limits_available":true,"rate_limits":{"five_hour":null}}`, 0, false},
		{`{"rate_limits_available":true,"rate_limits":{"five_hour":[]}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"five_hour":{}}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":101}}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":-1}}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"model_scoped":true}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"model_scoped":[{"display_name":"Model","utilization":50}]}}`, 1, false},
		{`{"rate_limits_available":true,"rate_limits":{"extra_usage":[]}}`, 0, true},
		{`{"rate_limits_available":true,"rate_limits":{"extra_usage":{}}}`, 0, false},
	} {
		m := sdkSubscription(json.RawMessage(tc.body), Metric{})
		if len(m.Limits) != tc.limits || (len(m.Errors) > 0) != tc.failure || m.CostUSD != nil {
			t.Fatal(tc, m)
		}
	}
	if m := sdkSubscription(json.RawMessage(`{"rate_limits_available":true,"rate_limits":{"extra_usage":{"monthly_limit":null}}}`), Metric{}); !strings.Contains(strings.Join(m.Notes, "\n"), "organization limits may apply") {
		t.Fatal(m)
	}
	p, _ := Find(Defaults(), "anthropic")
	p.SubscriptionMinVersion = ""
	if Validate([]Spec{p}) == nil {
		t.Fatal("unknown minimum version accepted")
	}
}

func TestAccountActivityNullsAndLegacyQuotaNotes(t *testing.T) {
	r := testReader()
	r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) {
		return json.RawMessage(`{"rateLimits":{},"usageNote":"Token activity unavailable","usage":{"summary":{"lifetimeTokens":null}}}`), nil
	}
	m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: "openai", Kind: "subscription", ReportHarness: "codex", Enabled: true}, time.Now())
	if lines := strings.Join(m.Notes, "\n"); !strings.Contains(lines, "Token activity unavailable") || !strings.Contains(lines, "Lifetime account tokens: unavailable") {
		t.Fatal(lines)
	}
}

func TestActivityShapesCannotEraseIndependentQuota(t *testing.T) {
	for _, body := range []string{`[]`, `null`, `"bad"`, `{"dailyUsageBuckets":[true,{"startDate":"bad"},{"startDate":"2026-10-14","tokens":0}]}`} {
		m := accountActivity(json.RawMessage(body), Metric{Limits: []string{"account: 10% used"}})
		if len(m.Limits) != 1 || len(m.Notes) == 0 || m.CostUSD != nil {
			t.Fatal(body, m)
		}
	}
	r := testReader()
	r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) {
		return json.RawMessage(`{"rateLimits":{"planType":"plus","primary":{"usedPercent":0}},"usage":[]}`), nil
	}
	m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true, ReportHarness: "codex"}, time.Now())
	if len(m.Limits) != 1 || !strings.Contains(strings.Join(m.Notes, "\n"), "plan: plus") {
		t.Fatal(m)
	}
}
