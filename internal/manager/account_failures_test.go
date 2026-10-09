package manager

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

func encryptedVault(t *testing.T, e *engine, data []byte) {
	t.Helper()
	id, err := e.identity(true)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	w, err := age.Encrypt(&b, id.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(data)
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = atomicWrite(filepath.Join(e.cfg.Root, "accounts.age"), b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAccountVaultRejectsCorruptionAndInvalidRecords(t *testing.T) {
	for _, body := range []string{`not-json`, `[{"id":"../escape","provider":"openai","kind":"api"}]`, `[{"id":"a","provider":"openai","kind":"api"},{"id":"a","provider":"openai","kind":"api"}]`, `[{"id":"a","provider":"unknown","kind":"api"}]`} {
		t.Run(body, func(t *testing.T) {
			e, _ := testEngine(t)
			encryptedVault(t, e, []byte(body))
			if _, err := e.loadAccounts(); err == nil {
				t.Fatal("invalid vault accepted")
			}
		})
	}
	e, _ := testEngine(t)
	path := filepath.Join(e.cfg.Root, "accounts.age")
	_ = atomicWrite(path, []byte("not-encrypted"), 0600)
	if _, err := e.loadAccounts(); err == nil {
		t.Fatal("unencrypted vault accepted")
	}
	encryptedVault(t, e, []byte(`[]`))
	data, _ := os.ReadFile(path)
	data[len(data)-1] ^= 1
	_ = os.WriteFile(path, data, 0600)
	if _, err := e.loadAccounts(); err == nil {
		t.Fatal("tampered vault accepted")
	}
	e.cfg.MetadataBytes = 1
	if _, err := e.loadAccounts(); err == nil {
		t.Fatal("oversized vault accepted")
	}
	if err := e.writeAccounts([]account{{ID: "a"}}); err == nil {
		t.Fatal("oversized write accepted")
	}
}

func TestAccountCredentialSeparationAndValidation(t *testing.T) {
	e, _ := testEngine(t)
	base := account{ID: "a", Provider: "openai", Kind: "api", Credential: "inference", MonitorCredential: "reporting", Enabled: true}
	if err := e.saveAccount(base); err != nil {
		t.Fatal(err)
	}
	env, err := e.accountLaunchEnvironment("a")
	if err != nil || env["OPENAI_API_KEY"] != "inference" {
		t.Fatal(env, err)
	}
	for _, mutate := range []func(*account){func(a *account) { a.ID = "../unsafe" }, func(a *account) { a.Provider = "missing" }, func(a *account) { a.Kind = "unknown" }, func(a *account) { a.Project = "unsafe/path" }, func(a *account) { a.Organization = "unsafe/path" }, func(a *account) { a.Credential = "newline\nkey" }} {
		a := base
		mutate(&a)
		if err = e.saveAccount(a); err == nil {
			t.Fatal("invalid account saved", a.ID)
		}
	}
	changed := base
	changed.Provider = "anthropic"
	changed.Credential = ""
	if err = e.saveAccount(changed); err == nil {
		t.Fatal("provider change reused another provider key")
	}
	if err = e.removeAccount("missing"); err == nil {
		t.Fatal("absent account removed")
	}
	base.Enabled = false
	if err = e.saveAccount(base); err != nil {
		t.Fatal(err)
	}
	if _, err = e.accountLaunchEnvironment("a"); err == nil {
		t.Fatal("disabled account used")
	}
	base.Enabled = true
	base.Kind = "subscription"
	if err = e.saveAccount(base); err != nil {
		t.Fatal(err)
	}
	if _, err = e.accountLaunchEnvironment("a"); err == nil {
		t.Fatal("subscription used as API key")
	}
	if _, err = e.accountLaunchEnvironment("missing"); err == nil {
		t.Fatal("unknown account used")
	}
	m := e.monitorAccount(context.Background(), account{Provider: "missing", Enabled: true}, time.Now())
	if len(m.Errors) == 0 {
		t.Fatal(m)
	}
	m = e.monitorAccount(context.Background(), account{Provider: "openai", Enabled: false}, time.Now())
	if len(m.Notes) == 0 {
		t.Fatal(m)
	}
}

func TestAccountsCLIControlsAndWatchCancellation(t *testing.T) {
	e, r := testEngine(t)
	a := account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true}
	file := filepath.Join(e.cfg.Home, "account.json")
	_ = writeJSON(file, a)
	var out bytes.Buffer
	if err := e.accountsCLI(context.Background(), []string{"set", file}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list"}, {"disable", "a"}, {"enable", "a"}, {"monitor", "a"}, {"open", "a", "billing"}, {"open", "a", "usage"}, {"open", "a", "keys"}, {"open", "a", "subscription"}} {
		if err := e.accountsCLI(context.Background(), args, strings.NewReader(""), &out); err != nil {
			t.Fatal(args, err)
		}
	}
	if len(r.calls) != 4 {
		t.Fatal(r.calls)
	}
	for _, args := range [][]string{nil, {"set"}, {"set", "missing-file"}, {"monitor", "--refresh", "0"}, {"monitor", "missing"}, {"open", "a"}, {"open", "a", "unknown"}, {"unknown", "a"}, {"enable"}, {"enable", "missing"}, {"monitor", "--unknown"}} {
		if err := e.accountsCLI(context.Background(), args, strings.NewReader(""), &out); err == nil {
			t.Fatal("invalid account action accepted", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.accountsCLI(ctx, []string{"monitor", "--watch", "a"}, strings.NewReader(""), &out); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := e.accountsCLI(context.Background(), []string{"remove", "a"}, strings.NewReader("no\n"), &out); err == nil {
		t.Fatal("unapproved credential removal executed")
	}
	if err := e.accountsCLI(context.Background(), []string{"remove", "a"}, strings.NewReader("remove-a\n"), &out); err != nil {
		t.Fatal(err)
	}
}
