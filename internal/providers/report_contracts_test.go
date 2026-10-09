package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionWindowReportsAndUnavailableFields(t *testing.T) {
	r := testReader()
	a := Account{ID: "personal", Provider: "openai", Kind: "subscription", Enabled: true, ReportHarness: "codex"}
	for _, tc := range []struct {
		body    string
		limits  int
		failure bool
	}{
		{`{"rateLimitsByLimitId":{"primary":{"primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":2000000000},"secondary":{"usedPercent":50}}}}`, 2, false},
		{`{"rateLimits":{"limitId":"legacy","primary":{"usedPercent":0}}}`, 1, false},
		{`{}`, 0, false}, {`{"rateLimits":{"primary":{}}}`, 0, true}, {`{"rateLimits":{"primary":{"usedPercent":-1}}}`, 0, true}, {`invalid`, 0, true},
	} {
		r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) { return json.RawMessage(tc.body), nil }
		m := r.monitorAccount(context.Background(), a, time.Now())
		if len(m.Limits) != tc.limits || (len(m.Errors) > 0) != tc.failure || m.CostUSD != nil {
			t.Fatal(tc, m)
		}
	}
	r.cfg.NativeReport = func(context.Context, Account) (json.RawMessage, error) {
		return nil, errors.New("synthetic unavailable")
	}
	if m := r.monitorAccount(context.Background(), a, time.Now()); len(m.Errors) == 0 {
		t.Fatal(m)
	}
	p := r.cfg.Specs[0]
	p.MinReportSeconds = -1
	if Validate([]Spec{p}) == nil {
		t.Fatal("negative polling")
	}
	p.MinReportSeconds = 0
	p.SubscriptionAdapter = "unknown"
	if Validate([]Spec{p}) == nil {
		t.Fatal("unknown subscription contract")
	}
}

func TestCachedReportsHaveIndependentValuesAndInvalidate(t *testing.T) {
	r := testReader()
	a := Account{ID: "key", Provider: "openrouter", Kind: "api", Enabled: true, Credential: "synthetic"}
	for i := range r.cfg.Specs {
		if r.cfg.Specs[i].ID == a.Provider {
			r.cfg.Specs[i].MinReportSeconds = 60
		}
	}
	r.client = fixtureHTTP{body: `{"data":{"usage":3,"limit":10}}`}
	now := time.Now()
	m := r.state.Monitor(context.Background(), a, now, r.cfg, r.client)
	*m.CreditUsage = 900
	m.Limits[0] = "changed"
	cached := r.state.Monitor(context.Background(), a, now.Add(time.Second), r.cfg, r.client)
	if *cached.CreditUsage != 3 || cached.Limits[0] == "changed" {
		t.Fatal("cache aliases caller", cached)
	}
	a.Credential = "replacement"
	if next := r.state.Monitor(context.Background(), a, now.Add(2*time.Second), r.cfg, r.client); next.Cached {
		t.Fatal("changed credential used cached report")
	}
	cost := 1.0
	stamp := time.Now()
	clone := cloneMetric(Metric{CostUSD: &cost, DataThrough: &stamp, NextFetch: &stamp})
	*clone.CostUSD = 2
	*clone.DataThrough = stamp.Add(time.Hour)
	if cost != 1 || stamp.Equal(*clone.DataThrough) {
		t.Fatal("scalar cache aliases caller")
	}
}

func TestBillingExportRowValidation(t *testing.T) {
	if ValidateExport(nil) != nil {
		t.Fatal("nil export")
	}
	if ValidateExport(&BillingExport{Project: "../unsafe"}) == nil {
		t.Fatal("unsafe export")
	}
	for _, v := range []any{nil, "invalid", "NaN", "Infinity"} {
		if _, err := exportNumber(v); err == nil {
			t.Fatal(v)
		}
	}
	for _, v := range []any{"-1", "1e30", "bad"} {
		if _, err := exportTime(v); err == nil {
			t.Fatal(v)
		}
	}
	for _, tc := range []struct {
		row    string
		schema []exportField
	}{
		{`bad`, nil}, {`{"f":[]}`, []exportField{{Name: "cost"}}}, {`{"f":[{"v":null}]}`, []exportField{{Name: "cost"}}}, {`{"f":[{"v":{}}]}`, []exportField{{Name: "credits", Mode: "REPEATED"}}}, {`{"f":[{"v":[{"v":{}}]}]}`, []exportField{{Name: "credits", Type: "RECORD", Mode: "REPEATED", Fields: []exportField{{Name: "amount"}}}}}, {`{"f":[{"v":{}}]}`, []exportField{{Name: "project", Type: "RECORD", Fields: []exportField{{Name: "id"}}}}},
	} {
		_, err := exportRecord(json.RawMessage(tc.row), tc.schema)
		if err == nil && !strings.Contains(tc.row, "null") {
			t.Fatal(tc)
		}
	}
	if _, err := exportValue(json.RawMessage(`invalid`), exportField{}); err == nil {
		t.Fatal("malformed scalar")
	}
	a := Account{Project: "demo", BillingExport: &BillingExport{ServiceIDs: []string{"service"}}}
	start := time.Unix(100, 0)
	end := time.Unix(200, 0)
	row := map[string]any{"project": map[string]any{"id": "demo"}, "service": map[string]any{"id": "service"}, "usage_start_time": "150", "cost": "20", "credits": []any{map[string]any{"amount": "-4"}}, "currency": "LOCAL", "currency_conversion_rate": "2", "export_time": "180"}
	cost, stamp, err := exportRowCost(row, a, start, end)
	if err != nil || cost != 8 || stamp == nil {
		t.Fatal(cost, stamp, err)
	}
	for _, key := range []string{"usage_start_time", "cost", "credits", "currency_conversion_rate", "currency", "export_time"} {
		copy := map[string]any{}
		for k, v := range row {
			copy[k] = v
		}
		copy[key] = true
		if _, _, err := exportRowCost(copy, a, start, end); err == nil {
			t.Fatal(key)
		}
	}
	for _, credits := range []any{[]any{true}, []any{map[string]any{"amount": "bad"}}} {
		row["credits"] = credits
		if _, _, err := exportRowCost(row, a, start, end); err == nil {
			t.Fatal(credits)
		}
	}
	row["credits"] = nil
	row["usage_start_time"] = "300"
	if cost, _, err := exportRowCost(row, a, start, end); err != nil || cost != 0 {
		t.Fatal(cost, err)
	}
	row["project"] = nil
	if cost, _, err := exportRowCost(row, a, start, end); err != nil || cost != 0 {
		t.Fatal(cost, err)
	}
	if _, err := exportNumber("1.5"); err != nil {
		t.Fatal(err)
	}
}

func TestGoogleCostFailureRemainsUnavailable(t *testing.T) {
	r := testReader()
	r.client = fixtureHTTP{body: `{}`}
	m := r.monitorAccount(context.Background(), Account{ID: "cloud", Provider: "google", Kind: "api", Enabled: true, Project: "demo", MonitorCredential: "token", BillingExport: &BillingExport{Project: "billing", Dataset: "dataset", Table: "table"}}, time.Now())
	if m.CostUSD != nil || len(m.Errors) == 0 {
		t.Fatal(m)
	}
}

func TestBillingExportRejectsIncompleteChangingAndInvalidPages(t *testing.T) {
	for _, scenario := range []string{"invalid-source", "schema-http", "schema-empty", "page-http", "invalid-count", "count-change", "malformed-row", "invalid-cost", "incomplete", "repeat", "limit", "empty", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			r := testReader()
			p, _ := Find(r.cfg.Specs, "google")
			a := Account{ID: "cloud", Provider: "google", Credential: "token", Project: "demo", BillingExport: &BillingExport{Project: "billing", Dataset: "dataset", Table: "table"}}
			if scenario == "invalid-source" {
				a.BillingExport.Project = "../invalid"
			}
			if scenario == "limit" {
				r.cfg.MaxPages = 1
			}
			r.client = accountHTTP(func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.Path, "/data") {
					if scenario == "schema-http" {
						return nil, errors.New("synthetic")
					}
					body := billingSchemaJSON
					if scenario == "schema-empty" {
						body = `{}`
					}
					return fixtureHTTP{body: body}.Do(req)
				}
				if scenario == "page-http" {
					return nil, errors.New("synthetic")
				}
				body := `{"totalRows":"0"}`
				switch scenario {
				case "invalid-count":
					body = `{}`
				case "count-change":
					body = `{"totalRows":"1","pageToken":"next"}`
					if req.URL.Query().Get("pageToken") != "" {
						body = `{"totalRows":"2"}`
					}
				case "malformed-row":
					body = `{"totalRows":"1","rows":[{"f":[]}]}`
				case "invalid-cost":
					body = `{"totalRows":"1","rows":[{"f":[{"v":{"f":[{"v":"demo"}]}},{"v":{"f":[{"v":"service"}]}},{"v":"150"},{"v":"invalid"},{"v":"USD"},{"v":"1"},{"v":[]},{"v":"180"}]}]}`
				case "incomplete":
					body = `{"totalRows":"2"}`
				case "repeat", "limit":
					body = `{"totalRows":"0","pageToken":"again"}`
				case "overflow":
					body = `{"totalRows":"-1"}`
				}
				return fixtureHTTP{body: body}.Do(req)
			})
			cost, _, err := r.googleExportCost(context.Background(), a, p, time.Unix(0, 0), time.Now())
			if (err == nil) != (scenario == "empty") || cost != 0 {
				t.Fatal(cost, err)
			}
		})
	}
}
