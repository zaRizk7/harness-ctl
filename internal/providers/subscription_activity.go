package providers

import (
	"encoding/json"
	"time"
)

// accountActivity formats read-only token summaries and daily buckets. It does
// not convert token counts to cost, infer quota use or expose other account data.
// Invalid optional activity does not erase an independent quota observation.
func accountActivity(raw json.RawMessage, m Metric) Metric {
	var report map[string]any
	if json.Unmarshal(raw, &report) != nil || report == nil {
		m.Notes = append(m.Notes, "Native account token activity unavailable")
		return m
	}
	for _, field := range []KeyUsageField{
		{Pointer: "/summary/lifetimeTokens", Label: "Lifetime account tokens", Type: "amount", Unit: "tokens"},
		{Pointer: "/summary/peakDailyTokens", Label: "Peak daily account tokens", Type: "amount", Unit: "tokens"},
		{Pointer: "/summary/longestRunningTurnSec", Label: "Longest account turn", Type: "amount", Unit: "seconds"},
		{Pointer: "/summary/currentStreakDays", Label: "Current account activity streak", Type: "amount", Unit: "days"},
		{Pointer: "/summary/longestStreakDays", Label: "Longest account activity streak", Type: "amount", Unit: "days"},
	} {
		m.Notes = append(m.Notes, keyUsageDetail(report, field))
	}
	if rows, ok := report["dailyUsageBuckets"].([]any); ok {
		for _, value := range rows {
			row, ok := value.(map[string]any)
			if !ok {
				m.Notes = append(m.Notes, "Daily account token row unavailable")
				continue
			}
			date, _ := row["startDate"].(string)
			if _, err := time.Parse("2006-01-02", date); err != nil {
				m.Notes = append(m.Notes, "Daily account token date unavailable")
				continue
			}
			m.Notes = append(m.Notes, keyUsageDetail(row, KeyUsageField{Pointer: "/tokens", Label: "Account tokens on " + date, Type: "amount", Unit: "tokens"}))
		}
	}
	m.Notes = append(m.Notes, "Account token activity is separate from API billing and quota percentages.")
	return m
}
