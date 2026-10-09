package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testReader() *reader {
	c := Config{Specs: Defaults(), WindowDays: 30, MaxPages: 100, ResponseBytes: 8 << 20, TimeoutSeconds: 8, RefreshSeconds: 5}
	return &reader{cfg: c, client: NewClient(c), state: New()}
}

type accountHTTP func(*http.Request) (*http.Response, error)

func (f accountHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }

type fixtureHTTP struct {
	body   string
	status int
}

func (f fixtureHTTP) Do(*http.Request) (*http.Response, error) {
	status := f.status
	if status == 0 {
		status = 200
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(f.body))}, nil
}

func TestAccountMonitoringUsesAdminContractsAndPagination(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic", "google"} {
		t.Run(provider, func(t *testing.T) {
			e := testReader()
			a := Account{ID: "team", Provider: provider, Kind: "api", Credential: "synthetic-key", MonitorCredential: "synthetic-key", Enabled: true, Project: "demo-project"}
			var paths []string
			e.client = accountHTTP(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				if r.Method != "GET" || r.URL.Scheme != "https" {
					t.Fatal("monitor mutated provider", r)
				}
				if provider == "anthropic" {
					if r.Header.Get("x-api-key") != a.Credential {
						t.Fatal("missing key")
					}
				} else if r.Header.Get("Authorization") != "Bearer "+a.Credential {
					t.Fatal("missing bearer")
				}
				body := `{"data":[],"has_more":false}`
				if strings.Contains(r.URL.Path, "completions") {
					body = `{"data":[{"results":[{"input_tokens":4,"output_tokens":2,"num_model_requests":1}]}],"has_more":true,"next_page":"next"}`
					if r.URL.Query().Get("page") == "next" {
						body = `{"data":[{"results":[{"input_tokens":3,"output_tokens":1,"num_model_requests":1}]}],"has_more":false}`
					}
				}
				if strings.Contains(r.URL.Path, "cost") {
					body = `{"data":[{"results":[{"amount":{"value":1.5,"currency":"usd"}}]}],"has_more":false}`
					if provider == "anthropic" {
						body = `{"data":[{"results":[{"amount":"150","currency":"USD"}]}],"has_more":false}`
					}
				}
				if strings.Contains(r.URL.Path, "usage_report") {
					body = `{"data":[{"results":[{"uncached_input_tokens":4,"output_tokens":2}]}],"has_more":false}`
				}
				if provider == "google" {
					body = `{"timeSeries":[{"points":[{"value":{"int64Value":"7"}}]}]}`
					if strings.Contains(r.URL.Path, "consumerQuotaMetrics") {
						body = `{"metrics":[{"displayName":"Requests","consumerQuotaLimits":[{"quotaBuckets":[{"effectiveLimit":"20"}]}]}]}`
					}
					if strings.Contains(r.URL.Path, "billingInfo") {
						body = `{"billingEnabled":true,"billingAccountName":"billingAccounts/demo"}`
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			m := e.monitorAccount(context.Background(), a, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC))
			if len(m.Errors) > 0 || len(paths) < 2 {
				t.Fatal(m, paths)
			}
			if provider == "openai" && (m.InputTokens != 7 || m.OutputTokens != 3 || m.CostUSD == nil || *m.CostUSD != 1.5) {
				t.Fatal(m)
			}
			if provider == "anthropic" && (m.InputTokens != 4 || m.CostUSD == nil || *m.CostUSD != 1.5) {
				t.Fatal(m)
			}
			if provider == "google" && (m.Requests != 7 || len(m.Limits) == 0 || !strings.Contains(m.Billing, "billingAccounts/demo")) {
				t.Fatal(m)
			}
		})
	}
}

func TestAccountUnavailableIsNotZero(t *testing.T) {
	e := testReader()
	a := Account{ID: "personal", Provider: "openai", Kind: "subscription", Enabled: true}
	m := e.monitorAccount(context.Background(), a, time.Now())
	if m.CostUSD != nil || m.UsageAvailable || len(m.Notes) == 0 {
		t.Fatal(m)
	}
	a.Kind = "api"
	m = e.monitorAccount(context.Background(), a, time.Now())
	if len(m.Errors) == 0 || m.UsageAvailable {
		t.Fatal(m)
	}
	a.MonitorCredential = "synthetic"
	e.client = fixtureHTTP{status: 429, body: "secret-provider-response"}
	m = e.monitorAccount(context.Background(), a, time.Now())
	if m.UsageAvailable || strings.Contains(strings.Join(m.Errors, " "), "secret-provider-response") {
		t.Fatal(m)
	}
	if e.cfg.RefreshSeconds != 5 {
		t.Fatal("default refresh must be five seconds")
	}
}

func TestNativeRateLimitPages(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			e := testReader()
			a := Account{Provider: provider, Credential: "reporting", Project: "project"}
			p, _ := Find(e.cfg.Specs, provider)
			calls := 0
			e.client = accountHTTP(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `{"data":[{"model":"test-model","max_requests_per_1_minute":10,"max_tokens_per_1_minute":20}],"has_more":true,"last_id":"last"}`
				if provider == "anthropic" {
					body = `{"data":[{"group":{"display_name":"Group"},"limits":[{"type":"requests_per_minute","value":10}]}],"next_page":"next"}`
				}
				if calls == 2 {
					if provider == "openai" && r.URL.Query().Get("after") != "last" || provider == "anthropic" && r.URL.Query().Get("page") != "next" {
						t.Fatal(r.URL)
					}
					body = `{"data":[],"has_more":false}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			limits, err := e.monitorLimits(context.Background(), a, p)
			if err != nil || len(limits) != 1 || calls != 2 {
				t.Fatal(limits, err, calls)
			}
			e.client = fixtureHTTP{body: `{"data":[],"has_more":true,"last_id":"repeat","next_page":"repeat"}`}
			if _, err = e.monitorLimits(context.Background(), a, p); err == nil {
				t.Fatal("repeated quota cursor accepted")
			}
		})
	}
}
func TestProviderConfigurationTrustBoundaries(t *testing.T) {
	for _, mutate := range []func(*Spec){func(p *Spec) { p.APIOrigin = "http://unsafe" }, func(p *Spec) { p.APIOrigin = "https://example.test/path" }, func(p *Spec) { p.KeyEnv = "BAD;ENV" }, func(p *Spec) { p.LimitsPath = "//unsafe" }, func(p *Spec) { p.QuotaOrigin = "http://unsafe" }, func(p *Spec) { p.BillingURL = "https://user:key@example.test" }, func(p *Spec) { p.APIOrigin = "" }} {
		specs := Defaults()
		mutate(&specs[0])
		if err := Validate(specs); err == nil {
			t.Fatal("invalid provider accepted")
		}
	}
	specs := Defaults()
	specs = append(specs, specs[0])
	if err := Validate(specs); err == nil {
		t.Fatal("duplicate provider accepted")
	}
	e := testReader()
	client := NewClient(e.cfg).(*http.Client)
	if err := client.CheckRedirect(&http.Request{}, nil); err != http.ErrUseLastResponse {
		t.Fatal(err)
	}
	a := Account{Provider: "openai", Credential: "synthetic"}
	var value any
	for _, origin := range []string{"http://unsafe", "https://user:key@example.test", "https://[bad"} {
		if err := e.get(context.Background(), a, origin, "/", url.Values{}, &value); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, body := range []string{"not-json", strings.Repeat("x", 128)} {
		e.cfg.ResponseBytes = 64
		e.client = fixtureHTTP{body: body}
		if err := e.get(context.Background(), a, "https://example.test", "/", url.Values{}, &value); err == nil {
			t.Fatal("invalid body accepted")
		}
	}
	e.client = accountHTTP(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret transport failure") })
	if err := e.get(context.Background(), a, "https://example.test", "/", url.Values{}, &value); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestMalformedUsageCannotLookLikeZeroConsumption(t *testing.T) {
	e := testReader()
	a := Account{ID: "a", Provider: "openai", Kind: "api", MonitorCredential: "reporting", Enabled: true}
	e.client = fixtureHTTP{body: `{}`}
	m := e.monitorAccount(context.Background(), a, time.Now())
	if m.UsageAvailable || m.CostUSD != nil {
		t.Fatal("malformed report appeared to be zero usage", m)
	}
}

func TestReportPaginationAndCostErrors(t *testing.T) {
	e := testReader()
	a := Account{ID: "a", Provider: "anthropic", Kind: "api", MonitorCredential: "reporting", Enabled: true}
	for _, body := range []string{`{"data":[],"has_more":true}`, `{"data":[],"has_more":true,"next_page":"repeat"}`, `{"data":[{"results":[{"amount":"NaN","currency":"USD"}]}]}`, `{"data":[{"results":[{"amount":"bad","currency":"USD"}]}]}`, `{"data":[{"results":[{"amount":"100","currency":"EUR"}]}]}`} {
		e.client = fixtureHTTP{body: body}
		m := e.monitorAccount(context.Background(), a, time.Now())
		if len(m.Errors) == 0 {
			t.Fatal("invalid report accepted", body, m)
		}
	}
	e.cfg.MaxPages = 1
	e.client = fixtureHTTP{body: `{"data":[],"has_more":true,"next_page":"next"}`}
	m := e.monitorAccount(context.Background(), a, time.Now())
	if m.UsageAvailable {
		t.Fatal("truncated report accepted")
	}
	a.Provider = "google"
	m = e.monitorAccount(context.Background(), a, time.Now())
	if len(m.Errors) == 0 {
		t.Fatal("missing Google project accepted")
	}
	a.Project = "demo"
	e.client = fixtureHTTP{body: `{"timeSeries":[{"points":[{"value":{"int64Value":"bad"}}]}]}`}
	m = e.monitorAccount(context.Background(), a, time.Now())
	if m.UsageAvailable {
		t.Fatal("malformed Google count accepted")
	}
}

func TestGoogleQuotaPaginationIsCompleteOrUnavailable(t *testing.T) {
	for _, mode := range []string{"complete", "repeat", "cap"} {
		t.Run(mode, func(t *testing.T) {
			e := testReader()
			if mode == "cap" {
				e.cfg.MaxPages = 1
			}
			calls := 0
			e.client = accountHTTP(func(r *http.Request) (*http.Response, error) {
				body := `{}`
				if strings.Contains(r.URL.Path, "consumerQuotaMetrics") {
					calls++
					body = `{"metrics":[{"displayName":"Requests","consumerQuotaLimits":[{"quotaBuckets":[{"effectiveLimit":"20"}]}]}],"nextPageToken":"next"}`
					if calls == 2 && mode == "complete" {
						if r.URL.Query().Get("pageToken") != "next" {
							t.Fatal(r.URL)
						}
						body = `{"metrics":[{"displayName":"Tokens","consumerQuotaLimits":[{"quotaBuckets":[{"effectiveLimit":"30"}]}]}]}`
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			m := e.monitorAccount(context.Background(), Account{ID: "a", Provider: "google", Kind: "api", MonitorCredential: "token", Project: "demo", Enabled: true}, time.Now())
			if mode == "complete" {
				if calls != 2 || len(m.Limits) != 2 {
					t.Fatal(calls, m)
				}
			} else if len(m.Limits) != 0 || !strings.Contains(strings.Join(m.Errors, " "), "Limits:") {
				t.Fatal("partial quota presented as complete", m)
			}
		})
	}
}

func TestAccountReportingHonorsRetryAfter(t *testing.T) {
	for _, value := range []string{"60", time.Now().Add(60 * time.Second).UTC().Format(http.TimeFormat), "invalid"} {
		t.Run(value, func(t *testing.T) {
			e := testReader()
			calls := 0
			e.client = accountHTTP(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {value}}, Body: io.NopCloser(strings.NewReader("secret"))}, nil
			})
			a := Account{ID: "a", Provider: "openai", Kind: "api", MonitorCredential: "admin", Enabled: true}
			first := e.monitorAccount(context.Background(), a, time.Now())
			second := e.monitorAccount(context.Background(), a, time.Now())
			if calls != 1 || first.RetryAfterSeconds < 1 || second.RetryAfterSeconds < 1 {
				t.Fatal(calls, first, second)
			}
			if strings.Contains(fmt.Sprint(first.Errors, second.Errors), "secret") {
				t.Fatal("provider body leaked")
			}
		})
	}
}

func TestProviderReportsRejectInvalidAmountsAndIncompleteGooglePages(t *testing.T) {
	for _, test := range []struct{ provider, body string }{{"openai", `{"data":[{"results":[{"amount":"wrong"}]}]}`}, {"anthropic", `{"data":[{"results":[{"amount":99}]}]}`}} {
		e := testReader()
		p, _ := Find(e.cfg.Specs, test.provider)
		e.client = fixtureHTTP{body: test.body}
		m := Metric{}
		if err := e.reportPages(context.Background(), Account{Provider: test.provider}, p, "/costs", time.Now(), time.Now(), true, &m); err == nil {
			t.Fatal("invalid amount accepted")
		}
	}
	for _, mode := range []string{"usage-error", "quota-error", "billing-error", "repeat", "cap", "complete"} {
		t.Run(mode, func(t *testing.T) {
			e := testReader()
			if mode == "cap" {
				e.cfg.MaxPages = 1
			}
			calls := 0
			e.client = accountHTTP(func(r *http.Request) (*http.Response, error) {
				status := 200
				body := `{}`
				if strings.Contains(r.URL.Path, "timeSeries") {
					calls++
					body = `{"timeSeries":[{"points":[{"value":{"int64Value":"1"}}]}],"nextPageToken":"next"}`
					if mode == "complete" && calls == 2 {
						if r.URL.Query().Get("pageToken") != "next" {
							t.Fatal("missing cursor")
						}
						body = `{"timeSeries":[{"points":[{"value":{"int64Value":"2"}}]}]}`
					}
					if mode == "usage-error" {
						status = 403
					}
				}
				if strings.Contains(r.URL.Path, "consumerQuotaMetrics") && mode == "quota-error" {
					status = 403
				}
				if strings.Contains(r.URL.Path, "billingInfo") && mode == "billing-error" {
					status = 403
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			m := e.monitorAccount(context.Background(), Account{ID: "a", Provider: "google", Kind: "api", Project: "demo", MonitorCredential: "reporting", Enabled: true}, time.Now())
			if mode == "complete" {
				if !m.UsageAvailable || m.Requests != 3 {
					t.Fatal(m)
				}
			} else if len(m.Errors) == 0 || m.UsageAvailable {
				t.Fatal("incomplete report accepted", m)
			}
		})
	}
	e := testReader()
	p, _ := Find(e.cfg.Specs, "anthropic")
	e.client = fixtureHTTP{status: 403}
	if _, err := e.monitorLimits(context.Background(), Account{Provider: p.ID}, p); err == nil {
		t.Fatal("limit failure ignored")
	}
	e.client = fixtureHTTP{body: `{}`}
	if _, err := e.monitorLimits(context.Background(), Account{Provider: p.ID}, p); err == nil {
		t.Fatal("missing limits accepted")
	}
}

func TestReportingOrganizationHeader(t *testing.T) {
	e := testReader()
	e.client = accountHTTP(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("OpenAI-Organization") != "org-demo" {
			t.Fatal("missing organization")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	var result any
	if err := e.get(context.Background(), Account{Provider: "openai", Organization: "org-demo"}, "https://example.test", "/", url.Values{}, &result); err != nil {
		t.Fatal(err)
	}
}
