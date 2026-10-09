package manager

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCLIBatchPreviewAndApply(t *testing.T) {
	e, _ := testEngine(t)
	for _, id := range []string{"pi", "codex"} {
		s, _ := e.specFor(id)
		e.reg.Installs = append(e.reg.Installs, syntheticInstall(t, e, s, "1.0.0"))
	}
	_ = writeJSON(e.statePath, e.reg)
	var out bytes.Buffer
	if err := e.cli(context.Background(), []string{"uninstall", "--preview", "pi", "codex"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if len(e.reg.Installs) != 2 || !strings.Contains(out.String(), "Batch") {
		t.Fatal("preview mutated or omitted batch", out.String())
	}
	out.Reset()
	if err := e.cli(context.Background(), []string{"uninstall", "--yes", "pi", "codex"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if len(e.reg.Installs) != 0 {
		t.Fatal("CLI did not apply batch")
	}
}

func TestCLIRejectsInvalidAndCancelledActions(t *testing.T) {
	e, _ := testEngine(t)
	for _, args := range [][]string{{"nonsense"}, {"install"}, {"reset", "--preserve", "bad", "pi"}, {"reset", "--unknown", "pi"}, {"components", "list", "pi", "bad"}, {"components"}} {
		var out bytes.Buffer
		if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err == nil {
			t.Fatal("invalid CLI accepted", args)
		}
	}
	s, _ := e.specFor("pi")
	e.reg.Installs = []installation{syntheticInstall(t, e, s, "1.0.0")}
	_ = writeJSON(e.statePath, e.reg)
	var out bytes.Buffer
	if err := e.cli(context.Background(), []string{"uninstall", "pi"}, strings.NewReader("no\n"), &out); err == nil {
		t.Fatal("cancelled operation executed")
	}
	if len(e.reg.Installs) != 1 {
		t.Fatal("cancelled CLI mutated state")
	}
}
