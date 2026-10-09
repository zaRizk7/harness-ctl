package providers

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestProviderPollingUsesCachedReportsBetweenNativeRefreshes(t *testing.T) {
	r := testReader()
	a := Account{ID: "api", Provider: "anthropic", Kind: "api", Enabled: true, MonitorCredential: "synthetic"}
	count := 0
	r.client = accountHTTP(func(req *http.Request) (*http.Response, error) {
		count++
		return fixtureHTTP{body: `{"data":[]}`}.Do(req)
	})
	now := time.Now()
	first := r.state.Monitor(context.Background(), a, now, r.cfg, r.client)
	requests := count
	second := r.state.Monitor(context.Background(), a, now.Add(5*time.Second), r.cfg, r.client)
	if count != requests || !second.Fetched.Equal(first.Fetched) {
		t.Fatal("five-second UI refresh repeated vendor reports", count, requests)
	}
	_ = r.state.Monitor(context.Background(), a, now.Add(time.Minute), r.cfg, r.client)
	if count == requests {
		t.Fatal("cache did not expire")
	}
}
