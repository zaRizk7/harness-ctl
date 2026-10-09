package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// marketplaceRecord identifies a locally cached native marketplace.
// Source metadata remains private. Account-hosted remote catalogs retain native control.
type marketplaceRecord struct {
	InstallLocation string         `json:"installLocation"`
	Source          map[string]any `json:"source"`
}

// readMarketplaces reads current native ledgers, including catalogs added outside the manager.
func readMarketplaces(root string) (map[string]marketplaceRecord, error) {
	path := filepath.Join(root, "plugins", "known_marketplaces.json")
	if err := validateOwnedPath(root, path); err != nil {
		return nil, err
	}
	items := map[string]marketplaceRecord{}
	if err := readJSON(path, &items); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return items, nil
}

// marketplaceItems exposes names and ownership without repository credentials.
func (e *engine) marketplaceItems(inst installation, scope string) ([]componentItem, error) {
	state, s, err := e.componentEngine(inst, scope)
	if err != nil {
		return nil, err
	}
	if s.ID == "codex" {
		owners := append([]string{s.ID}, s.SharedClients...)
		return codexMarketplaceItems(state.stateRoot(s), owners)
	}
	if s.ID != "claude" {
		return nil, fmt.Errorf("no verified marketplace contract for this harness")
	}
	records, err := readMarketplaces(state.stateRoot(s))
	if err != nil {
		return nil, err
	}
	owners := []string{s.ID}
	if !within(e.cfg.Root, state.stateRoot(s)) {
		owners = append(owners, s.SharedClients...)
	}
	items := []componentItem{}
	for _, name := range sortedKeys(records) {
		items = append(items, componentItem{Name: name, Category: marketplaces, Path: filepath.Join(state.stateRoot(s), "plugins", "known_marketplaces.json"), Native: true, Owners: owners, BuiltFor: []string{s.ID}})
	}
	return items, nil
}

// addLedgerPlugins merges native user installations that lack an enablement setting.
// This sees disabled/manual installs without relying on manager-written metadata.
func addLedgerPlugins(root string, owners []string, items []componentItem) ([]componentItem, error) {
	path := filepath.Join(root, "plugins", "installed_plugins.json")
	if err := validateOwnedPath(root, path); err != nil {
		return nil, err
	}
	var ledger claudePluginLedger
	if err := readJSON(path, &ledger); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	config, err := readConfig(filepath.Join(root, "settings.json"), "json")
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, name := range sortedKeys(ledger.Plugins) {
		if !strings.Contains(name, "@") {
			return nil, fmt.Errorf("invalid native plugin identity")
		}
		for _, registration := range ledger.Plugins[name] {
			if registration.Scope != "" && registration.Scope != "user" {
				items = append(items, componentItem{Name: name + " [" + registration.Scope + "]", Category: plugins, Path: registration.InstallPath, Native: true, ReadOnly: true, NativeScope: registration.Scope, Note: "External native registration. Payload ownership and enablement are not claimed. Project: " + registration.ProjectPath, Owners: owners, BuiltFor: []string{"claude"}})
			}
		}
		present := false
		for _, item := range items {
			if item.Native && item.Name == name {
				present = true
			}
		}
		if present {
			continue
		}
		for _, registration := range ledger.Plugins[name] {
			if registration.Scope == "user" {
				value, _ := pointerValue(config, []string{"enabledPlugins", name})
				items = append(items, componentItem{Name: name, Category: plugins, Path: filepath.Join(root, "settings.json"), Field: "/enabledPlugins/" + escapePointer(name), Native: true, Disabled: value != true, Owners: owners, BuiltFor: []string{"claude"}})
				break
			}
		}
	}
	return items, nil
}

// planMarketplace captures all selected local plugin data and uses explicit user scope.
// Native remove/edit can uninstall dependent plugins. Enable/disable has no native contract.
func (e *engine) planMarketplace(p *plan, change componentRequest) error {
	if p.Spec.ID == "codex" {
		return e.planCodexMarketplace(p, change)
	}
	if p.Spec.ID != "claude" || p.Install.ID == "" || p.Install.Method == "unknown" {
		return fmt.Errorf("marketplace commands require a verified supported harness installation")
	}
	if !targetPattern.MatchString(change.Name) {
		return fmt.Errorf("expected marketplace name is required")
	}
	if !contains([]string{"add", "install", "edit", "update", "remove"}, change.Operation) {
		return fmt.Errorf("marketplaces support add, edit source, refresh and remove. Native enable/disable is unavailable")
	}
	records, err := readMarketplaces(p.StateRoot)
	if err != nil {
		return err
	}
	_, exists := records[change.Name]
	if (change.Operation == "add" || change.Operation == "install") && exists {
		return fmt.Errorf("marketplace already exists")
	}
	if change.Operation != "add" && change.Operation != "install" && !exists {
		return fmt.Errorf("marketplace does not exist")
	}
	// Every cache that the command might remove must be within captured state.
	for _, record := range records {
		if record.InstallLocation != "" {
			if err := validateOwnedPath(filepath.Join(p.StateRoot, "plugins"), record.InstallLocation); err != nil {
				return err
			}
			if _, err := fileIO.lstat(record.InstallLocation); err == nil {
				if err := validateComponentTree(record.InstallLocation); err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	var ledger claudePluginLedger
	if err = readJSON(filepath.Join(p.StateRoot, "plugins", "installed_plugins.json"), &ledger); err != nil && !os.IsNotExist(err) {
		return err
	}
	for name, registrations := range ledger.Plugins {
		if strings.HasSuffix(name, "@"+change.Name) {
			for _, registration := range registrations {
				if registration.Scope == "user" {
					if err := validateOwnedPath(filepath.Join(p.StateRoot, "plugins"), registration.InstallPath); err != nil {
						return err
					}
					if err := validateComponentTree(registration.InstallPath); err != nil {
						return err
					}
				}
			}
		}
	}
	source := change.Source
	if change.Operation == "add" || change.Operation == "install" || change.Operation == "edit" {
		if source == "" || strings.HasPrefix(source, "-") || strings.ContainsAny(source, "\x00\r\n") {
			return fmt.Errorf("marketplace source is required")
		}
		if filepath.IsAbs(source) {
			if within(source, p.StateRoot) || within(p.StateRoot, source) {
				return fmt.Errorf("marketplace source must be independent of selected state")
			}
			if err := validateComponentTree(source); err != nil {
				return err
			}
			p.RootDigests[source], err = fingerprint(source)
			if err != nil {
				return err
			}
		}
	}
	owners := append([]string{p.Spec.ID}, p.Spec.SharedClients...)
	if !within(e.cfg.Root, p.StateRoot) && !ownersSelected(owners, p.Request) {
		p.Blockers = append(p.Blockers, "Select every affected owner for marketplace changes.")
	}
	env := e.launchEnvironment(p.Install, p.StateRoot)
	if change.Scope == "profile" {
		env["HOME"] = filepath.Join(e.reg.Profiles[p.Install.ID].Root, "home")
	}
	step := func(args []string) {
		p.Steps = append(p.Steps, command{Path: p.Install.Path, Args: args, Env: env, Dir: e.cfg.Home, Description: "Native marketplace " + change.Operation})
	}
	switch change.Operation {
	case "add", "install":
		step([]string{"plugin", "marketplace", "add", source, "--scope", "user"})
	case "remove":
		step([]string{"plugin", "marketplace", "remove", change.Name, "--scope", "user"})
	case "update":
		step([]string{"plugin", "marketplace", "update", change.Name})
	case "edit":
		step([]string{"plugin", "marketplace", "remove", change.Name, "--scope", "user"})
		step([]string{"plugin", "marketplace", "add", source, "--scope", "user"})
	}
	p.Component.Native = true
	p.Warnings = append(p.Warnings, "Marketplace remove/edit can uninstall every dependent plugin and delete saved local data. The preview captures the selected plugin tree for encrypted recovery. Remote authorization and external side effects retain native behavior.")
	p.Warnings = append(p.Warnings, "Adding/refreshing a marketplace can install missing plugin dependencies and execute upstream code. Review the selected source.")
	return nil
}

// verifyMarketplace checks the requested local registry state. Native exit alone is insufficient.
func verifyMarketplace(p *plan) error {
	if p.Spec.ID == "codex" {
		return verifyCodexMarketplace(p)
	}
	records, err := readMarketplaces(p.StateRoot)
	if err != nil {
		return err
	}
	record, exists := records[p.Component.Request.Name]
	if p.Component.Request.Operation == "remove" {
		if exists {
			return fmt.Errorf("marketplace remains registered")
		}
		return nil
	}
	if !exists {
		return fmt.Errorf("marketplace registration was not verified")
	}
	if record.InstallLocation == "" {
		return fmt.Errorf("marketplace payload location is missing")
	}
	if err := validateOwnedPath(filepath.Join(p.StateRoot, "plugins"), record.InstallLocation); err != nil {
		return err
	}
	if err := validateComponentTree(record.InstallLocation); err != nil {
		return err
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := readJSON(filepath.Join(record.InstallLocation, ".claude-plugin", "marketplace.json"), &manifest); err != nil || manifest.Name != p.Component.Request.Name {
		return fmt.Errorf("marketplace payload does not match the requested marketplace")
	}
	return nil
}

// managementCategories hides native categories unsupported by the selected harness.
func (m tuiModel) managementCategories() []category {
	result := []category{}
	for _, cat := range componentCategories {
		if cat != marketplaces || contains([]string{"claude", "codex"}, m.e.cfg.Harnesses[m.harness].ID) {
			result = append(result, cat)
		}
	}
	return result
}
