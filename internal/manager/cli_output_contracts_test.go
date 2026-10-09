package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLibraryRemovalRequiresVisiblePreview(t *testing.T) {
	e, _ := testEngine(t)
	if err := e.saveLibraryItem(reusableSkill("demo"), ""); err != nil {
		t.Fatal(err)
	}
	err := e.libraryCLI(context.Background(), []string{"remove", "--yes", "demo"}, strings.NewReader(""), brokenOutput{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("closed preview output ignored", err)
	}
	items, err := e.loadLibrary()
	if err != nil || len(items) != 1 {
		t.Fatal("unseen preview removed library entry", items, err)
	}
}

func TestCLIRejectsCancelledDiscoveryAndMalformedRequests(t *testing.T) {
	e, _ := testEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, args := range [][]string{{"components", "list", "pi", "mcp"}, {"launch", "pi"}, {"list"}, {"upgrade", "--preview", "pi"}} {
		if err := e.cli(ctx, args, strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{{"components", "list", "--unknown"}, {"components", "list", "pi"}, {"components", "list", "missing", "mcp"}, {"components", "apply", "pi", "bad.json"}, {"components", "list", "pi", "mcp"}} {
		if args[len(args)-1] == "mcp" && args[2] == "pi" {
			e.cfg.StateRoots["pi"] = e.cfg.Home
		}
		if err := e.cli(context.Background(), args, strings.NewReader(""), io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	for value, count := range map[string]int{"none": 0, "auth,mcp": 2} {
		cats, err := parseCategories(value)
		if err != nil || len(cats) != count {
			t.Fatal(value, cats, err)
		}
	}
	e.cfg.StateRoots = map[string]string{}
	if err := e.cli(context.Background(), []string{"reset", "--preview", "--owners", "one,two", "pi"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing installation accepted")
	}
	path := filepath.Join(e.cfg.Home, "request.json")
	_ = writeJSON(path, componentRequest{Operation: "add", Category: mcp, Path: filepath.Join(e.cfg.Home, ".pi/agent/settings.json"), Field: "/mcpServers/demo", Value: []byte(`{}`)})
	for _, out := range []io.Writer{brokenOutput{}, io.Discard} {
		if err := e.componentsCLI(context.Background(), []string{"apply", "pi", path}, strings.NewReader("cancel\n"), out); err == nil {
			t.Fatal("unseen or unapproved component applied")
		}
	}
	_ = writeJSON(path, componentRequest{Operation: "bad", Category: mcp})
	if err := e.componentsCLI(context.Background(), []string{"apply", "pi", path}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("invalid request accepted")
	}
	old := fingerprint
	defer func() { fingerprint = old }()
	fingerprint = func(string) (string, error) { return "", errors.New("CLI inventory failure") }
	if err := e.accountsCLI(context.Background(), []string{"list"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("failure lost")
	}
}

func TestStartupReadsOnlySelectedConfigurationAndRoutesTUI(t *testing.T) {
	e, _ := testEngine(t)
	t.Setenv("HOME", e.cfg.Home)
	file := filepath.Join(e.cfg.Root, "config.json")
	_ = atomicWrite(file, []byte("bad"), 0600)
	if status := mainWithIO([]string{"--root", e.cfg.Root, "config"}, strings.NewReader(""), io.Discard, io.Discard); status != 1 {
		t.Fatal(status)
	}
	_ = writeJSON(file, defaultConfig(e.cfg.Home))
	if status := mainWithIO([]string{"--root", e.cfg.Root, "config"}, strings.NewReader(""), io.Discard, io.Discard); status != 1 {
		t.Fatal("mismatched stored root accepted", status)
	}
	_ = os.Remove(file)
	old := newTUIProgram
	defer func() { newTUIProgram = old }()
	newTUIProgram = func(m tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, append(options, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard))...)
	}
	if status := mainWithIO(nil, strings.NewReader(""), io.Discard, io.Discard); status != 0 {
		t.Fatal("default TUI failed", status)
	}
	if status := mainWithIO([]string{"tui"}, strings.NewReader(""), io.Discard, io.Discard); status != 0 {
		t.Fatal(status)
	}
}

func TestSetupCLIApprovalFailuresAndVerifiedBinary(t *testing.T) {
	e, _ := testEngine(t)
	for _, args := range [][]string{{"--unknown"}, {"unexpected"}, {"--prefix", "relative"}, {"--headless"}} {
		if err := e.setupCLI(context.Background(), args, strings.NewReader("cancel\n"), io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	if err := e.setupCLI(context.Background(), []string{"--preview"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := e.setupCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), brokenOutput{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.setupCLI(ctx, []string{"--yes"}, strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	unlock, err := e.lock()
	if err != nil {
		t.Fatal(err)
	}
	err = e.setupCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), io.Discard)
	unlock()
	if err == nil {
		t.Fatal("setup ignored active writer")
	}
	binary := filepath.Join(e.cfg.Home, "download", "harness-ctl")
	data := []byte("fixture executable")
	_ = atomicWrite(binary, data, 0700)
	sum := sha256.Sum256(data)
	if err = e.setupCLI(context.Background(), []string{"--headless", "--yes", "--binary", binary, "--sha256", hex.EncodeToString(sum[:])}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	old := newTUIProgram
	defer func() { newTUIProgram = old }()
	newTUIProgram = func(m tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, append(options, tea.WithInput(strings.NewReader("\x1b")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())...)
	}
	if err = e.setupCLI(context.Background(), nil, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("cancelled interactive setup succeeded")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	newTUIProgram = func(m tea.Model, options ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, append(options, tea.WithContext(ctx), tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))...)
	}
	if err = e.setupCLI(context.Background(), nil, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("runtime failure lost")
	}
}
