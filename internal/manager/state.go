package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// readConfig reads path through the filesystem boundary and returns its structured
// object in format. Parse errors name the file without including configuration values.
func readConfig(path, format string) (map[string]any, error) {
	data, err := fileIO.readFile(path)
	if err != nil {
		return nil, err
	}
	value, err := stateconfig.Decode(data, format)
	if err != nil {
		return nil, fmt.Errorf("cannot safely parse %s: %w", filepath.Base(path), err)
	}
	return value, nil
}

// resourceCategories returns all categories represented by r, including
// selectively classified fields.
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

// valueDigest returns a stable JSON digest for value. Callers pass
// JSON-compatible configuration and plan values.
func valueDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fieldDigests returns individual fingerprints for the classified pointers in
// fields from value.
func fieldDigests(value map[string]any, fields map[string]category) map[string]string {
	digests := map[string]string{}
	for path := range fields {
		parent, key, ok := stateconfig.FieldParent(value, path)
		if ok {
			digests[path] = valueDigest(parent[key])
		}
	}
	return digests
}

// resources returns s's classified native state and fingerprints without
// reading unrelated homes. Linked/shared sources remain labelled and protected.
func (e *engine) resources(s harnessSpec) ([]resource, error) { return e.resourcesWithHome(s, true) }

// resourcesWithHome scans s's selected roots. includeHome controls separate
// HOME registrations, which must be omitted for explicit external inventory.
func (e *engine) resourcesWithHome(s harnessSpec, includeHome bool) ([]resource, error) {
	var result []resource
	for _, root := range e.rootsFor(s) {
		if root == e.cfg.Home || root == "/" || within(root, e.cfg.Root) {
			return nil, fmt.Errorf("unsafe state root")
		}
		if err := rejectLinkedAncestors(root); err != nil {
			return nil, err
		}
		entries, err := fileIO.readDir(root)
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
				parked, err := fileIO.readDir(filepath.Join(root, name))
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
			r := resource{Path: filepath.Join(root, name), Root: root, Category: resourceCategoryFor(s, name), Owners: []string{s.ID}}
			info, err := fileIO.lstat(r.Path)
			if err != nil {
				return nil, err
			}
			r.Linked = info.Mode()&os.ModeSymlink != 0
			if contains(s.ConfigFiles, name) && !r.Linked {
				r.Format = stateconfig.ConfigFormat(r.Path)
				if r.Format != "jsonc" {
					fields, err := readConfig(r.Path, r.Format)
					if err != nil {
						return nil, err
					}
					r.Fields = stateconfig.ClassifyFields(fields)
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
			for _, otherSpec := range e.cfg.Harnesses {
				if otherSpec.ID == s.ID {
					continue
				}
				candidate := filepath.Join(e.stateRoot(otherSpec), name)
				a, ea := fileIO.eval(r.Path)
				b, eb := fileIO.eval(candidate)
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
	if includeHome && s.ID == "claude" && !within(e.cfg.Root, e.stateRoot(s)) {
		path := filepath.Join(e.cfg.Home, ".claude.json")
		if _, err := fileIO.stat(path); err == nil {
			r := resource{Path: path, Root: e.cfg.Home, Category: settings, Owners: append([]string{s.ID}, s.SharedClients...), Format: "json", Note: "Shared Claude client configuration."}
			fields, err := readConfig(path, "json")
			if err != nil {
				return nil, err
			}
			r.Fields = stateconfig.ClassifyFields(fields)
			r.FieldDigests = fieldDigests(fields, r.Fields)
			r.Digest, err = fingerprint(path)
			if err != nil {
				return nil, err
			}
			result = append(result, r)
		}
	}
	if includeHome && s.ID == "codex" {
		skills, err := e.codexSkillResources()
		if err != nil {
			return nil, err
		}
		result = append(result, skills...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// shouldChange reports whether req authorizes changing any category in r and
// includes its affected owners.
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

// ownersSelected reports whether req selects all owners, treating its harness
// as already selected.
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

// applyState applies req's category preservation to r after ownership
// validation. It preserves siblings and returns unsafe/failed edits.
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
				stateconfig.DeleteField(value, path)
			}
		}
		if len(value) == 0 {
			return fileIO.remove(r.Path)
		}
		data, err := encodeConfig(r.Format, value)
		if err != nil {
			return err
		}
		return atomicWrite(r.Path, data, 0600)
	}
	// WalkDir does not follow links. Validate every child before removal so a
	// directory with external links is reported rather than partially deleted.
	if err := fileIO.walk(r.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return validateOwnedPath(r.Root, path)
	}); err != nil {
		return err
	}
	return fileIO.removeAll(r.Path)
}

// resourceCategoryFor keeps Pi package caches in their native plugin category.
func resourceCategoryFor(s harnessSpec, name string) category {
	if s.ID == "codex" && name == ".tmp" {
		return cache
	}
	if s.ID == "pi" && (name == "npm" || name == "git") {
		return plugins
	}
	return stateconfig.CategoryFor(name)
}
