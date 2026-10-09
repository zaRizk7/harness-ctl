package download

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureClient struct {
	body   string
	status int
	err    error
	reader io.Reader
}

func (f fixtureClient) Do(*http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	status := f.status
	if status == 0 {
		status = 200
	}
	r := f.reader
	if r == nil {
		r = strings.NewReader(f.body)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(r)}, nil
}

type badReader struct{}

func (badReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestBoundedHTTPSMetadataErrors(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		url    string
		client Client
		limit  int64
	}{{"https://[invalid", fixtureClient{}, 8}, {"http://example.test", fixtureClient{}, 8}, {"https://example.test", fixtureClient{err: io.ErrClosedPipe}, 8}, {"https://example.test", fixtureClient{status: 403}, 8}, {"https://example.test", fixtureClient{reader: badReader{}}, 8}, {"https://example.test", fixtureClient{body: "too-large"}, 1}} {
		if _, err := Get(ctx, test.client, test.url, test.limit); err == nil {
			t.Fatal("invalid download accepted")
		}
	}
	var result map[string]int
	if err := GetJSON(ctx, fixtureClient{body: `{"v":1}`}, "https://example.test", 64, &result); err != nil || result["v"] != 1 {
		t.Fatal(result, err)
	}
	if GetJSON(ctx, fixtureClient{status: 403}, "https://example.test", 64, &result) == nil || GetJSON(ctx, fixtureClient{body: "bad"}, "https://example.test", 64, &result) == nil {
		t.Fatal("metadata failure ignored")
	}
	client := NewClient(1).(*http.Client)
	req, _ := http.NewRequest("GET", "https://example.test", nil)
	if err := client.CheckRedirect(req, nil); err != nil {
		t.Fatal(err)
	}
	if client.CheckRedirect(req, make([]*http.Request, 9)) == nil {
		t.Fatal("redirect loop accepted")
	}
	req.URL.Scheme = "http"
	if client.CheckRedirect(req, nil) == nil {
		t.Fatal("unsafe redirect accepted")
	}
}

func artifact(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	hash := sha256.Sum256([]byte("payload"))
	return Options{Root: root, Artifact: filepath.Join(root, "download", "package"), PackageURL: "https://example.test/package", Integrity: "sha256-" + base64.StdEncoding.EncodeToString(hash[:]), MaxBytes: 64}
}

func TestArtifactIntegrityFailuresNeverPublish(t *testing.T) {
	ctx := context.Background()
	for _, change := range []func(*Options){func(p *Options) { p.Integrity = "invalid" }, func(p *Options) { p.Integrity = "sha1-AA==" }, func(p *Options) { p.Integrity = "sha256-invalid" }, func(p *Options) { p.PackageURL = "https://[invalid" }, func(p *Options) { p.PackageURL = "http://example.test" }, func(p *Options) { p.Integrity = "sha256-AA==" }, func(p *Options) { p.MaxBytes = 1 }, func(p *Options) { p.Artifact = "/foreign/artifact" }} {
		p := artifact(t)
		change(&p)
		if err := Stage(ctx, fixtureClient{body: "payload"}, p); err == nil {
			t.Fatal("invalid artifact accepted")
		}
		if filepath.IsAbs(p.Artifact) && strings.HasPrefix(p.Artifact, p.Root) {
			if _, err := os.Stat(p.Artifact); !os.IsNotExist(err) {
				t.Fatal("failed artifact published", err)
			}
		}
	}
	for _, client := range []Client{fixtureClient{err: io.ErrClosedPipe}, fixtureClient{status: 403}, fixtureClient{reader: badReader{}}} {
		p := artifact(t)
		if Stage(ctx, client, p) == nil {
			t.Fatal("native download failure ignored")
		}
	}
	p := artifact(t)
	if err := Stage(ctx, fixtureClient{body: "payload"}, p); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(p.Artifact); err != nil || string(data) != "payload" {
		t.Fatal("verified payload missing", err)
	}
}

func TestArtifactFilesystemFailuresCleanUp(t *testing.T) {
	for _, boundary := range []string{"validate", "mkdir", "create", "copy", "sync", "close", "rename"} {
		t.Run(boundary, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			fault := errors.New("fixture filesystem failure")
			switch boundary {
			case "validate":
				platform.validate = func(string, string) error { return fault }
			case "mkdir":
				platform.mkdir = func(string, os.FileMode) error { return fault }
			case "create":
				platform.create = func(string, string) (*os.File, error) { return nil, fault }
			case "copy":
				platform.copy = func(io.Writer, io.Reader) (int64, error) { return 0, fault }
			case "sync":
				platform.sync = func(*os.File) error { return fault }
			case "close":
				platform.close = func(f *os.File) error { _ = old.close(f); return fault }
			case "rename":
				platform.rename = func(string, string) error { return fault }
			}
			p := artifact(t)
			if err := Stage(context.Background(), fixtureClient{body: "payload"}, p); !errors.Is(err, fault) {
				t.Fatal("failure lost", err)
			}
			if _, err := os.Stat(p.Artifact); !os.IsNotExist(err) {
				t.Fatal("failed artifact published", err)
			}
			files, err := os.ReadDir(filepath.Dir(p.Artifact))
			if err != nil && !os.IsNotExist(err) || len(files) != 0 {
				t.Fatal("temporary artifact leaked", files, err)
			}
		})
	}
}
