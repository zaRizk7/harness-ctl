package providers

import (
	"encoding/json"
	"fmt"
)

// sdkSubscription reads quota percentages from the experimental native control
// response. Session cost estimates and local transcript behavior are ignored.
func sdkSubscription(data json.RawMessage, m Metric) Metric {
	var response struct {
		Plan      string                     `json:"subscription_type"`
		Available bool                       `json:"rate_limits_available"`
		Limits    map[string]json.RawMessage `json:"rate_limits"`
	}
	if json.Unmarshal(data, &response) != nil {
		m.Errors = append(m.Errors, "Invalid native SDK subscription report")
		return m
	}
	if response.Plan != "" {
		m.Notes = append(m.Notes, "Plan: "+response.Plan)
	}
	if !response.Available || response.Limits == nil {
		m.Notes = append(m.Notes, "Native subscription quota unavailable. Use native subscription sign-in with eligible account access.")
		return m
	}
	for _, key := range []string{"five_hour", "seven_day", "seven_day_oauth_apps", "seven_day_opus", "seven_day_sonnet"} {
		if raw := response.Limits[key]; len(raw) > 0 && string(raw) != "null" {
			m = sdkWindow(raw, key, m)
		}
	}
	if raw := response.Limits["model_scoped"]; len(raw) > 0 && string(raw) != "null" {
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil {
			m.Errors = append(m.Errors, "Native model quota rows are invalid")
		} else {
			for _, row := range rows {
				m = sdkWindow(row, "model_scoped", m)
			}
		}
	}
	if raw := response.Limits["extra_usage"]; len(raw) > 0 && string(raw) != "null" {
		var extra map[string]any
		if json.Unmarshal(raw, &extra) != nil || extra == nil {
			m.Errors = append(m.Errors, "Native usage credit report is invalid")
		} else {
			currency, _ := extra["currency"].(string)
			unit := "currency minor units (currency unavailable)"
			if currency != "" {
				unit = currency + " minor units"
			}
			for _, field := range []KeyUsageField{
				{Pointer: "/is_enabled", Label: "Usage credits enabled", Type: "boolean"},
				{Pointer: "/used_credits", Label: "Usage credits spent", Type: "amount", Unit: unit},
				{Pointer: "/monthly_limit", Label: "Usage credit cap", Type: "amount", Unit: unit, NullLabel: "no account cap reported; organization limits may apply"},
			} {
				m.Notes = append(m.Notes, keyUsageDetail(extra, field))
			}
		}
	}
	if len(m.Limits) == 0 && len(m.Errors) == 0 {
		m.Notes = append(m.Notes, "Native subscription quota windows unavailable")
	}
	m.Notes = append(m.Notes, "Native SDK reporting is experimental. Session estimates are not invoices. No prompt or local transcript scan is requested.")
	return m
}

// sdkWindow appends one documented percent/reset window. Missing percentages
// remain unavailable. A supplied model name describes only this native bucket.
func sdkWindow(raw json.RawMessage, label string, m Metric) Metric {
	var window map[string]any
	if json.Unmarshal(raw, &window) != nil || window == nil {
		m.Errors = append(m.Errors, "Invalid native subscription window")
		return m
	}
	if name, ok := window["display_name"].(string); ok && name != "" {
		label += " / " + name
	}
	used, ok := window["utilization"].(float64)
	if !ok || used < 0 || used > 100 {
		m.Errors = append(m.Errors, label+": usage unavailable")
		return m
	}
	line := fmt.Sprintf("%s: %.1f%% used", label, used)
	if reset, ok := window["resets_at"].(string); ok && reset != "" {
		line += " / resets " + reset
	}
	m.Limits = append(m.Limits, line)
	return m
}
