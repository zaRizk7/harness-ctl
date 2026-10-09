package manager

import (
	"strings"
	"testing"
)

func TestProviderChangeRequiresBothReplacementCredentials(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "a", Provider: "openai", Kind: "api", Credential: "inference", MonitorCredential: "admin", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	a.Provider, a.Credential, a.MonitorCredential = "anthropic", "replacement", ""
	if err := e.saveAccount(a); err == nil {
		t.Fatal("provider change retained foreign reporting key")
	}
	items, err := e.loadAccounts()
	if err != nil || items[0].Provider != "openai" {
		t.Fatal(items, err)
	}
	a.MonitorCredential = "replacement-admin"
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
}

func TestCodexDefaultRetainsSandboxWithAutomaticReviewer(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("codex")
	args := strings.Join(defaultLaunchArgs(s, nil, false), " ")
	for _, required := range []string{"--sandbox workspace-write", "--ask-for-approval on-request", `approvals_reviewer="auto_review"`} {
		if !strings.Contains(args, required) {
			t.Fatal("missing secure approval default", required, args)
		}
	}
	for _, forbidden := range []string{"--yolo", "dangerously", "danger-full-access"} {
		if strings.Contains(args, forbidden) {
			t.Fatal("unrestricted default", args)
		}
	}
}
