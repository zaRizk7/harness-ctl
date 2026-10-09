package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// codexMarketplace names the native user-configured source and scoped Git cache.
// Local source directories remain externally owned and are never deleted here.
type codexMarketplace struct{ Source, Kind, Root string }

// readCodexMarketplaces reads native TOML registrations, including direct additions.
// Native configuration-layer policy remains enforced by the native command.
func readCodexMarketplaces(root string) (map[string]codexMarketplace, error) {
	path := filepath.Join(root, "config.toml")
	if err := validateOwnedPath(root, path); err != nil {
		return nil, err
	}
	config, err := readConfig(path, "toml")
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	records := map[string]codexMarketplace{}
	value, exists := config["marketplaces"]
	if !exists {
		return records, nil
	}
	table, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid native marketplace table")
	}
	for name, entry := range table {
		fields, ok := entry.(map[string]any)
		if !ok || !targetPattern.MatchString(name) {
			return nil, fmt.Errorf("invalid native marketplace entry")
		}
		source, _ := fields["source"].(string)
		kind, _ := fields["source_type"].(string)
		record := codexMarketplace{Source: source, Kind: kind, Root: filepath.Join(root, ".tmp", "marketplaces", name)}
		if kind == "local" {
			record.Root = source
		}
		records[name] = record
	}
	return records, nil
}

// codexMarketplaceItems emits names and configuration paths without source URLs.
func codexMarketplaceItems(root string, owners []string) ([]componentItem, error) {
	records, err := readCodexMarketplaces(root)
	if err != nil {
		return nil, err
	}
	items := []componentItem{}
	for _, name := range sortedKeys(records) {
		items = append(items, componentItem{Name: name, Category: marketplaces, Path: filepath.Join(root, "config.toml"), Native: true, Owners: owners, BuiltFor: []string{"codex"}})
	}
	return items, nil
}

// planCodexMarketplace creates native scoped commands with captured cache checks.
// Source edit uses native remove/add. Local source roots are fingerprinted reads.
func (e *engine) planCodexMarketplace(p *plan, change componentRequest) error {
	if p.Install.ID == "" || p.Install.Method == "unknown" || !targetPattern.MatchString(change.Name) {
		return fmt.Errorf("marketplace changes require a verified installation and expected name")
	}
	if !contains([]string{"add", "install", "edit", "update", "remove"}, change.Operation) {
		return fmt.Errorf("native marketplaces support add, source edit, upgrade and remove")
	}
	records, err := readCodexMarketplaces(p.StateRoot)
	if err != nil {
		return err
	}
	_, exists := records[change.Name]
	if (change.Operation == "add" || change.Operation == "install") == exists {
		return fmt.Errorf("marketplace existence does not match the requested operation")
	}
	cache := filepath.Join(p.StateRoot, ".tmp", "marketplaces")
	if err := validateOwnedPath(p.StateRoot, cache); err != nil {
		return err
	}
	if _, err := fileIO.lstat(cache); err == nil {
		if err := validateComponentTree(cache); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	source := change.Source
	if contains([]string{"add", "install", "edit"}, change.Operation) {
		if source == "" || strings.HasPrefix(source, "-") || strings.ContainsAny(source, "\x00\r\n") {
			return fmt.Errorf("native marketplace source is required")
		}
		if filepath.IsAbs(source) {
			if within(source, p.StateRoot) || within(p.StateRoot, source) {
				return fmt.Errorf("local marketplace source must be independent of selected state")
			}
			if err := validateComponentTree(source); err != nil {
				return err
			}
			digest, err := fingerprint(source)
			if err != nil {
				return err
			}
			p.RootDigests[source] = digest
		}
	}
	owners := append([]string{p.Spec.ID}, p.Spec.SharedClients...)
	if !ownersSelected(owners, p.Request) {
		p.Blockers = append(p.Blockers, "Select every affected owner for native marketplace changes.")
	}
	step := func(args []string) error {
		c, err := e.launchCommand(p.Install, args, true)
		if err != nil {
			return err
		}
		c.Dir = e.cfg.Home
		c.Description = "Native marketplace " + change.Operation
		p.Steps = append(p.Steps, c)
		return nil
	}
	switch change.Operation {
	case "add", "install":
		err = step([]string{"plugin", "marketplace", "add", source})
	case "remove":
		err = step([]string{"plugin", "marketplace", "remove", change.Name})
	case "update":
		err = step([]string{"plugin", "marketplace", "upgrade", change.Name})
	case "edit":
		if err = step([]string{"plugin", "marketplace", "remove", change.Name}); err == nil {
			err = step([]string{"plugin", "marketplace", "add", source})
		}
	}
	if err != nil {
		return err
	}
	p.Component.Native = true
	p.Warnings = append(p.Warnings, "Native marketplace configuration and scoped caches are captured for recovery. Local source repositories retain external ownership. Installed-version/configuration policies may refuse the command. Review upstream code before adding or upgrading.")
	return nil
}

// verifyCodexMarketplace verifies the resulting registration and native manifest.
// Native exit success alone cannot prove the expected named source was registered.
func verifyCodexMarketplace(p *plan) error {
	records, err := readCodexMarketplaces(p.StateRoot)
	if err != nil {
		return err
	}
	record, exists := records[p.Component.Request.Name]
	if p.Component.Request.Operation == "remove" {
		if exists {
			return fmt.Errorf("native marketplace remains registered")
		}
		return nil
	}
	if !exists {
		return fmt.Errorf("native marketplace registration is absent")
	}
	if record.Kind == "local" {
		if !filepath.IsAbs(record.Root) {
			return fmt.Errorf("local marketplace source is not absolute")
		}
		if err := rejectLinkedAncestors(record.Root); err != nil {
			return err
		}
	} else if err := validateOwnedPath(p.StateRoot, record.Root); err != nil {
		return err
	}
	if err := validateComponentTree(record.Root); err != nil {
		return err
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := readJSON(filepath.Join(record.Root, ".agents", "plugins", "marketplace.json"), &manifest); err != nil || manifest.Name != p.Component.Request.Name {
		return fmt.Errorf("native marketplace manifest does not match the approved name")
	}
	return nil
}
