package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zaRizk7/harness-ctl/internal/library"
)

func TestTUIProfileScopeSharedOwnersAndUnavailableCapabilities(t *testing.T) {
	m := componentModel(t)
	inst := m.componentInstallation()
	m.e.cfg.StateRoots[inst.Harness] = inst.StateRoot
	if msg := m.loadOptions()().(optionsMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	m.e.reg.Profiles[inst.ID] = profile{Root: filepath.Join(m.e.cfg.Root, "profiles", inst.ID)}
	m.screen, m.cursor = "actions", actionIndex(m, "manage")
	model, _ := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if m.componentScope != "profile" {
		t.Fatal("active profile scope not selected")
	}
	// Unknown catalog identities fail closed in the capability view.
	m.e.cfg.Harnesses[m.harness].ID = "missing"
	if m.hasLocalState() {
		t.Fatal("unknown adapter exposes local state")
	}
	m = componentModel(t)
	inst = m.componentInstallation()
	inst.Managed = false
	m.installs = []installation{inst}
	if !contains(m.actions(), "migrate") {
		t.Fatal("tracked migration hidden")
	}
	// HOME skills name affected shared clients, independent of configured state.
	m = componentModel(t)
	for i, s := range m.e.cfg.Harnesses {
		if s.ID == "codex" {
			m.harness = i
		}
	}
	_ = atomicWrite(filepath.Join(m.e.codexSkillsRoot(), "skills/demo/SKILL.md"), []byte("fixture"), 0600)
	m.componentCat, m.componentScope = skills, "base"
	if msg := m.loadOptions()().(optionsMsg); msg.err != nil || len(msg.owners) < 2 {
		t.Fatal(msg)
	}
	if msg := m.loadComponents()().(componentsMsg); msg.err != nil || len(msg.owners) < 2 {
		t.Fatal(msg)
	}
}

func TestTUIPurgeErrorsAndDefaultConfirmationRemainVisible(t *testing.T) {
	for _, scenario := range []string{"locked", "pending", "success"} {
		t.Run(scenario, func(t *testing.T) {
			m := componentModel(t)
			e := m.e
			p := resetPlan(t, e)
			meta, err := e.snapshot(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			m.screen = "recovery"
			m.backups = []snapshotMeta{meta}
			m.cursor = 0
			model, _ := m.key(keyMessage("x"))
			m = model.(tuiModel)
			if m.screen != "purge-preview" || m.selectedBackup.ID != meta.ID {
				t.Fatal(m.screen)
			}
			unlock := func() {}
			if scenario == "locked" {
				unlock, err = e.lock()
				if err != nil {
					t.Fatal(err)
				}
			}
			defer unlock()
			if scenario == "pending" {
				_ = e.saveRecord(operationRecord{ID: randomID(), Status: "executing"})
			}
			m.typed = "purge"
			_, cmd := m.confirm()
			msg := cmd().(doneMsg)
			if (msg.err != nil) != (scenario != "success") {
				t.Fatal(msg.err)
			}
		})
	}
	m := componentModel(t)
	m.screen = "home"
	if _, cmd := m.confirm(); cmd != nil {
		t.Fatal("unrelated screen confirmed")
	}
	m.installs = append(m.installs, installation{Harness: "external", Path: "/synthetic/external", Method: "unsupported"})
	if !strings.Contains(m.View().Content, "Unsupported: external") {
		t.Fatal(m.View().Content)
	}
	m.screen = "batch-actions"
	if _, cmd, _ := m.managementKey("down"); cmd != nil {
		t.Fatal("navigation started batch")
	}
	m.screen = "library-record-preview"
	m.libraryAction = "disable"
	m.libraryID = "missing"
	m.typed = "wrong"
	if _, cmd := m.confirm(); cmd != nil {
		t.Fatal("library approval skipped")
	}
}

func TestLibrarySelectionViewAndParkedAssetEditor(t *testing.T) {
	m := componentModel(t)
	inst := m.componentInstallation()
	item := reusableSkill("demo")
	item.Targets = map[string]library.Target{inst.Harness: {Path: "skills/demo"}}
	if err := m.e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(m.loadLibraryView()())
	m = model.(tuiModel)
	model, _, _ = m.libraryKey("enter")
	m = model.(tuiModel)
	if !strings.Contains(m.View().Content, inst.Harness) {
		t.Fatal("compatible library target missing", m.View().Content)
	}
	path := filepath.Join(inst.StateRoot, "skills/demo")
	_ = atomicWrite(filepath.Join(path, "SKILL.md"), []byte("fixture"), 0600)
	applyComponentRequest(t, m.e, inst, componentRequest{Category: skills, Operation: "disable", Path: path})
	items, err := m.e.components(inst, "base", skills)
	if err != nil {
		t.Fatal(err)
	}
	asset := items[0]
	asset.Subpath = "SKILL.md"
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "/usr/bin/true")
	t.Setenv("VISUAL", "")
	p := tea.NewProgram(editorProgramModel{m, m.editComponent(&asset)}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	if _, err = p.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestSetupFinishedNavigationAndPreviewPathErrors(t *testing.T) {
	m := setupModel{done: true}
	if _, cmd := m.Update(keyMessage("down")); cmd != nil {
		t.Fatal("finished setup restarted")
	}
	e, _ := testEngine(t)
	old := marshalJSONIndent
	defer func() { marshalJSONIndent = old }()
	fault := errors.New("configuration serialization")
	calls := 0
	marshalJSONIndent = func(v any, p, i string) ([]byte, error) {
		calls++
		if calls == 2 {
			return nil, fault
		}
		return old(v, p, i)
	}
	if err := e.setupCLI(context.Background(), []string{"--preview"}, strings.NewReader(""), io.Discard); !errors.Is(err, fault) {
		t.Fatal(err)
	}
}

// signalProgram starts the actual signal path after Bubble Tea has initialized.
type signalProgram struct{ tuiModel }

func (m signalProgram) Init() tea.Cmd {
	return func() tea.Msg { _ = syscall.Kill(os.Getpid(), syscall.SIGTERM); return nil }
}

func TestTUITerminationSignalCancelsThroughProgram(t *testing.T) {
	e, _ := testEngine(t)
	old := newTUIProgram
	defer func() { newTUIProgram = old }()
	newTUIProgram = func(model tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(signalProgram{model.(tuiModel)}, append(options, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))...)
	}
	if err := runTUI(e); err != nil {
		t.Fatal(err)
	}
}
