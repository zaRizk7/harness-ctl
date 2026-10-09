package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type accountWatchOutput struct {
	e      *engine
	cancel context.CancelFunc
	calls  int
	data   bytes.Buffer
	err    error
}

func (w *accountWatchOutput) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		w.err = w.e.setAccountEnabled("a", false, "")
	}
	if w.calls == 2 {
		w.cancel()
	}
	return w.data.Write(p)
}

func TestAccountWatchObservesDisabledAccount(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &accountWatchOutput{e: e, cancel: cancel}
	err := e.accountsCLI(ctx, []string{"monitor", "--watch", "--refresh", "1", "a"}, strings.NewReader(""), out)
	if !errors.Is(err, context.Canceled) || out.err != nil || out.calls != 2 {
		t.Fatal(err, out.err, out.calls)
	}
	decoder := json.NewDecoder(&out.data)
	var first, second []accountMetric
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 0 {
		t.Fatal("disabled account still monitored", first, second)
	}
}

type accountHTTP func(*http.Request) (*http.Response, error)

func (f accountHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestAccountVaultNeverPublishesSecrets(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "team", Provider: "openai", Kind: "api", Label: "Team", Credential: "synthetic-key", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(e.cfg.Root, "accounts.age"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), a.Credential) {
		t.Fatal("credential stored in plaintext")
	}
	items, err := e.loadAccounts()
	if err != nil || len(items) != 1 || items[0].Credential != a.Credential {
		t.Fatal(items, err)
	}
	a.Credential = ""
	a.Label = "Edited"
	if err = e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	items, err = e.loadAccounts()
	if err != nil || items[0].Credential != "synthetic-key" {
		t.Fatal("metadata edit lost key", err)
	}
	var out strings.Builder
	if err = e.accountsCLI(context.Background(), []string{"list"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "synthetic-key") {
		t.Fatal("account listing exposed credential")
	}
	if err = e.removeAccount("team"); err != nil {
		t.Fatal(err)
	}
	items, err = e.loadAccounts()
	if err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}

func TestAccountMutationsRejectStalePreviewAndPreserveKeys(t *testing.T) {
	e, _ := testEngine(t)
	a := account{ID: "a", Provider: "openai", Kind: "api", Credential: "first", Enabled: true}
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	path, err := e.accountPath()
	if err != nil {
		t.Fatal(err)
	}
	before, err := fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	a.Credential = "replacement"
	if err := e.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	if err := e.removeAccountChecked("a", before); err == nil {
		t.Fatal("stale deletion accepted")
	}
	if err := e.setAccountEnabled("a", false, before); err == nil {
		t.Fatal("stale toggle accepted")
	}
	if err := e.setAccountEnabled("a", false, ""); err != nil {
		t.Fatal(err)
	}
	items, err := e.loadAccounts()
	if err != nil || items[0].Enabled || items[0].Credential != "replacement" {
		t.Fatal(items, err)
	}
}

func TestAccountPageUnavailableDoesNotExecuteOpen(t *testing.T) {
	e, r := testEngine(t)
	e.cfg.Providers = []providerSpec{{ID: "gateway", KeyEnv: "KEY"}}
	if err := e.saveAccount(account{ID: "a", Provider: "gateway", Kind: "api", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := e.accountsCLI(context.Background(), []string{"open", "a", "billing"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("empty account URL opened")
	}
	if len(r.calls) != 0 {
		t.Fatal(r.calls)
	}
}
