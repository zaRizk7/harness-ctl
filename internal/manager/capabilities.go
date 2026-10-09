package manager

import (
	"path/filepath"
	"slices"
	"strings"
)

// hasLocalState checks the selected native/managed roots without reading values.
func (m tuiModel) hasLocalState() bool {
	inst := m.selectedInst()
	// Selection comes from the validated catalog. Base scope has no profile lookup.
	state, s, _ := m.e.componentEngine(m.componentInstallation(), "base")
	roots := state.rootsFor(s)
	if s.ID == "codex" {
		root := state.codexSkillsRoot()
		roots = append(roots, filepath.Join(root, "skills"), filepath.Join(root, disabledComponentsDir, "skills"))
	}
	if !inst.Managed {
		roots = append(roots, m.e.managedStateRoot(s))
	}
	for _, root := range roots {
		if entries, err := fileIO.readDir(root); err == nil && len(entries) > 0 {
			return true
		}
	}
	return false
}

// actions exposes only operations supported by the selected installation/state.
func (m tuiModel) actions() []string {
	inst := m.selectedInst()
	state := m.hasLocalState()
	var result []string
	if !inst.Managed {
		result = append(result, "install")
	}
	if inst.ID != "" {
		if inst.Method != "unknown" {
			result = append(result, "update", "reinstall")
		}
		if state {
			result = append(result, "reset")
		}
		result = append(result, "uninstall")
		if !inst.Managed && inst.Method != "unknown" {
			result = append(result, "migrate")
		}
		if inst.Managed && state {
			result = append(result, "profile")
		}
	}
	if inst.ID != "" || state {
		result = append(result, "manage")
	}
	return result
}

// actionName resolves a protocol action to its display label.
func actionName(id string) string {
	for i, value := range actionIDs {
		if value == id {
			return actionNames[i]
		}
	}
	return id
}

// batchActions returns the lifecycle actions available to every selected harness.
func (m tuiModel) batchActions() []string {
	result := append([]string{}, actionIDs[:5]...)
	for i, s := range m.e.cfg.Harnesses {
		if m.batchSelected[s.ID] {
			copy := m
			copy.harness = i
			copy.installCursor = 0
			allowed := copy.actions()
			result = slices.DeleteFunc(result, func(id string) bool { return !slices.Contains(allowed, id) })
		}
	}
	return result
}

// option represents a visible preservation control identified independently of its row.
type option struct{ id, label string }

// options hides irrelevant state controls when there are no retained resources.
func (m tuiModel) options() []option {
	var rows []option
	if len(m.optionCategories) > 0 {
		rows = append(rows, option{"preset", "Cycle preservation preset"})
	}
	if m.req.Action == "install" || m.req.Action == "update" || m.req.Action == "reinstall" || m.req.Action == "migrate" {
		rows = append(rows, option{"target", "Target version: " + m.req.Target + " (blank = latest)"})
	}
	if len(m.optionCategories) > 0 {
		rows = append(rows, option{"permanent", mark(m.req.Permanent) + " Permanent discard, erase recovery after completion"})
		for _, cat := range m.optionCategories {
			rows = append(rows, option{"cat:" + string(cat), mark(m.req.Preserve[cat]) + " Preserve " + string(cat)})
		}
	}
	for _, owner := range m.owners {
		rows = append(rows, option{"owner:" + owner, mark(contains(m.req.Owners, owner)) + " Include affected owner: " + owner})
	}
	return rows
}

// presentCategories collects present preservation classes in stable protocol order.
func presentCategories(resources []resource) []category {
	seen := map[category]bool{}
	for _, r := range resources {
		if len(r.Fields) == 0 {
			seen[r.Category] = true
		} else {
			for _, cat := range r.Fields {
				seen[cat] = true
			}
		}
	}
	var result []category
	for _, cat := range categories {
		if seen[cat] {
			result = append(result, cat)
		}
	}
	return result
}

// toggleOption identifies the selected control without depending on hidden rows.
func (m *tuiModel) toggleOption() {
	rows := m.options()
	if m.cursor >= len(rows) {
		return
	}
	id := rows[m.cursor].id
	switch id {
	case "preset":
		all := true
		for _, cat := range m.optionCategories {
			all = all && m.req.Preserve[cat]
		}
		if all {
			m.req.Preserve = map[category]bool{auth: true}
		} else if len(m.req.Preserve) == 1 && m.req.Preserve[auth] {
			m.req.Preserve = map[category]bool{}
		} else {
			m.req.Preserve = keepAll()
		}
	case "target":
		m.editing = true
	case "permanent":
		m.req.Permanent = !m.req.Permanent
	default:
		if strings.HasPrefix(id, "cat:") {
			cat := category(strings.TrimPrefix(id, "cat:"))
			m.req.Preserve[cat] = !m.req.Preserve[cat]
		}
		if strings.HasPrefix(id, "owner:") {
			owner := strings.TrimPrefix(id, "owner:")
			if contains(m.req.Owners, owner) {
				m.req.Owners = slices.DeleteFunc(m.req.Owners, func(s string) bool { return s == owner })
			} else {
				m.req.Owners = append(m.req.Owners, owner)
			}
		}
	}
}

// retainedInstallation selects a manager-owned state root when only states remain.
func (e *engine) retainedInstallation(id string) installation {
	s, err := e.specFor(id)
	if err != nil {
		return installation{Harness: id}
	}
	root := e.managedStateRoot(s)
	if entries, err := fileIO.readDir(root); err == nil && len(entries) > 0 {
		return installation{Harness: id, Managed: true, StateRoot: filepath.Clean(root)}
	}
	return installation{Harness: id}
}
