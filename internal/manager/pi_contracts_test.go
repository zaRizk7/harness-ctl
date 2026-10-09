package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPiSourceValidationAndNativePayload(t *testing.T) {
	for _, source := range []string{"git:repo", "npm:demo@", "npm:demo@^1", "npm:a/b/c", "npm:a/b", "npm:@scope/..", "npm:.."} {
		if _, _, err := piPackageName(source); err == nil {
			t.Fatal("unsafe package source", source)
		}
	}
	root := t.TempDir()
	settings := filepath.Join(root, "settings.json")
	manifest := filepath.Join(root, "npm", "node_modules", "demo", "package.json")
	if verifyPiPackage(root, "bad", "install") == nil || verifyPiPackage(root, "npm:demo@1", "install") == nil {
		t.Fatal("missing configuration accepted")
	}
	if err := writeJSON(settings, map[string]any{"packages": []any{map[string]any{"source": "npm:demo@1"}, false}}); err != nil {
		t.Fatal(err)
	}
	if verifyPiPackage(root, "npm:demo@1", "remove") == nil || verifyPiPackage(root, "npm:demo@2", "install") == nil {
		t.Fatal("wrong registry state accepted")
	}
	if err := verifyPiPackage(root, "npm:demo@2", "remove"); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(manifest, map[string]any{"name": "demo", "version": "1"}); err != nil {
		t.Fatal(err)
	}
	if verifyPiPackage(root, "npm:demo@1", "install") == nil {
		t.Fatal("ordinary npm package claimed as Pi extension")
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(manifest), "extensions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyPiPackage(root, "npm:demo@1", "install"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Dir(manifest)); err != nil {
		t.Fatal(err)
	}
	if verifyPiPackage(root, "npm:demo@1", "install") == nil {
		t.Fatal("external payload claimed")
	}
	if piPackageSource(map[string]any{"source": "npm:demo"}) != "npm:demo" || piPackageSource(false) != "" {
		t.Fatal("declaration identities")
	}
}

func TestAssetCompatibilityUnreadableAndAbsentManifests(t *testing.T) {
	root := t.TempDir()
	if detected, err := assetHarnesses(""); err != nil || len(detected) != 0 {
		t.Fatal(detected, err)
	}
	if _, err := assetHarnesses(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing asset accepted")
	}
	if err := atomicWrite(filepath.Join(root, "package.json"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := assetHarnesses(root); err == nil {
		t.Fatal("invalid manifest accepted")
	}
	if validateCompatibility("pi", componentRequest{Category: plugins, Source: root}) == nil {
		t.Fatal("invalid source accepted")
	}
	_ = os.Remove(filepath.Join(root, "package.json"))
	_ = atomicWrite(filepath.Join(root, "gemini-extension.json"), []byte(`{}`), 0600)
	if validateCompatibility("pi", componentRequest{Category: plugins, Source: root}) == nil {
		t.Fatal("foreign native format accepted")
	}
}
