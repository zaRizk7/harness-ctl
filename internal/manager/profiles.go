package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (e *engine) createProfile(inst installation, disabled map[category]bool) error {
	if !inst.Managed {
		return fmt.Errorf("launch profiles require a managed installation. Migrate first")
	}
	for cat := range disabled {
		if !knownCategory(cat) {
			return fmt.Errorf("unsupported source category")
		}
	}
	root := filepath.Join(e.cfg.Root, "profiles", inst.ID)
	if err := validateOwnedPath(e.cfg.Root, root); err != nil {
		return err
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	s, _ := specFor(inst.Harness)
	stateRoot := nativeStateRoot(s, stage)
	copyEngine := *e
	copyEngine.cfg.StateRoots = map[string]string{s.ID: inst.StateRoot}
	rs, err := copyEngine.resources(s)
	if err != nil {
		return err
	}
	keep := keepAll()
	for cat, off := range disabled {
		if off {
			keep[cat] = false
		}
	}
	for _, r := range rs {
		if r.Linked {
			continue
		}
		dest, ok := e.resourceDestination(s, inst.StateRoot, stateRoot, r)
		if !ok {
			continue
		}
		if err = copyTree(r.Path, dest); err != nil {
			return err
		}
		if strings.Contains(r.Path, string(filepath.Separator)+disabledComponentsDir+string(filepath.Separator)) {
			if err = relocateParkedComponents(inst.StateRoot, nativeStateRoot(s, root), dest); err != nil {
				return err
			}
		}
		copied := r
		copied.Path = dest
		copied.Root = stage
		copied.Owners = []string{s.ID}
		copied.Linked = false
		if err = applyState(copied, request{Harness: s.ID, Preserve: keep}); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(stateRoot, 0700); err != nil {
		return err
	}
	if err = replaceTree(stage, root); err != nil {
		return err
	}
	e.reg.Profiles[inst.ID] = profile{Root: root, Disabled: cloneCategories(disabled)}
	return nil
}

func (e *engine) disableProfile(inst installation) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.refreshRegistry(); err != nil {
		return err
	}
	exists := false
	for _, current := range e.reg.Installs {
		if current.ID == inst.ID {
			exists = true
		}
	}
	if !exists {
		return fmt.Errorf("selected installation changed. Refresh inventory")
	}
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	// Retain profile state for recovery. The base launch is restored by
	// removing only the profile registration and regenerating the shim.
	delete(e.reg.Profiles, inst.ID)
	if err = e.writeShim(inst); err != nil {
		return err
	}
	return writeJSON(e.statePath, e.reg)
}
