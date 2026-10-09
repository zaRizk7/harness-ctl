package providers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// cachedReport binds one secret-free report to its account/configuration digest.
type cachedReport struct {
	key    string
	metric Metric
	until  time.Time
}

// cachedMonitor preserves original fetch timestamps while serving five-second
// UI refreshes from the provider's configured minimum report interval. Changed
// credentials, source selection or contracts invalidate the cached observation.
func (r *Reporter) cachedMonitor(ctx context.Context, a Account, now time.Time, c Config, client Client) Metric {
	r.reportMu.Lock()
	defer r.reportMu.Unlock()
	p, _ := Find(c.Specs, a.Provider)
	// These records contain only JSON-supported fields. Hashes never expose keys.
	data, _ := json.Marshal([]any{a, p, c.WindowDays, c.MaxPages, c.ResponseBytes})
	key := fmt.Sprintf("%x", sha256.Sum256(data))
	id := accountReportKey(a)
	if cached, exists := r.cache[id]; exists && cached.key == key && now.Before(cached.until) {
		m := cloneMetric(cached.metric)
		m.Cached = true
		until := cached.until
		m.NextFetch = &until
		return m
	}
	m := (&reader{cfg: c, client: client, state: r}).monitorAccount(ctx, a, now)
	if p.MinReportSeconds > 0 {
		until := now.Add(time.Duration(p.MinReportSeconds) * time.Second)
		r.cache[id] = cachedReport{key: key, metric: cloneMetric(m), until: until}
		m.NextFetch = &until
	}
	return m
}

// cloneMetric returns a report whose mutable slices/scalars are independent of m.
func cloneMetric(m Metric) Metric {
	m.Limits = slices.Clone(m.Limits)
	m.Notes = slices.Clone(m.Notes)
	m.Errors = slices.Clone(m.Errors)
	if m.CostUSD != nil {
		v := *m.CostUSD
		m.CostUSD = &v
	}
	if m.CreditUsage != nil {
		v := *m.CreditUsage
		m.CreditUsage = &v
	}
	if m.DataThrough != nil {
		v := *m.DataThrough
		m.DataThrough = &v
	}
	if m.NextFetch != nil {
		v := *m.NextFetch
		m.NextFetch = &v
	}
	return m
}
