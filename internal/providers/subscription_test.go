package providers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSubscriptionReportingUsesDocumentedNativeContract(t *testing.T) {
	r := testReader()
	var a Account
	if err := json.Unmarshal([]byte(`{"id":"personal","provider":"openai","kind":"subscription","enabled":true,"report_harness":"codex"}`), &a); err != nil {
		t.Fatal(err)
	}
	m := r.monitorAccount(context.Background(), a, time.Now())
	if len(m.Errors) == 0 {
		t.Fatalf("requested native reporting silently ignored: %+v", m)
	}
}
