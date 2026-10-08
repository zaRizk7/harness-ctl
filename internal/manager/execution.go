package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (e *engine) lock() (func(), error) {
	if err := rejectLinkedAncestors(e.cfg.Root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(e.cfg.Root, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(e.cfg.Root, "operation.lock")
	if err := validateOwnedPath(e.cfg.Root, path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another harness-ctl operation holds the mutation lock")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func (e *engine) execute(ctx context.Context, p *plan, approval string, progress func(string)) (result error) {
	if p == nil || approval != p.ID || !safeID(p.ID) {
		return fmt.Errorf("execution requires approval of the displayed plan")
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.validatePlan(p); err != nil {
		return err
	}
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	if progress == nil {
		progress = func(string) {}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(e.cfg.OperationSeconds)*time.Second)
	defer cancel()
	// Retention and permanent purge use authenticated encrypted manifests,
	// never the editable display index beside the archive.
	if err = e.expireSnapshots(); err != nil {
		return err
	}
	if p.Request.Permanent {
		if err = e.purgeAffected(p); err != nil {
			return err
		}
	}
	record := operationRecord{ID: p.ID, Harness: p.Spec.ID, Action: p.Request.Action, Status: "preparing", Started: time.Now().UTC(), Destination: p.Destination}
	if err = e.saveRecord(record); err != nil {
		return err
	}
	defer func() {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), time.Duration(e.cfg.OperationSeconds)*time.Second)
		defer cancel()
		if result != nil && record.Snapshot != "" {
			progress("Operation failed. Restoring encrypted recovery.")
			if restoreErr := e.restoreSnapshot(recoveryCtx, record.Snapshot); restoreErr != nil {
				record.Status = "recovery-required"
				result = errors.Join(result, fmt.Errorf("rollback failed: %w", restoreErr))
			} else {
				record.Status = "rolled-back"
			}
		} else if result != nil {
			record.Status = "failed"
		} else {
			record.Status = "complete"
		}
		// Staged installations are disposable only after the old state has
		// been restored. Never remove a tracked package-manager prefix.
		if result != nil && record.Status != "recovery-required" && p.Request.Model == "isolated" && p.Destination != "" && p.Destination != p.Install.Root {
			if removeErr := e.removeManagedTree(p.Destination); removeErr != nil {
				result = errors.Join(result, removeErr)
				record.Status = "recovery-required"
			}
		}
		if !(result == nil && p.Request.Action == "uninstall") {
			if restartErr := e.restartServices(recoveryCtx, record.Services); restartErr != nil {
				result = errors.Join(result, restartErr)
				record.Status = "recovery-required"
			}
		}
		if p.Request.Permanent && record.Snapshot != "" && record.Status != "recovery-required" {
			if purgeErr := e.purgeSnapshot(record.Snapshot); purgeErr != nil {
				result = errors.Join(result, purgeErr)
				record.Status = "recovery-required"
			}
		}
		if result != nil {
			record.Error = result.Error()
		}
		if journalErr := e.saveRecord(record); journalErr != nil {
			result = errors.Join(result, journalErr)
		}
	}()
	progress("Coordinating services and checking running clients.")
	if err = e.stopServices(ctx, p.Install, &record); err != nil {
		return err
	}
	if err = e.checkProcesses(ctx, p); err != nil {
		return err
	}
	if err = e.validatePlan(p); err != nil {
		return err
	}
	progress("Creating encrypted recovery snapshot.")
	meta, err := e.snapshot(ctx, p)
	if err != nil {
		return err
	}
	record.Snapshot = meta.ID
	record.Status = "executing"
	if err = e.saveRecord(record); err != nil {
		return err
	}
	for _, dependency := range p.DependencyCommands {
		progress(dependency.Description)
		if _, err = e.run.Run(ctx, dependency); err != nil {
			return err
		}
	}
	if p.PackageURL != "" {
		progress("Downloading and verifying the previewed npm package.")
		if err = e.stagePackage(ctx, p); err != nil {
			return err
		}
		defer os.Remove(p.Artifact)
	}
	if len(p.NativeScript) > 0 {
		if err = validateOwnedPath(e.cfg.Root, p.Artifact); err != nil {
			return err
		}
		if err = atomicWrite(p.Artifact, p.NativeScript, 0700); err != nil {
			return err
		}
		defer os.Remove(p.Artifact)
	}
	if p.Request.Action == "migrate" {
		if err = e.migrateState(p); err != nil {
			return err
		}
	}
	for _, step := range p.Steps {
		progress(step.Description)
		if _, err = e.run.Run(ctx, step); err != nil {
			return err
		}
	}
	if p.Request.Action == "uninstall" {
		if err = e.removeInstallation(p.Install); err != nil {
			return err
		}
		if _, err = os.Lstat(p.Install.Root); !os.IsNotExist(err) {
			return fmt.Errorf("uninstall did not remove the selected package root")
		}
		if _, err = os.Stat(p.Install.Path); !os.IsNotExist(err) {
			return fmt.Errorf("uninstall did not remove the selected command")
		}
	}
	if p.Request.Action != "install" && p.Request.Action != "migrate" && p.Request.Action != "profile" {
		for _, r := range p.Resources {
			if err = applyState(r, p.Request); err != nil {
				return err
			}
		}
	}
	if p.Request.Action == "migrate" {
		if err = e.applyMigratedState(p); err != nil {
			return err
		}
	}
	if p.Request.Action == "profile" {
		if err = e.createProfile(p.Install, p.Request.Disabled); err != nil {
			return err
		}
		if err = e.writeShim(p.Install); err != nil {
			return err
		}
	}
	if p.Request.Action != "reset" && p.Request.Action != "uninstall" && p.Request.Action != "profile" {
		if p.Request.Model == "isolated" {
			inst, err := e.installedResult(p)
			if err != nil {
				return err
			}
			e.reg.Installs = append(e.withoutInstall(p.Install.ID), inst)
			if old, ok := e.reg.Profiles[p.Install.ID]; ok {
				delete(e.reg.Profiles, p.Install.ID)
				oldRoot := old.Root
				old.Root = filepath.Join(e.cfg.Root, "profiles", inst.ID)
				if err = replaceTree(oldRoot, old.Root); err != nil {
					return err
				}
				e.reg.Profiles[inst.ID] = old
			}
			if err = e.writeShim(inst); err != nil {
				return err
			}
		} else {
			if err = e.verifyTracked(p); err != nil {
				return err
			}
		}
	}
	if err = e.verifyState(p); err != nil {
		return err
	}
	if p.Install.Managed && p.Request.Model == "isolated" && p.Destination != "" && p.Install.Root != p.Destination && p.Request.Action != "uninstall" && p.Request.Action != "reset" && p.Request.Action != "profile" {
		if err = e.removeManagedTree(p.Install.Root); err != nil {
			return err
		}
		profileRoot := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		if err = validateOwnedPath(e.cfg.Root, profileRoot); err != nil {
			return err
		}
		if err = os.RemoveAll(profileRoot); err != nil {
			return err
		}
	}
	if err = e.validateRegistry(e.reg); err != nil {
		return err
	}
	if err = writeJSON(e.statePath, e.reg); err != nil {
		return err
	}
	progress("Verified. Operation complete.")
	return nil
}

func (e *engine) withoutInstall(id string) []installation {
	var installs []installation
	for _, inst := range e.reg.Installs {
		if inst.ID != id {
			installs = append(installs, inst)
		}
	}
	return installs
}

func (e *engine) removeManagedTree(path string) error {
	if err := validateOwnedPath(filepath.Join(e.cfg.Root, "installs"), path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func (e *engine) removeInstallation(inst installation) error {
	if inst.Managed {
		if err := e.removeShim(inst); err != nil {
			return err
		}
		if err := e.removeManagedTree(inst.Root); err != nil {
			return err
		}
		e.reg.Installs = e.withoutInstall(inst.ID)
		delete(e.reg.Profiles, inst.ID)
	} else if strings.HasPrefix(inst.Method, "native-") {
		if err := validateOwnedPath(filepath.Dir(inst.Root), inst.Root); err != nil {
			return err
		}
		if err := e.removeOwnedLauncher(inst); err != nil {
			return err
		}
		if err := os.RemoveAll(inst.Root); err != nil {
			return err
		}
	}
	for _, path := range inst.ServicePaths {
		if err := validateOwnedPath(filepath.Join(e.cfg.Home, "Library/LaunchAgents"), path); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (e *engine) removeOwnedLauncher(inst installation) error {
	if filepath.Dir(inst.Path) != filepath.Join(e.cfg.Home, ".local/bin") {
		return fmt.Errorf("native launcher is outside the user-local ownership contract")
	}
	resolved, err := filepath.EvalSymlinks(inst.Path)
	if err == nil && within(inst.Root, resolved) {
		return os.Remove(inst.Path)
	}
	info, err := os.Lstat(inst.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(inst.Path)
		if err != nil {
			return err
		}
		if len(data) < 16384 && strings.Contains(string(data), inst.Root) {
			return os.Remove(inst.Path)
		}
	}
	return fmt.Errorf("launcher ownership changed")
}

func (e *engine) installedResult(p *plan) (installation, error) {
	path := filepath.Join(p.Destination, "bin", p.Spec.Command)
	if p.Spec.ID == "hermes" {
		path = filepath.Join(p.Destination, ".hermes", "bin", "hermes")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return installation{}, fmt.Errorf("installer did not publish its documented executable: %w", err)
	}
	if !within(p.Destination, resolved) {
		return installation{}, fmt.Errorf("installed executable escapes managed prefix")
	}
	info, err := os.Stat(path)
	if err != nil {
		return installation{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return installation{}, fmt.Errorf("installed command is not executable")
	}
	inst := installation{ID: installID(p.Spec.ID, path), Harness: p.Spec.ID, Method: "managed-" + p.Spec.Kind, Path: path, Root: p.Destination, Version: p.Request.Target, Managed: true, StateRoot: p.StateRoot, Package: p.Spec.Package}
	if p.Spec.Kind == "npm" {
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err = readJSON(filepath.Join(p.Destination, "lib", "node_modules", p.Spec.Package, "package.json"), &manifest); err != nil {
			return installation{}, err
		}
		if manifest.Name != p.Spec.Package || manifest.Version != p.Request.Target {
			return installation{}, fmt.Errorf("installed npm identity or version disagrees with preview")
		}
	}
	if p.Request.Action == "migrate" {
		inst.StateRoot = e.managedStateRoot(p.Spec)
	}
	return inst, e.validateRegistry(registry{Installs: []installation{inst}})
}

func (e *engine) verifyTracked(p *plan) error {
	if _, err := os.Stat(p.Install.Path); err != nil {
		return fmt.Errorf("installed executable is missing: %w", err)
	}
	if p.Install.Method == "npm" {
		var manifest struct {
			Version string `json:"version"`
		}
		if err := readJSON(filepath.Join(p.Install.Root, "package.json"), &manifest); err != nil {
			return err
		}
		if manifest.Version != p.Request.Target {
			return fmt.Errorf("npm installed a different version than the preview")
		}
	}
	return nil
}

func (e *engine) verifyState(p *plan) error {
	if p.Request.Action == "migrate" {
		return nil
	}
	for _, r := range p.Resources {
		if p.Request.Action == "profile" && within(filepath.Join(e.cfg.Root, "profiles"), r.Path) {
			continue
		}
		if !shouldChange(r, p.Request) {
			digest, err := fingerprint(r.Path)
			if err != nil {
				return err
			}
			if digest != r.Digest {
				return fmt.Errorf("preserved resource changed during operation: %s", r.Path)
			}
			continue
		}
		if len(r.Fields) > 0 {
			value, err := readConfig(r.Path, r.Format)
			if os.IsNotExist(err) {
				for _, cat := range r.Fields {
					if p.Request.Preserve[cat] {
						return fmt.Errorf("preserved setting was removed")
					}
				}
				continue
			}
			if err != nil {
				return err
			}
			for path, cat := range r.Fields {
				parent, key, ok := fieldParent(value, path)
				exists := false
				if ok {
					_, exists = parent[key]
				}
				if !p.Request.Preserve[cat] && exists {
					return fmt.Errorf("discarded setting remains: %s", path)
				}
				if p.Request.Preserve[cat] && !exists {
					return fmt.Errorf("preserved setting disappeared: %s", path)
				}
				if p.Request.Preserve[cat] && exists && r.FieldDigests[path] != "" && valueDigest(parent[key]) != r.FieldDigests[path] {
					return fmt.Errorf("preserved setting changed: %s", path)
				}
			}
		} else {
			if _, err := os.Lstat(r.Path); !os.IsNotExist(err) {
				return fmt.Errorf("discarded resource remains: %s", r.Path)
			}
		}
	}
	return nil
}

func (e *engine) saveRecord(record operationRecord) error {
	path := filepath.Join(e.cfg.Root, "operations", record.ID+".json")
	if !safeID(record.ID) {
		return fmt.Errorf("invalid operation identity")
	}
	if err := validateOwnedPath(e.cfg.Root, path); err != nil {
		return err
	}
	return writeJSON(path, record)
}
func (e *engine) records() ([]operationRecord, error) {
	var result []operationRecord
	dir := filepath.Join(e.cfg.Root, "operations")
	if err := validateOwnedPath(e.cfg.Root, dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var record operationRecord
		if err = readJSON(filepath.Join(dir, entry.Name()), &record); err != nil {
			return nil, err
		}
		if !safeID(record.ID) {
			return nil, fmt.Errorf("invalid operation record")
		}
		result = append(result, record)
	}
	return result, nil
}
func (e *engine) ensureNoPending() error {
	records, err := e.records()
	if err != nil {
		return err
	}
	for _, r := range records {
		switch r.Status {
		case "complete", "rolled-back", "failed", "acknowledged":
		default:
			return fmt.Errorf("operation %s needs recovery. Open Recovery before starting another mutation", shortID(r.ID))
		}
	}
	return nil
}

func (e *engine) expireSnapshots() error {
	metas, err := e.snapshots()
	if err != nil {
		return err
	}
	for _, display := range metas {
		meta, err := e.authenticatedSnapshot(display.ID)
		if err != nil {
			return err
		}
		if time.Now().After(meta.Expires) {
			if err = e.purgeSnapshot(meta.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *engine) purgeAffected(p *plan) error {
	metas, err := e.snapshots()
	if err != nil {
		return err
	}
	var purge []string
	for _, display := range metas {
		meta, err := e.authenticatedSnapshot(display.ID)
		if err != nil {
			return err
		}
		affected := false
		for _, item := range meta.Items {
			if meta.Harness == p.Spec.ID && ownersSelected(item.Owners, p.Request) {
				for _, cat := range item.Categories {
					if !p.Request.Preserve[cat] {
						affected = true
					}
				}
			}
			for _, r := range p.Resources {
				if shouldChange(r, p.Request) && (within(item.Path, r.Path) || within(r.Path, item.Path)) {
					affected = true
				}
			}
		}
		if affected {
			purge = append(purge, meta.ID)
		}
	}
	for _, id := range purge {
		if err = e.purgeSnapshot(id); err != nil {
			return err
		}
	}
	return nil
}

func cloneCategories(input map[category]bool) map[category]bool {
	out := map[category]bool{}
	for cat, keep := range input {
		out[cat] = keep
	}
	return out
}

func (e *engine) migrateState(p *plan) error {
	target := e.managedStateRoot(p.Spec)
	if err := validateOwnedPath(e.cfg.Root, target); err != nil {
		return err
	}
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return fmt.Errorf("migration destination already contains state")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	for _, r := range p.Resources {
		if r.Linked {
			continue
		}
		dest, ok := e.resourceDestination(p.Spec, p.StateRoot, target, r)
		if !ok {
			continue
		}
		if err := copyTree(r.Path, dest); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) applyMigratedState(p *plan) error {
	root := e.managedStateRoot(p.Spec)
	copyEngine := *e
	copyEngine.cfg.StateRoots = map[string]string{p.Spec.ID: root}
	rs, err := copyEngine.resources(p.Spec)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if err = applyState(r, p.Request); err != nil {
			return err
		}
	}
	return nil
}
