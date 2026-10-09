package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failAfterWriter simulates a preview sink closing at a specific write.
type failAfterWriter struct{ remaining int }

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, io.ErrClosedPipe
	}
	w.remaining--
	return len(p), nil
}

func TestSelfPreviewIOFailureCancelsBeforeExecutableRemoval(t *testing.T) {
	e, _ := testEngine(t)
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	if err := atomicWrite(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	defer func() { executablePath = old }()
	executablePath = func() (string, error) { return binary, nil }
	err := e.selfCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), brokenOutput{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("unseen self-removal preview accepted", err)
	}
	if _, err = os.Stat(binary); err != nil {
		t.Fatal("preview failure removed executable", err)
	}
}

func TestPreviewIOFailureCancelsBeforeMutation(t *testing.T) {
	for _, remaining := range []int{0, 1} {
		t.Run(string(rune('0'+remaining)), func(t *testing.T) {
			e, _ := testEngine(t)
			s, _ := e.specFor("pi")
			inst := syntheticInstall(t, e, s, "1")
			e.reg.Installs = []installation{inst}
			if err := writeJSON(e.statePath, e.reg); err != nil {
				t.Fatal(err)
			}
			before, _ := fingerprint(e.cfg.Root)
			err := e.lifecycleCLI(context.Background(), []string{"reset", "--yes", "pi"}, strings.NewReader(""), &failAfterWriter{remaining})
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("preview failure did not cancel: %v", err)
			}
			after, _ := fingerprint(e.cfg.Root)
			if before != after {
				t.Fatal("preview failure changed manager state")
			}
		})
	}
	if err := confirmCLI(strings.NewReader("approved\n"), brokenOutput{}, "approved", false); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("unseen approval prompt accepted", err)
	}
}
