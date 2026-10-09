// Package providers owns authentication metadata, native reporting contracts and HTTPS monitoring.
package providers

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
var environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Account stores provider-independent authentication and subscription metadata.
// Credential is encrypted at rest and omitted from every inventory/monitor result.
// API accounts hold a provider/admin key or Google Cloud OAuth access token.
type Account struct {
	ID                string         `json:"id"`
	Provider          string         `json:"provider"`
	Kind              string         `json:"kind"`
	Label             string         `json:"label"`
	MonitorCredential string         `json:"monitor_credential"`
	Credential        string         `json:"credential"`
	Organization      string         `json:"organization,omitempty"`
	Project           string         `json:"project,omitempty"`
	Enabled           bool           `json:"enabled"`
	Plan              string         `json:"plan,omitempty"`
	Notes             string         `json:"notes,omitempty"`
	BillingExport     *BillingExport `json:"billing_export,omitempty"`
	ReportHarness     string         `json:"report_harness,omitempty"`
	ReportInstallID   string         `json:"report_install_id,omitempty"`
}

// Spec defines documented API origins, endpoint paths and native Account
// pages. Configuration changes explicitly select where credentials may be sent.
type Spec struct {
	ReportAdapter       string            `json:"report_adapter,omitempty"`
	ReportCredential    string            `json:"report_credential,omitempty"`
	LaunchEnv           map[string]string `json:"launch_env,omitempty"`
	KeyUsage            *KeyUsageSpec     `json:"key_usage,omitempty"`
	ID                  string            `json:"id"`
	QuotaOrigin         string            `json:"quota_origin,omitempty"`
	BillingOrigin       string            `json:"billing_origin,omitempty"`
	APIOrigin           string            `json:"api_origin"`
	UsagePath           string            `json:"usage_path"`
	CostPath            string            `json:"cost_path"`
	LimitsPath          string            `json:"limits_path"`
	BillingURL          string            `json:"billing_url"`
	UsageURL            string            `json:"usage_url"`
	KeysURL             string            `json:"keys_url"`
	SubscriptionURL     string            `json:"subscription_url"`
	KeyEnv              string            `json:"key_env"`
	ExportOrigin        string            `json:"export_origin,omitempty"`
	SubscriptionAdapter string            `json:"subscription_adapter,omitempty"`
	MinReportSeconds    int               `json:"min_report_seconds,omitempty"`
}

//go:embed providers.json
var providerData []byte

// Defaults returns independent copies of the shipped public contracts.
func Defaults() []Spec {
	var specs []Spec
	if err := json.Unmarshal(providerData, &specs); err != nil {
		panic(err)
	}
	return specs
}

// Find returns the configured contract matching id in specs, or an unknown
// provider error. It never substitutes a different provider.
func Find(specs []Spec, id string) (Spec, error) {
	for _, p := range specs {
		if p.ID == id {
			return p, nil
		}
	}
	return Spec{}, fmt.Errorf("unknown provider %q", id)
}

// Validate returns an error for duplicate identities, unsupported reporting
// contracts, insecure endpoints or protected environment overrides in specs.
// Successful validation does not prove provider access or a vendor capability.
func Validate(specs []Spec) error {
	seen := map[string]bool{}
	for _, p := range specs {
		if !identityPattern.MatchString(p.ID) || seen[p.ID] || !environmentPattern.MatchString(p.KeyEnv) {
			return fmt.Errorf("invalid provider identity or key environment")
		}
		seen[p.ID] = true
		adapter := p.adapter()
		if p.MinReportSeconds < 0 || int64(p.MinReportSeconds) > int64((1<<63-1)/time.Second) {
			return fmt.Errorf("invalid minimum reporting interval")
		}
		if p.SubscriptionAdapter != "" && p.SubscriptionAdapter != "app_server" {
			return fmt.Errorf("unknown subscription reporting adapter")
		}
		if adapter != "" && adapter != "openai" && adapter != "anthropic" && adapter != "google" && adapter != "key_usage" {
			return fmt.Errorf("unknown reporting adapter")
		}
		if adapter != "" && (p.APIOrigin == "" || p.UsagePath == "") {
			return fmt.Errorf("reporting requires an API origin and usage path")
		}
		if p.ReportCredential != "" && p.ReportCredential != "monitor" && p.ReportCredential != "inference" {
			return fmt.Errorf("invalid reporting credential selection")
		}
		if adapter == "key_usage" && (p.KeyUsage == nil || p.KeyUsage.CostField == "") {
			return fmt.Errorf("key usage requires configured field mappings")
		}
		if protectedEnvironment(p.KeyEnv) {
			return fmt.Errorf("credentials cannot override process or state environment")
		}
		for k, v := range p.LaunchEnv {
			if !environmentPattern.MatchString(k) || protectedEnvironment(k) || k == p.KeyEnv || strings.ContainsAny(v, "\x00\r\n") {
				return fmt.Errorf("invalid provider launch environment")
			}
		}
		for _, raw := range []string{p.APIOrigin, p.BillingURL, p.UsageURL, p.KeysURL, p.SubscriptionURL, p.QuotaOrigin, p.BillingOrigin, p.ExportOrigin} {
			if raw == "" {
				continue
			}
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Fragment != "" && raw != p.SubscriptionURL) {
				return fmt.Errorf("provider URLs require credential-free HTTPS")
			}
		}
		u, _ := url.Parse(p.APIOrigin)
		if p.APIOrigin != "" && u.Path != "" {
			return fmt.Errorf("API origin cannot contain a path")
		}
		for _, path := range []string{p.UsagePath, p.CostPath, p.LimitsPath} {
			if path != "" && (!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#")) {
				return fmt.Errorf("invalid provider endpoint path")
			}
		}
	}
	return nil
}

// Config holds reporting limits and configured provider contracts.
type Config struct {
	Specs                                                []Spec
	ResponseBytes                                        int64
	WindowDays, MaxPages, TimeoutSeconds, RefreshSeconds int
	NativeReport                                         func(context.Context, Account) (json.RawMessage, error)
}

// Client is the HTTP boundary for native reporting and deterministic tests.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// reader binds one operation to immutable options while sharing provider cooldowns.
type reader struct {
	cfg    Config
	client Client
	state  *Reporter
}

// New returns an independent reporter with account-scoped cooldown tracking.
func New() *Reporter {
	return &Reporter{retry: map[string]time.Time{}, cache: map[string]cachedReport{}}
}

// Monitor reads a's available usage, quotas and billing metadata at now using c.
// client must refuse redirects. No inference or financial mutation is performed.
func (r *Reporter) Monitor(ctx context.Context, a Account, now time.Time, c Config, client Client) Metric {
	return r.cachedMonitor(ctx, a, now, c, client)
}

// Metric reports the last fetched data and its capabilities independently.
// A nil CostUSD or false UsageAvailable denotes unavailable data, never zero use.
type Metric struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	Kind              string     `json:"kind"`
	Fetched           time.Time  `json:"fetched"`
	WindowStart       time.Time  `json:"window_start"`
	UsageAvailable    bool       `json:"usage_available"`
	InputTokens       int64      `json:"input_tokens"`
	OutputTokens      int64      `json:"output_tokens"`
	Requests          int64      `json:"requests"`
	CostUSD           *float64   `json:"cost_usd"`
	CreditUsage       *float64   `json:"credit_usage,omitempty"`
	CreditUnit        string     `json:"credit_unit,omitempty"`
	Limits            []string   `json:"limits"`
	Billing           string     `json:"billing,omitempty"`
	Notes             []string   `json:"notes"`
	Errors            []string   `json:"errors"`
	RetryAfterSeconds int        `json:"retry_after_seconds,omitempty"`
	CostScope         string     `json:"cost_scope,omitempty"`
	DataThrough       *time.Time `json:"data_through,omitempty"`
	Cached            bool       `json:"cached,omitempty"`
	NextFetch         *time.Time `json:"next_fetch,omitempty"`
}

// NewClient returns an HTTPS reporting client using c.TimeoutSeconds and refusing
// all redirects. It shares only the HTTP interface with the download client.
func NewClient(c Config) Client {
	return &http.Client{Timeout: time.Duration(c.TimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Reporter shares provider cooldowns across scoped engine copies. Its
// mutex serializes network reads, independently of the filesystem mutation lock.
type Reporter struct {
	mu       sync.Mutex
	retry    map[string]time.Time
	reportMu sync.Mutex
	cache    map[string]cachedReport
}

// accountGET reads one authenticated JSON endpoint. Provider response bodies and
// transport error strings are withheld because they may contain Account secrets.
func (r *reader) get(ctx context.Context, a Account, origin, path string, q url.Values, value any) error {
	// Serialize reporting requests and enforce provider cooldowns across every
	// endpoint and both UI surfaces. Mutation locks never cover network reads.
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	key := accountReportKey(a)
	if until := r.state.retry[key]; time.Now().Before(until) {
		return fmt.Errorf("account reporting paused by provider rate limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
	if err != nil {
		return fmt.Errorf("invalid account endpoint")
	}
	if req.URL.Scheme != "https" || req.URL.User != nil {
		return fmt.Errorf("account endpoints require HTTPS")
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "harness-ctl")
	p, _ := Find(r.cfg.Specs, a.Provider)
	if p.adapter() == "anthropic" {
		req.Header.Set("x-api-key", a.Credential)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+a.Credential)
	}
	if p.adapter() == "openai" && a.Organization != "" {
		req.Header.Set("OpenAI-Organization", a.Organization)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("account endpoint request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		r.state.retry[key] = retryAfter(resp.Header.Get("Retry-After"), time.Now(), time.Duration(r.cfg.RefreshSeconds)*time.Second)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("account endpoint returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, r.cfg.ResponseBytes+1))
	if err != nil || int64(len(data)) > r.cfg.ResponseBytes {
		return fmt.Errorf("account response exceeds limit or is unreadable")
	}
	if err = json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("invalid account response")
	}
	return nil
}

// adapter preserves older configured native contracts while allowing provider
// identities to use a separately selected reporting protocol.
func (p Spec) adapter() string {
	if p.ReportAdapter != "" {
		return p.ReportAdapter
	}
	for _, shipped := range Defaults() {
		if shipped.ID == p.ID {
			return shipped.ReportAdapter
		}
	}
	return ""
}

// protectedEnvironment prevents account selection from changing execution or
// state ownership. Inference credentials and API endpoint names remain flexible.
func protectedEnvironment(key string) bool {
	return key == "SHELL" || key == "ENV" || key == "BASH_ENV" || key == "HOME" || key == "PATH" || key == "GOTOOLCHAIN" || strings.HasPrefix(key, "LD_") || strings.HasPrefix(key, "DYLD_") || strings.HasPrefix(key, "XDG_") || key == "HARNESS_CTL_NATIVE"
}

// LaunchEnvironment returns inference credentials and explicitly configured
// provider endpoint metadata. It never includes reporting credentials.
func LaunchEnvironment(p Spec, a Account) map[string]string {
	env := map[string]string{p.KeyEnv: a.Credential}
	for key, value := range p.LaunchEnv {
		env[key] = value
	}
	return env
}

// accountReportKey scopes a cooldown to an Account and provider organization.
// It contains no credentials and remains stable when a token is refreshed.
func accountReportKey(a Account) string {
	return strings.Join([]string{a.ID, a.Provider, a.Organization, a.Project}, "\x00")
}

// retryAfter accepts the HTTP delta-seconds and date formats. Missing, expired
// or overflowing values fall back to the configured monitoring interval.
func retryAfter(value string, now time.Time, fallback time.Duration) time.Time {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 && seconds <= int64((1<<63-1)/time.Second) {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until
	}
	return now.Add(fallback)
}

// reportingDelay returns the rounded-up cooldown remaining for a at now.
func (r *reader) reportingDelay(a Account, now time.Time) int {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	remaining := r.state.retry[accountReportKey(a)].Sub(now)
	if remaining <= 0 {
		return 0
	}
	return int(remaining/time.Second) + 1
}
