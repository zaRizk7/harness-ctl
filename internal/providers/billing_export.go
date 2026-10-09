package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// BillingExport selects an existing standard/detailed Cloud Billing export table.
// ServiceIDs optionally narrows project costs. Reads never create query jobs.
type BillingExport struct {
	Project    string   `json:"project"`
	Dataset    string   `json:"dataset"`
	Table      string   `json:"table"`
	ServiceIDs []string `json:"service_ids,omitempty"`
}

// ValidateExport rejects export path segments that are not safe BigQuery identities.
// A nil export disables cost reporting without implying zero cost.
func ValidateExport(x *BillingExport) error {
	if x == nil {
		return nil
	}
	for _, id := range append([]string{x.Project, x.Dataset, x.Table}, x.ServiceIDs...) {
		if !identityPattern.MatchString(id) {
			return fmt.Errorf("invalid billing export identity")
		}
	}
	return nil
}

// exportField describes BigQuery's ordered table schema, including repeated records.
type exportField struct {
	Name   string        `json:"name"`
	Type   string        `json:"type"`
	Mode   string        `json:"mode"`
	Fields []exportField `json:"fields"`
}

// validateExportSchema requires the standard billing fields before any row scan.
// Extra standard/detailed columns are allowed. Missing, duplicate or incompatible
// required fields return an error rather than reporting an unrelated table as zero.
func validateExportSchema(fields []exportField) error {
	return requireExportFields(fields, []exportField{
		{Name: "project", Type: "RECORD", Fields: []exportField{{Name: "id", Type: "STRING"}}},
		{Name: "service", Type: "RECORD", Fields: []exportField{{Name: "id", Type: "STRING"}}},
		{Name: "usage_start_time", Type: "TIMESTAMP"},
		{Name: "cost", Type: "FLOAT"},
		{Name: "currency", Type: "STRING"},
		{Name: "currency_conversion_rate", Type: "FLOAT"},
		{Name: "credits", Type: "RECORD", Mode: "REPEATED", Fields: []exportField{{Name: "amount", Type: "FLOAT"}}},
		{Name: "export_time", Type: "TIMESTAMP"},
	})
}

// requireExportFields validates required against fields recursively. Optional
// scalar modes and BigQuery type aliases are accepted without changing field order.
func requireExportFields(fields, required []exportField) error {
	byName := map[string]exportField{}
	for _, field := range fields {
		if _, exists := byName[field.Name]; exists {
			return fmt.Errorf("duplicate billing export field %s", field.Name)
		}
		byName[field.Name] = field
	}
	for _, want := range required {
		got, exists := byName[want.Name]
		typeOK := got.Type == want.Type || want.Type == "RECORD" && got.Type == "STRUCT" || want.Type == "FLOAT" && got.Type == "FLOAT64"
		modeOK := got.Mode == want.Mode || want.Mode == "" && (got.Mode == "NULLABLE" || got.Mode == "REQUIRED")
		if !exists || !typeOK || !modeOK {
			return fmt.Errorf("incompatible billing export field %s", want.Name)
		}
		if len(want.Fields) > 0 {
			if err := requireExportFields(got.Fields, want.Fields); err != nil {
				return err
			}
		}
	}
	return nil
}

// exportCell holds a value in the REST f/v row representation.
type exportCell struct {
	Value json.RawMessage `json:"v"`
}

// exportRecord decodes row with fields in schema order. Shape changes fail closed
// instead of reinterpreting a cost or project column as another field.
func exportRecord(data json.RawMessage, fields []exportField) (map[string]any, error) {
	var row struct {
		Fields []exportCell `json:"f"`
	}
	if err := json.Unmarshal(data, &row); err != nil || len(row.Fields) != len(fields) {
		return nil, fmt.Errorf("billing export row differs from schema")
	}
	result := map[string]any{}
	for i, f := range fields {
		cell := row.Fields[i].Value
		if string(cell) == "null" {
			result[f.Name] = nil
			continue
		}
		if f.Mode == "REPEATED" {
			var cells []exportCell
			if json.Unmarshal(cell, &cells) != nil {
				return nil, fmt.Errorf("invalid repeated export value")
			}
			var values []any
			for _, child := range cells {
				value, err := exportValue(child.Value, f)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			}
			result[f.Name] = values
		} else {
			value, err := exportValue(cell, f)
			if err != nil {
				return nil, err
			}
			result[f.Name] = value
		}
	}
	return result, nil
}

// exportValue decodes one scalar or nested record using f's table schema.
func exportValue(data json.RawMessage, f exportField) (any, error) {
	if f.Type == "RECORD" || f.Type == "STRUCT" {
		return exportRecord(data, f.Fields)
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil, fmt.Errorf("invalid export value")
	}
	return value, nil
}

// exportNumber parses BigQuery's string-encoded float values and rejects nonfinite numbers.
func exportNumber(value any) (float64, error) {
	s, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("missing numeric export field")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid numeric export field")
	}
	return n, nil
}

// exportTime converts a BigQuery Unix timestamp to UTC without rounding away seconds.
func exportTime(value any) (time.Time, error) {
	n, err := exportNumber(value)
	if err != nil || n < 0 || n > float64(1<<62) {
		return time.Time{}, fmt.Errorf("invalid export timestamp")
	}
	seconds, fraction := math.Modf(n)
	return time.Unix(int64(seconds), int64(fraction*1e9)).UTC(), nil
}

// exportRowCost returns net USD for row in a's selected project/services/window.
// Credits and the billing exchange rate belong to each row. Unrelated rows return zero.
func exportRowCost(row map[string]any, a Account, start, end time.Time) (float64, *time.Time, error) {
	project, _ := row["project"].(map[string]any)
	service, _ := row["service"].(map[string]any)
	id, _ := service["id"].(string)
	if project["id"] != a.Project || len(a.BillingExport.ServiceIDs) > 0 && !slices.Contains(a.BillingExport.ServiceIDs, id) {
		return 0, nil, nil
	}
	stamp, err := exportTime(row["usage_start_time"])
	if err != nil {
		return 0, nil, err
	}
	if stamp.Before(start) || !stamp.Before(end) {
		return 0, nil, nil
	}
	cost, err := exportNumber(row["cost"])
	if err != nil {
		return 0, nil, err
	}
	credits, ok := row["credits"].([]any)
	if !ok && row["credits"] != nil {
		return 0, nil, fmt.Errorf("invalid billing credits")
	}
	for _, credit := range credits {
		entry, ok := credit.(map[string]any)
		if !ok {
			return 0, nil, fmt.Errorf("invalid billing credit")
		}
		amount, err := exportNumber(entry["amount"])
		if err != nil {
			return 0, nil, err
		}
		cost += amount
	}
	rate, err := exportNumber(row["currency_conversion_rate"])
	if err != nil || rate <= 0 {
		return 0, nil, fmt.Errorf("invalid billing exchange rate")
	}
	if currency, ok := row["currency"].(string); !ok || currency == "" {
		return 0, nil, fmt.Errorf("missing billing currency")
	}
	through, err := exportTime(row["export_time"])
	if err != nil {
		return 0, nil, err
	}
	return cost / rate, &through, nil
}

// googleExportCost scans complete bounded table pages using read-only REST calls.
// Missing exports, changing row counts, invalid rows and incomplete pagination
// leave cost unavailable. No partial total is presented as a complete figure.
func (r *reader) googleExportCost(ctx context.Context, a Account, p Spec, start, end time.Time) (float64, *time.Time, error) {
	x := a.BillingExport
	if err := ValidateExport(x); err != nil {
		return 0, nil, err
	}
	path := "/bigquery/v2/projects/" + url.PathEscape(x.Project) + "/datasets/" + url.PathEscape(x.Dataset) + "/tables/" + url.PathEscape(x.Table)
	var table struct {
		Schema struct {
			Fields []exportField `json:"fields"`
		} `json:"schema"`
	}
	if err := r.get(ctx, a, p.ExportOrigin, path, url.Values{}, &table); err != nil {
		return 0, nil, err
	}
	if err := validateExportSchema(table.Schema.Fields); err != nil {
		return 0, nil, err
	}
	q := url.Values{}
	seen := map[string]bool{}
	var count, expected int64
	total := 0.0
	var through *time.Time
	for page := 0; page < r.cfg.MaxPages; page++ {
		var data struct {
			Rows  []json.RawMessage `json:"rows"`
			Next  string            `json:"pageToken"`
			Total string            `json:"totalRows"`
		}
		if err := r.get(ctx, a, p.ExportOrigin, path+"/data", q, &data); err != nil {
			return 0, nil, err
		}
		n, err := strconv.ParseInt(data.Total, 10, 64)
		if err != nil || n < 0 || page > 0 && n != expected {
			return 0, nil, fmt.Errorf("billing export row count is invalid or changed")
		}
		expected = n
		for _, raw := range data.Rows {
			row, err := exportRecord(raw, table.Schema.Fields)
			if err != nil {
				return 0, nil, err
			}
			cost, stamp, err := exportRowCost(row, a, start, end)
			if err != nil {
				return 0, nil, err
			}
			total += cost
			count++
			if stamp != nil && (through == nil || stamp.After(*through)) {
				through = stamp
			}
		}
		if data.Next == "" {
			if count != expected || math.IsInf(total, 0) || math.IsNaN(total) {
				return 0, nil, fmt.Errorf("billing export total is incomplete or invalid")
			}
			return total, through, nil
		}
		if seen[data.Next] {
			return 0, nil, fmt.Errorf("invalid export pagination")
		}
		seen[data.Next] = true
		q.Set("pageToken", data.Next)
	}
	return 0, nil, fmt.Errorf("billing export exceeds configured page limit")
}
