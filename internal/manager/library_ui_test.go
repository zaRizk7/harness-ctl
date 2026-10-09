package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/library"
)

func TestLibraryCLIAndTUIControls(t *testing.T) {
	e, inst, _ := componentFixture(t)
	item := library.Item{ID: "skill", Category: "skills", Enabled: true, Files: map[string][]byte{"SKILL.md": []byte("skill")}, Targets: map[string]library.Target{inst.Harness: {Path: "skills/shared"}}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := e.cli(context.Background(), []string{"library", "list"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SKILL.md") {
		t.Fatal("library inventory exposed payload")
	}
	m := newModel(e)
	m.installs = []installation{inst}
	model, cmd := m.key(keyMessage("l"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "library" || !strings.Contains(m.View().Content, "skill") {
		t.Fatal(m.screen)
	}
	model, _ = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.key(keyMessage("space"))
	m = model.(tuiModel)
	model, cmd = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	model, _ = m.Update(cmd())
	m = model.(tuiModel)
	if m.screen != "preview" || m.libraryApplication == nil {
		t.Fatal(m.screen, m.status)
	}
	if _, cmd = m.confirm(); cmd != nil {
		t.Fatal("fanout skipped approval")
	}
	m.typed = "apply"
	model, cmd = m.confirm()
	m = completeUIOperation(t, model.(tuiModel), cmd)
	if m.screen != "result" {
		t.Fatal(m.screen)
	}
	for _, action := range []string{"disable", "enable", "remove"} {
		out.Reset()
		if err := e.cli(context.Background(), []string{"library", action, "--yes", item.ID}, strings.NewReader(""), &out); err != nil {
			t.Fatal(err)
		}
	}
}
