package manager

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/library"
	"github.com/zaRizk7/harness-ctl/internal/setup"
)

func TestSelfRemovalOwnerNavigationAndPreservationPreview(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(e.cfg.Home, "tools", "harness-ctl")
	if err := atomicWrite(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	original := executablePath
	t.Cleanup(func() { executablePath = original })
	executablePath = func() (string, error) { return binary, nil }
	m := newModel(e)
	m.installs = []installation{inst, inst}
	m.screen = "self-options"
	m.selfRemoveHarnesses = true
	if m.itemCount() != 4 {
		t.Fatal("missing independent state choices")
	}
	model, _, _ := m.managementKey("o")
	m = model.(tuiModel)
	if !slices.Contains(m.selfOwnerCandidates, s.ID) || m.itemCount() != len(m.selfOwnerCandidates) {
		t.Fatal(m.selfOwnerCandidates)
	}
	if !strings.Contains(m.View().Content, s.SharedClients[0]) {
		t.Fatal(m.View().Content)
	}
	seen := map[string]bool{}
	for i, owner := range m.selfOwnerCandidates {
		if seen[owner] {
			t.Fatal("duplicate owner choice", owner)
		}
		seen[owner] = true
		m.cursor = i
		model, _ = m.key(keyMessage("space"))
		m = model.(tuiModel)
		if !slices.Contains(m.selfOwners, owner) {
			t.Fatal("owner not selected")
		}
		model, _ = m.key(keyMessage("space"))
		m = model.(tuiModel)
		if slices.Contains(m.selfOwners, owner) {
			t.Fatal("owner not deselected")
		}
	}
	model, _ = m.key(keyMessage("enter"))
	m = model.(tuiModel)
	if m.screen != "self-options" {
		t.Fatal(m.screen)
	}
	m.selfDiscardHarnessState = true
	model, cmd := m.key(keyMessage("enter"))
	m = model.(tuiModel)
	msg := cmd().(selfPlanMsg)
	if msg.err != nil || msg.plan.Batch == nil || len(msg.plan.Batch.Plans[0].Request.Preserve) != 0 {
		t.Fatal(msg)
	}
	m.screen = "self-options"
	executablePath = func() (string, error) { return "", errors.New("synthetic executable error") }
	_, cmd = m.key(keyMessage("enter"))
	if msg = cmd().(selfPlanMsg); msg.err == nil {
		t.Fatal("self preview dropped executable error")
	}
}

func TestSetupTUIPathFollowsApprovedLinkChoice(t *testing.T) {
	for _, direct := range []bool{true, false} {
		e, _ := testEngine(t)
		source := filepath.Join(e.cfg.Home, "download")
		if err := atomicWrite(source, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
		link := filepath.Join(e.cfg.Home, "bin")
		prefix := filepath.Join(e.cfg.Root, "app")
		m := setupModel{options: setup.Options{Home: e.cfg.Home, Root: e.cfg.Root, Prefix: prefix, Source: source, SHA256: hash, MaxBytes: 100, ShellFile: filepath.Join(e.cfg.Home, ".zshrc"), BinDirs: []string{e.cfg.BinDir}}, link: link}
		if !direct {
			model, _ := m.Update(keyMessage("space"))
			m = model.(setupModel)
		}
		model, _ := m.Update(keyMessage("enter"))
		m = model.(setupModel)
		if m.err != nil || m.plan == nil {
			t.Fatal(m.err)
		}
		want := prefix
		if !direct {
			want = link
		}
		if !slices.Equal(m.options.BinDirs, []string{e.cfg.BinDir, want}) || !strings.Contains(m.View().Content, "Approved PATH append") {
			t.Fatal(m.options, m.View().Content)
		}
		var out strings.Builder
		args := []string{"setup", "--binary", source, "--sha256", hash, "--prefix", prefix, "--preview"}
		if !direct {
			args = append(args, "--link-dir", link)
		}
		if err := e.cli(context.Background(), args, strings.NewReader(""), &out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCapturedPluginMalformedManifestIsRejected(t *testing.T) {
	e, _ := testEngine(t)
	item := library.Item{ID: "broken", Category: "plugins", Files: map[string][]byte{"package.json": []byte("invalid")}, Targets: map[string]library.Target{"pi": {Path: "plugins/demo"}}}
	if err := e.validateLibraryItem(item); err == nil {
		t.Fatal("malformed captured manifest accepted")
	}
}

func TestSubscriptionSourceRejectsUnknownProviderBeforeProcess(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "source", Provider: "missing", Kind: "subscription", ReportHarness: "codex"}
	if err := e.validateAccount(a); err == nil {
		t.Fatal("unknown provider source")
	}
	if _, err := e.nativeSubscriptionReport(context.Background(), a); err == nil {
		t.Fatal("invalid source reached process")
	}
	if _, err := os.Stat(e.cfg.Root); !os.IsNotExist(err) {
		t.Fatal("source validation wrote storage", err)
	}
}
