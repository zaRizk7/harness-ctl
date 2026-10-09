package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// usagePage decodes only usage/cost fields needed for the dashboard. Pagination
// is bounded and incomplete responses never appear as complete zero totals.
type usagePage struct {
	Data []struct {
		Results []struct {
			Input       int64           `json:"input_tokens"`
			Uncached    int64           `json:"uncached_input_tokens"`
			CacheRead   int64           `json:"cache_read_input_tokens"`
			CacheCreate int64           `json:"cache_creation_input_tokens"`
			Output      int64           `json:"output_tokens"`
			Requests    int64           `json:"num_model_requests"`
			Amount      json.RawMessage `json:"amount"`
			Currency    string          `json:"currency"`
		} `json:"results"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	Next    string `json:"next_page"`
}

// monitorAccount reads provider-side API usage, cost and available quota/billing
// metadata. It does not run inference probes, infer subscription quotas or change
// financial settings. now anchors a documented rolling reporting window.
func (r *reader) monitorAccount(ctx context.Context, a Account, now time.Time) (m Metric) {
	m = Metric{ID: a.ID, Provider: a.Provider, Kind: a.Kind, Fetched: now.UTC(), WindowStart: now.UTC().AddDate(0, 0, -r.cfg.WindowDays)}
	defer func() {
		m.RetryAfterSeconds = r.reportingDelay(a, time.Now())
		if m.RetryAfterSeconds > 0 {
			m.Notes = append(m.Notes, fmt.Sprintf("Provider rate limit: requests paused for %ds", m.RetryAfterSeconds))
		}
	}()
	p, err := Find(r.cfg.Specs, a.Provider)
	if err != nil {
		m.Errors = append(m.Errors, err.Error())
		return m
	}
	if !a.Enabled {
		m.Notes = append(m.Notes, "Monitoring disabled for this account")
		return m
	}
	m.Billing = p.BillingURL
	if a.Kind == "subscription" {
		return r.subscriptionLimits(ctx, a, p, m)
	}
	adapter := p.adapter()
	if adapter == "" {
		m.Notes = append(m.Notes, "No reporting adapter is configured. Use the provider account page.")
		return m
	}
	if p.ReportCredential == "inference" {
		a.MonitorCredential = a.Credential
	}
	if a.MonitorCredential == "" {
		m.Errors = append(m.Errors, "Monitoring requires a separate organization/admin credential, or Google Cloud OAuth token. Ordinary inference keys are not used for account reporting.")
		return m
	}
	a.Credential = a.MonitorCredential
	m.Notes = append(m.Notes, "Provider reporting may lag. Five-second polling does not guarantee five-second upstream data freshness.")
	if adapter == "key_usage" {
		return r.monitorKeyUsage(ctx, a, p, m)
	}
	if adapter == "google" {
		return r.monitorGoogle(ctx, a, p, m)
	}
	var usage Metric
	if err = r.reportPages(ctx, a, p, p.UsagePath, m.WindowStart, now, false, &usage); err != nil {
		m.Errors = append(m.Errors, "Usage: "+err.Error())
	} else {
		m.InputTokens = usage.InputTokens
		m.OutputTokens = usage.OutputTokens
		m.Requests = usage.Requests
		m.UsageAvailable = true
	}
	var cost Metric
	if err = r.reportPages(ctx, a, p, p.CostPath, m.WindowStart, now, true, &cost); err != nil {
		m.Errors = append(m.Errors, "Cost: "+err.Error())
	} else {
		m.CostUSD = cost.CostUSD
	}
	m.Billing = p.BillingURL
	if p.LimitsPath != "" && (adapter == "anthropic" || a.Project != "") {
		m.Limits, err = r.monitorLimits(ctx, a, p)
		if err != nil {
			m.Errors = append(m.Errors, "Limits: "+err.Error())
		}
	} else {
		m.Notes = append(m.Notes, "Rate limits unavailable through the selected reporting contract. View the provider usage/limits page.")
	}

	return m
}

// reportPages aggregates one complete organization report and rejects repeated,
// absent or excessive page cursors. Anthropic costs are decimal USD cents.
func (r *reader) reportPages(ctx context.Context, a Account, p Spec, path string, start, end time.Time, cost bool, m *Metric) error {
	q := url.Values{}
	if p.adapter() == "openai" {
		q.Set("start_time", strconv.FormatInt(start.Unix(), 10))
		q.Set("end_time", strconv.FormatInt(end.Unix(), 10))
		q.Set("bucket_width", "1d")
	} else {
		q.Set("starting_at", start.Format(time.RFC3339))
		q.Set("ending_at", end.Format(time.RFC3339))
		if !cost {
			q.Set("bucket_width", "1d")
		}
	}
	cursors := map[string]bool{}
	sum := 0.0
	for page := 0; page < r.cfg.MaxPages; page++ {
		var data usagePage
		if err := r.get(ctx, a, p.APIOrigin, path, q, &data); err != nil {
			return err
		}
		if data.Data == nil {
			return fmt.Errorf("report is missing its data array")
		}
		for _, bucket := range data.Data {
			for _, r := range bucket.Results {
				if cost {
					var value float64
					var currency string
					if p.adapter() == "openai" {
						var amount struct {
							Value    float64 `json:"value"`
							Currency string  `json:"currency"`
						}
						if err := json.Unmarshal(r.Amount, &amount); err != nil {
							return fmt.Errorf("invalid cost amount")
						}
						value, currency = amount.Value, amount.Currency
					} else {
						var amount string
						if err := json.Unmarshal(r.Amount, &amount); err != nil {
							return fmt.Errorf("invalid cost amount")
						}
						n, err := strconv.ParseFloat(amount, 64)
						if err != nil {
							return fmt.Errorf("invalid cost amount")
						}
						value, currency = n/100, r.Currency
					}
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return fmt.Errorf("invalid cost amount")
					}
					if !strings.EqualFold(currency, "usd") {
						return fmt.Errorf("cost response has unsupported currency")
					}
					sum += value
				} else {
					m.InputTokens += r.Input + r.Uncached + r.CacheRead + r.CacheCreate
					m.OutputTokens += r.Output
					m.Requests += r.Requests
				}
			}
		}
		if !data.HasMore {
			if cost {
				m.CostUSD = &sum
			}
			return nil
		}
		if data.Next == "" || cursors[data.Next] {
			return fmt.Errorf("invalid report pagination")
		}
		cursors[data.Next] = true
		q.Set("page", data.Next)
	}
	return fmt.Errorf("report exceeds configured page limit")
}

// monitorGoogle reads request counts, quota limits and linked billing metadata
// for a under ctx. An explicitly selected export adds bounded net project costs
// for m's reporting window. Unavailable fields remain absent and failures are
// attached to the returned metric. Invoices remain on the provider billing page.
func (r *reader) monitorGoogle(ctx context.Context, a Account, p Spec, m Metric) Metric {
	if a.Project == "" {
		m.Errors = append(m.Errors, "Google monitoring requires a Cloud project ID")
		return m
	}
	path := strings.ReplaceAll(p.UsagePath, "{project}", url.PathEscape(a.Project))
	q := url.Values{"filter": {`metric.type="serviceruntime.googleapis.com/api/request_count" AND resource.type="consumed_api" AND resource.labels.service="generativelanguage.googleapis.com"`}, "interval.startTime": {m.WindowStart.Format(time.RFC3339)}, "interval.endTime": {m.Fetched.Format(time.RFC3339)}, "view": {"FULL"}}
	type seriesPage struct {
		Series []struct {
			Points []struct {
				Value struct {
					Count string `json:"int64Value"`
				} `json:"value"`
			} `json:"points"`
		} `json:"timeSeries"`
		Next string `json:"nextPageToken"`
	}
	total := int64(0)
	seen := map[string]bool{}
	for page := 0; page < r.cfg.MaxPages; page++ {
		var data seriesPage
		if err := r.get(ctx, a, p.APIOrigin, path, q, &data); err != nil {
			m.Errors = append(m.Errors, "Usage: "+err.Error())
			break
		}
		invalid := false
		for _, series := range data.Series {
			for _, point := range series.Points {
				n, err := strconv.ParseInt(point.Value.Count, 10, 64)
				if err != nil {
					invalid = true
				} else {
					total += n
				}
			}
		}
		if invalid {
			m.Errors = append(m.Errors, "Usage: invalid request count")
			break
		}
		if data.Next == "" {
			m.Requests = total
			m.UsageAvailable = true
			break
		}
		if seen[data.Next] || page+1 == r.cfg.MaxPages {
			m.Errors = append(m.Errors, "Usage: incomplete pagination")
			break
		}
		seen[data.Next] = true
		q.Set("pageToken", data.Next)
	}
	path = strings.ReplaceAll(p.LimitsPath, "{project}", url.PathEscape(a.Project))
	var err error
	m.Limits, err = r.googleQuotaPages(ctx, a, p, path)
	if err != nil {
		m.Errors = append(m.Errors, "Limits: "+err.Error())
	}
	var billing struct {
		Enabled bool   `json:"billingEnabled"`
		Account string `json:"billingAccountName"`
	}
	path = "/v1/projects/" + url.PathEscape(a.Project) + "/billingInfo"
	if err := r.get(ctx, a, p.BillingOrigin, path, url.Values{}, &billing); err != nil {
		m.Errors = append(m.Errors, "Billing: "+err.Error())
	} else {
		m.Billing = fmt.Sprintf("Enabled: %t, %s", billing.Enabled, billing.Account)
	}
	if a.BillingExport != nil {
		cost, through, err := r.googleExportCost(ctx, a, p, m.WindowStart, m.Fetched)
		if err != nil {
			m.Errors = append(m.Errors, "Cost: "+err.Error())
		} else {
			m.CostUSD = &cost
			m.DataThrough = through
			m.CostScope = "Google Cloud project net exported cost (selected services)"
		}
	}
	m.Notes = append(m.Notes, "Google costs require a configured Cloud Billing export. Invoices remain in Cloud Billing. Request counts are not token counts or subscription usage.")
	return m
}

// googleQuotaPages reads all Service Usage quota pages. Repeated cursors or a
// configured page limit return an error and discard incomplete limits.
func (r *reader) googleQuotaPages(ctx context.Context, a Account, p Spec, path string) ([]string, error) {
	q := url.Values{"view": {"FULL"}}
	seen := map[string]bool{}
	var lines []string
	for page := 0; page < r.cfg.MaxPages; page++ {
		var quotas struct {
			Metrics []struct {
				Name   string `json:"displayName"`
				Limits []struct {
					Buckets []struct {
						Limit string `json:"effectiveLimit"`
					} `json:"quotaBuckets"`
				} `json:"consumerQuotaLimits"`
			} `json:"metrics"`
			Next string `json:"nextPageToken"`
		}
		if err := r.get(ctx, a, p.QuotaOrigin, path, q, &quotas); err != nil {
			return nil, err
		}
		for _, metric := range quotas.Metrics {
			for _, limit := range metric.Limits {
				for _, b := range limit.Buckets {
					lines = append(lines, metric.Name+": "+b.Limit)
				}
			}
		}
		if quotas.Next == "" {
			return lines, nil
		}
		if seen[quotas.Next] {
			return nil, fmt.Errorf("invalid quota pagination")
		}
		seen[quotas.Next] = true
		q.Set("pageToken", quotas.Next)
	}
	return nil, fmt.Errorf("quota report exceeds configured page limit")
}

// monitorLimits reads complete native quota pages. Provider cursor protocols
// differ, so organization/project limits are never inferred from token usage.
func (r *reader) monitorLimits(ctx context.Context, a Account, p Spec) ([]string, error) {
	path := strings.ReplaceAll(p.LimitsPath, "{project}", url.PathEscape(a.Project))
	q := url.Values{}
	seen := map[string]bool{}
	var lines []string
	for page := 0; page < r.cfg.MaxPages; page++ {
		var data struct {
			Data []struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				RPM   int64  `json:"max_requests_per_1_minute"`
				TPM   int64  `json:"max_tokens_per_1_minute"`
				Group struct {
					Name string `json:"display_name"`
					Type string `json:"type"`
				} `json:"group"`
				Models []string `json:"models"`
				Limits []struct {
					Type  string  `json:"type"`
					Value float64 `json:"value"`
				} `json:"limits"`
			} `json:"data"`
			More bool   `json:"has_more"`
			Last string `json:"last_id"`
			Next string `json:"next_page"`
		}
		if err := r.get(ctx, a, p.APIOrigin, path, q, &data); err != nil {
			return nil, err
		}
		if data.Data == nil {
			return nil, fmt.Errorf("rate-limit response is missing its data array")
		}
		for _, l := range data.Data {
			if p.adapter() == "anthropic" {
				label := l.Group.Name
				if label == "" {
					label = l.Group.Type
				}
				for _, limit := range l.Limits {
					lines = append(lines, fmt.Sprintf("%s %s: %g", label, limit.Type, limit.Value))
				}
			} else {
				lines = append(lines, fmt.Sprintf("%s: %d requests/min, %d tokens/min", l.Model, l.RPM, l.TPM))
			}
		}
		cursor, param := data.Next, "page"
		if p.adapter() == "openai" {
			if !data.More {
				return lines, nil
			}
			cursor, param = data.Last, "after"
		} else if cursor == "" {
			return lines, nil
		}
		if cursor == "" || seen[cursor] {
			return nil, fmt.Errorf("invalid rate-limit pagination")
		}
		seen[cursor] = true
		q.Set(param, cursor)
	}
	return nil, fmt.Errorf("rate-limit report exceeds configured page limit")
}
