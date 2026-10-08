package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func newHTTPClient(c config) httpClient {
	return &http.Client{Timeout: time.Duration(c.OperationSeconds) * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 8 || req.URL.Scheme != "https" {
			return fmt.Errorf("unsafe download redirect")
		}
		return nil
	}}
}

func (e *engine) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("downloads require HTTPS")
	}
	req.Header.Set("User-Agent", "harness-ctl")
	req.Header.Set("Accept", "application/json")
	resp, err := e.client.Do(req)
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

func (e *engine) getJSON(ctx context.Context, url string, value any) error {
	data, err := e.get(ctx, url, e.cfg.MetadataBytes)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
