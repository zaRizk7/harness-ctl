package manager

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/zaRizk7/harness-ctl/internal/component"
	"github.com/zaRizk7/harness-ctl/internal/library"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

// libraryPath validates the reusable component vault within manager ownership.
func (e *engine) libraryPath() (string, error) {
	path := filepath.Join(e.cfg.Root, "library.age")
	return path, validateOwnedPath(e.cfg.Root, path)
}

// loadLibrary authenticates every reusable record before inventory/application.
func (e *engine) loadLibrary() ([]library.Item, error) {
	path, err := e.libraryPath()
	if err != nil {
		return nil, err
	}
	items := []library.Item{}
	if err = vault.Read(path, e.cfg.MetadataBytes, e.identity, &items); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, item := range items {
		if err = e.validateLibraryItem(item); err != nil {
			return nil, err
		}
		if seen[item.ID] {
			return nil, fmt.Errorf("duplicate library identity")
		}
		seen[item.ID] = true
	}
	return items, nil
}

// validateLibraryItem checks protocol categories and explicitly compatible recipes.
func (e *engine) validateLibraryItem(item library.Item) error {
	if err := library.Validate(item); err != nil {
		return err
	}
	if !slices.Contains(componentCategories, category(item.Category)) {
		return fmt.Errorf("unsupported library category")
	}
	for id, target := range item.Targets {
		if _, err := e.specFor(id); err != nil {
			return err
		}
		if target.HomeSkills && (id != "codex" || item.Category != "skills") {
			return fmt.Errorf("HOME skills only applies to the documented skill source")
		}
	}
	if item.Category == "plugins" {
		detected, err := component.Compatibility(item.Files)
		if err != nil {
			return err
		}
		for id := range item.Targets {
			if err = component.CheckCompatibility(id, nil, detected); err != nil {
				return err
			}
		}
	}
	return nil
}

// libraryViews lists compatibility/status without stored configuration or files.
func libraryViews(items []library.Item) []library.View {
	result := []library.View{}
	for _, item := range items {
		ids := sortedKeys(item.Targets)
		result = append(result, library.View{ID: item.ID, Category: item.Category, Enabled: item.Enabled, Harnesses: ids})
	}
	return result
}

// saveLibraryItem imports/edits a reusable record under the manager mutation lock.
// expected binds editor approval to the vault fingerprint. Existing target copies
// are changed only by a subsequent approved application.
func (e *engine) saveLibraryItem(item library.Item, expected string) error {
	var err error
	item, err = library.Capture(item, e.cfg.MetadataBytes)
	if err != nil {
		return err
	}
	if err = e.validateLibraryItem(item); err != nil {
		return err
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	path, err := e.libraryPath()
	if err != nil {
		return err
	}
	if err = checkFingerprint(path, expected); err != nil {
		return err
	}
	items, err := e.loadLibrary()
	if err != nil {
		return err
	}
	replaced := false
	for i, old := range items {
		if old.ID == item.ID {
			items[i] = item
			replaced = true
		}
	}
	if !replaced {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return vault.Write(path, e.cfg.MetadataBytes, e.identity, items)
}

// checkFingerprint rejects stale storage when expected is supplied.
func checkFingerprint(path, expected string) error {
	if expected == "" {
		return nil
	}
	current, err := fingerprint(path)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("storage changed after preview")
	}
	return nil
}

// changeLibraryItem enables/disables or removes id without applying target changes.
func (e *engine) changeLibraryItem(id, action, expected string) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	path, err := e.libraryPath()
	if err != nil {
		return err
	}
	if err = checkFingerprint(path, expected); err != nil {
		return err
	}
	items, err := e.loadLibrary()
	if err != nil {
		return err
	}
	found := false
	kept := []library.Item{}
	for _, item := range items {
		if item.ID == id {
			found = true
			switch action {
			case "remove":
				continue
			case "enable":
				item.Enabled = true
			case "disable":
				item.Enabled = false
			default:
				return fmt.Errorf("unsupported library action")
			}
		}
		kept = append(kept, item)
	}
	if !found {
		return fmt.Errorf("library entry does not exist")
	}
	return vault.Write(path, e.cfg.MetadataBytes, e.identity, kept)
}

// libraryApply binds one immutable library record to its selected harness batch.
type libraryApply struct {
	Batch        *batchPlan
	Path, Before string
	Digest       string
}

// buildLibraryApply renders compatible per-harness recipes without filesystem writes.
// Source bytes/configuration belong to the library, and targets receive owned copies.
func (e *engine) buildLibraryApply(ctx context.Context, id string, ids, owners []string) (*libraryApply, error) {
	path, err := e.libraryPath()
	if err != nil {
		return nil, err
	}
	before, err := fingerprint(path)
	if err != nil {
		return nil, err
	}
	items, err := e.loadLibrary()
	if err != nil {
		return nil, err
	}
	var item library.Item
	for _, candidate := range items {
		if candidate.ID == id {
			item = candidate
		}
	}
	if item.ID == "" || !item.Enabled {
		return nil, fmt.Errorf("library entry is missing or disabled")
	}
	installs, err := e.discover(ctx)
	if err != nil {
		return nil, err
	}
	var requests []request
	for _, harness := range ids {
		target, ok := item.Targets[harness]
		if !ok {
			return nil, fmt.Errorf("library entry has no compatible recipe for %s", harness)
		}
		inst := e.retainedInstallation(harness)
		for _, candidate := range installs {
			if candidate.Harness == harness {
				inst = candidate
				if candidate.Active {
					break
				}
			}
		}
		// loadLibrary validated target adapters. Base scope needs no profile lookup.
		state, s, _ := e.componentEngine(inst, "base")
		root := state.stateRoot(s)
		if target.HomeSkills {
			root = state.codexSkillsRoot()
		}
		builtFor := sortedKeys(item.Targets)
		change := componentRequest{Operation: "add", Category: category(item.Category), Path: filepath.Join(root, target.Path), Field: target.Field, Value: target.Value, Native: target.Native, Name: target.Name, Source: target.Source, BuiltFor: builtFor, Files: item.Files}
		requests = append(requests, request{Harness: harness, InstallID: inst.ID, Action: "manage", Component: &change, Owners: append([]string{}, owners...)})
	}
	batch, err := e.buildBatch(ctx, requests)
	if err != nil {
		return nil, err
	}
	p := &libraryApply{Batch: batch, Path: path, Before: before}
	p.Digest = valueDigest([]any{batch.ID, batch.Digest, path, before})
	return p, nil
}

// executeLibraryApply checks vault freshness and executes the approved fan-out
// sequentially under one lock with the existing per-harness recovery guarantees.
func (e *engine) executeLibraryApply(ctx context.Context, p *libraryApply, approval string, progress func(string)) error {
	if p == nil || p.Batch == nil || approval != p.Batch.ID {
		return fmt.Errorf("library application requires preview approval")
	}
	if p.Digest != valueDigest([]any{p.Batch.ID, p.Batch.Digest, p.Path, p.Before}) {
		return fmt.Errorf("library preview changed")
	}
	path, err := e.libraryPath()
	if err != nil {
		return err
	}
	if p.Path != path {
		return fmt.Errorf("library preview path changed")
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = checkFingerprint(path, p.Before); err != nil {
		return err
	}
	return e.executeBatchLocked(ctx, p.Batch, approval, progress)
}
