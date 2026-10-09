package manager

import (
	"context"
	"github.com/zaRizk7/harness-ctl/internal/download"
)

// httpClient is the native transport boundary shared by downloads and reports.
type httpClient = download.Client

// newHTTPClient returns a bounded HTTPS downloader using c's operation timeout.
func newHTTPClient(c config) httpClient { return download.NewClient(c.OperationSeconds) }

// get reads a bounded public HTTPS payload without publishing filesystem state.
func (e *engine) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	return download.Get(ctx, e.client, url, limit)
}

// getJSON decodes a bounded public metadata payload into value.
func (e *engine) getJSON(ctx context.Context, url string, value any) error {
	return download.GetJSON(ctx, e.client, url, e.cfg.MetadataBytes, value)
}
