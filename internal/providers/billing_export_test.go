package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGoogleBillingExportReportsCompleteNetCost(t *testing.T) {
	r := testReader()
	var a Account
	if err := json.Unmarshal([]byte(`{"id":"cloud","provider":"google","kind":"api","project":"demo","enabled":true,"monitor_credential":"test-token","billing_export":{"project":"billing-project","dataset":"billing","table":"gcp_billing_export_v1_TEST","service_ids":["service-1"]}}`), &a); err != nil {
		t.Fatal(err)
	}
	r.client = accountHTTP(func(req *http.Request) (*http.Response, error) {
		body := `{}`
		if strings.Contains(req.URL.Path, "/tables/") {
			if req.Method != "GET" || req.Header.Get("Authorization") != "Bearer test-token" {
				t.Fatal(req)
			}
			if strings.HasSuffix(req.URL.Path, "/data") {
				body = `{"totalRows":"1","rows":[{"f":[{"v":{"f":[{"v":"demo"}]}},{"v":{"f":[{"v":"service-1"}]}},{"v":"1781481600"},{"v":"10"},{"v":"USD"},{"v":"1"},{"v":[{"v":{"f":[{"v":"-2"}]}}]},{"v":"1781568000"}]}]}`
			} else {
				body = billingSchemaJSON
			}
		}
		return fixtureHTTP{body: body}.Do(req)
	})
	m := r.monitorAccount(context.Background(), a, time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC))
	if m.CostUSD == nil || *m.CostUSD != 8 {
		t.Fatalf("missing net Cloud cost: %+v", m)
	}
}

// billingSchemaJSON is the required standard-export subset in REST field order.
const billingSchemaJSON = `{"schema":{"fields":[{"name":"project","type":"RECORD","fields":[{"name":"id","type":"STRING"}]},{"name":"service","type":"RECORD","fields":[{"name":"id","type":"STRING"}]},{"name":"usage_start_time","type":"TIMESTAMP"},{"name":"cost","type":"FLOAT"},{"name":"currency","type":"STRING"},{"name":"currency_conversion_rate","type":"FLOAT"},{"name":"credits","type":"RECORD","mode":"REPEATED","fields":[{"name":"amount","type":"FLOAT"}]},{"name":"export_time","type":"TIMESTAMP"}]}}`

func TestBillingExportRejectsWrongSchemaBeforeReadingRows(t *testing.T) {
	for _, scenario := range []string{"unrelated", "missing", "wrong-type", "wrong-mode", "nested-missing", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			var table struct {
				Schema struct {
					Fields []exportField `json:"fields"`
				} `json:"schema"`
			}
			if err := json.Unmarshal([]byte(billingSchemaJSON), &table); err != nil {
				t.Fatal(err)
			}
			fields := table.Schema.Fields
			switch scenario {
			case "unrelated":
				fields = []exportField{{Name: "other", Type: "STRING"}}
			case "missing":
				fields = fields[:len(fields)-1]
			case "wrong-type":
				fields[3].Type = "STRING"
			case "wrong-mode":
				fields[6].Mode = "NULLABLE"
			case "nested-missing":
				fields[0].Fields = nil
			case "duplicate":
				fields = append(fields, fields[0])
			}
			table.Schema.Fields = fields
			data, err := json.Marshal(table)
			if err != nil {
				t.Fatal(err)
			}
			r := testReader()
			r.client = accountHTTP(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/data") {
					t.Fatal("invalid schema reached row scan")
				}
				return fixtureHTTP{body: string(data)}.Do(req)
			})
			p, _ := Find(r.cfg.Specs, "google")
			a := Account{Project: "demo", Credential: "token", BillingExport: &BillingExport{Project: "billing", Dataset: "dataset", Table: "table"}}
			if _, _, err := r.googleExportCost(context.Background(), a, p, time.Unix(0, 0), time.Now()); err == nil {
				t.Fatal("invalid schema presented as cost")
			}
		})
	}
}

func TestBillingSchemaAcceptsAliasesModesAndAdditionalColumns(t *testing.T) {
	var table struct {
		Schema struct {
			Fields []exportField `json:"fields"`
		} `json:"schema"`
	}
	if err := json.Unmarshal([]byte(billingSchemaJSON), &table); err != nil {
		t.Fatal(err)
	}
	fields := table.Schema.Fields
	fields[0].Type = "STRUCT"
	fields[0].Mode = "REQUIRED"
	fields[3].Type = "FLOAT64"
	fields[3].Mode = "NULLABLE"
	fields = append(fields, exportField{Name: "additional", Type: "STRING"})
	if err := validateExportSchema(fields); err != nil {
		t.Fatal(err)
	}
}
