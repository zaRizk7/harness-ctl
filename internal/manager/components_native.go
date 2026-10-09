package manager

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// claudePluginRegistration decodes native plugin scope and captured payload ownership.
type claudePluginRegistration struct {
	ProjectPath string `json:"projectPath,omitempty"`
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
}

// claudePluginLedger decodes native plugin registrations for result verification.
type claudePluginLedger struct {
	Plugins map[string][]claudePluginRegistration `json:"plugins"`
}

// planNativeComponent adds the verified native plugin contract for change to p.
// Unsupported formats, sources and scopes return errors.
func (e *engine) planNativeComponent(p *plan, change componentRequest) error {
	if change.Category != plugins || p.Install.ID == "" || p.Install.Method == "unknown" {
		return fmt.Errorf("native installation requires a verified installed harness and a supported plugin contract")
	}
	name := change.Name
	if name == "" {
		name = change.Source
	}
	if name == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("enter a native plugin identifier")
	}
	operation := change.Operation
	if operation == "add" {
		operation = "install"
	}
	if operation == "remove" {
		operation = "uninstall"
	}
	if !contains([]string{"install", "enable", "disable", "uninstall"}, operation) {
		return fmt.Errorf("edit the plugin's local configuration or asset instead")
	}
	var args []string
	switch p.Spec.ID {
	case "claude":
		if !strings.Contains(name, "@") {
			return fmt.Errorf("use the qualified plugin-name@marketplace identifier")
		}
		file := filepath.Join(p.StateRoot, "plugins", "installed_plugins.json")
		if err := validateOwnedPath(p.StateRoot, file); err != nil {
			return err
		}
		var ledger claudePluginLedger
		if err := readJSON(file, &ledger); err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, registration := range ledger.Plugins[name] {
			if registration.Scope != "user" {
				continue
			}
			if !within(filepath.Join(p.StateRoot, "plugins"), registration.InstallPath) {
				return fmt.Errorf("native plugin payload is outside captured state. Reinstall it in the selected scope using its native installer")
			}
			if err := validateComponentTree(registration.InstallPath); err != nil {
				return err
			}
			if operation == "install" {
				return fmt.Errorf("plugin already installed. Edit, enable or remove it first")
			}
		}
		args = []string{"plugin", operation, name, "--scope", "user"}
	case "pi":
		if operation != "install" && operation != "uninstall" {
			return fmt.Errorf("Pi enable/disable and edits use the packages declaration in settings.json")
		}
		source := change.Source
		if source == "" {
			source = name
		}
		name = source
		packageName, _, err := piPackageName(source)
		if err != nil {
			return err
		}
		payload := filepath.Join(p.StateRoot, "npm", "node_modules", filepath.FromSlash(packageName))
		if err = validateOwnedPath(p.StateRoot, payload); err != nil {
			return err
		}
		if operation == "uninstall" {
			if _, err = fileIO.stat(payload); err != nil {
				return fmt.Errorf("Pi package payload is not scoped here. External legacy/global packages must be removed natively")
			}
			if err = validateComponentTree(payload); err != nil {
				return err
			}
			args = []string{"remove", source}
		} else {
			args = []string{"install", source}
		}
		p.Component.Request.Name = name
		p.Component.Request.Source = source
	case "gemini":
		if filepath.Base(p.StateRoot) != ".gemini" {
			return fmt.Errorf("Gemini native commands require a HOME containing the selected .gemini directory")
		}
		args = []string{"extensions", operation, name}
		if operation == "install" {
			if change.Source == "" || strings.HasPrefix(change.Source, "-") || change.Name == "" {
				return fmt.Errorf("Gemini install requires source and the expected extension name")
			}
			args[2] = change.Source
			args = append(args, "--consent", "--skip-settings")
		} else if operation != "uninstall" {
			args = append(args, "--scope", "user")
		}
	default:
		return fmt.Errorf("this harness has no verified native plugin command. Add a local asset or native configuration registration")
	}
	if p.Spec.ID == "gemini" && (filepath.Base(name) != name || name == "." || name == "..") {
		return fmt.Errorf("extension name must be a single path component")
	}
	if p.Spec.ID == "gemini" {
		target := filepath.Join(p.StateRoot, "extensions", name)
		if err := validateOwnedPath(p.StateRoot, target); err != nil {
			return err
		}
		if _, err := fileIO.lstat(target); err == nil {
			if err := validateComponentTree(target); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		} else if operation != "install" {
			return fmt.Errorf("extension no longer exists in the selected scope")
		}
	}
	if p.Spec.ID == "gemini" && operation == "install" && filepath.IsAbs(change.Source) {
		if within(change.Source, p.StateRoot) || within(p.StateRoot, change.Source) {
			return fmt.Errorf("native install source must be independent of selected state")
		}
		if err := validateComponentTree(change.Source); err != nil {
			return err
		}
		var manifest struct {
			Name string `json:"name"`
		}
		if readJSON(filepath.Join(change.Source, "gemini-extension.json"), &manifest) != nil || manifest.Name != name {
			return fmt.Errorf("local extension source does not match expected name")
		}
		var err error
		p.RootDigests[change.Source], err = fingerprint(change.Source)
		if err != nil {
			return err
		}
	}
	owners := append([]string{p.Spec.ID}, p.Spec.SharedClients...)
	if !within(e.cfg.Root, p.StateRoot) && !ownersSelected(owners, p.Request) {
		p.Blockers = append(p.Blockers, "Select every affected owner for this native component command.")
	}
	p.Component.Native = true
	env := e.launchEnvironment(p.Install, p.StateRoot)
	if change.Scope == "profile" {
		env["HOME"] = filepath.Join(e.reg.Profiles[p.Install.ID].Root, "home")
	}
	p.Steps = append(p.Steps, command{Path: p.Install.Path, Args: args, Env: env, Dir: e.cfg.Home, Description: "Native plugin " + operation})
	p.Warnings = append(p.Warnings, "Native plugin installation can download and execute upstream code. Command-based installers that require their own interactive approval may refuse and roll back. Remote account authorization remains native.")
	p.Warnings = append(p.Warnings, "Recovery covers captured local files. Upstream scripts, OS secret-store changes and external side effects cannot be reversed by local snapshots.")
	return nil
}

// geminiExtensionDisabled returns name's user-scope disabled status from root's
// native enablement rules. Missing rules mean enabled.
func geminiExtensionDisabled(root, name string) (bool, error) {
	path := filepath.Join(root, "extensions", "extension-enablement.json")
	if err := validateOwnedPath(root, path); err != nil {
		return false, err
	}
	var entries map[string]struct {
		Overrides []string `json:"overrides"`
	}
	if err := readJSON(path, &entries); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	rule := filepath.ToSlash(filepath.Dir(root)) + "/*"
	disabled := false
	for _, override := range entries[name].Overrides {
		if override == rule {
			disabled = false
		}
		if override == "!"+rule {
			disabled = true
		}
	}
	return disabled, nil
}

// verifyNativeComponent checks vendor registrations and loaded payloads. A zero
// exit status alone cannot prove installation, removal or preserved settings.
func (e *engine) verifyNativeComponent(p *plan) error {
	state, _, err := e.componentEngine(p.Install, p.Component.Request.Scope)
	if err != nil {
		return err
	}
	change := p.Component.Request
	name := change.Name
	if name == "" {
		name = change.Source
	}
	removing := change.Operation == "remove"
	disabling := change.Operation == "disable"
	if change.Category == marketplaces {
		if err := verifyMarketplace(p); err != nil {
			return err
		}
	}
	switch p.Spec.ID {
	case "pi":
		if err := verifyPiPackage(p.StateRoot, name, change.Operation); err != nil {
			return err
		}
	case "claude":
		if change.Category == marketplaces {
			break
		}
		var ledger claudePluginLedger
		file := filepath.Join(p.StateRoot, "plugins", "installed_plugins.json")
		if err := validateOwnedPath(p.StateRoot, file); err != nil {
			return err
		}
		err := readJSON(file, &ledger)
		if err != nil && !(removing && os.IsNotExist(err)) {
			return fmt.Errorf("native plugin registration cannot be verified")
		}
		installed := false
		for _, registration := range ledger.Plugins[name] {
			if registration.Scope != "user" {
				continue
			}
			installed = true
			if !removing {
				manifest := filepath.Join(registration.InstallPath, ".claude-plugin", "plugin.json")
				if !within(filepath.Join(p.StateRoot, "plugins"), manifest) {
					return fmt.Errorf("native plugin payload is outside captured state")
				}
				if err := validateOwnedPath(p.StateRoot, manifest); err != nil {
					return err
				}
				var value struct {
					Name string `json:"name"`
				}
				if readJSON(manifest, &value) != nil || value.Name != strings.Split(name, "@")[0] {
					return fmt.Errorf("native plugin manifest does not match the requested plugin")
				}
			}
		}
		if installed == removing {
			return fmt.Errorf("native plugin registration did not reach the approved state")
		}
		config, err := readConfig(filepath.Join(p.StateRoot, "settings.json"), "json")
		if err != nil {
			return err
		}
		value, exists := pointerValue(config, []string{"enabledPlugins", name})
		if removing && exists || !removing && value != !disabling {
			return fmt.Errorf("native plugin enablement did not reach the approved state")
		}
	case "gemini":
		path := filepath.Join(p.StateRoot, "extensions", name)
		if err := validateOwnedPath(p.StateRoot, path); err != nil {
			return err
		}
		if removing {
			if _, err := fileIO.lstat(path); !os.IsNotExist(err) {
				return fmt.Errorf("native extension remains installed")
			}
		} else {
			var manifest struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if readJSON(filepath.Join(path, "gemini-extension.json"), &manifest) != nil || manifest.Name != name || manifest.Version == "" {
				return fmt.Errorf("native extension manifest does not match the requested extension")
			}
			disabled, err := geminiExtensionDisabled(p.StateRoot, name)
			if err != nil || disabled != disabling {
				return fmt.Errorf("native extension enablement did not reach the approved state")
			}
		}
	}
	current, err := state.resources(p.Spec)
	if err != nil {
		return err
	}
	before := map[string]string{}
	after := map[string]string{}
	for _, rs := range []struct {
		items  []resource
		values map[string]string
	}{{p.Resources, before}, {current, after}} {
		for _, r := range rs.items {
			if r.Category == marketplaces || r.Category == plugins || r.Category == cache {
				continue
			}
			if r.Format != "" {
				for field, cat := range r.Fields {
					if cat != marketplaces && cat != plugins && cat != cache {
						rs.values[r.Path+field] = r.FieldDigests[field]
					}
				}
			} else {
				rs.values[r.Path] = r.Digest
			}
		}
	}
	// Base plans include retained profiles. Only compare the native command's
	// selected scope, since it cannot legitimately write another profile.
	for key := range before {
		if within(filepath.Join(e.cfg.Root, "profiles"), key) && change.Scope != "profile" {
			delete(before, key)
		}
	}
	if !maps.Equal(before, after) {
		return fmt.Errorf("native plugin command changed unrelated user state")
	}
	return nil
}
