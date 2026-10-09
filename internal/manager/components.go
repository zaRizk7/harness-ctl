package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/zaRizk7/harness-ctl/internal/component"
)

const disabledComponentsDir = ".harness-ctl-disabled"

var componentCategories = []category{skills, mcp, plugins, marketplaces, connectors, proxies, hooks, settings, memory, other}

// codexSkillsRoot returns the HOME skill root, independently of CODEX_HOME.
// Profiles have a private HOME. Base managed launches inherit the user's HOME.
func (e *engine) codexSkillsRoot() string {
	s, _ := e.specFor("codex")
	root := e.stateRoot(s)
	if within(filepath.Join(e.cfg.Root, "profiles"), root) {
		return filepath.Join(root, "home", ".agents")
	}
	return filepath.Join(e.cfg.Home, ".agents")
}

// codexSkillResources returns shared/private HOME skill resources and their
// affected owners. Linked/unsafe roots and fingerprint errors are returned.
func (e *engine) codexSkillResources() ([]resource, error) {
	root := e.codexSkillsRoot()
	var result []resource
	for _, path := range []string{filepath.Join(root, "skills"), filepath.Join(root, disabledComponentsDir, string(skills))} {
		if err := validateOwnedPath(root, path); err != nil {
			return nil, err
		}
		if _, err := fileIO.lstat(path); os.IsNotExist(err) {
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

// componentItem is the domain inventory record without configuration values.
type componentItem = component.Item

// componentRequest is a domain item operation in a selected state scope.
type componentRequest = component.Request

// parkedComponent retains disabled registrations for restoration.
type parkedComponent = component.Parked

// componentWrite is a planned domain write applied by the transaction engine.
type componentWrite = component.Write

// componentMutation binds a domain request and its approved concrete changes.
type componentMutation = component.Mutation

// componentEngine returns an engine and adapter scoped to inst and scope
// without changing base configuration. Missing profiles/unknown scopes return
// errors.
func (e *engine) componentEngine(inst installation, scope string) (*engine, harnessSpec, error) {
	s, err := e.specFor(inst.Harness)
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

// components returns named local/native entries for inst, scope and cat,
// including disabled registrations. Values are never included in inventory.
func (e *engine) components(inst installation, scope string, cat category) ([]componentItem, error) {
	if cat == marketplaces {
		return e.marketplaceItems(inst, scope)
	}
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
			entries, err := fileIO.readDir(r.Path)
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
					info, err := fileIO.lstat(filepath.Join(parked, "payload"))
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
					if s.ID == "pi" && strings.HasPrefix(pointer, "/packages/") {
						if source := piPackageSource(value); source != "" {
							item.Name = source
							item.Native = true
						}
					}
					if registration, ok := value.(map[string]any); ok && component.EnabledFlag(s.ID, cat, pointer) {
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
		info, err := fileIO.lstat(r.Path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() && !r.Linked {
			entries, err := fileIO.readDir(r.Path)
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
	if s.ID == "claude" && cat == plugins {
		owners := []string{s.ID}
		if !within(e.cfg.Root, state.stateRoot(s)) {
			owners = append(owners, s.SharedClients...)
		}
		items, err = addLedgerPlugins(state.stateRoot(s), owners, items)
		if err != nil {
			return nil, err
		}
	}
	for i := range items {
		if cat == plugins {
			if items[i].Field != "" || items[i].Native {
				items[i].BuiltFor = []string{s.ID}
				continue
			}
			path := items[i].Path
			if items[i].Parked != "" {
				path = filepath.Join(items[i].Parked, "payload")
			}
			items[i].BuiltFor, err = assetHarnesses(path)
			if err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// componentResource returns the owned native resource for s, path, field and
// cat. Cross-category or unknown paths/fields return errors.
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
			if stateconfig.FieldCategory(part) != settings {
				fieldCat = stateconfig.FieldCategory(part)
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
		r.Format = stateconfig.ConfigFormat(path)
		if r.Format != "json" && r.Format != "toml" && r.Format != "yaml" {
			return r, fmt.Errorf("use a structured JSON, TOML or YAML configuration")
		}
		value, err := readConfig(path, r.Format)
		if err != nil && !os.IsNotExist(err) {
			return r, err
		}
		for classified, owner := range stateconfig.ClassifyFields(value) {
			if owner != cat && (classified == field || strings.HasPrefix(classified, field+"/") || strings.HasPrefix(field, classified+"/")) {
				return r, fmt.Errorf("component field crosses another state category")
			}
		}
	} else {
		rel, _ := fileIO.rel(r.Root, path)
		first := strings.Split(rel, string(filepath.Separator))[0]
		if stateconfig.CategoryFor(first) != cat || first == "packages" || first == "hermes-agent" || first == "tools" || first == "bin" || first == "workspace" || first == ".git" || first == "node_modules" {
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

// planComponent adds p's scoped item mutation and rollback resources without
// writing. Validation/ownership/unsupported native contracts return errors.
func (e *engine) planComponent(p *plan) error {
	if p.Request.Component == nil {
		return fmt.Errorf("missing component request")
	}
	change := *p.Request.Component
	if err := validateCompatibility(p.Spec.ID, change); err != nil {
		return err
	}
	change.Value = append(json.RawMessage{}, change.Value...)
	change.Content = append([]byte{}, change.Content...)
	if !slices.Contains(componentCategories, change.Category) {
		return fmt.Errorf("unsupported management category")
	}
	if !contains([]string{"add", "install", "edit", "enable", "disable", "remove", "update"}, change.Operation) {
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
	if change.Native && change.Category == marketplaces {
		return state.planMarketplace(p, change)
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
	planned, err := component.PlanLocal(component.LocalInput{Request: change, Harness: p.Spec.ID, Path: r.Path, Root: r.Root, Format: r.Format, StateRoot: p.StateRoot, ParkRoot: parkRoot, ParkedPath: parked}, componentIO())
	if err != nil {
		return err
	}
	p.Component.Writes = planned.Writes
	for path, digest := range planned.SourceDigests {
		p.RootDigests[path] = digest
	}
	p.Warnings = append(p.Warnings, "This changes only the selected local source. Project/system sources, remote account authorization and inherited environment values retain native behavior.")
	return nil
}

// Copies used by migration and profiles must restore parked entries into the
// new scope. Keep active-source paths out of the copied records.
func relocateParkedComponents(sourceRoot, targetRoot, copied string) error {
	return fileIO.walk(copied, func(path string, entry fs.DirEntry, err error) error {
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
		rel, err := fileIO.rel(sourceRoot, record.Path)
		if err != nil {
			return err
		}
		record.Path = filepath.Join(targetRoot, rel)
		return writeJSON(path, record)
	})
}

// componentContentDigest returns the content fingerprint for path that verifies
// copy semantics without depending on copied modification times.
func componentContentDigest(path string) (string, error) {
	hash := sha256.New()
	err := fileIO.walk(path, func(child string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := fileIO.rel(path, child)
		info, err := fileIO.info(entry)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", rel, info.Mode())
		if info.Mode().IsRegular() {
			file, err := fileIO.open(child)
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

// componentPlanDigest returns a fingerprint binding p's component request,
// writes, native steps, resources and ownership to its approval.
func componentPlanDigest(p *plan) string {
	return valueDigest([]any{p.Component.Request, p.Component.Writes, p.Component.Native, p.Steps, p.Request, p.StateRoot, p.Install, p.Resources, p.RootDigests})
}

// validateComponentTree returns an error if root contains links, nonregular
// payloads or invalid ownership. Imported directories must be independently
// captured.
func validateComponentTree(path string) error {
	return fileIO.walk(path, func(child string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := validateOwnedPath(filepath.Dir(path), child); err != nil {
			return err
		}
		info, err := fileIO.info(entry)
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("component contains a special or linked file")
		}
		return nil
	})
}

// applyComponent applies p's approved item writes with ctx and verifies
// preserved siblings/native results. Failures trigger the surrounding
// transaction rollback.
func (e *engine) applyComponent(ctx context.Context, p *plan) error {
	for _, write := range p.Component.Writes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateOwnedPath(write.Root, write.Path); err != nil {
			return err
		}
		if write.Remove {
			if err := fileIO.removeAll(write.Path); err != nil {
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
			if _, err := fileIO.lstat(write.Path); !os.IsNotExist(err) {
				return fmt.Errorf("removed component state remains")
			}
		} else if write.Source == "" {
			data, err := fileIO.readFile(write.Path)
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
		return fileIO.readFile(path)
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
	return marshalJSONIndent(current, "", "  ")
}

// componentIO returns read/codec boundaries for domain planning. It supplies no
// filesystem mutation capability, keeping previews read-only.
func componentIO() component.LocalIO {
	return component.LocalIO{Lstat: fileIO.lstat, ReadDir: fileIO.readDir, ReadJSON: readJSON, ReadConfig: readConfig, Encode: encodeConfig, Marshal: marshalJSON, ValidatePath: validateOwnedPath, ValidateTree: validateComponentTree, Fingerprint: fingerprint, ContentDigest: componentContentDigest, Decode: decodeComponentValue, Mutate: mutatePointer}
}
