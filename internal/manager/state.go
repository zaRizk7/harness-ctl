package manager

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func categoryFor(name string) category {
	n := strings.ToLower(name)
	switch {
	case n == ".env" || strings.Contains(n, "auth") || strings.Contains(n, "credential") || strings.Contains(n, "oauth") || strings.Contains(n, "token"):
		return auth
	case strings.Contains(n, "skill"):
		return skills
	case strings.Contains(n, "plugin") || strings.Contains(n, "extension"):
		return plugins
	case strings.Contains(n, "mcp"):
		return mcp
	case strings.Contains(n, "hook"):
		return hooks
	case strings.Contains(n, "prox"):
		return proxies
	case strings.Contains(n, "connector") || n == "channels" || n == "pairing" || strings.Contains(n, "integration"):
		return connectors
	case strings.Contains(n, "memor") || n == "soul.md" || n == "user.md" || n == "agents.md":
		return memory
	case strings.Contains(n, "session") || strings.Contains(n, "histor") || strings.Contains(n, "conversation") || strings.Contains(n, "transcript") || strings.Contains(n, "checkpoint"):
		return history
	case strings.Contains(n, "cache") || strings.Contains(n, "log") || n == "tmp" || n == "debug":
		return cache
	case strings.Contains(n, "config") || strings.Contains(n, "setting") || strings.Contains(n, "prefer"):
		return settings
	default:
		return other
	}
}

func fieldCategory(key string) category {
	n := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	switch n {
	case "mcp", "mcpservers":
		return mcp
	case "skills", "skillpaths", "skillsources", "disabledskills":
		return skills
	case "plugin", "plugins", "extensions", "enabledplugins", "extraknownmarketplaces":
		return plugins
	case "hooks", "disableallhooks":
		return hooks
	case "apps", "connectors", "channels", "integrations", "pairing":
		return connectors
	case "proxy", "httpproxy", "httpsproxy", "noproxy", "proxies":
		return proxies
	case "apikey", "auth", "authentication", "credentials", "accesstoken", "refreshtoken", "oauth":
		return auth
	case "memory", "memories":
		return memory
	default:
		return settings
	}
}

func readConfig(path, format string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	switch format {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		err = decoder.Decode(&value)
	case "toml":
		err = toml.Unmarshal(data, &value)
	case "yaml":
		err = yaml.Unmarshal(data, &value)
	default:
		return nil, fmt.Errorf("selective edits are unavailable for %s", format)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot safely parse %s: %w", filepath.Base(path), err)
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

func encodeConfig(format string, value map[string]any) ([]byte, error) {
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

func configFormat(path string) string {
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

func resourceCategories(r resource) []category {
	if len(r.Fields) == 0 {
		return []category{r.Category}
	}
	var out []category
	for _, cat := range categories {
		for _, value := range r.Fields {
			if value == cat {
				out = append(out, cat)
				break
			}
		}
	}
	return out
}

func classifyFields(value map[string]any) map[string]category {
	fields := map[string]category{}
	var walk func(map[string]any, string)
	walk = func(value map[string]any, prefix string) {
		for key, v := range value {
			path := prefix + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			cat := fieldCategory(key)
			nested, ok := v.(map[string]any)
			if cat == settings && ok && len(nested) > 0 {
				walk(nested, path)
			} else {
				fields[path] = cat
			}
		}
	}
	walk(value, "")
	return fields
}

func valueDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func fieldDigests(value map[string]any, fields map[string]category) map[string]string {
	digests := map[string]string{}
	for path := range fields {
		parent, key, ok := fieldParent(value, path)
		if ok {
			digests[path] = valueDigest(parent[key])
		}
	}
	return digests
}

func fieldParent(value map[string]any, path string) (map[string]any, string, bool) {
	if !strings.HasPrefix(path, "/") {
		return value, path, true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		if i == len(parts)-1 {
			return value, part, true
		}
		nested, ok := value[part].(map[string]any)
		if !ok {
			return nil, "", false
		}
		value = nested
	}
	return nil, "", false
}

func deleteField(value map[string]any, path string) {
	parent, key, ok := fieldParent(value, path)
	if !ok {
		return
	}
	delete(parent, key)
	if strings.HasPrefix(path, "/") {
		index := strings.LastIndex(path, "/")
		if index > 0 && len(parent) == 0 {
			deleteField(value, path[:index])
		}
	}
}

func (e *engine) resources(s harnessSpec) ([]resource, error) {
	var result []resource
	for _, root := range e.rootsFor(s) {
		if root == e.cfg.Home || root == "/" || within(root, e.cfg.Root) {
			return nil, fmt.Errorf("unsafe state root")
		}
		if err := rejectLinkedAncestors(root); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == disabledComponentsDir {
				if err := validateOwnedPath(root, filepath.Join(root, name)); err != nil {
					return nil, err
				}
				parked, err := os.ReadDir(filepath.Join(root, name))
				if err != nil {
					return nil, err
				}
				for _, group := range parked {
					cat := category(group.Name())
					if !knownCategory(cat) || !group.IsDir() {
						return nil, fmt.Errorf("unclassified disabled component state")
					}
					r := resource{Path: filepath.Join(root, name, group.Name()), Root: root, Category: cat, Owners: []string{s.ID}}
					if len(s.SharedClients) > 0 && !within(e.cfg.Root, root) {
						r.Owners = append(r.Owners, s.SharedClients...)
					}
					r.Digest, err = fingerprint(r.Path)
					if err != nil {
						return nil, err
					}
					result = append(result, r)
				}
				continue
			}
			// Runtime payloads, source checkouts and workspaces have independent
			// ownership. A reset must never remove them as generic user state.
			if name == "packages" || name == "hermes-agent" || name == "tools" || name == "installs" || name == "node_modules" || name == "bin" || name == ".git" || name == "workspace" || strings.HasPrefix(name, "workspace-") {
				continue
			}
			r := resource{Path: filepath.Join(root, name), Root: root, Category: categoryFor(name), Owners: []string{s.ID}}
			info, err := os.Lstat(r.Path)
			if err != nil {
				return nil, err
			}
			r.Linked = info.Mode()&os.ModeSymlink != 0
			if contains(s.ConfigFiles, name) && !r.Linked {
				r.Format = configFormat(r.Path)
				if r.Format != "jsonc" {
					fields, err := readConfig(r.Path, r.Format)
					if err != nil {
						return nil, err
					}
					r.Fields = classifyFields(fields)
					r.FieldDigests = fieldDigests(fields, r.Fields)
				}
				if r.Format == "jsonc" {
					r.Note = "JSONC is preserved or discarded as one settings resource."
				}
			}
			if len(s.SharedClients) > 0 && !within(e.cfg.Root, root) {
				r.Owners = append(r.Owners, s.SharedClients...)
				r.Note = "Shared with other clients of this product."
			}
			if info.Mode()&os.ModeSymlink != 0 {
				r.Linked = true
				r.Note = "Linked resource. Its shared target is protected."
			}
			for _, otherSpec := range catalog {
				if otherSpec.ID == s.ID {
					continue
				}
				candidate := filepath.Join(e.stateRoot(otherSpec), name)
				a, ea := filepath.EvalSymlinks(r.Path)
				b, eb := filepath.EvalSymlinks(candidate)
				if ea == nil && eb == nil && a == b && !contains(r.Owners, otherSpec.ID) {
					r.Owners = append(r.Owners, otherSpec.ID)
				}
			}
			digest, err := fingerprint(r.Path)
			if err != nil {
				return nil, err
			}
			r.Digest = digest
			result = append(result, r)
		}
	}
	if s.ID == "claude" && !within(e.cfg.Root, e.stateRoot(s)) {
		path := filepath.Join(e.cfg.Home, ".claude.json")
		if _, err := os.Stat(path); err == nil {
			r := resource{Path: path, Root: e.cfg.Home, Category: settings, Owners: append([]string{s.ID}, s.SharedClients...), Format: "json", Note: "Shared Claude client configuration."}
			fields, err := readConfig(path, "json")
			if err != nil {
				return nil, err
			}
			r.Fields = classifyFields(fields)
			r.FieldDigests = fieldDigests(fields, r.Fields)
			r.Digest, err = fingerprint(path)
			if err != nil {
				return nil, err
			}
			result = append(result, r)
		}
	}
	if s.ID == "codex" {
		skills, err := e.codexSkillResources()
		if err != nil {
			return nil, err
		}
		result = append(result, skills...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func shouldChange(r resource, req request) bool {
	if r.Linked {
		return false
	}
	if !ownersSelected(r.Owners, req) {
		return false
	}
	for _, cat := range resourceCategories(r) {
		if !req.Preserve[cat] {
			return true
		}
	}
	return false
}

func ownersSelected(owners []string, req request) bool {
	if len(owners) <= 1 {
		return true
	}
	for _, owner := range owners {
		if owner != req.Harness && !contains(req.Owners, owner) {
			return false
		}
	}
	return true
}

func applyState(r resource, req request) error {
	if !shouldChange(r, req) {
		return nil
	}
	if err := validateOwnedPath(r.Root, r.Path); err != nil {
		return err
	}
	if len(r.Fields) > 0 {
		value, err := readConfig(r.Path, r.Format)
		if err != nil {
			return err
		}
		for path, cat := range r.Fields {
			if !req.Preserve[cat] {
				deleteField(value, path)
			}
		}
		if len(value) == 0 {
			return os.Remove(r.Path)
		}
		data, err := encodeConfig(r.Format, value)
		if err != nil {
			return err
		}
		return atomicWrite(r.Path, data, 0600)
	}
	// WalkDir does not follow links. Validate every child before removal so a
	// directory with external links is reported rather than partially deleted.
	if err := filepath.WalkDir(r.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return validateOwnedPath(r.Root, path)
	}); err != nil {
		return err
	}
	return os.RemoveAll(r.Path)
}
