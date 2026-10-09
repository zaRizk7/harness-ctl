package providers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCustomProviderUsesConfiguredLaunchAndReportingContracts(t *testing.T) {
	p := Spec{ID: "gateway", KeyEnv: "OPENAI_API_KEY", LaunchEnv: map[string]string{"OPENAI_BASE_URL": "https://gateway.example/api/v1"}, BillingURL: "https://gateway.example/billing"}
	if err := Validate([]Spec{p}); err != nil {
		t.Fatal(err)
	}
	r := testReader()
	r.cfg.Specs = []Spec{p}
	r.client = accountHTTP(func(*http.Request) (*http.Response, error) {
		t.Fatal("unconfigured reporting sent credentials")
		return nil, nil
	})
	m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: "gateway", Kind: "api", Credential: "inference", Enabled: true}, time.Now())
	if len(m.Errors) != 0 || m.UsageAvailable || m.Billing != p.BillingURL {
		t.Fatal(m)
	}
}

func TestConfiguredKeyUsageReportsCostsWithoutPretendingTokenUsage(t *testing.T) {
	p := Spec{ID: "gateway", KeyEnv: "OPENAI_API_KEY", APIOrigin: "https://gateway.example", UsagePath: "/api/v1/key", ReportAdapter: "key_usage", ReportCredential: "inference", KeyUsage: &KeyUsageSpec{ObjectPointer: "/data", CostField: "usage", LimitField: "limit", RemainingField: "limit_remaining"}}
	if err := Validate([]Spec{p}); err != nil {
		t.Fatal(err)
	}
	r := testReader()
	r.cfg.Specs = []Spec{p}
	calls := 0
	r.client = accountHTTP(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != p.UsagePath || req.Header.Get("Authorization") != "Bearer inference" {
			t.Fatal(req.URL, req.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"usage":12.5,"limit":40,"limit_remaining":27.5}}`))}, nil
	})
	m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: "gateway", Kind: "api", Credential: "inference", Enabled: true}, time.Now())
	if calls != 1 || m.CreditUsage == nil || *m.CreditUsage != 12.5 || m.CostUSD != nil || m.UsageAvailable || len(m.Limits) != 2 || len(m.Errors) != 0 {
		t.Fatal(calls, m)
	}
}

func TestKeyUsageRejectsMalformedAndNegativeLimits(t *testing.T) {
	p := Spec{ID: "gateway", KeyEnv: "KEY", APIOrigin: "https://gateway.example", UsagePath: "/key", ReportAdapter: "key_usage", ReportCredential: "inference", KeyUsage: &KeyUsageSpec{ObjectPointer: "/data", CostField: "usage", LimitField: "limit"}}
	for _, body := range []string{`{}`, `{"data":{"usage":-1}}`, `{"data":{"usage":"0"}}`, `{"data":{"usage":1,"limit":-1}}`, `{"data":{"usage":1}}`} {
		r := testReader()
		r.cfg.Specs = []Spec{p}
		r.client = accountHTTP(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: p.ID, Kind: "api", Credential: "key", Enabled: true}, time.Now())
		if len(m.Errors) == 0 {
			t.Fatalf("malformed report accepted: %s", body)
		}
	}
}

func TestProviderLaunchCannotReplaceExecutionEnvironment(t *testing.T) {
	for _, key := range []string{"HOME", "PATH", "GOTOOLCHAIN", "DYLD_INSERT_LIBRARIES", "LD_PRELOAD", "XDG_CONFIG_HOME", "HARNESS_CTL_NATIVE", "SHELL", "ENV", "BASH_ENV"} {
		p := Spec{ID: "gateway", KeyEnv: key}
		if Validate([]Spec{p}) == nil {
			t.Fatalf("protected key accepted: %s", key)
		}
	}
}

func TestProviderValidationAndExplicitLaunchContracts(t *testing.T) {
	base := Spec{ID: "gateway", KeyEnv: "KEY"}
	for _, mutate := range []func(*Spec){func(p *Spec) { p.ReportAdapter = "unknown" }, func(p *Spec) { p.ReportCredential = "wrong" }, func(p *Spec) {
		p.ReportAdapter = "key_usage"
		p.APIOrigin = "https://example.com"
		p.UsagePath = "/key"
	}, func(p *Spec) { p.LaunchEnv = map[string]string{"HOME": "unsafe"} }, func(p *Spec) { p.LaunchEnv = map[string]string{"BAD-ENV": "unsafe"} }} {
		p := base
		mutate(&p)
		if Validate([]Spec{p}) == nil {
			t.Fatal(p)
		}
	}
	env := LaunchEnvironment(Spec{KeyEnv: "KEY", LaunchEnv: map[string]string{"API_URL": "https://example.com"}}, Account{Credential: "inference", MonitorCredential: "admin"})
	if env["KEY"] != "inference" || env["API_URL"] != "https://example.com" {
		t.Fatal(env)
	}
	r := New()
	cfg := Config{Specs: []Spec{base}, WindowDays: 30, ResponseBytes: 4096, MaxPages: 2, RefreshSeconds: 5}
	client := accountHTTP(func(*http.Request) (*http.Response, error) {
		t.Fatal("disabled/unknown account made request")
		return nil, nil
	})
	if metric := r.Monitor(context.Background(), Account{ID: "a", Provider: "missing", Enabled: true}, time.Now(), cfg, client); len(metric.Errors) == 0 {
		t.Fatal(metric)
	}
	if metric := r.Monitor(context.Background(), Account{ID: "a", Provider: base.ID, Enabled: false}, time.Now(), cfg, client); len(metric.Notes) == 0 {
		t.Fatal(metric)
	}
	if (Spec{ID: "openai"}).adapter() != "openai" {
		t.Fatal("legacy contract compatibility")
	}
}
func TestKeyUsageUnavailableUnlimitedAndRootMappings(t *testing.T) {
	p := Spec{ID: "gateway", APIOrigin: "https://example.com", UsagePath: "/key", KeyEnv: "KEY", ReportAdapter: "key_usage", ReportCredential: "inference", KeyUsage: &KeyUsageSpec{CostField: "usage", LimitField: "limit"}}
	r := testReader()
	r.cfg.Specs = []Spec{p}
	r.client = accountHTTP(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"usage":1,"limit":null}`))}, nil
	})
	m := r.monitorAccount(context.Background(), Account{ID: "a", Provider: p.ID, Kind: "api", Credential: "key", Enabled: true}, time.Now())
	if m.CreditUsage == nil || len(m.Limits) != 1 || !strings.Contains(m.Limits[0], "unlimited") {
		t.Fatal(m)
	}
	p.KeyUsage = nil
	m = r.monitorKeyUsage(context.Background(), Account{}, p, Metric{})
	if len(m.Errors) == 0 {
		t.Fatal("missing mapping accepted")
	}
	r.client = accountHTTP(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})
	m = r.monitorKeyUsage(context.Background(), Account{}, p, Metric{})
	if len(m.Errors) == 0 {
		t.Fatal("service failure lost")
	}
}
