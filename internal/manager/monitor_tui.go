package manager

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// globalMonitorMsg carries secret-free reports independently of account approval.
type globalMonitorMsg struct {
	metrics []accountMetric
	err     error
}

// loadGlobalMonitor reads currently enabled accounts under a bounded deadline.
// Every completion schedules one timer, so screens never create competing pollers.
func (m tuiModel) loadGlobalMonitor() tea.Cmd {
	return func() tea.Msg {
		items, err := m.e.loadAccounts()
		if err != nil {
			return globalMonitorMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.e.cfg.ProbeSeconds)*time.Second)
		defer cancel()
		var metrics []accountMetric
		for _, a := range items {
			if a.Enabled {
				metrics = append(metrics, m.e.monitorAccount(ctx, a, time.Now()))
			}
		}
		return globalMonitorMsg{metrics: metrics}
	}
}

// metricLines renders separate available token, dollar, credit and quota values.
func metricLines(metric accountMetric) []string {
	lines := []string{metric.ID + " / " + metric.Provider + " / " + metric.Kind, "Fetched: " + metric.Fetched.Format(time.RFC3339)}
	if metric.NextFetch != nil {
		lines = append(lines, "Next provider fetch: "+metric.NextFetch.Format(time.RFC3339))
	}
	if metric.DataThrough != nil {
		lines = append(lines, "Latest matching billing export: "+metric.DataThrough.Format(time.RFC3339))
	}
	if metric.CostScope != "" {
		lines = append(lines, "Cost scope: "+metric.CostScope)
	}
	if metric.UsageAvailable {
		lines = append(lines, fmt.Sprintf("Usage since %s: %d requests, %d input tokens, %d output tokens", metric.WindowStart.Format("2006-01-02"), metric.Requests, metric.InputTokens, metric.OutputTokens))
	} else {
		lines = append(lines, "Usage: unavailable")
	}
	if metric.CostUSD != nil {
		lines = append(lines, fmt.Sprintf("API cost: USD %.4f", *metric.CostUSD))
	} else {
		lines = append(lines, "API cost: unavailable")
	}
	if metric.CreditUsage != nil {
		lines = append(lines, fmt.Sprintf("Key usage: %g %s", *metric.CreditUsage, metric.CreditUnit))
	}
	lines = append(lines, metric.Limits...)
	lines = append(lines, "Billing: "+metric.Billing)
	lines = append(lines, metric.Notes...)
	lines = append(lines, metric.Errors...)
	return lines
}

// monitorView displays compact metrics across screens, or the complete F2 overlay.
func (m tuiModel) monitorView() []string {
	if m.monitorExpanded {
		lines := []string{fmt.Sprintf("Account reports / refresh every %ds / F2 return", m.e.cfg.RefreshSeconds)}
		for _, metric := range m.accountMetrics {
			lines = append(lines, metricLines(metric)...)
			lines = append(lines, "")
		}
		if m.monitorError != "" {
			lines = append(lines, m.monitorError)
		}
		if len(m.accountMetrics) == 0 {
			lines = append(lines, "No enabled account reports. Add accounts from the home screen.")
		}
		copy := m
		copy.cursor = m.monitorCursor
		return copy.scroll(lines)
	}
	label := fmt.Sprintf("Monitor %ds · F2 details", m.e.cfg.RefreshSeconds)
	if m.monitorError != "" {
		return []string{label + " · " + m.monitorError}
	}
	if len(m.accountMetrics) == 0 {
		return []string{label + " · no enabled account reports"}
	}
	var values []string
	for _, metric := range m.accountMetrics {
		value := metric.ID
		if metric.Cached {
			value += " [cached " + metric.Fetched.Format("15:04:05") + "]"
		}
		if metric.CostUSD != nil {
			value += fmt.Sprintf(" USD %.4f", *metric.CostUSD)
		}
		if metric.CreditUsage != nil {
			value += fmt.Sprintf(" %g %s", *metric.CreditUsage, metric.CreditUnit)
		}
		if len(metric.Limits) > 0 {
			value += " " + strings.Join(metric.Limits, ", ")
		}
		if metric.CostUSD == nil && metric.CreditUsage == nil && len(metric.Limits) == 0 {
			value += " usage/limits unavailable"
		}
		if len(metric.Errors) > 0 {
			value += " [report error]"
		}
		values = append(values, cleanText(value))
	}
	return []string{label, strings.Join(values, " | ")}
}

// terminalContent reserves visible footer rows for approval, status and monitoring.
// Preview rows wrap before scrolling. Footer rows are bounded to prevent wrapping.
func (m tuiModel) terminalContent(body, footer []string) string {
	body = m.wrapRows(body)
	if m.height > 0 {
		if len(footer) > m.height {
			footer = footer[len(footer)-m.height:]
		}
		body = body[:min(len(body), max(0, m.height-len(footer)))]
	}
	if m.width > 0 {
		for i, line := range footer {
			footer[i] = lipgloss.NewStyle().MaxWidth(m.width).Render(line)
		}
	}
	lines := append(append([]string{}, body...), footer...)
	return strings.Join(lines, "\n")
}

// wrapRows preserves complete preview text while fitting native terminal cells.
func (m tuiModel) wrapRows(lines []string) []string {
	if m.width <= 0 {
		return lines
	}
	var result []string
	for _, line := range lines {
		result = append(result, strings.Split(lipgloss.NewStyle().Width(m.width).Render(line), "\n")...)
	}
	return result
}
