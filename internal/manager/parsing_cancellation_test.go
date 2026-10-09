package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryCancellationStopsBeforeMetadataReads(t *testing.T) {
	e, _ := testEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.discover(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled discovery continued", err)
	}
	if err := e.cli(ctx, []string{"list"}, strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled CLI discovery continued", err)
	}
}

func TestStructuredConfigRejectsTrailingJSONDocuments(t *testing.T) {
	for _, data := range []string{`{} {}`, `{"mcpServers":{}} trailing`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readConfig(path, "json"); err == nil {
			t.Fatal("multiple or trailing JSON accepted")
		}
	}
}
