package manager

import (
	"context"
	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// nativeAuthDone returns to native management after its terminal process exits.
type nativeAuthDone struct{ err error }

// authOperations lists only the selected catalog entry's declared contracts.
func (m tuiModel) authOperations() []string {
	return sortedKeys(m.e.cfg.Harnesses[m.harness].Auth.Commands)
}

// nativeAuthUpdate routes native selection and terminal completion messages.
// Status attaches read-only. Other operations use the normal transaction preview.
func (m tuiModel) nativeAuthUpdate(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if owners, ok := msg.(nativeOwnersMsg); ok {
		m.owners = owners.items
		m.screen, m.cursor = "native-auth-owners", 0
		m.status = ""
		if owners.err != nil {
			m.status = owners.err.Error()
		}
		return m, nil, true
	}
	if profiles, ok := msg.(nativeCredentialMsg); ok {
		m.credentialProfiles = profiles.items
		m.screen, m.cursor = "native-auth-profiles", 0
		m.status = ""
		if profiles.err != nil {
			m.status = profiles.err.Error()
		}
		return m, nil, true
	}
	if done, ok := msg.(nativeAuthDone); ok {
		m.screen = "native-auth"
		m.status = "Native command finished. Credentials remain with the harness."
		if done.err != nil {
			m.status = "Native command failed or was cancelled."
		}
		return m, nil, true
	}
	key, ok := msg.(tea.KeyPressMsg)
	if ok && m.monitorExpanded {
		return m, nil, false
	}
	if ok && m.screen == "native-auth-argument" {
		switch key.String() {
		case "enter":
			if strings.TrimSpace(m.nativeAuthInput) == "" || strings.HasPrefix(m.nativeAuthInput, "-") {
				m.status = "Enter the native identifier. Secrets belong in native prompts."
				return m, nil, true
			}
			m.req = request{Harness: m.e.cfg.Harnesses[m.harness].ID, InstallID: m.selectedInst().ID, Action: "auth", Target: m.nativeAuthOperation, AuthArgs: []string{m.nativeAuthInput}, Owners: m.req.Owners, Preserve: keepAll()}
			m.screen = "loading"
			return m, m.preview(), true
		case "backspace":
			if len(m.nativeAuthInput) > 0 {
				m.nativeAuthInput = m.nativeAuthInput[:len(m.nativeAuthInput)-1]
			}
			return m, nil, true
		case "esc", "ctrl+c":
			m.screen = "native-auth"
			m.nativeAuthInput = ""
			return m, nil, true
		}
		if len(m.nativeAuthInput) < 128 && key.Text != "" {
			m.nativeAuthInput += key.Text
		}
		return m, nil, true
	}
	if ok && m.screen == "native-auth-owners" {
		switch key.String() {
		case "space":
			if len(m.owners) > 0 {
				owner := m.owners[m.cursor]
				if contains(m.req.Owners, owner) {
					m.req.Owners = slices.DeleteFunc(m.req.Owners, func(v string) bool { return v == owner })
				} else {
					m.req.Owners = append(m.req.Owners, owner)
				}
			}
			return m, nil, true
		case "enter", "esc":
			m.screen, m.cursor = "native-auth", 0
			return m, nil, true
		}
	}
	if ok && m.screen == "native-auth-profiles" {
		switch key.String() {
		case "c":
			model, cmd := m.previewCredential("capture", m.e.cfg.Harnesses[m.harness].ID+"-"+shortID(m.selectedInst().ID))
			return model, cmd, true
		case "enter", "x":
			if len(m.credentialProfiles) > 0 {
				op := "apply"
				if key.String() == "x" {
					op = "remove"
				}
				model, cmd := m.previewCredential(op, m.credentialProfiles[m.cursor].ID)
				return model, cmd, true
			}
			return m, nil, true
		case "o":
			m.screen = "native-auth"
			return m.nativeAuthUpdate(msg)
		}
	}
	if !ok || m.screen != "native-auth" {
		return m, nil, false
	}
	switch key.String() {
	case "p":
		return m, m.loadNativeCredentials(), true
	case "c":
		model, cmd := m.previewCredential("capture", m.e.cfg.Harnesses[m.harness].ID+"-"+shortID(m.selectedInst().ID))
		return model, cmd, true
	case "tab":
		m.installCursor++
		m.cursor = 0
		return m, nil, true
	case "o":
		m.screen = "loading"
		return m, m.loadNativeOwners(), true
	case "enter":
		op := m.authOperations()[m.cursor]
		if m.e.cfg.Harnesses[m.harness].Auth.RequiredArgument[op] != "" {
			m.nativeAuthOperation, m.nativeAuthInput, m.screen = op, "", "native-auth-argument"
			return m, nil, true
		}
		inst := m.selectedInst()
		if op == "status" {
			m.screen = "native-auth-running"
			c, err := m.e.launchCommand(inst, m.e.cfg.Harnesses[m.harness].Auth.Commands[op], true)
			if err != nil {
				return m, func() tea.Msg { return nativeAuthDone{err} }, true
			}
			return m, tea.Exec(&nativeAuthTerminal{e: m.e, command: &c}, nativeAuthFinished), true
		}
		m.req = request{Harness: inst.Harness, InstallID: inst.ID, Action: "auth", Target: op, Owners: m.req.Owners, Preserve: keepAll()}
		m.screen = "loading"
		return m, m.preview(), true
	}
	return m, nil, false
}

// nativeAuthTerminal runs approved work while Bubble Tea releases the terminal.
// Streams are supplied by the terminal runtime and never buffered in UI state.
type nativeAuthTerminal struct {
	e       *engine
	p       *plan
	command *command
	in      io.Reader
	out     io.Writer
}

// Run executes the selected native status or approved authentication transaction.
func (c *nativeAuthTerminal) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if c.command != nil {
		bounded, stop := context.WithTimeout(ctx, time.Duration(c.e.cfg.ProbeSeconds)*time.Second)
		defer stop()
		return interactiveCommand(bounded, *c.command, c.in, c.out)
	}
	return c.e.executeNativeAuth(ctx, c.p, c.p.ID, c.in, c.out)
}

// SetStdin attaches native prompt input supplied by the terminal runtime.
func (c *nativeAuthTerminal) SetStdin(r io.Reader) { c.in = r }

// SetStdout attaches native output supplied by the terminal runtime.
func (c *nativeAuthTerminal) SetStdout(w io.Writer) { c.out = w }

// SetStderr retains native diagnostics in the same terminal as stdout.
func (c *nativeAuthTerminal) SetStderr(w io.Writer) { c.out = w }

// authTerminal returns a suspended-terminal command for p's approved operation.
func (m tuiModel) authTerminal(p *plan) tea.Cmd {
	return tea.Exec(&nativeAuthTerminal{e: m.e, p: p}, nativeAuthFinished)
}

// nativeCredentialMsg carries secret-free profiles for the selected harness.
type nativeCredentialMsg struct {
	items []credentials.View
	err   error
}

// loadNativeCredentials lists manager-owned encrypted profiles without secrets.
func (m tuiModel) loadNativeCredentials() tea.Cmd {
	return func() tea.Msg {
		profiles, err := m.e.loadCredentialProfiles()
		views := []credentials.View{}
		for _, view := range credentials.Views(profiles) {
			if view.Harness == m.e.cfg.Harnesses[m.harness].ID {
				views = append(views, view)
			}
		}
		return nativeCredentialMsg{views, err}
	}
}

// previewCredential builds a native file capture/apply/remove preview for id.
func (m tuiModel) previewCredential(op, id string) (tea.Model, tea.Cmd) {
	inst := m.selectedInst()
	m.req = request{Harness: m.e.cfg.Harnesses[m.harness].ID, InstallID: inst.ID, Action: "credentials", Target: op, CredentialID: id, Owners: m.req.Owners, Preserve: keepAll()}
	m.screen = "loading"
	return m, m.preview()
}

// nativeAuthFinished translates terminal completion without retaining output.
func nativeAuthFinished(err error) tea.Msg { return nativeAuthDone{err} }

// nativeOwnersMsg carries explicit owner choices without configuration values.
type nativeOwnersMsg struct {
	items []string
	err   error
}

// loadNativeOwners collects selected native/profile and shared-vault owners.
func (m tuiModel) loadNativeOwners() tea.Cmd {
	return func() tea.Msg {
		inst := m.selectedInst()
		scope := "base"
		if _, ok := m.e.reg.Profiles[inst.ID]; ok {
			scope = "profile"
		}
		state, s, err := m.e.componentEngine(inst, scope)
		if err != nil {
			return nativeOwnersMsg{err: err}
		}
		resources, err := state.resources(s)
		if err != nil {
			return nativeOwnersMsg{err: err}
		}
		owners := append([]string{}, s.SharedClients...)
		for _, r := range resources {
			for _, owner := range r.Owners {
				if owner != s.ID && !contains(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
		profiles, err := m.e.loadCredentialProfiles()
		if err != nil {
			return nativeOwnersMsg{items: owners, err: err}
		}
		for _, p := range profiles {
			if p.Harness != s.ID && !contains(owners, p.Harness) {
				owners = append(owners, p.Harness)
			}
		}
		return nativeOwnersMsg{items: owners}
	}
}
