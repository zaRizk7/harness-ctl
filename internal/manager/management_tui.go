package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/editor"
	"io"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// batchPlanMsg returns the combined ordered lifecycle preview.
type batchPlanMsg struct {
	batch *batchPlan
	err   error
}

// selfPlanMsg returns a manager-removal preview without executing it.
type selfPlanMsg struct {
	plan *selfPlan
	err  error
}

// accountsMsg returns secret-free account inventory and current provider reports.
type accountsMsg struct {
	items   []accountView
	metrics []accountMetric
	before  string
	err     error
}

// accountTick requests a report refresh while the account screen is open.
type accountTick struct{}

// accountEditMsg carries a validated editor request and its original vault
// fingerprint. Cancelled editors produce no vault mutation.
type accountEditMsg struct {
	item      account
	before    string
	err       error
	cancelled bool
}

// batchRequests applies the displayed common options to each checked harness.
func (m tuiModel) batchRequests() []request {
	var requests []request
	for i, s := range m.e.cfg.Harnesses {
		if m.batchSelected[s.ID] {
			copy := m
			copy.harness = i
			copy.installCursor = 0
			req := m.req
			req.Harness = s.ID
			req.InstallID = copy.selectedInst().ID
			requests = append(requests, req)
		}
	}
	return requests
}

// batchOptions unions affected owners so one preview can show complete shared
// ownership controls before building the ordered transaction batch.
func (m tuiModel) batchOptions() tea.Cmd {
	return func() tea.Msg {
		var owners []string
		var cats []category
		for i, s := range m.e.cfg.Harnesses {
			if m.batchSelected[s.ID] {
				copy := m
				copy.harness = i
				copy.installCursor = 0
				msg := copy.loadOptions()().(optionsMsg)
				if msg.err != nil {
					return msg
				}
				for _, cat := range msg.categories {
					if !slices.Contains(cats, cat) {
						cats = append(cats, cat)
					}
				}
				for _, o := range msg.owners {
					if !contains(owners, o) {
						owners = append(owners, o)
					}
				}
			}
		}
		return optionsMsg{owners: owners, categories: cats}
	}
}

// loadAccountsView fetches only enabled accounts when the account screen is open.
// Decrypted credentials never enter the rendered inventory or model messages.
func (m tuiModel) loadAccountsView() tea.Cmd {
	return func() tea.Msg {
		items, before, err := m.e.readAccountState()
		if err != nil {
			return accountsMsg{err: err}
		}
		return accountsMsg{items: accountViews(items), before: before}
	}
}

// accountTimer schedules another refresh without blocking keyboard input.
func (m tuiModel) accountTimer() tea.Cmd {
	return tea.Tick(time.Duration(m.e.cfg.RefreshSeconds)*time.Second, func(time.Time) tea.Msg { return accountTick{} })
}

// managementUpdate owns messages for batch/self approval and account monitoring.
func (m tuiModel) managementUpdate(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case globalMonitorMsg:
		m.monitorError = ""
		if msg.err != nil {
			m.monitorError = msg.err.Error()
		} else {
			m.accountMetrics = msg.metrics
		}
		return m, m.accountTimer(), true
	case batchPlanMsg:
		m.batch = msg.batch
		m.screen = "preview"
		m.cursor = 0
		m.typed = ""
		m.status = ""
		if msg.err != nil {
			m.screen = "result"
			m.status = msg.err.Error()
		}
		return m, nil, true
	case selfPlanMsg:
		m.self = msg.plan
		m.screen = "self-preview"
		m.typed = ""
		m.cursor = 0
		if msg.err != nil {
			m.screen = "result"
			m.status = msg.err.Error()
		}
		return m, nil, true
	case accountsMsg:
		if m.screen != "accounts" && m.screen != "accounts-loading" {
			return m, nil, true
		}
		if msg.err == nil {
			m.accountItems = msg.items

			m.accountBefore = msg.before
		}
		m.screen = "accounts"
		m.cursor = min(m.cursor, max(0, len(msg.items)-1))
		m.status = ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, nil, true
	case accountTick:
		return m, m.loadGlobalMonitor(), true
	case accountEditMsg:
		if msg.err != nil {
			m.screen = "accounts"
			m.status = msg.err.Error()
			return m, nil, true
		}
		if msg.cancelled {
			m.screen = "accounts"
			return m, nil, true
		}
		m.accountEdit = msg
		m.accountAction = "save"
		m.screen = "account-preview"
		m.typed = ""
		return m, nil, true
	}
	return m, nil, false
}

// managementKey dispatches dedicated screen controls after common navigation.
func (m tuiModel) managementKey(key string) (tea.Model, tea.Cmd, bool) {
	switch m.screen {
	case "home":
		switch key {
		case "b":
			m.libraryApplication = nil
			m.batchMode = true
			m.batchSelected = map[string]bool{}
			m.batch = nil
			m.screen = "batch-select"
			m.cursor = 0
			return m, nil, true
		case "a":
			m.screen = "accounts-loading"
			m.cursor = 0
			m.status = "Reading account reports…"
			return m, m.loadAccountsView(), true
		case "u":
			m.screen = "self-options"
			m.cursor = 0
			m.self = nil
			m.selfOwners = nil
			return m, nil, true
		}
	case "batch-select":
		if key == "space" {
			s := m.e.cfg.Harnesses[m.cursor]
			m.batchSelected[s.ID] = !m.batchSelected[s.ID]
		}
		if key == "enter" {
			if len(m.batchRequests()) == 0 {
				m.status = "Select at least one harness"
			} else {
				m.screen = "batch-actions"
				m.cursor = 0
			}
		}
		return m, nil, true
	case "batch-actions":
		if key == "enter" {
			actions := m.batchActions()
			if m.cursor >= len(actions) {
				return m, nil, true
			}
			m.req = request{Action: actions[m.cursor], Preserve: keepAll()}
			if m.req.Action == "reset" {
				m.req.Preserve = map[category]bool{}
			}
			m.screen = "loading"
			return m, m.batchOptions(), true
		}
		return m, nil, true
	case "self-options":
		if key == "o" && m.selfRemoveHarnesses {
			m.selfOwnerCandidates = nil
			for _, inst := range m.installs {
				s, _ := m.e.specFor(inst.Harness)
				for _, owner := range append([]string{s.ID}, s.SharedClients...) {
					if !contains(m.selfOwnerCandidates, owner) {
						m.selfOwnerCandidates = append(m.selfOwnerCandidates, owner)
					}
				}
				if s.ID == "codex" && !contains(m.selfOwnerCandidates, "Other agents using ~/.agents/skills") {
					m.selfOwnerCandidates = append(m.selfOwnerCandidates, "Other agents using ~/.agents/skills")
				}
			}
			m.screen = "self-owners"
			m.cursor = 0
			return m, nil, true
		}
		if key == "space" {
			switch m.cursor {
			case 0:
				m.selfRemoveHarnesses = !m.selfRemoveHarnesses
			case 1:
				m.selfRemoveState = !m.selfRemoveState
			case 2:
				m.selfDiscardHarnessState = !m.selfDiscardHarnessState
			case 3:
				m.selfPermanent = !m.selfPermanent
			}
		}
		if key == "enter" {
			m.screen = "loading"
			return m, func() tea.Msg {
				binary, err := executablePath()
				if err != nil {
					return selfPlanMsg{err: err}
				}
				keep := keepAll()
				if m.selfDiscardHarnessState {
					keep = map[category]bool{}
				}
				p, err := m.e.buildSelfPlan(context.Background(), binary, m.selfRemoveHarnesses, m.selfRemoveState, request{Preserve: keep, Owners: m.selfOwners, Permanent: m.selfPermanent})
				return selfPlanMsg{p, err}
			}, true
		}
		return m, nil, true
	case "self-owners":
		if key == "enter" {
			m.screen = "self-options"
			m.cursor = 0
		}
		if key == "space" && m.cursor < len(m.selfOwnerCandidates) {
			owner := m.selfOwnerCandidates[m.cursor]
			if contains(m.selfOwners, owner) {
				m.selfOwners = slices.DeleteFunc(m.selfOwners, func(value string) bool { return value == owner })
			} else {
				m.selfOwners = append(m.selfOwners, owner)
			}
		}
		return m, nil, true
	case "accounts":
		if key == "a" {
			return m, m.editAccount(""), true
		}
		if len(m.accountItems) == 0 {
			return m, nil, true
		}
		chosen := m.accountItems[m.cursor]
		switch key {
		case "e":
			return m, m.editAccount(chosen.ID), true
		case "d", "u", "x":
			m.accountAction = map[string]string{"d": "disable", "u": "enable", "x": "remove"}[key]
			m.accountID = chosen.ID
			m.screen = "account-preview"
			m.typed = ""
			return m, nil, true
		case "r":
			return m, m.loadAccountsView(), true
		case "b", "l", "k", "s":
			page := map[string]string{"b": "billing", "l": "usage", "k": "keys", "s": "subscription"}[key]
			return m, func() tea.Msg {
				var out strings.Builder
				err := m.e.accountsCLI(context.Background(), []string{"open", chosen.ID, page}, strings.NewReader(""), &out)
				if err != nil {
					return accountsMsg{err: err}
				}
				return m.loadAccountsView()()
			}, true
		}
		return m, nil, true
	}
	return m, nil, false
}

// managementConfirm validates the typed approval for non-lifecycle manager data.
func (m tuiModel) managementConfirm() (tea.Model, tea.Cmd, bool) {
	if m.screen == "self-preview" {
		if m.typed != "uninstall" || m.self == nil {
			m.status = "Type uninstall to approve manager removal"
			return m, nil, true
		}
		p := m.self
		return m, m.startOperation(func(ctx context.Context, progress func(string)) error { return m.e.executeSelf(ctx, p, p.ID, progress) }), true
	}
	if m.screen == "account-preview" {
		if m.typed != "apply" {
			m.status = "Type apply to approve this account change"
			return m, nil, true
		}
		m.screen = "accounts-loading"
		return m, func() tea.Msg {
			var err error
			if m.accountAction == "save" {
				err = m.e.saveAccountChecked(m.accountEdit.item, m.accountEdit.before)
			} else if m.accountAction == "remove" {
				err = m.e.removeAccountChecked(m.accountID, m.accountBefore)
			} else {
				err = m.e.setAccountEnabled(m.accountID, m.accountAction == "enable", m.accountBefore)
			}
			if err != nil {
				return accountsMsg{err: err}
			}
			return m.loadAccountsView()()
		}, true
	}
	return m, nil, false
}

// managementView renders account capability/freshness labels and self-removal
// choices without exposing API/admin keys, configuration values or account notes.
func (m tuiModel) managementView() ([]string, string) {
	switch m.screen {
	case "batch-select":
		var rows []string
		for _, s := range m.e.cfg.Harnesses {
			rows = append(rows, mark(m.batchSelected[s.ID])+" "+s.Name)
		}
		return m.listWindow(rows), "Space selects harness · Enter choose action · Esc home"
	case "batch-actions":
		var rows []string
		for _, id := range m.batchActions() {
			rows = append(rows, actionName(id))
		}
		return m.listWindow(rows), "Enter select batch action · Esc home"
	case "self-options":
		rows := []string{mark(m.selfRemoveHarnesses) + " Also uninstall supported harnesses", mark(m.selfRemoveState) + " Permanently discard manager accounts, registry and recovery"}
		if m.selfRemoveHarnesses {
			rows = append(rows, mark(m.selfDiscardHarnessState)+" Discard harness user state", mark(m.selfPermanent)+" Permanently erase harness recovery after success")
		}
		return m.listWindow(rows), "Space toggle · o affected owners · Enter preview · Esc home"
	case "self-owners":
		var rows []string
		for _, owner := range m.selfOwnerCandidates {
			rows = append(rows, mark(contains(m.selfOwners, owner))+" "+owner)
		}
		return m.listWindow(rows), "Space select each affected owner · Enter return · Esc cancel"
	case "self-preview":
		if m.self == nil {
			return nil, "Esc home"
		}
		lines := []string{"REMOVE executable: " + m.self.Binary, "Harness preservation and recovery choices are listed in the batch preview."}
		if m.self.Receipt != "" {
			lines = append(lines, "REMOVE installation receipt: "+m.self.Receipt)
		}
		for link := range m.self.Links {
			lines = append(lines, "REMOVE launcher: "+link)
		}
		for _, p := range m.self.Paths {
			lines = append(lines, "PERMANENT REMOVE "+p)
		}
		if m.self.Batch != nil {
			for _, p := range m.self.Batch.Plans {
				copy := m
				copy.p = p
				copy.height = 1 << 30
				lines = append(lines, copy.previewLines()...)
			}
		}
		return m.scroll(lines), "Type uninstall, Enter confirm · Esc cancel"
	case "accounts-loading":
		return []string{"Reading account reports…"}, "Esc home"
	case "account-preview":
		id := m.accountID
		if m.accountAction == "save" {
			id = m.accountEdit.item.ID
		}
		lines := []string{"Account: " + id, "Action: " + m.accountAction}
		if m.accountAction == "save" {
			a := m.accountEdit.item
			lines = append(lines, "Provider: "+a.Provider, "Kind: "+a.Kind, fmt.Sprintf("Enabled: %t / replace inference key: %t / replace reporting key: %t", a.Enabled, a.Credential != "", a.MonitorCredential != ""))
		}
		lines = append(lines, "Changes only the local encrypted account vault. Native account plans and keys remain provider-managed.")
		return lines, "Type apply, Enter confirm · Esc cancel"
	case "accounts":
		lines := []string{fmt.Sprintf("Accounts / refresh every %ds", m.e.cfg.RefreshSeconds), "API usage, subscriptions and billing have separate capabilities."}
		var rows []string
		for _, a := range m.accountItems {
			rows = append(rows, fmt.Sprintf("%s / %s / %s [%s]", a.ID, a.Provider, a.Kind, map[bool]string{true: "enabled", false: "disabled"}[a.Enabled]))
		}
		lines = append(lines, m.listWindow(rows)...)
		if len(m.accountItems) > 0 {
			id := m.accountItems[m.cursor].ID
			for _, metric := range m.accountMetrics {
				if metric.ID == id {
					lines = append(lines, metricLines(metric)...)
				}
			}
		}
		return lines, "a add · e edit · d disable · u enable · x remove · b billing · l limits/usage · k keys · s subscription"
	}
	return nil, ""
}

// editAccount opens a private JSON request with keys omitted. The editor may
// insert replacement keys. Its temporary file is removed on every callback path.
func (m tuiModel) editAccount(id string) tea.Cmd {
	a := account{ID: "my-account", Provider: "openai", Kind: "api", Label: "My account", Enabled: true}
	items, err := m.e.loadAccounts()
	if err != nil {
		return func() tea.Msg { return accountEditMsg{err: err} }
	}
	if id != "" {
		for _, item := range items {
			if item.ID == id {
				a = item
			}
		}
	}
	a.Credential = ""
	a.MonitorCredential = ""
	path, err := m.e.accountPath()
	if err != nil {
		return func() tea.Msg { return accountEditMsg{err: err} }
	}
	before, err := fingerprint(path)
	if err != nil {
		return func() tea.Msg { return accountEditMsg{err: err} }
	}
	data, err := marshalJSONIndent(a, "", "  ")
	if err != nil {
		return func() tea.Msg { return accountEditMsg{err: err} }
	}
	return editor.Open("harness-ctl-account-*.json", data, func(path string, editorErr error) tea.Msg { return m.finishAccountEdit(path, data, before, editorErr) })
}

// finishAccountEdit validates a bounded single-document request and cleans up.
func (m tuiModel) finishAccountEdit(path string, beforeData []byte, before string, editorErr error) accountEditMsg {
	defer fileIO.remove(path)
	if editorErr != nil {
		return accountEditMsg{err: fmt.Errorf("account editor failed")}
	}
	f, err := fileIO.open(path)
	if err != nil {
		return accountEditMsg{err: err}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, m.e.cfg.MetadataBytes+1))
	if err != nil || int64(len(data)) > m.e.cfg.MetadataBytes {
		return accountEditMsg{err: fmt.Errorf("account edit exceeds size limit")}
	}
	if bytes.Equal(data, beforeData) {
		return accountEditMsg{cancelled: true}
	}
	var a account
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&a); err != nil {
		return accountEditMsg{err: fmt.Errorf("invalid account request")}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return accountEditMsg{err: fmt.Errorf("account edit requires one JSON document")}
	}
	if err = m.e.validateAccount(a); err != nil {
		return accountEditMsg{err: err}
	}
	return accountEditMsg{item: a, before: before}
}
