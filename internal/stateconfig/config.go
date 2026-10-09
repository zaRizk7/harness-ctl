// Package stateconfig owns native configuration codecs and state classification.
package stateconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
	"io"
	"path/filepath"
	"strings"
)

// Category identifies a preservation class shared by inventory, planning and recovery.
type Category string

const (
	Auth         Category = "auth"
	Settings     Category = "settings"
	Skills       Category = "skills"
	Plugins      Category = "plugins"
	Marketplaces Category = "marketplaces"
	Connectors   Category = "connectors"
	MCP          Category = "mcp"
	Hooks        Category = "hooks"
	Proxies      Category = "proxies"
	History      Category = "history"
	Memory       Category = "memory"
	Cache        Category = "cache"
	Other        Category = "other"
)

var Categories = []Category{Auth, Settings, Skills, Plugins, Marketplaces, Connectors, MCP, Hooks, Proxies, History, Memory, Cache, Other}

// CategoryFor returns a conservative whole-resource Category for name. Unknown
// resources remain explicitly classed as Other.
func CategoryFor(name string) Category {
	n := strings.ToLower(name)
	switch {
	case n == ".env" || strings.Contains(n, "auth") || strings.Contains(n, "credential") || strings.Contains(n, "oauth") || strings.Contains(n, "token"):
		return Auth
	case strings.Contains(n, "skill"):
		return Skills
	case strings.Contains(n, "plugin") || strings.Contains(n, "extension"):
		return Plugins
	case strings.Contains(n, "mcp"):
		return MCP
	case strings.Contains(n, "hook"):
		return Hooks
	case strings.Contains(n, "prox"):
		return Proxies
	case strings.Contains(n, "connector") || n == "channels" || n == "pairing" || strings.Contains(n, "integration"):
		return Connectors
	case strings.Contains(n, "memor") || n == "soul.md" || n == "user.md" || n == "agents.md":
		return Memory
	case strings.Contains(n, "session") || strings.Contains(n, "histor") || strings.Contains(n, "conversation") || strings.Contains(n, "transcript") || strings.Contains(n, "checkpoint"):
		return History
	case strings.Contains(n, "cache") || strings.Contains(n, "log") || n == "tmp" || n == "debug":
		return Cache
	case strings.Contains(n, "config") || strings.Contains(n, "setting") || strings.Contains(n, "prefer"):
		return Settings
	default:
		return Other
	}
}

// FieldCategory returns the Category for a native configuration key.
// Unrecognized fields remain Settings.
func FieldCategory(key string) Category {
	n := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	switch n {
	case "mcp", "mcpservers":
		return MCP
	case "skills", "skillpaths", "skillsources", "disabledskills":
		return Skills
	case "extraknownmarketplaces", "marketplaces":
		return Marketplaces
	case "packages", "plugin", "plugins", "extensions", "enabledplugins":
		return Plugins
	case "hooks", "disableallhooks":
		return Hooks
	case "apps", "connectors", "channels", "integrations", "pairing":
		return Connectors
	case "proxy", "httpproxy", "httpsproxy", "noproxy", "proxies":
		return Proxies
	case "apikey", "auth", "authentication", "credentials", "accesstoken", "refreshtoken", "oauth":
		return Auth
	case "memory", "memories":
		return Memory
	default:
		return Settings
	}
}

// ConfigFormat returns the supported structured format derived from path's
// extension, or an opaque format marker.
func ConfigFormat(path string) string {
	switch filepath.Ext(path) {
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".yaml", ".yml":
		return "yaml"
	case ".jsonc":
		return "jsonc"
	}
	return ""
}

// ClassifyFields returns configuration field pointers mapped to state
// Categories while retaining unknown Settings fields.
func ClassifyFields(value map[string]any) map[string]Category {
	fields := map[string]Category{}
	var walk func(map[string]any, string)
	walk = func(value map[string]any, prefix string) {
		for key, v := range value {
			path := prefix + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			cat := FieldCategory(key)
			nested, ok := v.(map[string]any)
			if cat == Settings && ok && len(nested) > 0 {
				walk(nested, path)
			} else {
				fields[path] = cat
			}
		}
	}
	walk(value, "")
	return fields
}

// FieldParent returns the object parent and key for path in value, with
// false for absent or non-object traversal.
func FieldParent(value map[string]any, path string) (map[string]any, string, bool) {
	if !strings.HasPrefix(path, "/") {
		return value, path, true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		nested, ok := value[part].(map[string]any)
		if !ok {
			return nil, "", false
		}
		value = nested
	}
	key := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "~1", "/"), "~0", "~")
	return value, key, true
}

// DeleteField removes path from value and prunes empty ancestors while retaining
// siblings. Missing fields leave value unchanged. It mutates value in place.
func DeleteField(value map[string]any, path string) {
	parent, key, ok := FieldParent(value, path)
	if !ok {
		return
	}
	delete(parent, key)
	if strings.HasPrefix(path, "/") {
		index := strings.LastIndex(path, "/")
		if index > 0 && len(parent) == 0 {
			DeleteField(value, path[:index])
		}
	}
}

// Decode returns data as one structured object using format. Invalid/opaque
// input returns an error rather than permitting selective mutation.
func Decode(data []byte, format string) (map[string]any, error) {
	var err error
	var value map[string]any
	switch format {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		err = decoder.Decode(&value)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = fmt.Errorf("configuration requires one JSON document")
			}
		}
	case "toml":
		err = toml.Unmarshal(data, &value)
	case "yaml":
		err = yaml.Unmarshal(data, &value)
	default:
		return nil, fmt.Errorf("selective edits are unavailable for %s", format)
	}
	if err != nil {
		return nil, err
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

// Encode returns value encoded in format, preserving its object
// structure. Unsupported formats return an error.
func Encode(format string, value map[string]any) ([]byte, error) {
	switch format {
	case "json":
		data, err := json.MarshalIndent(value, "", "  ")
		return append(data, '\n'), err
	case "toml":
		return toml.Marshal(value)
	case "yaml":
		return yaml.Marshal(value)
	}
	return nil, fmt.Errorf("unsupported configuration format")
}
