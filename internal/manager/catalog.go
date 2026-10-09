package manager

import (
	"fmt"
	"os"
	"path/filepath"
)

// specFor resolves id against this engine's validated configurable catalog.
// Unknown identifiers return an error rather than using a vendor fallback.
func (e *engine) specFor(id string) (harnessSpec, error) {
	for _, s := range e.cfg.Harnesses {
		if s.ID == id {
			return s, nil
		}
	}
	return harnessSpec{}, fmt.Errorf("unsupported harness %q", id)
}

// stateRoot returns s's configured/environment/default state root without
// creating it.
func (e *engine) stateRoot(s harnessSpec) string {
	if p := e.cfg.StateRoots[s.ID]; p != "" {
		return filepath.Clean(p)
	}
	if s.HomeEnv != "" {
		if p := os.Getenv(s.HomeEnv); filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
	}
	if s.ID == "opencode" {
		if root := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(root) {
			return filepath.Join(root, "opencode")
		}
	}
	return filepath.Join(e.cfg.Home, s.DefaultHome)
}

// rootsFor returns every documented state root for s, including its native XDG
// roots where applicable.
func (e *engine) rootsFor(s harnessSpec) []string {
	roots := []string{e.stateRoot(s)}
	if s.ID == "opencode" {
		if within(e.cfg.Root, roots[0]) {
			base := filepath.Dir(filepath.Dir(filepath.Dir(roots[0])))
			for _, p := range []string{"data", "state", "cache"} {
				roots = append(roots, filepath.Join(base, "xdg", p, "opencode"))
			}
		} else {
			for i, p := range []string{".local/share/opencode", ".local/state/opencode", ".cache/opencode"} {
				root := filepath.Join(e.cfg.Home, p)
				if override := os.Getenv([]string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"}[i]); filepath.IsAbs(override) {
					root = filepath.Join(override, "opencode")
				}
				roots = append(roots, root)
			}
		}
	}
	return roots
}

// nativeStateRoot returns s's native state layout beneath base. It performs no
// IO.
func nativeStateRoot(s harnessSpec, base string) string {
	if s.ID == "gemini" {
		return filepath.Join(base, "home", ".gemini")
	}
	if s.ID == "opencode" {
		return filepath.Join(base, "xdg", "config", "opencode")
	}
	return base
}

// managedStateRoot returns s's independently owned state root beneath manager
// storage.
func (e *engine) managedStateRoot(s harnessSpec) string {
	return nativeStateRoot(s, filepath.Join(e.cfg.Root, "states", s.ID))
}

// resourceDestination maps resource r from sourceRoot to targetRoot for s. It
// returns false when r is not in a documented source root.
func (e *engine) resourceDestination(s harnessSpec, sourceRoot, targetRoot string, r resource) (string, bool) {
	source := *e
	source.cfg.StateRoots = map[string]string{s.ID: sourceRoot}
	target := *e
	target.cfg.StateRoots = map[string]string{s.ID: targetRoot}
	sourceRoots := source.rootsFor(s)
	targetRoots := target.rootsFor(s)
	for i, root := range sourceRoots {
		if canonicalSystemPath(root) == canonicalSystemPath(r.Root) && within(root, r.Path) {
			rel, err := fileIO.rel(root, r.Path)
			if err == nil {
				return filepath.Join(targetRoots[i], rel), true
			}
		}
	}
	return "", false
}
