package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const disabledComponentsDir = ".harness-ctl-disabled"

var componentCategories = []category{skills, mcp, plugins, connectors, proxies, hooks, settings, memory, other}

// Modern Codex user skills follow HOME, independently of CODEX_HOME. Profiles
// have a private HOME, while base managed launches inherit the user's HOME.
func (e *engine) codexSkillsRoot() string {
	s, _ := specFor("codex")
	root := e.stateRoot(s)
	if within(filepath.Join(e.cfg.Root, "profiles"), root) {
		return filepath.Join(root, "home", ".agents")
	}
	return filepath.Join(e.cfg.Home, ".agents")
}

func (e *engine) codexSkillResources() ([]resource, error) {
	root := e.codexSkillsRoot()
	var result []resource
	for _, path := range []string{filepath.Join(root, "skills"), filepath.Join(root, disabledComponentsDir, string(skills))} {
		if err := validateOwnedPath(root, path); err != nil {
			return nil, err
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		r := resource{Path: path, Root: root, Category: skills, Owners: []string{"codex"}, Note: "Codex HOME skill source."}
		if !within(e.cfg.Root, root) {
			r.Owners = append(r.Owners, "Codex desktop / IDE", "Other agents using ~/.agents/skills")
		}
		var err error
		r.Digest, err = fingerprint(path)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

// A component is a native registration or a local file asset. Values are never
// part of the inventory display. Disabled registrations retain their full data.
type componentItem struct {
	Name, Path, Field string
	Subpath           string
	Category          category
	Owners            []string
	Disabled          bool
	Parked            string
	Directory         bool
	Native            bool
}

type componentRequest struct {
	Operation string          `json:"operation"`
	Category  category        `json:"category"`
	Path      string          `json:"path,omitempty"`
	Field     string          `json:"field,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
	Source    string          `json:"source,omitempty"`
	Name      string          `json:"name,omitempty"`
	Native    bool            `json:"native,omitempty"`
	Scope     string          `json:"scope,omitempty"`
	Text      string          `json:"text,omitempty"`
	Parked    string          `json:"-"`
	Subpath   string          `json:"-"`
	Content   []byte          `json:"-"`
}

type parkedComponent struct {
	Path     string          `json:"path"`
	Field    string          `json:"field,omitempty"`
	Category category        `json:"category"`
	Value    json.RawMessage `json:"value,omitempty"`
}

type componentWrite struct {
	Path, Root, Source string
	SourceDigest       string
	Data               []byte
	Remove             bool
	Mode               fs.FileMode
}

type componentMutation struct {
	Request componentRequest
	Writes  []componentWrite
	Native  bool
	Digest  string
}

func (e *engine) componentEngine(inst installation, scope string) (*engine, harnessSpec, error) {
	s, err := specFor(inst.Harness)
	if err != nil {
		return nil, s, err
	}
	copyEngine := *e
	copyEngine.cfg.StateRoots = map[string]string{}
	for id, root := range e.cfg.StateRoots {
		copyEngine.cfg.StateRoots[id] = root
	}
	if inst.Managed {
		copyEngine.cfg.StateRoots[s.ID] = inst.StateRoot
	}
	if scope == "profile" {
		prof, ok := e.reg.Profiles[inst.ID]
		if !ok {
			return nil, s, fmt.Errorf("there is no active launch profile")
		}
		copyEngine.cfg.StateRoots[s.ID] = nativeStateRoot(s, prof.Root)
	} else if scope != "" && scope != "base" {
		return nil, s, fmt.Errorf("unknown component scope")
	}
	return &copyEngine, s, nil
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func pointerParts(pointer string) ([]string, error) {
	if !strings.HasPrefix(pointer, "/") || pointer == "/" {
		return nil, fmt.Errorf("a component field requires a non-empty JSON pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func pointerValue(node any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return node, true
	}
	switch value := node.(type) {
	case map[string]any:
		next, exists := value[parts[0]]
		if exists {
			return pointerValue(next, parts[1:])
		}
	case []any:
		index, err := strconv.Atoi(parts[0])
		if err == nil && index >= 0 && index < len(value) {
			return pointerValue(value[index], parts[1:])
		}
	}
	return nil, false
}

// mutatePointer preserves siblings and rejects traversal through scalar values.
func mutatePointer(node any, parts []string, replacement any, remove bool) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("cannot replace the complete configuration through a field edit")
	}
	key := parts[0]
	switch value := node.(type) {
	case map[string]any:
		if len(parts) == 1 {
			if remove {
				delete(value, key)
			} else {
				value[key] = replacement
			}
			return value, nil
		}
		next, exists := value[key]
		if !exists && !remove {
			next = map[string]any{}
		}
		updated, err := mutatePointer(next, parts[1:], replacement, remove)
		if err != nil {
			return nil, err
		}
		value[key] = updated
		return value, nil
	case []any:
		if key == "-" && len(parts) == 1 && !remove {
			return append(value, replacement), nil
		}
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(value) {
			return nil, fmt.Errorf("invalid component array index")
		}
		if len(parts) == 1 {
			if remove {
				return append(value[:index], value[index+1:]...), nil
			}
			value[index] = replacement
			return value, nil
		}
		updated, err := mutatePointer(value[index], parts[1:], replacement, remove)
		value[index] = updated
		return value, err
	}
	return nil, fmt.Errorf("component field crosses a non-container value")
}

func decodeComponentValue(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing component value")
	}
	var normalize func(any) (any, error)
	normalize = func(value any) (any, error) {
		switch v := value.(type) {
		case json.Number:
			if integer, err := v.Int64(); err == nil {
				return integer, nil
			}
			return v.Float64()
		case map[string]any:
			for key, child := range v {
				result, err := normalize(child)
				if err != nil {
					return nil, err
				}
				v[key] = result
			}
		case []any:
			for i, child := range v {
				result, err := normalize(child)
				if err != nil {
					return nil, err
				}
				v[i] = result
			}
		}
		return value, nil
	}
	return normalize(value)
}

func (e *engine) components(inst installation, scope string, cat category) ([]componentItem, error) {
	state, s, err := e.componentEngine(inst, scope)
	if err != nil {
		return nil, err
	}
	resources, err := state.resources(s)
	if err != nil {
		return nil, err
	}
	var items []componentItem
	for _, r := range resources {
		if within(filepath.Join(r.Root, disabledComponentsDir), r.Path) {
			if r.Category != cat {
				continue
			}
			entries, err := os.ReadDir(r.Path)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				parked := filepath.Join(r.Path, entry.Name())
				if err := validateOwnedPath(r.Root, filepath.Join(parked, "meta.json")); err != nil {
					return nil, err
				}
				var record parkedComponent
				if err := readJSON(filepath.Join(parked, "meta.json"), &record); err != nil {
					return nil, err
				}
				if record.Category != cat {
					return nil, fmt.Errorf("disabled component category mismatch")
				}
				if _, err := state.componentResource(s, record.Path, record.Field, cat); err != nil {
					return nil, err
				}
				name := filepath.Base(record.Path)
				if record.Field != "" {
					name = record.Field
				}
				item := componentItem{Name: name, Path: record.Path, Field: record.Field, Category: cat, Owners: r.Owners, Disabled: true, Parked: parked}
				if record.Field == "" {
					info, err := os.Lstat(filepath.Join(parked, "payload"))
					if err != nil {
						return nil, err
					}
					item.Directory = info.IsDir()
				}
				items = append(items, item)
			}
			continue
		}
		if len(r.Fields) > 0 {
			config, err := readConfig(r.Path, r.Format)
			if err != nil {
				return nil, err
			}
			for _, field := range sortedKeys(r.Fields) {
				if r.Fields[field] != cat {
					continue
				}
				parts, _ := pointerParts(field)
				value, _ := pointerValue(config, parts)
				add := func(name, pointer string, value any) {
					item := componentItem{Name: name, Path: r.Path, Field: pointer, Category: cat, Owners: r.Owners}
					if registration, ok := value.(map[string]any); ok && componentEnabledFlag(s.ID, cat, pointer) {
						if enabled, ok := registration["enabled"].(bool); ok {
							item.Disabled = !enabled
						}
					}
					if s.ID == "claude" && strings.HasPrefix(pointer, "/enabledPlugins/") {
						item.Native = true
						if enabled, ok := value.(bool); ok {
							item.Disabled = !enabled
						}
					}
					items = append(items, item)
				}
				switch group := value.(type) {
				case map[string]any:
					for _, name := range sortedKeys(group) {
						if entries, ok := group[name].([]any); ok && cat == hooks {
							for i, entry := range entries {
								add(fmt.Sprintf("%s [%d]", name, i), field+"/"+escapePointer(name)+"/"+strconv.Itoa(i), entry)
							}
							continue
						}
						add(name, field+"/"+escapePointer(name), group[name])
					}
				case []any:
					for i, child := range group {
						add(fmt.Sprintf("%s [%d]", field, i), field+"/"+strconv.Itoa(i), child)
					}
				default:
					add(field, field, value)
				}
			}
			continue
		}
		if r.Category != cat {
			continue
		}
		info, err := os.Lstat(r.Path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() && !r.Linked {
			entries, err := os.ReadDir(r.Path)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				path := filepath.Join(r.Path, entry.Name())
				if s.ID == "claude" && cat == plugins && filepath.Base(r.Path) == "plugins" {
					continue
				}
				if s.ID == "gemini" && cat == plugins && entry.Name() == "extension-enablement.json" {
					continue
				}
				item := componentItem{Name: entry.Name(), Path: path, Category: cat, Owners: r.Owners, Directory: entry.IsDir()}
				if s.ID == "gemini" && cat == plugins && entry.IsDir() {
					var manifest struct {
						Name string `json:"name"`
					}
					if readJSON(filepath.Join(path, "gemini-extension.json"), &manifest) == nil && manifest.Name != "" {
						item.Name, item.Native = manifest.Name, true
						item.Disabled, err = geminiExtensionDisabled(state.stateRoot(s), item.Name)
						if err != nil {
							return nil, err
						}
					}
				}
				items = append(items, item)
			}
		} else {
			items = append(items, componentItem{Name: filepath.Base(r.Path), Path: r.Path, Category: cat, Owners: r.Owners})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (e *engine) componentResource(s harnessSpec, path, field string, cat category) (resource, error) {
	path = filepath.Clean(path)
	r := resource{Path: path, Category: cat, Owners: []string{s.ID}}
	for _, root := range e.rootsFor(s) {
		if within(root, path) && path != root {
			r.Root = root
			break
		}
	}
	if s.ID == "claude" && path == filepath.Join(e.cfg.Home, ".claude.json") && !within(e.cfg.Root, e.stateRoot(s)) {
		r.Root = e.cfg.Home
	}
	if s.ID == "codex" && cat == skills && within(filepath.Join(e.codexSkillsRoot(), "skills"), path) {
		r.Root = e.codexSkillsRoot()
	}
	if r.Root == "" || within(filepath.Join(r.Root, disabledComponentsDir), path) {
		return r, fmt.Errorf("component target is outside selected native state")
	}
	if err := validateOwnedPath(r.Root, path); err != nil {
		return r, err
	}
	if field != "" {
		parts, err := pointerParts(field)
		if err != nil {
			return r, err
		}
		fieldCat := settings
		for _, part := range parts {
			if fieldCategory(part) != settings {
				fieldCat = fieldCategory(part)
				break
			}
		}
		knownConfig := false
		for _, file := range s.ConfigFiles {
			knownConfig = knownConfig || path == filepath.Join(e.stateRoot(s), file)
		}
		if s.ID == "claude" && path == filepath.Join(e.cfg.Home, ".claude.json") {
			knownConfig = true
		}
		if fieldCat != cat || !knownConfig {
			return r, fmt.Errorf("component field is not in a known native configuration for this category")
		}
		r.Format = configFormat(path)
		if r.Format != "json" && r.Format != "toml" && r.Format != "yaml" {
			return r, fmt.Errorf("use a structured JSON, TOML or YAML configuration")
		}
		value, err := readConfig(path, r.Format)
		if err != nil && !os.IsNotExist(err) {
			return r, err
		}
		for classified, owner := range classifyFields(value) {
			if owner != cat && (classified == field || strings.HasPrefix(classified, field+"/") || strings.HasPrefix(field, classified+"/")) {
				return r, fmt.Errorf("component field crosses another state category")
			}
		}
	} else {
		rel, _ := filepath.Rel(r.Root, path)
		first := strings.Split(rel, string(filepath.Separator))[0]
		if categoryFor(first) != cat || first == "packages" || first == "hermes-agent" || first == "tools" || first == "bin" || first == "workspace" || first == ".git" || first == "node_modules" {
			return r, fmt.Errorf("component asset does not match this category or is a runtime/workspace")
		}
	}
	if len(s.SharedClients) > 0 && !within(e.cfg.Root, r.Root) {
		r.Owners = append(r.Owners, s.SharedClients...)
	}
	if s.ID == "codex" && r.Root == e.codexSkillsRoot() && !within(e.cfg.Root, r.Root) {
		r.Owners = append(r.Owners, "Other agents using ~/.agents/skills")
	}
	digest, err := fingerprint(path)
	r.Digest = digest
	return r, err
}

func (e *engine) planComponent(p *plan) error {
	if p.Request.Component == nil {
		return fmt.Errorf("missing component request")
	}
	change := *p.Request.Component
	change.Value = append(json.RawMessage{}, change.Value...)
	change.Content = append([]byte{}, change.Content...)
	if !slices.Contains(componentCategories, change.Category) {
		return fmt.Errorf("unsupported management category")
	}
	if !contains([]string{"add", "install", "edit", "enable", "disable", "remove"}, change.Operation) {
		return fmt.Errorf("unsupported component action")
	}
	if change.Subpath != "" && (change.Operation != "edit" || change.Parked == "") {
		return fmt.Errorf("disabled component child files support editing only")
	}
	p.Component = &componentMutation{Request: change}
	state, _, err := e.componentEngine(p.Install, change.Scope)
	if err != nil {
		return err
	}
	if change.Native {
		return state.planNativeComponent(p, change)
	}
	r, err := state.componentResource(p.Spec, change.Path, change.Field, change.Category)
	if err != nil {
		return err
	}
	if !ownersSelected(r.Owners, p.Request) {
		p.Blockers = append(p.Blockers, "Select every affected owner before changing this shared component.")
	}
	addResource := func(r resource) {
		for _, existing := range p.Resources {
			if existing.Path == r.Path {
				return
			}
		}
		p.Resources = append(p.Resources, r)
	}
	addResource(r)
	parkRoot := p.StateRoot
	if p.Spec.ID == "codex" && r.Root == state.codexSkillsRoot() {
		parkRoot = r.Root
	}
	parked := filepath.Join(parkRoot, disabledComponentsDir, string(change.Category), installID("component", r.Path+"\x00"+change.Field))
	if change.Parked != "" {
		if filepath.Dir(change.Parked) != filepath.Join(parkRoot, disabledComponentsDir, string(change.Category)) {
			return fmt.Errorf("disabled component storage is outside this scope")
		}
		parked = change.Parked
	} else if change.Operation == "disable" && change.Field != "" {
		value, err := readConfig(r.Path, r.Format)
		if err != nil {
			return err
		}
		parts, _ := pointerParts(change.Field)
		if parent, exists := pointerValue(value, parts[:len(parts)-1]); exists {
			if _, array := parent.([]any); array {
				parked += "-" + shortID(p.ID)
			}
		}
	}
	if err := validateOwnedPath(parkRoot, filepath.Join(parked, "meta.json")); err != nil {
		return err
	}
	parkResource := resource{Path: parked, Root: parkRoot, Category: change.Category, Owners: r.Owners}
	parkResource.Digest, err = fingerprint(parked)
	if err != nil {
		return err
	}
	addResource(parkResource)
	write := func(path, root string, data []byte, remove bool) {
		p.Component.Writes = append(p.Component.Writes, componentWrite{Path: path, Root: root, Data: data, Remove: remove, Mode: 0600})
	}
	var record parkedComponent
	parkExists := false
	if err := readJSON(filepath.Join(parked, "meta.json"), &record); err == nil {
		parkExists = true
		if record.Path != r.Path || record.Field != change.Field || record.Category != change.Category {
			return fmt.Errorf("disabled component identity mismatch")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if change.Parked != "" && !parkExists {
		return fmt.Errorf("disabled component no longer exists")
	}
	if change.Field != "" {
		value, err := readConfig(r.Path, r.Format)
		if os.IsNotExist(err) {
			value, err = map[string]any{}, nil
		}
		if err != nil {
			return err
		}
		parts, _ := pointerParts(change.Field)
		current, exists := pointerValue(value, parts)
		parent, _ := pointerValue(value, parts[:len(parts)-1])
		array, isArray := parent.([]any)
		if registration, ok := current.(map[string]any); ok && !parkExists && componentEnabledFlag(p.Spec.ID, change.Category, change.Field) && (change.Operation == "enable" || change.Operation == "disable") {
			registration["enabled"] = change.Operation == "enable"
			data, err := encodeConfig(r.Format, value)
			if err != nil {
				return err
			}
			write(r.Path, r.Root, data, false)
			return nil
		}
		// A parked array item keeps its original position, which can now be
		// occupied by a different active item. Never edit that active sibling.
		selectedParked := parkExists && (change.Parked != "" || !exists)
		if selectedParked && (change.Operation == "edit" || change.Operation == "remove") {
			if change.Operation == "edit" {
				if _, err := decodeComponentValue(change.Value); err != nil {
					return fmt.Errorf("invalid component value: %w", err)
				}
				record.Value = append(json.RawMessage{}, change.Value...)
				data, err := json.Marshal(record)
				if err != nil {
					return err
				}
				write(filepath.Join(parked, "meta.json"), p.StateRoot, data, false)
			} else {
				write(parked, p.StateRoot, nil, true)
				if isArray {
					index, _ := strconv.Atoi(parts[len(parts)-1])
					if err := rebaseDisabledArray(p, filepath.Dir(parked), r.Path, parts[:len(parts)-1], index, parked); err != nil {
						return err
					}
				}
			}
			return nil
		}
		remove := false
		var replacement any
		switch change.Operation {
		case "add", "install":
			if exists || parkExists {
				return fmt.Errorf("component already exists. Choose edit or enable")
			}
			replacement, err = decodeComponentValue(change.Value)
		case "edit":
			if !exists && !parkExists {
				return fmt.Errorf("component no longer exists")
			}
			replacement, err = decodeComponentValue(change.Value)
		case "disable":
			if !exists || parkExists {
				return fmt.Errorf("component is missing or already disabled")
			}
			record = parkedComponent{Path: r.Path, Field: change.Field, Category: change.Category}
			if isArray {
				index, _ := strconv.Atoi(parts[len(parts)-1])
				indices, err := disabledArrayIndices(filepath.Dir(parked), r.Path, parts[:len(parts)-1], "")
				if err != nil {
					return err
				}
				for _, disabled := range indices {
					if disabled <= index {
						index++
					}
				}
				record.Field = change.Field[:strings.LastIndex(change.Field, "/")+1] + strconv.Itoa(index)
			}
			record.Value, err = json.Marshal(current)
			data, marshalErr := json.Marshal(record)
			if err != nil || marshalErr != nil {
				return fmt.Errorf("cannot retain disabled configuration")
			}
			write(filepath.Join(parked, "meta.json"), p.StateRoot, data, false)
			remove = true
		case "enable":
			if exists && !isArray || !parkExists {
				return fmt.Errorf("active component exists or disabled state is missing")
			}
			replacement, err = decodeComponentValue(record.Value)
			write(parked, p.StateRoot, nil, true)
		case "remove":
			if !exists && !parkExists {
				return fmt.Errorf("component no longer exists")
			}
			if parkExists {
				write(parked, p.StateRoot, nil, true)
			}
			remove = true
			if isArray {
				index, _ := strconv.Atoi(parts[len(parts)-1])
				indices, err := disabledArrayIndices(filepath.Dir(parked), r.Path, parts[:len(parts)-1], "")
				if err != nil {
					return err
				}
				for _, disabled := range indices {
					if disabled <= index {
						index++
					}
				}
				if err := rebaseDisabledArray(p, filepath.Dir(parked), r.Path, parts[:len(parts)-1], index, ""); err != nil {
					return err
				}
			}
		}
		if err != nil {
			return fmt.Errorf("invalid component value: %w", err)
		}
		if change.Operation == "enable" && isArray {
			index, _ := strconv.Atoi(parts[len(parts)-1])
			indices, err := disabledArrayIndices(filepath.Dir(parked), r.Path, parts[:len(parts)-1], parked)
			if err != nil {
				return err
			}
			for _, disabled := range indices {
				original, _ := strconv.Atoi(parts[len(parts)-1])
				if disabled < original {
					index--
				}
			}
			if index < 0 || index > len(array) {
				return fmt.Errorf("disabled array position no longer fits. Edit the native collection before enabling")
			}
			array = append(array, nil)
			copy(array[index+1:], array[index:])
			array[index] = replacement
			if _, err := mutatePointer(value, parts[:len(parts)-1], array, false); err != nil {
				return err
			}
		} else if _, err := mutatePointer(value, parts, replacement, remove); err != nil {
			return err
		}
		data, err := encodeConfig(r.Format, value)
		if err != nil {
			return err
		}
		write(r.Path, r.Root, data, false)
	} else {
		info, statErr := os.Lstat(r.Path)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		switch change.Operation {
		case "add", "install":
			if exists || parkExists {
				return fmt.Errorf("component already exists")
			}
			if change.Source == "" {
				data := []byte(change.Text)
				if err := validateComponentContent(r.Path, data); err != nil {
					return err
				}
				write(r.Path, r.Root, data, false)
			} else {
				if !filepath.IsAbs(change.Source) || within(change.Source, r.Path) || within(r.Path, change.Source) {
					return fmt.Errorf("import source must be an independent absolute local path")
				}
				if err := validateComponentTree(change.Source); err != nil {
					return err
				}
				p.RootDigests[change.Source], err = fingerprint(change.Source)
				if err != nil {
					return err
				}
				p.Component.Writes = append(p.Component.Writes, componentWrite{Path: r.Path, Root: r.Root, Source: change.Source})
			}
		case "edit":
			target := r.Path
			root := r.Root
			if parkExists && (change.Parked != "" || !exists) {
				target, root = filepath.Join(parked, "payload"), parkRoot
				if change.Subpath != "" {
					if !filepath.IsLocal(change.Subpath) {
						return fmt.Errorf("disabled asset child path is outside its payload")
					}
					target = filepath.Join(target, change.Subpath)
					if err := validateOwnedPath(filepath.Join(parked, "payload"), target); err != nil {
						return err
					}
				}
				info, statErr = os.Lstat(target)
			}
			if statErr != nil || info.IsDir() {
				return fmt.Errorf("select a regular file inside this component to edit")
			}
			validationPath := r.Path
			if change.Subpath != "" {
				validationPath = change.Subpath
			}
			if err := validateComponentContent(validationPath, change.Content); err != nil {
				return err
			}
			write(target, root, change.Content, false)
			p.Component.Writes[len(p.Component.Writes)-1].Mode = info.Mode().Perm()
		case "disable":
			if !exists || parkExists {
				return fmt.Errorf("component is missing or already disabled")
			}
			if err := validateComponentTree(r.Path); err != nil {
				return err
			}
			p.Component.Writes = append(p.Component.Writes, componentWrite{Path: filepath.Join(parked, "payload"), Root: parkRoot, Source: r.Path})
			data, _ := json.Marshal(parkedComponent{Path: r.Path, Category: change.Category})
			write(filepath.Join(parked, "meta.json"), parkRoot, data, false)
			write(r.Path, r.Root, nil, true)
		case "enable":
			if exists || !parkExists {
				return fmt.Errorf("active component exists or disabled state is missing")
			}
			payload := filepath.Join(parked, "payload")
			if err := validateComponentTree(payload); err != nil {
				return err
			}
			p.Component.Writes = append(p.Component.Writes, componentWrite{Path: r.Path, Root: r.Root, Source: payload})
			write(parked, parkRoot, nil, true)
		case "remove":
			if exists && change.Parked == "" {
				if err := validateComponentTree(r.Path); err != nil {
					return err
				}
				write(r.Path, r.Root, nil, true)
			}
			if parkExists {
				write(parked, parkRoot, nil, true)
			}
			if !exists && !parkExists {
				return fmt.Errorf("component no longer exists")
			}
		}
	}
	for i := range p.Component.Writes {
		write := &p.Component.Writes[i]
		if write.Source != "" {
			write.SourceDigest, err = componentContentDigest(write.Source)
			if err != nil {
				return err
			}
			p.RootDigests[write.Source], err = fingerprint(write.Source)
			if err != nil {
				return err
			}
		}
	}
	p.Warnings = append(p.Warnings, "This changes only the selected local source. Project/system sources, remote account authorization and inherited environment values retain native behavior.")
	return nil
}

// Only documented registration-level flags are treated as native switches.
func componentEnabledFlag(id string, cat category, field string) bool {
	parts, err := pointerParts(field)
	if err != nil || len(parts) != 2 {
		return false
	}
	return id == "codex" && (cat == mcp && parts[0] == "mcp_servers" || cat == connectors && parts[0] == "apps") || id == "opencode" && cat == mcp && parts[0] == "mcp"
}

// Copies used by migration and profiles must restore parked entries into the
// new scope. Keep active-source paths out of the copied records.
func relocateParkedComponents(sourceRoot, targetRoot, copied string) error {
	return filepath.WalkDir(copied, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(path) != "meta.json" {
			return nil
		}
		categoryDir := filepath.Dir(filepath.Dir(path))
		if filepath.Base(filepath.Dir(categoryDir)) != disabledComponentsDir {
			return nil
		}
		var record parkedComponent
		if err := readJSON(path, &record); err != nil {
			return err
		}
		if !within(sourceRoot, record.Path) || record.Path == sourceRoot {
			return fmt.Errorf("copied disabled component points outside its source scope")
		}
		rel, err := filepath.Rel(sourceRoot, record.Path)
		if err != nil {
			return err
		}
		record.Path = filepath.Join(targetRoot, rel)
		return writeJSON(path, record)
	})
}

// rebaseDisabledArray closes a removed original position so later restores
// retain order even when active and disabled entries are removed independently.
func rebaseDisabledArray(p *plan, root, path string, parent []string, removed int, except string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if dir == except {
			continue
		}
		file := filepath.Join(dir, "meta.json")
		if err := validateOwnedPath(root, file); err != nil {
			return err
		}
		var record parkedComponent
		if err := readJSON(file, &record); err != nil {
			return err
		}
		parts, err := pointerParts(record.Field)
		if err != nil || record.Path != path || len(parts) != len(parent)+1 || !slices.Equal(parts[:len(parent)], parent) {
			continue
		}
		index, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || index <= removed {
			continue
		}
		record.Field = record.Field[:strings.LastIndex(record.Field, "/")+1] + strconv.Itoa(index-1)
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		p.Component.Writes = append(p.Component.Writes, componentWrite{Path: file, Root: p.StateRoot, Data: data, Mode: 0600})
	}
	return nil
}

func validateComponentContent(path string, data []byte) error {
	var value any
	var err error
	switch configFormat(path) {
	case "json":
		_, err = decodeComponentValue(data)
	case "toml":
		err = toml.Unmarshal(data, &value)
	case "yaml":
		err = yaml.Unmarshal(data, &value)
	}
	if err != nil {
		return fmt.Errorf("invalid %s asset: %w", configFormat(path), err)
	}
	return nil
}

func disabledArrayIndices(root, path string, parent []string, except string) ([]int, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var indices []int
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if dir == except {
			continue
		}
		file := filepath.Join(dir, "meta.json")
		if err := validateOwnedPath(root, file); err != nil {
			return nil, err
		}
		var record parkedComponent
		if err := readJSON(file, &record); err != nil {
			return nil, err
		}
		parts, err := pointerParts(record.Field)
		if err == nil && record.Path == path && len(parts) == len(parent)+1 && slices.Equal(parts[:len(parent)], parent) {
			index, err := strconv.Atoi(parts[len(parts)-1])
			if err == nil && index >= 0 {
				indices = append(indices, index)
			}
		}
	}
	sort.Ints(indices)
	return indices, nil
}

func componentContentDigest(path string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(path, func(child string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(path, child)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", rel, info.Mode())
		if info.Mode().IsRegular() {
			file, err := os.Open(child)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil)), err
}

func componentPlanDigest(p *plan) string {
	return valueDigest([]any{p.Component.Request, p.Component.Writes, p.Component.Native, p.Steps, p.Request, p.StateRoot, p.Install, p.Resources, p.RootDigests})
}

func validateComponentTree(path string) error {
	return filepath.WalkDir(path, func(child string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := validateOwnedPath(filepath.Dir(path), child); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("component contains a special or linked file")
		}
		return nil
	})
}

func (e *engine) applyComponent(ctx context.Context, p *plan) error {
	for _, write := range p.Component.Writes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateOwnedPath(write.Root, write.Path); err != nil {
			return err
		}
		if write.Remove {
			if err := os.RemoveAll(write.Path); err != nil {
				return err
			}
		} else if write.Source != "" {
			if err := validateComponentTree(write.Source); err != nil {
				return err
			}
			if err := copyTreeContext(ctx, write.Source, write.Path); err != nil {
				return err
			}
			digest, err := componentContentDigest(write.Path)
			if err != nil || digest != write.SourceDigest {
				return fmt.Errorf("component import did not match the previewed source")
			}
		} else if err := atomicWrite(write.Path, write.Data, write.Mode); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, write := range p.Component.Writes {
		if write.Remove {
			if _, err := os.Lstat(write.Path); !os.IsNotExist(err) {
				return fmt.Errorf("removed component state remains")
			}
		} else if write.Source == "" {
			data, err := os.ReadFile(write.Path)
			if err != nil || !bytes.Equal(data, write.Data) {
				return fmt.Errorf("component write did not match the approved value")
			}
		}
	}
	for _, r := range p.Resources {
		changed := false
		for _, write := range p.Component.Writes {
			changed = changed || within(r.Path, write.Path) || within(write.Path, r.Path)
		}
		if !changed && !p.Component.Native {
			digest, err := fingerprint(r.Path)
			if err != nil || digest != r.Digest {
				return fmt.Errorf("unrelated component state changed: %s", r.Path)
			}
		}
	}
	if p.Component.Native {
		return e.verifyNativeComponent(p)
	}
	return nil
}

// componentEditData is used by the TUI's external editor. It does not mutate
// native state, and never puts the edited value in progress messages or journals.
func (e *engine) componentEditData(inst installation, scope string, item componentItem) ([]byte, error) {
	if item.Parked != "" && item.Field != "" {
		var record parkedComponent
		if err := readJSON(filepath.Join(item.Parked, "meta.json"), &record); err != nil {
			return nil, err
		}
		return record.Value, nil
	}
	path := item.Path
	if item.Parked != "" {
		path = filepath.Join(item.Parked, "payload")
		if item.Subpath != "" {
			if !filepath.IsLocal(item.Subpath) {
				return nil, fmt.Errorf("disabled asset child path is outside its payload")
			}
			path = filepath.Join(path, item.Subpath)
		}
	}
	if item.Field == "" {
		if err := validateComponentTree(path); err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	}
	state, s, err := e.componentEngine(inst, scope)
	if err != nil {
		return nil, err
	}
	r, err := state.componentResource(s, item.Path, item.Field, item.Category)
	if err != nil {
		return nil, err
	}
	value, err := readConfig(r.Path, r.Format)
	if err != nil {
		return nil, err
	}
	parts, _ := pointerParts(item.Field)
	current, exists := pointerValue(value, parts)
	if !exists {
		return nil, fmt.Errorf("component no longer exists")
	}
	return json.MarshalIndent(current, "", "  ")
}
