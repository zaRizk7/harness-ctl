package manager

import (
	"errors"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestStateClassificationAndPointerTraversal(t *testing.T) {
	for name, want := range map[string]category{"mcp.json": mcp, "proxy.json": proxies, "channels": connectors, "memory": memory, "sessions": history, "debug": cache} {
		if got := stateconfig.CategoryFor(name); got != want {
			t.Fatal(name, got)
		}
	}
	for name, want := range map[string]category{"extraKnownMarketplaces": marketplaces, "memories": memory} {
		if got := stateconfig.FieldCategory(name); got != want {
			t.Fatal(name, got)
		}
	}
	value := map[string]any{"scalar": 1, "nested": map[string]any{"a/b~c": true}}
	if _, _, ok := stateconfig.FieldParent(value, "/scalar/child"); ok {
		t.Fatal("scalar traversal accepted")
	}
	parent, key, ok := stateconfig.FieldParent(value, "/nested/a~1b~0c")
	if !ok || parent[key] != true {
		t.Fatal("escaped pointer not resolved")
	}
	stateconfig.DeleteField(value, "/absent/child")
	if len(value) != 2 {
		t.Fatal("absent pointer changed siblings")
	}
	if shouldChange(resource{Linked: true, Category: auth}, request{}) {
		t.Fatal("linked state can be discarded")
	}
	if stateconfig.ConfigFormat("settings.jsonc") != "jsonc" {
		t.Fatal("JSONC format lost")
	}
	path := filepath.Join(t.TempDir(), "empty.json")
	_ = os.WriteFile(path, []byte("null"), 0600)
	if value, err := readConfig(path, "json"); err != nil || value == nil || len(value) != 0 {
		t.Fatal(value, err)
	}
	if _, err := readConfig(path, "opaque"); err == nil {
		t.Fatal("opaque selective edit accepted")
	}
	if _, err := encodeConfig("opaque", value); err == nil {
		t.Fatal("opaque encoding accepted")
	}
}

func TestResourceInventoryProtectsRootsLinksAndSharedFiles(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	root := e.stateRoot(s)
	_ = atomicWrite(filepath.Join(root, "settings.json"), []byte(`{"mcpServers":{"one":{}}}`), 0600)
	_ = atomicWrite(filepath.Join(e.cfg.Home, ".claude.json"), []byte(`{"auth":{"token":"fixture"},"theme":"dark"}`), 0600)
	_ = atomicWrite(filepath.Join(root, "opaque.jsonc"), []byte("// comment\n{}"), 0600)
	s.ConfigFiles = append(s.ConfigFiles, "opaque.jsonc")
	_ = os.Symlink(filepath.Join(root, "settings.json"), filepath.Join(root, "linked.json"))
	other, _ := e.specFor("pi")
	_ = os.MkdirAll(e.stateRoot(other), 0700)
	_ = os.Symlink(filepath.Join(root, "settings.json"), filepath.Join(e.stateRoot(other), "settings.json"))
	resources, err := e.resources(s)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]resource{}
	for _, r := range resources {
		seen[filepath.Base(r.Path)] = r
	}
	if !seen["linked.json"].Linked || seen["opaque.jsonc"].Format != "jsonc" || !contains(seen["settings.json"].Owners, "pi") || len(seen[".claude.json"].Fields) != 2 {
		t.Fatal(seen)
	}
	for _, root := range []string{e.cfg.Home, "/", filepath.Dir(e.cfg.Root)} {
		e.cfg.StateRoots = map[string]string{s.ID: root}
		if _, err = e.resources(s); err == nil {
			t.Fatal("unsafe root accepted", root)
		}
	}
	e.cfg.StateRoots = map[string]string{s.ID: root}
	_ = os.WriteFile(filepath.Join(root, disabledComponentsDir), nil, 0600)
	_ = os.Remove(filepath.Join(root, disabledComponentsDir))
	_ = atomicWrite(filepath.Join(root, disabledComponentsDir, "unknown", "payload"), nil, 0600)
	if _, err = e.resources(s); err == nil {
		t.Fatal("unclassified parked state accepted")
	}
	_ = os.RemoveAll(filepath.Join(root, disabledComponentsDir))
	_ = atomicWrite(filepath.Join(root, disabledComponentsDir, string(skills), "payload"), nil, 0600)
	resources, err = e.resources(s)
	if err != nil {
		t.Fatal(err)
	}
	shared := false
	for _, r := range resources {
		if r.Category == skills && len(r.Owners) > 1 {
			shared = true
		}
	}
	if !shared {
		t.Fatal("parked native state lost shared ownership")
	}
}

func TestStateInventoryAndDiscardReturnBoundaryFailures(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("claude")
	root := e.stateRoot(s)
	shared := filepath.Join(e.cfg.Home, ".claude.json")
	_ = atomicWrite(shared, []byte(`{"theme":"dark"}`), 0600)
	for _, boundary := range []string{"shared-read", "shared-digest", "ancestors"} {
		t.Run(boundary, func(t *testing.T) {
			oldIO, oldDigest, oldAncestors := fileIO, fingerprint, rejectLinkedAncestors
			defer func() { fileIO, fingerprint, rejectLinkedAncestors = oldIO, oldDigest, oldAncestors }()
			fault := errors.New("inventory boundary")
			switch boundary {
			case "shared-read":
				fileIO.readFile = func(path string) ([]byte, error) {
					if path == shared {
						return nil, fault
					}
					return oldIO.readFile(path)
				}
			case "shared-digest":
				fingerprint = func(path string) (string, error) {
					if path == shared {
						return "", fault
					}
					return oldDigest(path)
				}
			case "ancestors":
				rejectLinkedAncestors = func(path string) error {
					if path == root {
						return fault
					}
					return oldAncestors(path)
				}
			}
			if _, err := e.resources(s); !errors.Is(err, fault) {
				t.Fatal(err)
			}
		})
	}
	path := filepath.Join(root, "settings.json")
	_ = atomicWrite(path, []byte(`{"mcpServers":{"one":{}},"theme":"dark"}`), 0600)
	r := resource{Path: path, Root: root, Fields: map[string]category{"/mcpServers": mcp, "/theme": settings}, Format: "json"}
	oldIO := fileIO
	defer func() { fileIO = oldIO }()
	fault := errors.New("state boundary")
	fileIO.readFile = func(string) ([]byte, error) { return nil, fault }
	if err := applyState(r, request{Preserve: map[category]bool{settings: true}}); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fileIO = oldIO
	r.Fields = nil
	fileIO.walk = func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, fault) }
	if err := applyState(r, request{}); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	fileIO.walk = func(string, fs.WalkDirFunc) error { return fault }
	if err := applyState(r, request{}); !errors.Is(err, fault) {
		t.Fatal(err)
	}
}
