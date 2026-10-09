package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"
)

// subscriptionLimits reads the explicitly selected native account's quota windows.
// Missing fields stay unavailable. Reported usage is never converted to money.
func (r *reader) subscriptionLimits(ctx context.Context, a Account, p Spec, m Metric) Metric {
	m.Billing = p.SubscriptionURL
	if a.ReportHarness == "" {
		m.Notes = append(m.Notes, "Subscription usage, invoices and plan limits require the provider's account page. No subscription report source is selected.")
		return m
	}
	if p.SubscriptionAdapter != "app_server" || r.cfg.NativeReport == nil {
		m.Errors = append(m.Errors, "Selected native subscription reporting contract is unavailable")
		return m
	}
	data, err := r.cfg.NativeReport(ctx, a)
	if err != nil {
		m.Errors = append(m.Errors, err.Error())
		return m
	}
	type window struct {
		Used     *float64 `json:"usedPercent"`
		Duration *int     `json:"windowDurationMins"`
		Reset    *int64   `json:"resetsAt"`
	}
	type bucket struct {
		ID                 string `json:"limitId"`
		Primary, Secondary *window
		Plan               string `json:"planType"`
	}
	var response struct {
		Single  *bucket           `json:"rateLimits"`
		Buckets map[string]bucket `json:"rateLimitsByLimitId"`
	}
	if json.Unmarshal(data, &response) != nil {
		m.Errors = append(m.Errors, "Invalid native subscription report")
		return m
	}
	if response.Buckets == nil && response.Single != nil {
		response.Buckets = map[string]bucket{response.Single.ID: *response.Single}
	}
	ids := []string{}
	for id := range response.Buckets {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		b := response.Buckets[id]
		for index, w := range []*window{b.Primary, b.Secondary} {
			if w == nil {
				continue
			}
			if w.Used == nil || math.IsNaN(*w.Used) || math.IsInf(*w.Used, 0) || *w.Used < 0 {
				m.Errors = append(m.Errors, "Subscription window usage unavailable")
				continue
			}
			line := fmt.Sprintf("%s window %d: %.1f%% used", id, index+1, *w.Used)
			if w.Duration != nil {
				line += fmt.Sprintf(" / %d minutes", *w.Duration)
			}
			if w.Reset != nil {
				line += " / resets " + time.Unix(*w.Reset, 0).UTC().Format(time.RFC3339)
			}
			m.Limits = append(m.Limits, line)
		}
	}
	if len(m.Limits) == 0 && len(m.Errors) == 0 {
		m.Notes = append(m.Notes, "Native subscription quota windows unavailable")
	}
	m.Notes = append(m.Notes, "Quota percentages are native subscription usage. API costs and invoices remain separate. The report uses the selected installation's signed-in account.")
	return m
}
