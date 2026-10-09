package providers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// KeyUsageSpec maps a provider's key introspection response without binding its
// identity to code. Null limits mean unlimited. Credits are distinct from tokens.
type KeyUsageSpec struct {
	ObjectPointer  string `json:"object_pointer"`
	CostField      string `json:"cost_field"`
	LimitField     string `json:"limit_field"`
	RemainingField string `json:"remaining_field"`
	Unit           string `json:"unit"`
}

// monitorKeyUsage reads configured key credit usage, without an inference call.
func (r *reader) monitorKeyUsage(ctx context.Context, a Account, p Spec, m Metric) Metric {
	var document map[string]any
	if err := r.get(ctx, a, p.APIOrigin, p.UsagePath, url.Values{}, &document); err != nil {
		m.Errors = append(m.Errors, err.Error())
		return m
	}
	if p.KeyUsage == nil {
		m.Errors = append(m.Errors, "Key usage mappings are missing")
		return m
	}
	object := document
	for _, part := range strings.Split(strings.TrimPrefix(p.KeyUsage.ObjectPointer, "/"), "/") {
		if part == "" {
			continue
		}
		var ok bool
		object, ok = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")].(map[string]any)
		if !ok {
			m.Errors = append(m.Errors, "Key usage object is missing")
			return m
		}
	}
	value, ok := object[p.KeyUsage.CostField].(float64)
	if !ok || value < 0 {
		m.Errors = append(m.Errors, "Key usage amount is unavailable")
		return m
	}
	m.CreditUsage = &value
	m.CreditUnit = p.KeyUsage.Unit
	if m.CreditUnit == "" {
		m.CreditUnit = "credits"
	}
	for _, field := range []struct{ key, label string }{{p.KeyUsage.LimitField, "Credit limit"}, {p.KeyUsage.RemainingField, "Credits remaining"}} {
		if field.key == "" {
			continue
		}
		v, exists := object[field.key]
		if !exists {
			m.Errors = append(m.Errors, field.label+": unavailable")
			continue
		}
		if v == nil {
			m.Limits = append(m.Limits, field.label+": unlimited")
			continue
		}
		if n, ok := v.(float64); ok && n >= 0 {
			m.Limits = append(m.Limits, fmt.Sprintf("%s: %g %s", field.label, n, m.CreditUnit))
		} else {
			m.Errors = append(m.Errors, field.label+": invalid value")
		}
	}
	m.Notes = append(m.Notes, "Key credit usage uses the provider's configured accounting period. It is not token usage or a consumer subscription limit.")
	return m
}
