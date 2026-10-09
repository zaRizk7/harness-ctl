package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// validateRegistry checks reg's managed installations, profile paths and
// identities before accepting ownership. It never writes reg.
func (e *engine) validateRegistry(reg registry) error {
	seen := map[string]bool{}
	for _, inst := range reg.Installs {
		s, err := e.specFor(inst.Harness)
		if err != nil {
			return err
		}
		if inst.ID != installID(inst.Harness, inst.Path) || seen[inst.ID] {
			return fmt.Errorf("invalid or duplicate installation identity")
		}
		seen[inst.ID] = true
		if !inst.Managed {
			return fmt.Errorf("registry may only own manager installations")
		}
		if err := validateOwnedPath(filepath.Join(e.cfg.Root, "installs", inst.Harness), inst.Root); err != nil {
			return err
		}
		if !within(inst.Root, inst.Path) || inst.Path == inst.Root {
			return fmt.Errorf("registered executable escapes installation")
		}
		if inst.StateRoot != e.managedStateRoot(s) {
			return fmt.Errorf("registered state root is unverified")
		}
	}
	for id, p := range reg.Profiles {
		if !seen[id] {
			return fmt.Errorf("profile refers to an unknown installation")
		}
		if p.Root != filepath.Join(e.cfg.Root, "profiles", id) {
			return fmt.Errorf("invalid profile root")
		}
		for cat := range p.Disabled {
			if !knownCategory(cat) {
				return fmt.Errorf("unknown profile category")
			}
		}
	}
	return nil
}

// refreshRegistry loads and validates current persisted ownership. Missing
// storage uses an empty registry, invalid storage returns an error.
func (e *engine) refreshRegistry() error {
	if err := validateOwnedPath(e.cfg.Root, e.statePath); err != nil {
		return err
	}
	reg := registry{Profiles: map[string]profile{}}
	if err := readJSON(e.statePath, &reg); err != nil && !os.IsNotExist(err) {
		return err
	}
	if reg.Profiles == nil {
		reg.Profiles = map[string]profile{}
	}
	if err := e.validateRegistry(reg); err != nil {
		return err
	}
	e.reg = reg
	return nil
}

// knownCategory reports whether cat belongs to the supported state category
// protocol.
func knownCategory(cat category) bool {
	for _, c := range categories {
		if c == cat {
			return true
		}
	}
	return false
}

// shellQuote returns value as one POSIX shell argument, preserving embedded
// quotes without evaluation.
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// sortedKeys returns m's keys in lexical order for deterministic encoding,
// commands and previews.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// launchEnvironment returns native state/runtime environment overrides for inst
// and root. It never reads credential values.
func (e *engine) launchEnvironment(inst installation, root string) map[string]string {
	s, _ := e.specFor(inst.Harness)
	env := map[string]string{"GOTOOLCHAIN": "local", "DISABLE_AUTOUPDATER": "1"}
	if s.HomeEnv != "" {
		env[s.HomeEnv] = root
	}
	if s.ID == "gemini" {
		env["HOME"] = filepath.Dir(root)
	}
	if s.ID == "opencode" {
		state := *e
		state.cfg.StateRoots = map[string]string{s.ID: root}
		roots := state.rootsFor(s)
		for i, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
			env[key] = filepath.Dir(roots[i])
		}
	}
	if s.ID == "hermes" {
		env["HERMES_RUNTIME_DIR"] = filepath.Join(inst.Root, "runtime")
		env["UV_PYTHON_INSTALL_DIR"] = filepath.Join(inst.Root, "python")
	}
	return env
}

// writeShim publishes inst's scoped direct launcher. Registered ownership is
// required and foreign existing commands are rejected.
func (e *engine) writeShim(inst installation) error {
	if err := e.validateRegistry(registry{Installs: []installation{inst}}); err != nil {
		return err
	}
	s, _ := e.specFor(inst.Harness)
	root := inst.StateRoot
	prof, hasProfile := e.reg.Profiles[inst.ID]
	if hasProfile {
		root = nativeStateRoot(s, prof.Root)
	}
	// Gemini uses homedir() rather than a verified native home override.
	// Give only shim launches a private HOME with the expected .gemini child.
	if err := fileIO.mkdir(root, 0700); err != nil {
		return err
	}
	if s.ID == "opencode" {
		base := filepath.Dir(filepath.Dir(filepath.Dir(root)))
		for _, path := range []string{"config/opencode", "data/opencode", "state/opencode", "cache/opencode"} {
			if err := fileIO.mkdir(filepath.Join(base, "xdg", path), 0700); err != nil {
				return err
			}
		}
	}
	env := e.launchEnvironment(inst, root)
	if hasProfile {
		home := filepath.Join(prof.Root, "home")
		if err := fileIO.mkdir(home, 0700); err != nil {
			return err
		}
		env["HOME"] = home
		if s.ID != "opencode" {
			env["XDG_CONFIG_HOME"] = filepath.Join(prof.Root, "xdg", "config")
			env["XDG_DATA_HOME"] = filepath.Join(prof.Root, "xdg", "data")
			env["XDG_STATE_HOME"] = filepath.Join(prof.Root, "xdg", "state")
			env["XDG_CACHE_HOME"] = filepath.Join(prof.Root, "xdg", "cache")
		}
	}
	var body strings.Builder
	body.WriteString("#!/bin/sh\n# Generated by harness-ctl. Applies only to this shim.\n")
	if hasProfile && prof.Disabled[proxies] {
		body.WriteString("unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy\n")
	}
	for _, key := range sortedKeys(env) {
		fmt.Fprintf(&body, "export %s=%s\n", key, shellQuote(env[key]))
	}
	body.WriteString(launchPolicy(s).Shell(s.LaunchNote))
	fmt.Fprintf(&body, "exec %s \"$@\"\n", shellQuote(inst.Path))
	path := filepath.Join(e.cfg.BinDir, s.Command)
	if err := validateOwnedPath(e.cfg.Root, path); err != nil {
		return err
	}
	if data, err := fileIO.readFile(path); err == nil && !strings.Contains(string(data), "Generated by harness-ctl.") {
		return fmt.Errorf("refusing to replace a foreign command: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return atomicWrite(path, []byte(body.String()), 0700)
}

// removeShim removes only the generated launcher that still refers to inst. A
// foreign or retargeted launcher is retained.
func (e *engine) removeShim(inst installation) error {
	s, _ := e.specFor(inst.Harness)
	path := filepath.Join(e.cfg.BinDir, s.Command)
	if err := validateOwnedPath(e.cfg.Root, path); err != nil {
		return err
	}
	data, err := fileIO.readFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "Generated by harness-ctl.") || !strings.Contains(string(data), shellQuote(inst.Path)) {
		return nil
	}
	return fileIO.remove(path)
}
