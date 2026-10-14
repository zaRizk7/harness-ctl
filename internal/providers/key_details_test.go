package providers

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterReportsSpendPeriodsAndEffectiveBudget(t *testing.T) {
	r := testReader()
	r.client = fixtureHTTP{body: `{"data":{"usage":12,"limit":20,"limit_remaining":8,"usage_daily":0,"usage_weekly":4,"usage_monthly":10,"byok_usage":2,"limit_reset":"monthly","include_byok_in_limit":true,"effective_budget":{"scope":"workspace","limit_usd":50,"remaining_usd":30,"spend_usd":20,"reset_interval":"monthly","resets_at":"2026-11-01T00:00:00Z"}}}`}
	m := r.monitorAccount(context.Background(), Account{ID: "router", Provider: "openrouter", Kind: "api", Enabled: true, Credential: "synthetic"}, time.Now())
	lines := strings.Join(m.Notes, "\n")
	for _, want := range []string{"Daily key spend: 0 credits", "Weekly key spend: 4 credits", "Monthly key spend: 10 credits", "Own-provider key spend: 2 credits", "Key limit reset: monthly", "Own-provider spend counts toward key limit: true", "Effective budget scope: workspace", "Effective budget remaining: 30 USD", "Effective budget reset: 2026-11-01T00:00:00Z"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("missing %q: %s", want, lines)
		}
	}
	if m.CostUSD != nil || m.UsageAvailable || m.CreditUsage == nil || *m.CreditUsage != 12 {
		t.Fatal("key spend became invoice or token use", m)
	}
}

func TestConfiguredKeyDetailsHandleOptionalValuesAndDifferentProviders(t *testing.T) {
	field := KeyUsageField{Pointer: "/a~1b/~0value", Label: "Spend", Type: "amount", Unit: "units", NullLabel: "no cap"}
	for _, tc := range []struct {
		value any
		want  string
	}{
		{0.0, "0 units"}, {1.0, "1 units"}, {nil, "no cap"}, {-1.0, "unavailable"}, {math.Inf(1), "unavailable"}, {math.NaN(), "unavailable"}, {"bad", "unavailable"},
	} {
		if got := keyUsageDetail(map[string]any{"a/b": map[string]any{"~value": tc.value}}, field); got != "Spend: "+tc.want {
			t.Fatal(got, tc)
		}
	}
	for _, object := range []map[string]any{{}, {"a/b": true}, {"a/b": map[string]any{}}} {
		if !strings.Contains(keyUsageDetail(object, field), "unavailable") {
			t.Fatal(object)
		}
	}
	for _, tc := range []struct {
		kind  string
		value any
		want  string
	}{
		{"text", "monthly", "monthly"}, {"text", "", "unavailable"}, {"text", true, "unavailable"}, {"boolean", true, "true"}, {"boolean", false, "false"}, {"boolean", "true", "unavailable"}, {"unknown", 1, "unavailable"},
	} {
		if got := keyUsageDetail(map[string]any{"v": tc.value}, KeyUsageField{Pointer: "/v", Label: "Value", Type: tc.kind}); got != "Value: "+tc.want {
			t.Fatal(got, tc)
		}
	}
	p := Spec{ID: "different", KeyEnv: "KEY", KeyUsage: &KeyUsageSpec{Fields: []KeyUsageField{field}}}
	if err := Validate([]Spec{p}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []KeyUsageField{{Pointer: "relative", Label: "Value", Type: "amount"}, {Pointer: "/v", Type: "text"}, {Pointer: "/v", Label: "Value", Type: "unknown"}} {
		p.KeyUsage.Fields = []KeyUsageField{bad}
		if Validate([]Spec{p}) == nil {
			t.Fatal("invalid mapping accepted", bad)
		}
	}
}
