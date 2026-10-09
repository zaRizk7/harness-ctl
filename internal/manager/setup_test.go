package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledCatalogLoadsWithoutStartupWrites(t *testing.T) {
	c := defaultConfig(t.TempDir())
	catalogPath := filepath.Join(c.Root, "catalog.json")
	if err := writeJSON(catalogPath, []harnessSpec{{ID: "custom", Name: "Custom", Command: "custom", DefaultHome: ".custom", Kind: "npm", Package: "custom"}}); err != nil {
		t.Fatal(err)
	}
	before, _ := fingerprint(c.Root)
	e, err := newEngine(c, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.specFor("custom"); err != nil {
		t.Fatal(err)
	}
	after, _ := fingerprint(c.Root)
	if before != after {
		t.Fatal("startup wrote state")
	}
}

func TestSetupSeedsLocalCatalogAndKeepsEdits(t *testing.T) {
	e, _ := testEngine(t)
	var out strings.Builder
	if err := e.cli(context.Background(), []string{"setup", "--headless", "--yes"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.cfg.Root, "catalog.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	specs := []harnessSpec{{ID: "custom", Name: "Custom", Command: "custom", DefaultHome: ".custom", Kind: "npm", Package: "custom"}}
	if err := writeJSON(path, specs); err != nil {
		t.Fatal(err)
	}
	before, _ := fingerprint(path)
	if err := e.cli(context.Background(), []string{"setup", "--headless", "--yes"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	after, _ := fingerprint(path)
	if before != after {
		t.Fatal("setup overwrote edited catalog")
	}
}

func TestSetupExplicitShellPathPreviewAndApproval(t *testing.T) {
	e, _ := testEngine(t)
	path := filepath.Join(e.cfg.Home, ".zshrc")
	original := "# keep user settings\n"
	if err := atomicWrite(path, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	args := []string{"setup", "--shell-file", path, "--headless", "--preview"}
	if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("preview wrote shell settings")
	}
	args[len(args)-1] = "--yes"
	if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(data), original) || !strings.Contains(string(data), shellQuote(e.cfg.BinDir)) {
		t.Fatal(string(data))
	}
	before := string(data)
	if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != before {
		t.Fatal("duplicate PATH setup")
	}
}
