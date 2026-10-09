package manager

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zaRizk7/harness-ctl/internal/setup"
)

func TestSetupTUIRequiresConcreteApprovalAndReportsFailures(t *testing.T) {
	e, _ := testEngine(t)
	o := setup.Options{Home: e.cfg.Home, Root: e.cfg.Root, Files: map[string][]byte{"catalog.json": []byte("[]")}}
	calls := 0
	m := setupModel{options: o, apply: func(p *setup.Plan) error { calls++; return setup.Apply(p, p.ID()) }}
	if m.Init() != nil {
		t.Fatal("setup init mutates")
	}
	_ = m.View()
	model, _ := m.Update(tea.WindowSizeMsg{})
	m = model.(setupModel)
	model, _ = m.Update(keyMessage("enter"))
	m = model.(setupModel)
	if m.plan == nil {
		t.Fatal(m.err)
	}
	_ = m.View()
	model, _ = m.Update(keyMessage("enter"))
	m = model.(setupModel)
	if calls != 0 {
		t.Fatal("unapproved setup")
	}
	for _, key := range []string{"s", "e", "t", "u", "x", "backspace", "p", "enter"} {
		model, _ = m.Update(keyMessage(key))
		m = model.(setupModel)
	}
	if !m.done || calls != 1 {
		t.Fatal(m.status, m.typed)
	}
	_ = m.View()
	_, cmd := m.Update(keyMessage("enter"))
	if cmd == nil {
		t.Fatal("completion exit")
	}
	m = setupModel{options: o, apply: func(*setup.Plan) error { return errors.New("fixture failure") }}
	model, _ = m.Update(keyMessage("enter"))
	m = model.(setupModel)
	m.typed = "setup"
	model, _ = m.Update(keyMessage("enter"))
	m = model.(setupModel)
	if m.err == nil {
		t.Fatal("failure lost")
	}
	m = setupModel{options: setup.Options{Home: e.cfg.Home, Root: "relative"}}
	model, _ = m.Update(keyMessage("enter"))
	m = model.(setupModel)
	if m.err == nil {
		t.Fatal("unsafe setup accepted")
	}
	_, cmd = m.Update(keyMessage("esc"))
	if cmd == nil {
		t.Fatal("cancel ignored")
	}
}

func TestSetupAndMainProgramBoundaries(t *testing.T) {
	e, _ := testEngine(t)
	old := newTUIProgram
	t.Cleanup(func() { newTUIProgram = old })
	input := "\x1b"
	newTUIProgram = func(m tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, append(options, tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())...)
	}
	o := setup.Options{Home: e.cfg.Home, Root: e.cfg.Root, Prefix: filepath.Join(e.cfg.Root, "app"), Files: map[string][]byte{"catalog.json": []byte("[]")}}
	if runSetupTUI(o, func(*setup.Plan) error { return nil }) == nil {
		t.Fatal("cancelled setup accepted")
	}
	input = "q"
	if err := runTUI(e); err != nil {
		t.Fatal(err)
	}
}

func TestSetupTUILinkChoicesAndRuntimeOutcomes(t *testing.T) {
	e, _ := testEngine(t)
	o := setup.Options{Home: e.cfg.Home, Root: e.cfg.Root, Prefix: filepath.Join(e.cfg.Root, "app"), Source: "fixture"}
	m := setupModel{options: o, link: filepath.Join(e.cfg.Home, "bin")}
	model, _ := m.Update(keyMessage("space"))
	m = model.(setupModel)
	if m.options.LinkDir == "" {
		t.Fatal("link choice ignored")
	}
	model, _ = m.Update(keyMessage("space"))
	m = model.(setupModel)
	if m.options.LinkDir != "" {
		t.Fatal("direct choice ignored")
	}
	old := newTUIProgram
	t.Cleanup(func() { newTUIProgram = old })
	o.Source = ""
	o.Files = map[string][]byte{"catalog.json": []byte("[]")}
	input := "\rsetup\r\r"
	newTUIProgram = func(m tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, append(options, tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())...)
	}
	if err := runSetupTUI(o, func(*setup.Plan) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := runSetupTUI(o, func(*setup.Plan) error { return errors.New("fixture failure") }); err == nil {
		t.Fatal("runtime apply failure lost")
	}
}
