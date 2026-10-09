package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfUninstallChoices(t *testing.T) {
	for _, removeHarnesses := range []bool{false, true} {
		for _, removeState := range []bool{false, true} {
			t.Run(map[bool]string{false: "keep", true: "remove"}[removeHarnesses]+map[bool]string{false: "keep-state", true: "remove-state"}[removeState], func(t *testing.T) {
				e, _ := testEngine(t)
				s, _ := e.specFor("pi")
				inst := syntheticInstall(t, e, s, "1.0.0")
				e.reg.Installs = []installation{inst}
				_ = writeJSON(e.statePath, e.reg)
				state := filepath.Join(inst.StateRoot, "auth.json")
				_ = atomicWrite(state, []byte("synthetic"), 0600)
				binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
				_ = atomicWrite(binary, []byte("synthetic executable"), 0700)
				p, err := e.buildSelfPlan(context.Background(), binary, removeHarnesses, removeState, request{Preserve: keepAll()})
				if err != nil {
					t.Fatal(err)
				}
				if err = e.executeSelf(context.Background(), p, "", nil); err == nil {
					t.Fatal("unapproved self uninstall executed")
				}
				if err = e.executeSelf(context.Background(), p, p.ID, nil); err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(binary); !os.IsNotExist(err) {
					t.Fatal("manager executable remains")
				}
				_, err = os.Stat(inst.Path)
				if removeHarnesses != os.IsNotExist(err) {
					t.Fatal("harness removal choice ignored", err)
				}
				if _, err = os.Stat(state); err != nil {
					t.Fatal("manager state removal discarded harness auth", err)
				}
				_, err = os.Stat(e.statePath)
				if removeState != os.IsNotExist(err) {
					t.Fatal("manager state choice ignored", err)
				}
			})
		}
	}
}

func TestSelfUninstallGuardsAndStalePreview(t *testing.T) {
	e, _ := testEngine(t)
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	_ = atomicWrite(binary, []byte("synthetic"), 0700)
	p, err := e.buildSelfPlan(context.Background(), binary, false, true, request{Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	_ = atomicWrite(filepath.Join(e.cfg.Root, "accounts.json"), []byte("new"), 0600)
	if err = e.executeSelf(context.Background(), p, p.ID, nil); err == nil {
		t.Fatal("stale self preview applied")
	}
	if _, err = e.buildSelfPlan(context.Background(), filepath.Join(e.cfg.Home, "foreign"), false, false, request{Preserve: keepAll()}); err == nil {
		t.Fatal("foreign binary accepted")
	}
	link := filepath.Join(e.cfg.Home, "harness-ctl")
	_ = os.Symlink(binary, link)
	if _, err = e.buildSelfPlan(context.Background(), link, false, false, request{Preserve: keepAll()}); err == nil {
		t.Fatal("linked binary accepted")
	}
}

func (k *memoryKeys) Delete(string) error { k.value = ""; return nil }

func TestSelfUninstallAcceptsShippedArchitectureNames(t *testing.T) {
	for _, name := range []string{"harness-ctl-darwin-arm64", "harness-ctl-darwin-amd64"} {
		e, _ := testEngine(t)
		path := filepath.Join(e.cfg.Home, "tools", name)
		if err := atomicWrite(path, []byte("synthetic executable"), 0700); err != nil {
			t.Fatal(err)
		}
		p, err := e.buildSelfPlan(context.Background(), path, false, false, request{Preserve: keepAll()})
		if err != nil {
			t.Fatal(name, err)
		}
		if err = e.executeSelf(context.Background(), p, p.ID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("manager binary remains", err)
		}
	}
}

func TestSelfUninstallRemovesOnlyRecordedLauncher(t *testing.T) {
	e, _ := testEngine(t)
	binary := filepath.Join(e.cfg.Root, "app", "harness-ctl")
	if err := atomicWrite(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(e.cfg.Home, "bin", "harness-ctl")
	if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(e.cfg.Root, "installation.json"), map[string]string{"Binary": binary, "Link": link}); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildSelfPlan(context.Background(), binary, false, false, request{Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeSelf(context.Background(), p, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("manager launcher remains", err)
	}
}

func TestSelfCLIUsesApprovedSyntheticExecutable(t *testing.T) {
	e, _ := testEngine(t)
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	if err := atomicWrite(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	t.Cleanup(func() { executablePath = old })
	executablePath = func() (string, error) { return binary, nil }
	var out strings.Builder
	for _, args := range [][]string{{"--preview"}, {"--state", "--preview"}, {"--harnesses", "--preview"}} {
		if err := e.selfCLI(context.Background(), args, strings.NewReader(""), &out); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{{"--unknown"}, {"unexpected"}, {"--preserve", "bad"}} {
		if e.selfCLI(context.Background(), args, strings.NewReader(""), &out) == nil {
			t.Fatal(args)
		}
	}
	if e.selfCLI(context.Background(), nil, strings.NewReader("cancel\n"), &out) == nil {
		t.Fatal("cancelled self removal")
	}
	if err := e.selfCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary); !os.IsNotExist(err) {
		t.Fatal("binary retained")
	}
	executablePath = func() (string, error) { return "", errors.New("fixture executable error") }
	if e.selfCLI(context.Background(), nil, strings.NewReader(""), &out) == nil {
		t.Fatal("executable failure lost")
	}
}

func TestSelfCLIProvidesHarnessOwnershipAndPermanentChoices(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(inst.StateRoot, "config.toml"), []byte("enabled=true"), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	if err := atomicWrite(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	t.Cleanup(func() { executablePath = old })
	executablePath = func() (string, error) { return binary, nil }
	var out strings.Builder
	if err := e.selfCLI(context.Background(), []string{"--harnesses", "--preserve", "none", "--owners", "codex,Codex desktop / IDE", "--permanent", "--preview"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Permanent") {
		t.Fatal(out.String())
	}
}
