package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/editor"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// componentsMsg returns item inventory for the selected category and state scope.
type componentsMsg struct {
	items  []componentItem
	owners []string
	err    error
}

// componentEditorMsg carries a validated edit and its source freshness check.
type componentEditorMsg struct {
	change    componentRequest
	err       error
	cancelled bool
}

// componentInstallation returns the selected installation identity for
// component operations, including a state-only harness when absent.
func (m tuiModel) componentInstallation() installation {
	inst := m.selectedInst()
	if inst.Harness == "" {
		inst = m.e.retainedInstallation(m.e.cfg.Harnesses[m.harness].ID)
	}
	return inst
}

// loadComponents returns a command collecting named entries and affected owners
// for the selected category/scope.
func (m tuiModel) loadComponents() tea.Cmd {
	return func() tea.Msg {
		inst := m.componentInstallation()
		items, err := m.e.components(inst, m.componentScope, m.componentCat)
		var owners []string
		for _, item := range items {
			for _, owner := range item.Owners {
				if owner != inst.Harness && !contains(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
		if m.componentScope != "profile" && !inst.Managed {
			for _, owner := range m.e.cfg.Harnesses[m.harness].SharedClients {
				if !contains(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
		return componentsMsg{items: items, owners: owners, err: err}
	}
}

// componentKey handles the category, item and owner control key. It returns
// updated state and an optional preview/editor command.
func (m tuiModel) componentKey(key string) (tea.Model, tea.Cmd) {
	switch m.screen {
	case "component-groups":
		if key == "enter" {
			m.componentCat = m.managementCategories()[m.cursor]
			m.screen, m.status = "loading", "Reading selected component sources…"
			return m, m.loadComponents()
		}
	case "component-owners":
		if key == "enter" {
			m.screen, m.cursor = "components", 0
		}
		if key == "space" && m.cursor < len(m.owners) {
			owner := m.owners[m.cursor]
			if contains(m.req.Owners, owner) {
				var selected []string
				for _, existing := range m.req.Owners {
					if existing != owner {
						selected = append(selected, existing)
					}
				}
				m.req.Owners = selected
			} else {
				m.req.Owners = append(m.req.Owners, owner)
			}
		}
	case "components":
		if key == "o" {
			m.screen, m.cursor = "component-owners", 0
			return m, nil
		}
		if key == "p" {
			if _, exists := m.e.reg.Profiles[m.componentInstallation().ID]; !exists {
				m.status = "No active launch profile. This screen manages base state."
				return m, nil
			}
			if m.componentScope == "base" {
				m.componentScope = "profile"
			} else {
				m.componentScope = "base"
			}
			m.req.Owners = nil
			return m, m.loadComponents()
		}
		if key == "a" {
			return m, m.editComponent(nil)
		}
		if key == "r" {
			return m, m.loadComponents()
		}
		if len(m.componentItems) == 0 || m.cursor >= len(m.componentItems) {
			return m, nil
		}
		item := m.componentItems[m.cursor]
		if key == "e" || key == "enter" {
			if item.Directory {
				browsePath := item.Path
				if item.Parked != "" {
					browsePath = filepath.Join(item.Parked, "payload")
				}
				if err := validateComponentTree(browsePath); err != nil {
					m.status = err.Error()
					return m, nil
				}
				var files []componentItem
				err := fileIO.walk(browsePath, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !entry.IsDir() {
						child := item
						child.Path, child.Name = path, strings.TrimPrefix(path, browsePath+string(filepath.Separator))
						if item.Parked != "" {
							child.Path, child.Subpath = item.Path, child.Name
						}
						child.Directory, child.Native = false, false
						files = append(files, child)
					}
					return nil
				})
				m.componentItems, m.cursor = files, 0
				if err != nil {
					m.status = err.Error()
				} else {
					m.status = "Select a file to edit. r returns to the component list."
				}
				return m, nil
			}
			return m, m.editComponent(&item)
		}
		operation := map[string]string{"d": "disable", "u": "enable", "x": "remove", "t": "update"}[key]
		if key == "space" {
			operation = "disable"
			if item.Disabled {
				operation = "enable"
			}
		}
		if operation != "" {
			if item.Subpath != "" {
				m.status = "Disabled child files support editing. r returns to the component for enable/remove."
				return m, nil
			}
			m.req.Component = &componentRequest{Operation: operation, Category: item.Category, Path: item.Path, Field: item.Field, Scope: m.componentScope, Parked: item.Parked, Native: item.Native && !(m.req.Harness == "pi" && (operation == "disable" || operation == "enable")), Name: item.Name}
			m.screen, m.status = "loading", "Creating a component preview…"
			return m, m.preview()
		}
	}
	return m, nil
}

// componentView returns secret-free component rows and owner/source metadata
// for the active component screen.
func (m tuiModel) componentView() []string {
	lines := []string{"Component management / " + m.componentScope + " state"}
	switch m.screen {
	case "component-groups":
		var groups []string
		for _, cat := range m.managementCategories() {
			groups = append(groups, string(cat))
		}
		lines = append(lines, m.listWindow(groups)...)
	case "component-owners":
		if len(m.owners) == 0 {
			return append(lines, "No additional affected owners.")
		}
		var rows []string
		for _, owner := range m.owners {
			rows = append(rows, mark(contains(m.req.Owners, owner))+" Include affected owner: "+owner)
		}
		lines = append(lines, m.listWindow(rows)...)
	case "components":
		lines = append(lines, string(m.componentCat), "Local assets and registrations. Account connections retain native authorization.", "")
		var rows []string
		for _, item := range m.componentItems {
			status := "registered"
			if item.Disabled {
				status = "disabled"
			} else if item.Native {
				status = "native managed"
			}
			rows = append(rows, item.Name+" ["+status+"]")
		}
		if len(rows) == 0 {
			lines = append(lines, "No entries. a opens an add/install request in your editor.")
		} else {
			lines = append(lines, m.listWindow(rows)...)
			if m.cursor < len(m.componentItems) {
				item := m.componentItems[m.cursor]
				lines = append(lines, "", "Source: "+item.Path, "Field: "+item.Field, "Owners: "+strings.Join(item.Owners, ", "))
				if item.Category == plugins {
					compatibility := strings.Join(item.BuiltFor, ", ")
					if compatibility == "" {
						compatibility = "unverified local asset"
					}
					lines = append(lines, "Built for: "+compatibility)
				}
			}
		}
	}
	return lines
}

// addComponentTemplate returns a scoped request template for the selected
// native category, without creating or installing an asset.
func (m tuiModel) addComponentTemplate() componentRequest {
	inst := m.componentInstallation()
	state, s, err := m.e.componentEngine(inst, m.componentScope)
	if err != nil {
		return componentRequest{Operation: "add", Category: m.componentCat}
	}
	root := state.stateRoot(s)
	change := componentRequest{Operation: "add", Category: m.componentCat, Scope: m.componentScope}
	if m.componentCat == marketplaces {
		change.Native = true
		change.Name = "my-marketplace"
		change.Source = "owner/repository"
		return change
	}
	if m.componentCat == skills {
		if s.ID == "codex" {
			root = state.codexSkillsRoot()
		}
		change.Operation, change.Path, change.Source = "install", filepath.Join(root, "skills", "my-skill"), "/absolute/path/to/skill"
		return change
	}
	if m.componentCat == plugins && (s.ID == "claude" || s.ID == "gemini" || s.ID == "pi") && inst.ID != "" {
		change.Operation, change.Native, change.Source = "install", true, "plugin-name@marketplace"
		if s.ID == "pi" {
			change.Source = "npm:@example/pi-tools@1.0.0"
			change.Name = change.Source
		}
		if s.ID == "gemini" {
			change.Source = "/absolute/path/to/extension"
			change.Name = "my-extension"
		}
		return change
	}
	change.Path = filepath.Join(root, string(m.componentCat), "my-entry.json")
	change.Text = "{}\n"
	group := ""
	if m.componentCat == mcp {
		if s.ID == "claude" || s.ID == "gemini" {
			group = "mcpServers"
		}
		if s.ID == "codex" {
			group = "mcp_servers"
		} else if s.ID == "opencode" {
			group = "mcp"
		} else if s.ID == "claude" && !inst.Managed {
			change.Path = filepath.Join(m.e.cfg.Home, ".claude.json")
		}
	}
	if s.ID == "codex" && m.componentCat == connectors {
		group = "apps"
	}
	if group != "" {
		if !(s.ID == "claude" && m.componentCat == mcp && !inst.Managed) {
			change.Path = filepath.Join(root, s.ConfigFiles[0])
		}
		change.Field, change.Value, change.Text = "/"+group+"/my-entry", json.RawMessage(`{}`), ""
	}
	if group != "" && m.componentCat == mcp {
		change.Value = json.RawMessage(`{"command":"/absolute/path/to/server","args":[]}`)
		if s.ID == "opencode" {
			change.Value = json.RawMessage(`{"type":"local","command":["/absolute/path/to/server"],"enabled":true}`)
		}
	}
	if s.ID == "codex" && m.componentCat == connectors {
		change.Value = json.RawMessage(`{"enabled":true}`)
	}
	for _, item := range m.componentItems {
		if item.Field != "" {
			change.Path = item.Path
			change.Field = item.Field[:strings.LastIndex(item.Field, "/")+1] + "my-entry"
			change.Text = ""
			if len(change.Value) == 0 {
				change.Value = json.RawMessage(`{}`)
			}
			state, s, err := m.e.componentEngine(inst, m.componentScope)
			parts, _ := pointerParts(item.Field)
			if err == nil {
				r, resourceErr := state.componentResource(s, item.Path, item.Field, item.Category)
				if resourceErr == nil {
					value, readErr := readConfig(item.Path, r.Format)
					parent, _ := pointerValue(value, parts[:len(parts)-1])
					if _, isArray := parent.([]any); readErr == nil && isArray {
						change.Field = item.Field[:strings.LastIndex(item.Field, "/")+1] + "-"
					}
				}
			}
			break
		}
	}
	return change
}

// editComponent opens a private external editor for item or a new request when
// item is nil. Its command reports validated changes or cancellation.
func (m tuiModel) editComponent(item *componentItem) tea.Cmd {
	var data []byte
	var err error
	change := m.addComponentTemplate()
	before := ""
	if item == nil {
		data, err = marshalJSONIndent(change, "", "  ")
	} else {
		change = componentRequest{Operation: "edit", Category: item.Category, Path: item.Path, Field: item.Field, Scope: m.componentScope, Parked: item.Parked, Subpath: item.Subpath}
		before, err = componentEditFingerprint(*item)
		if err == nil {
			data, err = m.e.componentEditData(m.componentInstallation(), m.componentScope, *item)
		}
		if err == nil {
			after, fingerprintErr := componentEditFingerprint(*item)
			if fingerprintErr != nil || after != before {
				err = fmt.Errorf("source changed while preparing the editor")
			}
		}
	}
	if err != nil {
		return func() tea.Msg { return componentEditorMsg{err: err} }
	}
	extension := ".json"
	if item != nil && item.Field == "" {
		extension = filepath.Ext(item.Path)
		if item.Subpath != "" {
			extension = filepath.Ext(item.Subpath)
		}
	}
	return editor.Open("harness-ctl-component-*"+extension, data, func(path string, editorErr error) tea.Msg {
		return m.finishComponentEdit(path, data, change, item, before, editorErr)
	})
}

// componentEditFingerprint returns the active/parked source fingerprint for
// item, so editing cannot silently overwrite a newer source.
func componentEditFingerprint(item componentItem) (string, error) {
	path := item.Path
	if item.Parked != "" {
		path = item.Parked
	}
	return fingerprint(path)
}

// finishComponentEdit validates the external editor result and always removes
// its private temporary file, including cancellation and validation failures.
func (m tuiModel) finishComponentEdit(path string, data []byte, change componentRequest, item *componentItem, before string, editorErr error) componentEditorMsg {
	defer fileIO.remove(path)
	if editorErr != nil {
		return componentEditorMsg{err: fmt.Errorf("editor failed: %w", editorErr)}
	}
	file, err := fileIO.open(path)
	if err != nil {
		return componentEditorMsg{err: err}
	}
	defer file.Close()
	edited, err := io.ReadAll(io.LimitReader(file, m.e.cfg.MetadataBytes+1))
	if err != nil || int64(len(edited)) > m.e.cfg.MetadataBytes {
		return componentEditorMsg{err: fmt.Errorf("edited component exceeds metadata limit or cannot be read")}
	}
	if item == nil {
		decoder := json.NewDecoder(bytes.NewReader(edited))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&change); err != nil {
			return componentEditorMsg{err: fmt.Errorf("add/install request must be valid JSON: %w", err)}
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return componentEditorMsg{err: fmt.Errorf("add/install request must contain one JSON document")}
		}
		if change.Operation != "add" && change.Operation != "install" {
			return componentEditorMsg{err: fmt.Errorf("add/install editor supports add or install only")}
		}
		if change.Category != m.componentCat {
			return componentEditorMsg{err: fmt.Errorf("request category changed. Select that category first")}
		}
	} else {
		current, err := componentEditFingerprint(*item)
		if err != nil || current != before {
			return componentEditorMsg{err: fmt.Errorf("source changed while editing. Refresh and edit again")}
		}
		if item.Field == "" {
			change.Content = edited
		} else {
			change.Value = edited
		}
	}
	if bytes.Equal(data, edited) {
		return componentEditorMsg{cancelled: true}
	}
	return componentEditorMsg{change: change}
}
