// Package download bounds HTTPS payloads and verifies artifacts before publication.
package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the transport boundary for bounded public downloads and
// separately configured authenticated provider reporting.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// NewClient returns a bounded downloader using timeoutSeconds. Redirects must
// remain HTTPS and cannot exceed the redirect limit.
func NewClient(timeoutSeconds int) Client {
	return &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 8 || req.URL.Scheme != "https" {
			return fmt.Errorf("unsafe download redirect")
		}
		return nil
	}}
}

// Get returns HTTPS response bytes from url within limit using ctx. Status,
// transport and oversized payloads return errors.
func Get(ctx context.Context, client Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("downloads require HTTPS")
	}
	req.Header.Set("User-Agent", "harness-ctl")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds configured size limit")
	}
	return data, nil
}

// GetJSON fetches url with ctx and decodes JSON into value, returning download
// or format errors.
func GetJSON(ctx context.Context, client Client, url string, limit int64, value any) error {
	data, err := Get(ctx, client, url, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
