package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var versionPattern = regexp.MustCompile(`\b[vV]?\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z._-]+)?\b`)

func installID(harness, path string) string {
	sum := sha256.Sum256([]byte(harness + "\x00" + path))
	return harness + "-" + hex.EncodeToString(sum[:6])
}

func commandPaths(name string) []string {
	var paths []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 && !contains(paths, p) {
			paths = append(paths, p)
		}
	}
	return paths
}

func (e *engine) discover(ctx context.Context) ([]installation, error) {
	var found []installation
	for _, s := range catalog {
		paths := commandPaths(s.Command)
		// Common user-local launchers can be installed but absent from PATH.
		for _, p := range []string{filepath.Join(e.cfg.Home, ".local/bin", s.Command), filepath.Join(e.cfg.BinDir, s.Command)} {
			if info, err := os.Stat(p); err == nil && info.Mode()&0111 != 0 && !info.IsDir() && !contains(paths, p) {
				paths = append(paths, p)
			}
		}
		for n, p := range paths {
			if within(e.cfg.BinDir, p) {
				continue
			} // A shim must never be probed recursively.
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue
			}
			inst := installation{ID: installID(s.ID, p), Harness: s.ID, Method: "unknown", Path: p, Version: "unknown", Active: n == 0}
			for _, registered := range e.reg.Installs {
				if registered.Path == p {
					inst = registered
					inst.Active = n == 0
					break
				}
			}
			if !inst.Managed {
				e.identify(s, resolved, &inst)
			}
			// Even --help and --version can initialize a native CLI's state.
			// Inventory uses package manifests and version-directory metadata.
			if inst.Version == "unknown" {
				if version := versionPattern.FindString(strings.TrimPrefix(resolved, inst.Root)); version != "" {
					inst.Version = strings.TrimPrefix(version, "v")
				}
			}
			if inst.Method == "unknown" {
				inst.Note = "Installation ownership is unverified. Lifecycle mutations are disabled."
			}
			inst.ServicePaths = e.servicePaths(s)
			found = append(found, inst)
		}
		for _, inst := range e.reg.Installs {
			if inst.Harness != s.ID || !inst.Managed {
				continue
			}
			duplicate := false
			for _, f := range found {
				if f.ID == inst.ID {
					duplicate = true
					break
				}
			}
			if !duplicate {
				inst.Active = len(paths) > 0 && paths[0] == filepath.Join(e.cfg.BinDir, s.Command)
				inst.ServicePaths = e.servicePaths(s)
				var services []string
				for _, path := range inst.ServicePaths {
					if e.validateService(inst, path) == nil {
						services = append(services, path)
					}
				}
				inst.ServicePaths = services
				found = append(found, inst)
			}
		}
	}
	for _, name := range additionalCommands {
		for _, path := range commandPaths(name) {
			found = append(found, installation{ID: installID(name, path), Harness: name, Path: path, Method: "unsupported", Version: "unknown", Note: "Detected additional harness. A tested adapter is required."})
		}
	}
	return found, nil
}

func (e *engine) identify(s harnessSpec, resolved string, inst *installation) {
	// npm ownership comes from the package manifest, never from a command name.
	p := filepath.Dir(resolved)
	for n := 0; n < 8 && p != "/"; n++ {
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if data, err := os.ReadFile(filepath.Join(p, "package.json")); err == nil && json.Unmarshal(data, &manifest) == nil && (manifest.Name == s.Package || contains(s.LegacyPackages, manifest.Name)) && strings.HasSuffix(p, "/lib/node_modules/"+manifest.Name) {
			inst.Method = "npm"
			inst.Root = p
			inst.Package = manifest.Name
			inst.Version = manifest.Version
			return
		}
		p = filepath.Dir(p)
	}
	if strings.Contains(resolved, "/Cellar/") || strings.Contains(resolved, "/Caskroom/") {
		parts := strings.Split(resolved, "/")
		for i, part := range parts {
			if (part == "Cellar" || part == "Caskroom") && i+2 < len(parts) {
				inst.Method = "brew"
				inst.Package = parts[i+1]
				inst.Root = strings.Join(parts[:i+3], "/")
				return
			}
		}
	}
	if s.ID == "codex" {
		root := filepath.Join(e.stateRoot(s), "packages/standalone")
		if within(root, resolved) {
			inst.Method = "native-codex"
			inst.Root = root
			return
		}
	}
	if s.ID == "claude" {
		root := filepath.Join(e.cfg.Home, ".local/share/claude")
		if within(root, resolved) {
			inst.Method = "native-claude"
			inst.Root = root
			return
		}
	}
	if s.ID == "prime-agent" {
		root := filepath.Join(e.cfg.Home, ".local/share/prime-agent")
		if within(root, resolved) {
			inst.Method = "native-prime"
			inst.Root = root
			return
		}
	}
	if s.ID == "hermes" {
		root := filepath.Join(e.stateRoot(s), "hermes-agent")
		if info, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil && !info.IsDir() {
			data, err := os.ReadFile(inst.Path)
			if err == nil && len(data) < 16384 && strings.Contains(string(data), root) {
				inst.Method = "native-hermes"
				inst.Root = root
				return
			}
		}
	}
}

func (e *engine) servicePaths(s harnessSpec) []string {
	var paths []string
	for _, label := range s.LaunchLabels {
		p := filepath.Join(e.cfg.Home, "Library/LaunchAgents", label+".plist")
		if _, err := os.Lstat(p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

func lookPath(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("required dependency %s is missing", name)
	}
	return p, nil
}
