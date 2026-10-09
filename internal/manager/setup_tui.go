package manager

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zaRizk7/harness-ctl/internal/setup"
)

// setupModel presents the selected prefix, optional symlink and concrete preview.
// Approval never starts a harness installer or uses elevated permissions.
type setupModel struct {
	options setup.Options
	link    string
	plan    *setup.Plan
	apply   func(*setup.Plan) error
	typed   string
	status  string
	err     error
	done    bool
}

// Init starts with options. No filesystem changes happen until typed approval.
func (m setupModel) Init() tea.Cmd { return nil }

// Update consumes msg and returns the changed setup model plus an optional quit
// command. Only typed setup approval calls apply. Failures stay visible in err.
func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	k := key.String()
	if k == "ctrl+c" || k == "esc" {
		return m, tea.Quit
	}
	if m.done {
		if k == "enter" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.plan == nil {
		if k == "space" && m.options.Source != "" {
			if m.options.LinkDir == "" {
				m.options.LinkDir = m.link
			} else {
				m.options.LinkDir = ""
			}
		}
		if k == "enter" {
			if m.options.Source != "" && len(m.options.BinDirs) > 0 {
				m.options.BinDirs = m.options.BinDirs[:1]
				if m.options.LinkDir != "" {
					m.options.BinDirs = append(m.options.BinDirs, m.options.LinkDir)
				} else {
					m.options.BinDirs = append(m.options.BinDirs, m.options.Prefix)
				}
			}
			m.plan, m.err = setup.Build(m.options)
			if m.err != nil {
				m.status = m.err.Error()
			}
		}
		return m, nil
	}
	if k == "backspace" {
		if len(m.typed) > 0 {
			m.typed = m.typed[:len(m.typed)-1]
		}
		return m, nil
	}
	if k == "enter" {
		if m.typed != "setup" {
			m.status = "Type setup to approve the displayed paths"
			return m, nil
		}
		m.err = m.apply(m.plan)
		m.done = true
		m.status = "Setup complete"
		if m.err != nil {
			m.status = m.err.Error()
		}
		return m, nil
	}
	if len(m.typed) < 32 {
		m.typed += cleanText(key.Text)
	}
	return m, nil
}

// View returns the setup paths, approval prompt and status as a terminal view,
// without credential or configuration values.
func (m setupModel) View() tea.View {
	lines := []string{"harness-ctl setup", "Manager state: " + m.options.Root, "Executable directory: " + m.options.Prefix, "Symlink directory: " + m.options.LinkDir, "Existing configuration is preserved. No elevated permissions are used."}
	if m.options.ShellFile != "" {
		lines = append(lines, "Approved PATH append: "+m.options.ShellFile)
	}
	if m.plan == nil {
		lines = append(lines, "Space toggle symlink/direct · Enter preview · Esc cancel")
	} else {
		for _, path := range m.plan.Paths {
			lines = append(lines, "WRITE "+path)
		}
		lines = append(lines, "Type setup and Enter to approve: "+m.typed)
	}
	if m.done {
		lines = append(lines, "Enter exit")
	}
	lines = append(lines, m.status)
	return tea.NewView(strings.Join(lines, "\n"))
}

// runSetupTUI runs interactive setup using caller-owned locking/publication.
func runSetupTUI(o setup.Options, apply func(*setup.Plan) error) error {
	link := o.LinkDir
	if link == "" {
		link = fmt.Sprintf("%s/.local/bin", o.Home)
	}
	model, err := newTUIProgram(setupModel{options: o, link: link, apply: apply}).Run()
	if err != nil {
		return err
	}
	result := model.(setupModel)
	if result.err != nil {
		return result.err
	}
	if !result.done {
		return fmt.Errorf("setup cancelled")
	}
	return nil
}
