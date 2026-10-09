package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/editor"
	"io"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zaRizk7/harness-ctl/internal/library"
)

// libraryMsg carries secret-free reusable inventory with a vault freshness token.
type libraryMsg struct {
	items  []library.View
	before string
	err    error
}

// libraryEditMsg carries a private editor request and its original vault fingerprint.
type libraryEditMsg struct {
	item      library.Item
	before    string
	err       error
	cancelled bool
}

// libraryApplyMsg returns a compatible fan-out preview.
type libraryApplyMsg struct {
	plan *libraryApply
	err  error
}

// loadLibraryView inventories reusable metadata without putting secrets in the model.
func (m tuiModel) loadLibraryView() tea.Cmd {
	return func() tea.Msg {
		path, err := m.e.libraryPath()
		if err != nil {
			return libraryMsg{err: err}
		}
		before, err := fingerprint(path)
		if err != nil {
			return libraryMsg{err: err}
		}
		items, err := m.e.loadLibrary()
		return libraryMsg{items: libraryViews(items), before: before, err: err}
	}
}

// libraryUpdate integrates reusable inventory/edit messages without mutations.
func (m tuiModel) libraryUpdate(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case libraryMsg:
		m.screen = "library"
		m.libraryItems = msg.items
		m.libraryBefore = msg.before
		m.cursor = min(m.cursor, max(0, len(msg.items)-1))
		m.status = ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, nil, true
	case libraryEditMsg:
		m.screen = "library"
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil, true
		}
		if msg.cancelled {
			return m, nil, true
		}
		m.libraryEdit = msg
		m.libraryAction = "save"
		m.screen = "library-record-preview"
		m.typed = ""
		return m, nil, true
	case libraryApplyMsg:
		if msg.err != nil {
			m.screen = "library-select"
			m.status = msg.err.Error()
			return m, nil, true
		}
		m.libraryApplication = msg.plan
		m.batch = msg.plan.Batch
		m.batchMode = true
		m.screen = "preview"
		m.req = request{Action: "manage"}
		m.cursor = 0
		m.typed = ""
		return m, nil, true
	}
	return m, nil, false
}

// libraryTargets returns explicitly compatible harnesses with local install/state.
func (m tuiModel) libraryTargets() []string {
	var compatible []string
	for _, item := range m.libraryItems {
		if item.ID == m.libraryID {
			compatible = item.Harnesses
		}
	}
	var ids []string
	for i, s := range m.e.cfg.Harnesses {
		if slices.Contains(compatible, s.ID) {
			copy := m
			copy.harness = i
			copy.installCursor = 0
			if copy.selectedInst().ID != "" || copy.hasLocalState() {
				ids = append(ids, s.ID)
			}
		}
	}
	return ids
}

// libraryKey owns reusable records, compatible target selection and shared owners.
func (m tuiModel) libraryKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.screen == "home" && key == "l" {
		m.screen = "loading"
		m.cursor = 0
		return m, m.loadLibraryView(), true
	}
	switch m.screen {
	case "library":
		if key == "a" {
			return m, m.editLibrary(""), true
		}
		if key == "r" {
			return m, m.loadLibraryView(), true
		}
		if len(m.libraryItems) == 0 {
			return m, nil, true
		}
		item := m.libraryItems[m.cursor]
		m.libraryID = item.ID
		switch key {
		case "e":
			return m, m.editLibrary(item.ID), true
		case "d", "u", "x":
			m.libraryAction = map[string]string{"d": "disable", "u": "enable", "x": "remove"}[key]
			m.screen = "library-record-preview"
			m.typed = ""
			return m, nil, true
		case "enter":
			if !item.Enabled {
				m.status = "Enable the library entry before applying"
				return m, nil, true
			}
			m.librarySelected = map[string]bool{}
			m.libraryApplication = nil
			m.req.Owners = nil
			m.screen = "library-select"
			m.cursor = 0
			return m, nil, true
		}
	case "library-select":
		ids := m.libraryTargets()
		if key == "space" && m.cursor < len(ids) {
			id := ids[m.cursor]
			m.librarySelected[id] = !m.librarySelected[id]
		}
		if key == "o" {
			owners := []string{}
			for _, id := range ids {
				if m.librarySelected[id] {
					s, _ := m.e.specFor(id)
					for _, owner := range s.SharedClients {
						if !contains(owners, owner) {
							owners = append(owners, owner)
						}
					}
					if id == "codex" {
						owners = append(owners, "Other agents using ~/.agents/skills")
					}
				}
			}
			m.owners = owners
			m.screen = "library-owners"
			m.cursor = 0
		}
		if key == "enter" {
			selected := []string{}
			for _, id := range ids {
				if m.librarySelected[id] {
					selected = append(selected, id)
				}
			}
			m.screen = "loading"
			return m, func() tea.Msg {
				p, err := m.e.buildLibraryApply(context.Background(), m.libraryID, selected, m.req.Owners)
				return libraryApplyMsg{p, err}
			}, true
		}
		return m, nil, true
	case "library-owners":
		if key == "enter" {
			m.screen = "library-select"
			m.cursor = 0
		}
		if key == "space" && m.cursor < len(m.owners) {
			owner := m.owners[m.cursor]
			if contains(m.req.Owners, owner) {
				m.req.Owners = slices.DeleteFunc(m.req.Owners, func(s string) bool { return s == owner })
			} else {
				m.req.Owners = append(m.req.Owners, owner)
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// libraryConfirm applies a reusable record change only after typed approval.
func (m tuiModel) libraryConfirm() (tea.Model, tea.Cmd, bool) {
	if m.screen != "library-record-preview" {
		return m, nil, false
	}
	if m.typed != "apply" {
		m.status = "Type apply to approve the local library change"
		return m, nil, true
	}
	return m, m.startOperation(func(_ context.Context, _ func(string)) error {
		if m.libraryAction == "save" {
			return m.e.saveLibraryItem(m.libraryEdit.item, m.libraryEdit.before)
		}
		return m.e.changeLibraryItem(m.libraryID, m.libraryAction, m.libraryBefore)
	}), true
}

// libraryView renders shared entry controls and compatibility without content.
func (m tuiModel) libraryView() ([]string, string) {
	switch m.screen {
	case "library":
		rows := []string{}
		for _, item := range m.libraryItems {
			rows = append(rows, fmt.Sprintf("%s / %s %s / %s", item.ID, item.Category, mark(item.Enabled), strings.Join(item.Harnesses, ", ")))
		}
		if len(rows) == 0 {
			rows = append(rows, "No reusable entries")
		}
		return m.listWindow(rows), "a add · e edit · d disable · u enable · x remove · Enter apply to harnesses · r refresh"
	case "library-select":
		rows := []string{}
		for _, id := range m.libraryTargets() {
			rows = append(rows, mark(m.librarySelected[id])+" "+id)
		}
		return m.listWindow(rows), "Space selects compatible installed/retained harness · o affected owners · Enter preview"
	case "library-owners":
		rows := []string{}
		for _, owner := range m.owners {
			rows = append(rows, mark(contains(m.req.Owners, owner))+" "+owner)
		}
		return m.listWindow(rows), "Space select affected owner · Enter return to targets"
	case "library-record-preview":
		id := m.libraryID
		if m.libraryAction == "save" {
			id = m.libraryEdit.item.ID
		}
		return []string{"Library entry: " + id, "Action: " + m.libraryAction, "Local reusable library changes. Applying to harnesses requires a separate approved preview."}, "Type apply and Enter · Esc cancel"
	}
	return nil, ""
}

// editLibrary opens an explicit private JSON editor. Its temporary file is removed
// after bounded validation. Existing source bytes remain independent of harness files.
func (m tuiModel) editLibrary(id string) tea.Cmd {
	path, err := m.e.libraryPath()
	if err != nil {
		return func() tea.Msg { return libraryEditMsg{err: err} }
	}
	before, err := fingerprint(path)
	if err != nil {
		return func() tea.Msg { return libraryEditMsg{err: err} }
	}
	item := library.Item{ID: "my-skill", Category: "skills", Enabled: true, Source: "/absolute/path/to/skill", Targets: map[string]library.Target{"codex": {Path: "skills/my-skill", HomeSkills: true}}}
	if id != "" {
		items, err := m.e.loadLibrary()
		if err != nil {
			return func() tea.Msg { return libraryEditMsg{err: err} }
		}
		for _, entry := range items {
			if entry.ID == id {
				item = entry
			}
		}
	}
	data, err := marshalJSONIndent(item, "", "  ")
	if err != nil {
		return func() tea.Msg { return libraryEditMsg{err: err} }
	}
	return editor.Open("harness-ctl-library-*.json", data, func(temp string, editorErr error) tea.Msg {
		return m.finishLibraryEdit(temp, data, item, before, editorErr)
	})
}

// finishLibraryEdit validates a bounded single document and removes the private request.
func (m tuiModel) finishLibraryEdit(temp string, data []byte, item library.Item, before string, editorErr error) libraryEditMsg {
	defer fileIO.remove(temp)
	if editorErr != nil {
		return libraryEditMsg{err: fmt.Errorf("library editor failed")}
	}
	f, err := fileIO.open(temp)
	if err != nil {
		return libraryEditMsg{err: err}
	}
	defer f.Close()
	edited, err := io.ReadAll(io.LimitReader(f, m.e.cfg.MetadataBytes+1))
	if err != nil || int64(len(edited)) > m.e.cfg.MetadataBytes {
		return libraryEditMsg{err: fmt.Errorf("library edit exceeds size limit")}
	}
	if string(data) == string(edited) {
		return libraryEditMsg{cancelled: true}
	}
	decoder := json.NewDecoder(strings.NewReader(string(edited)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&item); err != nil {
		return libraryEditMsg{err: fmt.Errorf("invalid library request")}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return libraryEditMsg{err: fmt.Errorf("library edit requires one document")}
	}
	item, err = library.Capture(item, m.e.cfg.MetadataBytes)
	if err == nil {
		err = m.e.validateLibraryItem(item)
	}
	return libraryEditMsg{item: item, before: before, err: err}
}
