package manager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type inventoryMsg struct {
	installs []installation
	err      error
}
type optionsMsg struct {
	owners []string
	err    error
}
type planMsg struct {
	plan *plan
	err  error
}
type progressMsg string
type doneMsg struct{ err error }
type recoveryMsg struct {
	snapshots []snapshotMeta
	records   []operationRecord
	err       error
}
type snapshotMsg struct {
	meta snapshotMeta
	err  error
}
type interruptMsg struct{}

type tuiModel struct {
	e               *engine
	screen          string
	cursor          int
	harness         int
	installCursor   int
	width, height   int
	installs        []installation
	req             request
	owners          []string
	p               *plan
	status          string
	typed           string
	editing         bool
	events          chan tea.Msg
	cancel          context.CancelFunc
	backups         []snapshotMeta
	records         []operationRecord
	selectedBackup  snapshotMeta
	selectedRecord  operationRecord
	profileDisabled map[category]bool
	componentItems  []componentItem
	componentCat    category
	componentScope  string
}

var actionNames = []string{"Install isolated", "Update / upgrade", "Reinstall", "Factory reset", "Uninstall", "Migrate to isolated", "Launch source profile", "Manage components"}
var actionIDs = []string{"install", "update", "reinstall", "reset", "uninstall", "migrate", "profile", "manage"}

func newModel(e *engine) tuiModel {
	return tuiModel{e: e, screen: "home", width: 90, height: 28, status: "Reading executable and package metadata…"}
}

// Init reads metadata without executing a harness or mutating its state.
func (m tuiModel) Init() tea.Cmd { return m.inventory() }
func (m tuiModel) inventory() tea.Cmd {
	return func() tea.Msg {
		if err := m.e.refreshRegistry(); err != nil {
			return inventoryMsg{err: err}
		}
		installs, err := m.e.discover(context.Background())
		return inventoryMsg{installs, err}
	}
}
func (m tuiModel) selectedInst() installation {
	var list []installation
	for _, i := range m.installs {
		if i.Harness == catalog[m.harness].ID {
			list = append(list, i)
		}
	}
	if len(list) == 0 {
		return installation{}
	}
	return list[m.installCursor%len(list)]
}
func (m tuiModel) loadOptions() tea.Cmd {
	return func() tea.Msg {
		s := catalog[m.harness]
		e := *m.e
		e.cfg.StateRoots = map[string]string{}
		for id, root := range m.e.cfg.StateRoots {
			e.cfg.StateRoots[id] = root
		}
		if inst := m.selectedInst(); inst.Managed {
			e.cfg.StateRoots[s.ID] = inst.StateRoot
		}
		rs, err := e.resources(s)
		if err != nil {
			return optionsMsg{err: err}
		}
		var owners []string
		for _, r := range rs {
			for _, owner := range r.Owners {
				if owner != s.ID && !contains(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
		for _, owner := range s.SharedClients {
			if !contains(owners, owner) {
				owners = append(owners, owner)
			}
		}
		return optionsMsg{owners: owners}
	}
}
func (m tuiModel) preview() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.e.cfg.ProbeSeconds)*time.Second)
		defer cancel()
		p, err := m.e.buildPlan(ctx, m.req)
		return planMsg{p, err}
	}
}
func (m tuiModel) loadRecovery() tea.Cmd {
	return func() tea.Msg {
		backups, err := m.e.snapshots()
		if err != nil {
			return recoveryMsg{err: err}
		}
		records, err := m.e.records()
		var pending []operationRecord
		for _, r := range records {
			if r.Status != "complete" && r.Status != "failed" && r.Status != "rolled-back" && r.Status != "acknowledged" {
				pending = append(pending, r)
			}
		}
		return recoveryMsg{backups, pending, err}
	}
}
func waitEvent(events <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-events } }

func (m *tuiModel) startOperation(work func(context.Context, func(string)) error) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.events = make(chan tea.Msg, 64)
	events := m.events
	m.screen = "busy"
	m.cursor = 0
	m.typed = ""
	m.status = "Starting approved operation…"
	go func() {
		err := work(ctx, func(status string) {
			select {
			case events <- progressMsg(status):
			default:
			}
		})
		events <- doneMsg{err}
		cancel()
	}()
	return waitEvent(events)
}

// Update advances the interaction or reports progress from an approved operation.
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case componentsMsg:
		m.componentItems, m.owners = msg.items, msg.owners
		m.screen, m.cursor = "components", 0
		m.status = ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
	case componentEditorMsg:
		if msg.cancelled {
			m.screen, m.status = "components", "Editor closed without changes."
			return m, nil
		}
		if msg.err != nil {
			m.screen, m.status = "components", msg.err.Error()
			return m, nil
		}
		m.req.Component = &msg.change
		m.req.Component.Scope = m.componentScope
		m.screen, m.status = "loading", "Creating a component preview…"
		return m, m.preview()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case inventoryMsg:
		m.installs = msg.installs
		if m.screen == "home" {
			m.status = "Metadata inventory ready. Lifecycle actions require a preview."
		}
		if msg.err != nil && m.screen == "home" {
			m.status = msg.err.Error()
		}
	case optionsMsg:
		if msg.err != nil {
			m.screen = "result"
			m.status = msg.err.Error()
		} else {
			m.owners = msg.owners
			m.screen = "options"
			m.status = ""
		}
		m.cursor = 0
	case planMsg:
		if msg.err != nil {
			m.screen = "result"
			if m.req.Action == "manage" {
				m.screen = "components"
			}
			m.status = msg.err.Error()
		} else {
			m.p = msg.plan
			m.screen = "preview"
			m.status = ""
		}
		m.cursor = 0
		m.typed = ""
	case progressMsg:
		m.status = string(msg)
		return m, waitEvent(m.events)
	case doneMsg:
		m.screen = "result"
		m.cancel = nil
		m.status = "Operation verified. Recovery snapshots are available for the configured retention period."
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, m.inventory()
	case recoveryMsg:
		m.backups = msg.snapshots
		m.records = msg.records
		m.screen = "recovery"
		m.cursor = 0
		m.status = ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
	case snapshotMsg:
		if msg.err != nil {
			m.screen = "result"
			m.status = msg.err.Error()
		} else {
			m.selectedBackup = msg.meta
			m.owners = nil
			m.req.Owners = nil
			for _, item := range msg.meta.Items {
				for _, owner := range item.Owners {
					if owner != msg.meta.Harness && !contains(m.owners, owner) {
						m.owners = append(m.owners, owner)
					}
				}
			}
			m.screen = "restore-preview"
			m.cursor = 0
			m.typed = ""
		}
	case interruptMsg:
		if m.screen == "busy" && m.cancel != nil {
			m.cancel()
			m.status = "Cancellation requested. Waiting for rollback."
		} else {
			return m, tea.Quit
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m tuiModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.screen == "busy" {
		if key == "ctrl+c" && m.cancel != nil {
			m.cancel()
			m.status = "Cancellation requested. Waiting for rollback."
		}
		return m, nil
	}
	if m.screen == "loading" {
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.editing {
		if key == "enter" || key == "esc" {
			m.editing = false
			return m, nil
		}
		if key == "backspace" {
			if len(m.req.Target) > 0 {
				m.req.Target = m.req.Target[:len(m.req.Target)-1]
			}
			return m, nil
		}
		for _, r := range msg.Text {
			if (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._+-", r)) && len(m.req.Target) < 128 {
				m.req.Target += string(r)
			}
		}
		return m, nil
	}
	if m.screen == "preview" || m.screen == "restore-preview" || m.screen == "purge-preview" || m.screen == "journal-preview" || m.screen == "profile-preview" {
		if key == "esc" {
			m.screen = "home"
			m.typed = ""
			return m, nil
		}
		if m.screen == "restore-preview" && key == "space" && m.cursor < len(m.owners) {
			owner := m.owners[m.cursor]
			if contains(m.req.Owners, owner) {
				var selected []string
				for _, o := range m.req.Owners {
					if o != owner {
						selected = append(selected, o)
					}
				}
				m.req.Owners = selected
			} else {
				m.req.Owners = append(m.req.Owners, owner)
			}
			return m, nil
		}
		if key == "up" {
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		}
		if key == "down" {
			m.cursor++
			return m, nil
		}
		if key == "backspace" {
			if len(m.typed) > 0 {
				m.typed = m.typed[:len(m.typed)-1]
			}
			return m, nil
		}
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if key == "enter" {
			return m.confirm()
		}
		if len(m.typed) < 32 {
			m.typed += cleanText(msg.Text)
		}
		return m, nil
	}
	if key == "ctrl+c" || key == "q" {
		return m, tea.Quit
	}
	if key == "esc" {
		if m.screen == "components" {
			m.screen, m.cursor, m.status = "component-groups", 0, ""
			return m, nil
		}
		if m.screen == "component-owners" {
			m.screen, m.cursor = "components", 0
			return m, nil
		}
		m.screen = "home"
		m.cursor = m.harness
		m.status = ""
		return m, nil
	}
	if key == "up" || key == "k" {
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	}
	if key == "down" || key == "j" {
		if m.cursor < m.itemCount()-1 {
			m.cursor++
		}
		return m, nil
	}
	if strings.HasPrefix(m.screen, "component") {
		return m.componentKey(key)
	}
	switch m.screen {
	case "home":
		if key == "r" {
			m.screen = "loading"
			m.status = "Loading recovery metadata…"
			return m, m.loadRecovery()
		}
		if key == "d" {
			m.status = "Refreshing metadata…"
			return m, m.inventory()
		}
		if key == "enter" {
			m.harness = m.cursor
			m.screen = "actions"
			m.cursor = 0
			m.installCursor = 0
			m.status = ""
		}
	case "actions":
		if key == "tab" {
			m.installCursor++
			return m, nil
		}
		if key == "enter" {
			inst := m.selectedInst()
			action := actionIDs[m.cursor]
			if action == "manage" {
				m.componentScope = "base"
				if _, exists := m.e.reg.Profiles[inst.ID]; exists {
					m.componentScope = "profile"
				}
				m.req = request{Harness: catalog[m.harness].ID, InstallID: inst.ID, Action: "manage", Preserve: keepAll()}
				m.screen, m.cursor = "component-groups", 0
				return m, nil
			}
			if action != "install" && inst.ID == "" {
				m.status = "Select an installed harness, or choose Install isolated."
				return m, nil
			}
			if action == "profile" {
				if !inst.Managed {
					m.status = "Profiles require a managed installation. Migrate first."
					return m, nil
				}
				m.profileDisabled = map[category]bool{}
				m.screen = "profile"
				m.cursor = 0
				return m, nil
			}
			m.req = request{Harness: catalog[m.harness].ID, InstallID: inst.ID, Action: action, Model: "isolated", Preserve: keepAll()}
			if action == "reset" {
				m.req.Preserve = map[category]bool{}
			}
			m.screen = "loading"
			m.status = "Reading selected state paths for preservation controls…"
			return m, m.loadOptions()
		}
	case "options":
		if key == "enter" {
			m.screen = "loading"
			m.status = "Resolving the native recipe and creating a read-only preview…"
			return m, m.preview()
		}
		if key == "space" {
			m.toggleOption()
		}
		if key == "v" {
			m.editing = true
		}
	case "recovery":
		if m.cursor < len(m.records) && key == "enter" {
			m.selectedRecord = m.records[m.cursor]
			m.screen = "journal-preview"
			m.cursor = 0
			m.typed = ""
		}
		if m.cursor >= len(m.records) && m.cursor < len(m.records)+len(m.backups) {
			m.selectedBackup = m.backups[m.cursor-len(m.records)]
			if key == "enter" {
				id := m.selectedBackup.ID
				m.screen = "loading"
				m.status = "Authenticating the encrypted recovery manifest…"
				return m, func() tea.Msg { meta, err := m.e.authenticatedSnapshot(id); return snapshotMsg{meta, err} }
			}
			if key == "x" {
				m.screen = "purge-preview"
				m.typed = ""
				m.cursor = 0
			}
		}
	case "profile":
		if key == "space" {
			cat := categories[m.cursor]
			m.profileDisabled[cat] = !m.profileDisabled[cat]
		}
		if key == "enter" {
			inst := m.selectedInst()
			m.req = request{Harness: inst.Harness, InstallID: inst.ID, Action: "profile", Preserve: keepAll(), Disabled: cloneCategories(m.profileDisabled)}
			m.screen = "loading"
			m.status = "Creating a launch-profile preview…"
			return m, m.preview()
		}
		if key == "b" {
			inst := m.selectedInst()
			return m, m.startOperation(func(_ context.Context, _ func(string)) error { return m.e.disableProfile(inst) })
		}
	case "result":
		if key == "enter" {
			m.screen = "home"
			m.cursor = m.harness
			m.status = ""
		}
	}
	return m, nil
}

func (m tuiModel) itemCount() int {
	switch m.screen {
	case "home":
		return len(catalog)
	case "actions":
		return len(actionNames)
	case "options":
		return 3 + len(categories) + len(m.owners)
	case "recovery":
		return len(m.records) + len(m.backups)
	case "profile":
		return len(categories)
	case "component-groups":
		return len(componentCategories)
	case "components":
		return len(m.componentItems)
	case "component-owners":
		return len(m.owners)
	}
	return 1
}
func (m *tuiModel) toggleOption() {
	switch m.cursor {
	case 0:
		all := true
		for _, cat := range categories {
			all = all && m.req.Preserve[cat]
		}
		if all {
			m.req.Preserve = map[category]bool{auth: true}
		} else if len(m.req.Preserve) == 1 && m.req.Preserve[auth] {
			m.req.Preserve = map[category]bool{}
		} else {
			m.req.Preserve = keepAll()
		}
	case 1:
		m.editing = true
	case 2:
		m.req.Permanent = !m.req.Permanent
	default:
		if m.cursor < 3+len(categories) {
			cat := categories[m.cursor-3]
			m.req.Preserve[cat] = !m.req.Preserve[cat]
		} else {
			owner := m.owners[m.cursor-3-len(categories)]
			if contains(m.req.Owners, owner) {
				var owners []string
				for _, o := range m.req.Owners {
					if o != owner {
						owners = append(owners, o)
					}
				}
				m.req.Owners = owners
			} else {
				m.req.Owners = append(m.req.Owners, owner)
			}
		}
	}
}

func (m tuiModel) confirm() (tea.Model, tea.Cmd) {
	switch m.screen {
	case "preview":
		word := "apply"
		if m.req.Action == "profile" {
			word = "profile"
		}
		if m.req.Permanent {
			word = "discard"
		}
		if m.typed != word {
			m.status = "Type " + word + " to approve this preview."
			return m, nil
		}
		if m.p == nil || len(m.p.Blockers) > 0 {
			m.status = "Resolve the blockers before executing."
			return m, nil
		}
		p := m.p
		return m, m.startOperation(func(ctx context.Context, progress func(string)) error { return m.e.execute(ctx, p, p.ID, progress) })
	case "restore-preview":
		if m.typed != "restore" {
			m.status = "Type restore to replace the listed paths."
			return m, nil
		}
		id := m.selectedBackup.ID
		owners := append([]string{}, m.req.Owners...)
		return m, m.startOperation(func(ctx context.Context, _ func(string)) error { return m.e.restoreApproved(ctx, id, owners) })
	case "purge-preview":
		if m.typed != "purge" {
			m.status = "Type purge to permanently delete this snapshot."
			return m, nil
		}
		id := m.selectedBackup.ID
		return m, m.startOperation(func(_ context.Context, _ func(string)) error {
			unlock, err := m.e.lock()
			if err != nil {
				return err
			}
			defer unlock()
			if err = m.e.ensureNoPending(); err != nil {
				return err
			}
			return m.e.purgeSnapshot(id)
		})
	case "journal-preview":
		if m.typed != "recover" {
			m.status = "Type recover to restore this interrupted operation."
			return m, nil
		}
		r := m.selectedRecord
		return m, m.startOperation(func(ctx context.Context, _ func(string)) error { return m.e.recoverOperation(ctx, r.ID) })
	}
	return m, nil
}

func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func mark(v bool) string {
	if v {
		return "[x]"
	}
	return "[ ]"
}

// View renders metadata and approval controls without displaying state values.
func (m tuiModel) View() tea.View {
	var lines []string
	inst := m.selectedInst()
	title := "harness-ctl"
	if m.screen != "home" && m.screen != "recovery" {
		title += " / " + catalog[m.harness].Name
	}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(title), "")
	hint := "↑/↓ navigate · Enter select · Esc home · q quit"
	switch m.screen {
	case "component-groups", "components", "component-owners":
		lines = append(lines, m.componentView()...)
		hint = "Enter browse · a add/install · e edit · d disable · u enable · x remove · o owners · p scope"
		if m.screen == "component-groups" {
			hint = "↑/↓ select category · Enter browse · Esc home"
		}
		if m.screen == "component-owners" {
			hint = "↑/↓ navigate · Space select affected owner · Enter return"
		}
	case "home":
		for i, s := range catalog {
			count := 0
			for _, inst := range m.installs {
				if inst.Harness == s.ID {
					count++
				}
			}
			label := fmt.Sprintf("%-18s %d installation(s)", s.Name, count)
			lines = append(lines, m.row(i, label))
		}
		lines = append(lines, "", "r recovery · d refresh metadata")
		for _, inst := range m.installs {
			if inst.Method == "unsupported" {
				lines = append(lines, "Unsupported: "+cleanText(inst.Harness)+" at "+cleanText(inst.Path))
			}
		}
	case "actions":
		if inst.ID != "" {
			lines = append(lines, "Selected: "+cleanText(inst.Method)+" · "+cleanText(inst.Version), cleanText(inst.Path), "Tab cycles detected installations", "")
		}
		for i, name := range actionNames {
			lines = append(lines, m.row(i, name))
		}
	case "options":
		preset := "custom"
		all := true
		none := true
		for _, cat := range categories {
			all = all && m.req.Preserve[cat]
			none = none && !m.req.Preserve[cat]
		}
		if all {
			preset = "preserve all"
		} else if none {
			preset = "discard all"
		} else if len(m.req.Preserve) == 1 && m.req.Preserve[auth] {
			preset = "keep auth only"
		}
		items := []string{"Preset: " + preset, "Target version: " + m.req.Target + " (blank = latest, reinstall = current)", mark(m.req.Permanent) + " Permanent discard, erase recovery after completion"}
		for _, cat := range categories {
			items = append(items, mark(m.req.Preserve[cat])+" Preserve "+string(cat))
		}
		for _, owner := range m.owners {
			items = append(items, mark(contains(m.req.Owners, owner))+" Include affected owner: "+owner)
		}
		lines = append(lines, m.listWindow(items)...)
		hint = "Space toggle · v edit target · Enter preview · Esc home"
		if m.editing {
			lines = append(lines, "Editing version. Backspace deletes, Enter finishes.")
		}
	case "preview":
		if m.p != nil {
			lines = append(lines, m.previewLines()...)
		}
		word := "apply"
		if m.req.Action == "profile" {
			word = "profile"
		}
		if m.req.Permanent {
			word = "discard"
		}
		hint = "↑/↓ scroll · Type " + word + ", Enter execute · Esc cancel"
	case "busy":
		lines = append(lines, "Approved operation is running.", "Ctrl+C requests cancellation and rollback.")
	case "result":
		lines = append(lines, "Enter returns to the harness list.")
	case "loading":
		lines = append(lines, "Working…")
	case "recovery":
		var items []string
		for _, r := range m.records {
			items = append(items, "Recovery required: "+r.Harness+" / "+r.Action+" / "+shortID(r.ID))
		}
		for _, s := range m.backups {
			items = append(items, s.Harness+" / "+s.Action+" / "+s.Created.Format("2006-01-02 15:04")+" / "+shortID(s.ID))
		}
		if len(items) == 0 {
			lines = append(lines, "No recovery snapshots or interrupted operations.")
		} else {
			lines = append(lines, m.listWindow(items)...)
		}
		hint = "Enter recovery preview · x permanent purge · Esc home"
	case "restore-preview":
		var detail []string
		for i, owner := range m.owners {
			detail = append(detail, m.row(i, mark(contains(m.req.Owners, owner))+" Include affected owner: "+owner))
		}
		detail = append(detail, "Restore replaces the following harness paths, including shared paths captured by the operation:")
		for _, item := range m.selectedBackup.Items {
			action := "replace "
			if item.Absent {
				action = "remove  "
			}
			detail = append(detail, action+cleanText(item.Path))
		}
		lines = append(lines, m.scroll(detail)...)
		hint = "↑/↓ scroll · Space selects owner · Type restore, Enter confirm · Esc cancel"
	case "purge-preview":
		lines = append(lines, "Permanently delete encrypted recovery "+shortID(m.selectedBackup.ID)+"?")
		hint = "Type purge, Enter confirm · Esc cancel"
	case "journal-preview":
		lines = append(lines, "Recover "+m.selectedRecord.Action+" for "+m.selectedRecord.Harness, "Snapshot: "+shortID(m.selectedRecord.Snapshot), "Restores saved state and resumes previously loaded services.")
		hint = "Type recover, Enter confirm · Esc cancel"
	case "profile":
		var items []string
		for _, cat := range categories {
			items = append(items, mark(m.profileDisabled[cat])+" Exclude "+string(cat)+" from this shim profile")
		}
		lines = append(lines, m.listWindow(items)...)
		hint = "Space toggle · Enter preview · b return to base state · Esc home"
	}
	if m.typed != "" {
		lines = append(lines, "", "> "+cleanText(m.typed))
	}
	if m.status != "" {
		lines = append(lines, "", cleanText(m.status))
	}
	lines = append(lines, "", hint)
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (m tuiModel) row(index int, label string) string {
	prefix := "  "
	if index == m.cursor {
		prefix = "> "
	}
	return prefix + cleanText(label)
}
func (m tuiModel) listWindow(items []string) []string {
	height := m.height - 10
	if height < 4 {
		height = 4
	}
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := min(len(items), start+height)
	var out []string
	for i := start; i < end; i++ {
		out = append(out, m.row(i, items[i]))
	}
	if end < len(items) {
		out = append(out, "↓ more options")
	}
	return out
}
func (m tuiModel) scroll(lines []string) []string {
	height := m.height - 10
	if height < 4 {
		height = 4
	}
	start := min(m.cursor, max(0, len(lines)-height))
	end := min(len(lines), start+height)
	return lines[start:end]
}
func (m tuiModel) previewLines() []string {
	p := m.p
	var lines []string
	lines = append(lines, fmt.Sprintf("Action: %s · model: %s · target: %s", p.Request.Action, p.Request.Model, p.Request.Target), "State: "+p.StateRoot)
	if p.Request.Action == "profile" {
		lines = append(lines, "Create or replace a persistent launch-state copy. Existing profile-written state is captured in encrypted recovery.", "The shim uses a private HOME. Excluded file categories and links are omitted. Project/system sources, OS credentials and environment credentials remain native.")
		for _, cat := range categories {
			if p.Request.Disabled[cat] {
				lines = append(lines, "EXCLUDE "+string(cat))
			}
		}
	}
	if p.Component != nil {
		change := p.Component.Request
		lines = append(lines, "Component: "+string(change.Category)+" / "+change.Operation+" / "+change.Scope, "Native source: "+change.Path+" "+change.Field, "Configuration values are hidden. Review them in your editor before approving.")
		for _, write := range p.Component.Writes {
			action := "WRITE "
			if write.Remove {
				action = "REMOVE "
			}
			lines = append(lines, action+write.Path)
			if write.Source != "" {
				lines = append(lines, "  IMPORT "+write.Source+" / "+write.SourceDigest)
			}
		}
	}
	if p.Destination != "" {
		lines = append(lines, "Install destination: "+p.Destination)
	}
	if p.Integrity != "" {
		lines = append(lines, "Payload: "+p.Integrity)
	}
	for _, r := range p.Resources {
		label := "KEEP"
		if p.Component != nil {
			if p.Component.Native && slices.Contains(resourceCategories(r), plugins) {
				label = "NATIVE CHANGE"
			}
			for _, write := range p.Component.Writes {
				if within(r.Path, write.Path) || within(write.Path, r.Path) {
					label = "CHANGE"
				}
			}
		} else if shouldChange(r, p.Request) {
			label = "EDIT / DISCARD"
		}
		lines = append(lines, label+" "+r.Path+" ["+strings.Join(categoryStrings(resourceCategories(r)), ",")+"]")
	}
	for _, step := range append(append([]command{}, p.DependencyCommands...), p.Steps...) {
		lines = append(lines, step.Description+": "+step.Path+" "+strings.Join(step.Args, " "))
		for _, key := range sortedKeys(step.Env) {
			lines = append(lines, "  "+key+"="+step.Env[key])
		}
	}
	for _, path := range p.Install.ServicePaths {
		lines = append(lines, "Coordinate service: "+path)
	}
	for _, warning := range p.Warnings {
		lines = append(lines, "NOTICE: "+warning)
	}
	for _, blocker := range p.Blockers {
		lines = append(lines, "BLOCKED: "+blocker)
	}
	return m.scroll(lines)
}
func categoryStrings(cats []category) []string {
	var out []string
	for _, cat := range cats {
		out = append(out, string(cat))
	}
	return out
}

func runTUI(e *engine) error {
	program := tea.NewProgram(newModel(e), tea.WithoutSignalHandler())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-signals:
				program.Send(interruptMsg{})
			case <-done:
				return
			}
		}
	}()
	_, err := program.Run()
	return err
}
