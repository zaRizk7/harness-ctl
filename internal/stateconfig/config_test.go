package stateconfig

import (
	"reflect"
	"testing"
)

func TestCodecsAndClassificationPreserveUnknownSiblings(t *testing.T) {
	value := map[string]any{"unknown": map[string]any{"enabled": true}, "mcpServers": map[string]any{"demo": map[string]any{"command": "demo"}}}
	for _, format := range []string{"json", "toml", "yaml"} {
		data, err := Encode(format, value)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(data, format)
		if err != nil || !reflect.DeepEqual(value, decoded) {
			t.Fatal(decoded, err)
		}
		fields := ClassifyFields(decoded)
		if fields["/mcpServers"] != MCP || fields["/unknown/enabled"] != Settings {
			t.Fatal(fields)
		}
		DeleteField(decoded, "/mcpServers/demo")
		if _, exists := decoded["mcpServers"]; exists {
			t.Fatal(decoded)
		}
		if decoded["unknown"] == nil {
			t.Fatal("unknown settings lost")
		}
	}
	for name, want := range map[string]Category{"auth.json": Auth, "skills": Skills, "extensions": Plugins, "mcp.json": MCP, "hooks.json": Hooks, "proxies": Proxies, "connectors": Connectors, "memory": Memory, "sessions": History, "cache": Cache, "settings": Settings, "unrecognized": Other} {
		if CategoryFor(name) != want {
			t.Fatal(name)
		}
	}
	for name, want := range map[string]Category{"mcp_servers": MCP, "skills": Skills, "extraKnownMarketplaces": Marketplaces, "plugins": Plugins, "hooks": Hooks, "connectors": Connectors, "proxy": Proxies, "auth": Auth, "memory": Memory, "unknown": Settings} {
		if FieldCategory(name) != want {
			t.Fatal(name)
		}
	}
	for path, want := range map[string]string{"a.json": "json", "a.toml": "toml", "a.yaml": "yaml", "a.yml": "yaml", "a.jsonc": "jsonc", "opaque": ""} {
		if ConfigFormat(path) != want {
			t.Fatal(path)
		}
	}
	for _, tc := range []struct{ format, data string }{{"json", "{} {}"}, {"json", "bad"}, {"toml", "bad = ["}, {"yaml", "bad: ["}, {"opaque", "{}"}} {
		if _, err := Decode([]byte(tc.data), tc.format); err == nil {
			t.Fatal(tc)
		}
	}
	if _, err := Encode("opaque", value); err == nil {
		t.Fatal("opaque encoded")
	}
	if v, err := Decode([]byte("null"), "json"); err != nil || v == nil {
		t.Fatal(v, err)
	}
	if _, err := Encode("json", map[string]any{"invalid": make(chan int)}); err == nil {
		t.Fatal("invalid encoded")
	}
	if parent, key, ok := FieldParent(map[string]any{"literal": true}, "literal"); !ok || parent[key] != true {
		t.Fatal(parent, key, ok)
	}
	DeleteField(map[string]any{"scalar": true}, "/scalar/child")
}
