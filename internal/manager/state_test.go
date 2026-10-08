package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectiveResetKeepsUnknownSettingsAndAuthentication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	before := []byte(`{"unknownSetting":{"nested":true},"auth":{"token":"synthetic"},"mcpServers":{"demo":{"command":"example"}},"hooks":{"example":[]}}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	r := resource{Path: path, Root: root, Category: settings, Owners: []string{"test"}, Format: "json", Fields: map[string]category{"unknownSetting": settings, "auth": auth, "mcpServers": mcp, "hooks": hooks}}
	keep := keepAll()
	keep[mcp] = false
	keep[hooks] = false
	if err := applyState(r, request{Preserve: keep}); err != nil {
		t.Fatal(err)
	}
	value, err := readConfig(path, "json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value["mcpServers"]; ok {
		t.Fatal("discarded MCP configuration survived")
	}
	if _, ok := value["hooks"]; ok {
		t.Fatal("discarded hooks survived")
	}
	if _, ok := value["auth"]; !ok {
		t.Fatal("preserved authentication was removed")
	}
	if _, ok := value["unknownSetting"]; !ok {
		t.Fatal("unrelated settings were removed")
	}
}

func TestSharedStateRequiresAllOwners(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	r := resource{Path: path, Root: root, Category: skills, Owners: []string{"codex", "claude"}}
	if err := applyState(r, request{Preserve: map[category]bool{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("shared resource was deleted without owner selection")
	}
	if err := applyState(r, request{Harness: "codex", Preserve: map[category]bool{}, Owners: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("approved shared reset did not remove resource")
	}
}

func TestSelectiveTOMLAndYAML(t *testing.T) {
	for _, format := range []string{"toml", "yaml"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config."+format)
			data, err := encodeConfig(format, map[string]any{"unknown": map[string]any{"nested": "preserved"}, "mcp_servers": map[string]any{"demo": map[string]any{"command": "example"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			keep := keepAll()
			keep[mcp] = false
			r := resource{Path: path, Root: root, Owners: []string{"test"}, Format: format, Fields: map[string]category{"unknown": settings, "mcp_servers": mcp}}
			if err := applyState(r, request{Preserve: keep}); err != nil {
				t.Fatal(err)
			}
			result, err := readConfig(path, format)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := result["mcp_servers"]; ok {
				t.Fatal("MCP section survived")
			}
			if _, ok := result["unknown"]; !ok {
				t.Fatal("unknown settings were lost")
			}
		})
	}
}

func TestNestedSettingsPreserveAuthentication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	if err := os.WriteFile(path, []byte(`{"security":{"auth":{"selectedType":"oauth"},"sandbox":true},"tools":{"mcpServers":{"x":{"command":"demo"}}},"max":9007199254740993}`), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := readConfig(path, "json")
	if err != nil {
		t.Fatal(err)
	}
	fields := classifyFields(value)
	keep := keepAll()
	keep[settings] = false
	keep[mcp] = false
	r := resource{Path: path, Root: root, Owners: []string{"gemini"}, Format: "json", Fields: fields}
	if err := applyState(r, request{Harness: "gemini", Preserve: keep}); err != nil {
		t.Fatal(err)
	}
	after, err := readConfig(path, "json")
	if err != nil {
		t.Fatal(err)
	}
	security, ok := after["security"].(map[string]any)
	if !ok || security["auth"] == nil {
		t.Fatal("nested auth was lost while resetting settings")
	}
	if _, ok := security["sandbox"]; ok {
		t.Fatal("discarded nested setting survived")
	}
}

func TestSelectiveResetKeepsPreservedEmptyMaps(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	value := map[string]any{"auth": map[string]any{}, "hooks": map[string]any{"example": "discard"}}
	data, err := encodeConfig("json", value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	keep := keepAll()
	keep[hooks] = false
	r := resource{Path: path, Root: root, Owners: []string{"pi"}, Format: "json", Fields: classifyFields(value)}
	if err = applyState(r, request{Harness: "pi", Preserve: keep}); err != nil {
		t.Fatal(err)
	}
	value, err = readConfig(path, "json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value["auth"]; !ok {
		t.Fatal("preserved empty auth map was pruned")
	}
}
