package component

import (
	"encoding/json"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"github.com/zaRizk7/harness-ctl/internal/storage"
	"gopkg.in/yaml.v3"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Item identifies a native registration or local asset. Values are never
// part of the inventory display. Disabled registrations retain their full data.
type Item struct {
	BuiltFor          []string `json:"built_for,omitempty"`
	Name, Path, Field string
	Subpath           string
	Category          stateconfig.Category
	Owners            []string
	Disabled          bool
	Parked            string
	Directory         bool
	Native            bool
}

// Request describes one item operation in base/profile state.
// Value is JSON even when the destination format is TOML or YAML. Internal
// parked/edit fields cannot be supplied by a JSON request.
type Request struct {
	Files     map[string][]byte    `json:"files,omitempty"`
	BuiltFor  []string             `json:"built_for,omitempty"`
	Operation string               `json:"operation"`
	Category  stateconfig.Category `json:"category"`
	Path      string               `json:"path,omitempty"`
	Field     string               `json:"field,omitempty"`
	Value     json.RawMessage      `json:"value,omitempty"`
	Source    string               `json:"source,omitempty"`
	Name      string               `json:"name,omitempty"`
	Native    bool                 `json:"native,omitempty"`
	Scope     string               `json:"scope,omitempty"`
	Text      string               `json:"text,omitempty"`
	Parked    string               `json:"-"`
	Subpath   string               `json:"-"`
	Content   []byte               `json:"-"`
}

// Parked retains a disabled entry's original path, field and value.
type Parked struct {
	Path     string               `json:"path"`
	Field    string               `json:"field,omitempty"`
	Category stateconfig.Category `json:"category"`
	Value    json.RawMessage      `json:"value,omitempty"`
}

// Write describes one approved local write, import or removal
// and the source fingerprint required before applying it.
type Write struct {
	Path, Root, Source string
	SourceDigest       string
	Data               []byte
	Remove             bool
	Mode               fs.FileMode
}

// Mutation binds an item request and its concrete changes to a
// digest. Native mutations use adapter commands instead of local writes.
type Mutation struct {
	Request Request
	Writes  []Write
	Native  bool
	Digest  string
}

// LocalInput describes validated paths for one read-only local mutation preview.
// The caller validates category ownership and adds affected recovery resources.
type LocalInput struct {
	Request                                                      Request
	Harness, Path, Root, Format, StateRoot, ParkRoot, ParkedPath string
}

// LocalResult contains writes and import fingerprints. It changes no filesystem state.
type LocalResult struct {
	Writes        []Write
	SourceDigests map[string]string
}

// LocalIO binds native reads, codecs and safety checks used while constructing a
// local preview. Publication and recovery remain the caller's transaction responsibility.
type LocalIO struct {
	Lstat         func(string) (os.FileInfo, error)
	ReadDir       func(string) ([]os.DirEntry, error)
	ReadJSON      func(string, any) error
	ReadConfig    func(string, string) (map[string]any, error)
	Encode        func(string, map[string]any) ([]byte, error)
	Marshal       func(any) ([]byte, error)
	ValidatePath  func(string, string) error
	ValidateTree  func(string) error
	Fingerprint   func(string) (string, error)
	ContentDigest func(string) (string, error)
	Decode        func([]byte) (any, error)
	Mutate        func(any, []string, any, bool) (any, error)
}

// PlanLocal constructs the field/asset writes for input without applying changes.
// IO errors, invalid payloads and mismatched parked identities return an error.
func PlanLocal(input LocalInput, io LocalIO) (LocalResult, error) {
	result := LocalResult{SourceDigests: map[string]string{}}
	change := input.Request
	parked, parkRoot := input.ParkedPath, input.ParkRoot
	var err error
	write := func(path, root string, data []byte, remove bool) {
		result.Writes = append(result.Writes, Write{Path: path, Root: root, Data: data, Remove: remove, Mode: 0600})
	}
	var record Parked
	parkExists := false
	if err := io.ReadJSON(filepath.Join(parked, "meta.json"), &record); err == nil {
		parkExists = true
		if record.Path != input.Path || record.Field != change.Field || record.Category != change.Category {
			return result, fmt.Errorf("disabled component identity mismatch")
		}
	} else if !os.IsNotExist(err) {
		return result, err
	}
	if change.Parked != "" && !parkExists {
		return result, fmt.Errorf("disabled component no longer exists")
	}
	if change.Field != "" {
		value, err := io.ReadConfig(input.Path, input.Format)
		if os.IsNotExist(err) {
			value, err = map[string]any{}, nil
		}
		if err != nil {
			return result, err
		}
		parts, _ := PointerParts(change.Field)
		current, exists := PointerValue(value, parts)
		parent, _ := PointerValue(value, parts[:len(parts)-1])
		array, isArray := parent.([]any)
		if registration, ok := current.(map[string]any); ok && !parkExists && EnabledFlag(input.Harness, change.Category, change.Field) && (change.Operation == "enable" || change.Operation == "disable") {
			registration["enabled"] = change.Operation == "enable"
			data, err := io.Encode(input.Format, value)
			if err != nil {
				return result, err
			}
			write(input.Path, input.Root, data, false)
			return result, nil
		}
		// A parked array item keeps its original position, which can now be
		// occupied by a different active item. Never edit that active sibling.
		selectedParked := parkExists && (change.Parked != "" || !exists)
		if selectedParked && (change.Operation == "edit" || change.Operation == "remove") {
			if change.Operation == "edit" {
				if _, err := io.Decode(change.Value); err != nil {
					return result, fmt.Errorf("invalid component value: %w", err)
				}
				record.Value = append(json.RawMessage{}, change.Value...)
				data, err := io.Marshal(record)
				if err != nil {
					return result, err
				}
				write(filepath.Join(parked, "meta.json"), input.StateRoot, data, false)
			} else {
				write(parked, input.StateRoot, nil, true)
				if isArray {
					index, _ := strconv.Atoi(parts[len(parts)-1])
					if err := RebaseDisabledArray(&result, io, input.StateRoot, filepath.Dir(parked), input.Path, parts[:len(parts)-1], index, parked); err != nil {
						return result, err
					}
				}
			}
			return result, nil
		}
		remove := false
		var replacement any
		switch change.Operation {
		case "add", "install":
			if exists || parkExists {
				return result, fmt.Errorf("component already exists. Choose edit or enable")
			}
			replacement, err = io.Decode(change.Value)
		case "edit":
			if !exists && !parkExists {
				return result, fmt.Errorf("component no longer exists")
			}
			replacement, err = io.Decode(change.Value)
		case "disable":
			if !exists || parkExists {
				return result, fmt.Errorf("component is missing or already disabled")
			}
			record = Parked{Path: input.Path, Field: change.Field, Category: change.Category}
			if isArray {
				index, _ := strconv.Atoi(parts[len(parts)-1])
				indices, err := DisabledArrayIndices(io, filepath.Dir(parked), input.Path, parts[:len(parts)-1], "")
				if err != nil {
					return result, err
				}
				for _, disabled := range indices {
					if disabled <= index {
						index++
					}
				}
				record.Field = change.Field[:strings.LastIndex(change.Field, "/")+1] + strconv.Itoa(index)
			}
			record.Value, err = io.Marshal(current)
			data, marshalErr := io.Marshal(record)
			if err != nil || marshalErr != nil {
				return result, fmt.Errorf("cannot retain disabled configuration")
			}
			write(filepath.Join(parked, "meta.json"), input.StateRoot, data, false)
			remove = true
		case "enable":
			if exists && !isArray || !parkExists {
				return result, fmt.Errorf("active component exists or disabled state is missing")
			}
			replacement, err = io.Decode(record.Value)
			write(parked, input.StateRoot, nil, true)
		case "remove":
			if !exists && !parkExists {
				return result, fmt.Errorf("component no longer exists")
			}
			if parkExists {
				write(parked, input.StateRoot, nil, true)
			}
			remove = true
			if isArray {
				index, _ := strconv.Atoi(parts[len(parts)-1])
				indices, err := DisabledArrayIndices(io, filepath.Dir(parked), input.Path, parts[:len(parts)-1], "")
				if err != nil {
					return result, err
				}
				for _, disabled := range indices {
					if disabled <= index {
						index++
					}
				}
				if err := RebaseDisabledArray(&result, io, input.StateRoot, filepath.Dir(parked), input.Path, parts[:len(parts)-1], index, ""); err != nil {
					return result, err
				}
			}
		}
		if err != nil {
			return result, fmt.Errorf("invalid component value: %w", err)
		}
		if change.Operation == "enable" && isArray {
			index, _ := strconv.Atoi(parts[len(parts)-1])
			indices, err := DisabledArrayIndices(io, filepath.Dir(parked), input.Path, parts[:len(parts)-1], parked)
			if err != nil {
				return result, err
			}
			for _, disabled := range indices {
				original, _ := strconv.Atoi(parts[len(parts)-1])
				if disabled < original {
					index--
				}
			}
			if index < 0 || index > len(array) {
				return result, fmt.Errorf("disabled array position no longer fits. Edit the native collection before enabling")
			}
			array = append(array, nil)
			copy(array[index+1:], array[index:])
			array[index] = replacement
			if _, err := io.Mutate(value, parts[:len(parts)-1], array, false); err != nil {
				return result, err
			}
		} else if _, err := io.Mutate(value, parts, replacement, remove); err != nil {
			return result, err
		}
		data, err := io.Encode(input.Format, value)
		if err != nil {
			return result, err
		}
		write(input.Path, input.Root, data, false)
	} else {
		info, statErr := io.Lstat(input.Path)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return result, statErr
		}
		switch change.Operation {
		case "add", "install":
			if exists || parkExists {
				return result, fmt.Errorf("component already exists")
			}
			if len(change.Files) > 0 {
				for _, name := range slices.Sorted(maps.Keys(change.Files)) {
					if !filepath.IsLocal(name) || name == "." {
						return result, fmt.Errorf("component asset file escapes target")
					}
					path := filepath.Join(input.Path, name)
					if err := io.ValidatePath(input.Path, path); err != nil {
						return result, err
					}
					data := change.Files[name]
					if err := ValidateContent(path, data); err != nil {
						return result, err
					}
					write(path, input.Root, append([]byte{}, data...), false)
				}
			} else if change.Source == "" {
				data := []byte(change.Text)
				if err := ValidateContent(input.Path, data); err != nil {
					return result, err
				}
				write(input.Path, input.Root, data, false)
			} else {
				if !filepath.IsAbs(change.Source) || storage.Within(change.Source, input.Path) || storage.Within(input.Path, change.Source) {
					return result, fmt.Errorf("import source must be an independent absolute local path")
				}
				if err := io.ValidateTree(change.Source); err != nil {
					return result, err
				}
				result.SourceDigests[change.Source], err = io.Fingerprint(change.Source)
				if err != nil {
					return result, err
				}
				result.Writes = append(result.Writes, Write{Path: input.Path, Root: input.Root, Source: change.Source})
			}
		case "edit":
			target := input.Path
			root := input.Root
			if parkExists && (change.Parked != "" || !exists) {
				target, root = filepath.Join(parked, "payload"), parkRoot
				if change.Subpath != "" {
					if !filepath.IsLocal(change.Subpath) {
						return result, fmt.Errorf("disabled asset child path is outside its payload")
					}
					target = filepath.Join(target, change.Subpath)
					if err := io.ValidatePath(filepath.Join(parked, "payload"), target); err != nil {
						return result, err
					}
				}
				info, statErr = io.Lstat(target)
			}
			if statErr != nil || info.IsDir() {
				return result, fmt.Errorf("select a regular file inside this component to edit")
			}
			validationPath := input.Path
			if change.Subpath != "" {
				validationPath = change.Subpath
			}
			if err := ValidateContent(validationPath, change.Content); err != nil {
				return result, err
			}
			write(target, root, change.Content, false)
			result.Writes[len(result.Writes)-1].Mode = info.Mode().Perm()
		case "disable":
			if !exists || parkExists {
				return result, fmt.Errorf("component is missing or already disabled")
			}
			if err := io.ValidateTree(input.Path); err != nil {
				return result, err
			}
			result.Writes = append(result.Writes, Write{Path: filepath.Join(parked, "payload"), Root: parkRoot, Source: input.Path})
			data, _ := io.Marshal(Parked{Path: input.Path, Category: change.Category})
			write(filepath.Join(parked, "meta.json"), parkRoot, data, false)
			write(input.Path, input.Root, nil, true)
		case "enable":
			if exists || !parkExists {
				return result, fmt.Errorf("active component exists or disabled state is missing")
			}
			payload := filepath.Join(parked, "payload")
			if err := io.ValidateTree(payload); err != nil {
				return result, err
			}
			result.Writes = append(result.Writes, Write{Path: input.Path, Root: input.Root, Source: payload})
			write(parked, parkRoot, nil, true)
		case "remove":
			if exists && change.Parked == "" {
				if err := io.ValidateTree(input.Path); err != nil {
					return result, err
				}
				write(input.Path, input.Root, nil, true)
			}
			if parkExists {
				write(parked, parkRoot, nil, true)
			}
			if !exists && !parkExists {
				return result, fmt.Errorf("component no longer exists")
			}
		}
	}
	for i := range result.Writes {
		write := &result.Writes[i]
		if write.Source != "" {
			write.SourceDigest, err = io.ContentDigest(write.Source)
			if err != nil {
				return result, err
			}
			result.SourceDigests[write.Source], err = io.Fingerprint(write.Source)
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

// EnabledFlag reports whether field in harness id and category cat has a verified
// native registration-level enabled flag. Invalid pointers return false.
func EnabledFlag(id string, cat stateconfig.Category, field string) bool {
	parts, err := PointerParts(field)
	if err != nil || len(parts) != 2 {
		return false
	}
	return id == "codex" && (cat == stateconfig.MCP && parts[0] == "mcp_servers" || cat == stateconfig.Connectors && parts[0] == "apps") || id == "opencode" && cat == stateconfig.MCP && parts[0] == "mcp"
}

// RebaseDisabledArray appends writes to result that close logical position removed
// in path's array at parent. It reads parked records under root through io, skips
// except, and confines generated writes to stateRoot. Missing parking storage is
// unchanged. Invalid metadata and IO failures return errors without publication.
func RebaseDisabledArray(result *LocalResult, io LocalIO, stateRoot string, root, path string, parent []string, removed int, except string) error {
	entries, err := io.ReadDir(root)
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
		if err := io.ValidatePath(root, file); err != nil {
			return err
		}
		var record Parked
		if err := io.ReadJSON(file, &record); err != nil {
			return err
		}
		parts, err := PointerParts(record.Field)
		if err != nil || record.Path != path || len(parts) != len(parent)+1 || !slices.Equal(parts[:len(parent)], parent) {
			continue
		}
		index, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || index <= removed {
			continue
		}
		record.Field = record.Field[:strings.LastIndex(record.Field, "/")+1] + strconv.Itoa(index-1)
		data, err := io.Marshal(record)
		if err != nil {
			return err
		}
		result.Writes = append(result.Writes, Write{Path: file, Root: stateRoot, Data: data, Mode: 0600})
	}
	return nil
}

// ValidateContent returns an error if data is invalid for path's
// structured format. Opaque asset content remains native.
func ValidateContent(path string, data []byte) error {
	var value any
	var err error
	switch stateconfig.ConfigFormat(path) {
	case "json":
		_, err = DecodeValue(data)
	case "toml":
		err = toml.Unmarshal(data, &value)
	case "yaml":
		err = yaml.Unmarshal(data, &value)
	}
	if err != nil {
		return fmt.Errorf("invalid %s asset: %w", stateconfig.ConfigFormat(path), err)
	}
	return nil
}

// DisabledArrayIndices returns sorted logical positions parked under root for
// path's array addressed by parent, excluding the record at except. io supplies
// validated reads. Missing parking storage returns an empty result, while invalid
// metadata or IO failures return errors.
func DisabledArrayIndices(io LocalIO, root, path string, parent []string, except string) ([]int, error) {
	entries, err := io.ReadDir(root)
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
		if err := io.ValidatePath(root, file); err != nil {
			return nil, err
		}
		var record Parked
		if err := io.ReadJSON(file, &record); err != nil {
			return nil, err
		}
		parts, err := PointerParts(record.Field)
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
