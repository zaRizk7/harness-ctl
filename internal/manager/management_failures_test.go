package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// managementFailureFixture returns one authorized fixture operation and unrelated marker.
func managementFailureFixture(t *testing.T, kind string) (*engine, func() error, string) {
	t.Helper()
	e, _ := testEngine(t)
	marker := filepath.Join(e.cfg.Home, "unrelated", "keep")
	if err := atomicWrite(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	item := reusableSkill("a")
	a := account{ID: "a", Provider: "openai", Kind: "subscription", Enabled: true}
	var operation func() error
	switch kind {
	case "library-save":
		path, _ := e.libraryPath()
		before, _ := fingerprint(path)
		operation = func() error { return e.saveLibraryItem(item, before) }
	case "library-change", "library-preview":
		if err := e.saveLibraryItem(item, ""); err != nil {
			t.Fatal(err)
		}
		path, _ := e.libraryPath()
		before, _ := fingerprint(path)
		if kind == "library-change" {
			operation = func() error { return e.changeLibraryItem("a", "disable", before) }
		} else {
			operation = func() error {
				_, err := e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil)
				return err
			}
		}
	case "account-save":
		path, _ := e.accountPath()
		before, _ := fingerprint(path)
		operation = func() error { return e.saveAccountChecked(a, before) }
	case "account-change":
		if err := e.saveAccount(a); err != nil {
			t.Fatal(err)
		}
		path, _ := e.accountPath()
		before, _ := fingerprint(path)
		operation = func() error { return e.setAccountEnabled("a", false, before) }
	}
	return e, operation, marker
}

func TestManagementFailuresPreserveUnrelatedStateAndValidVaults(t *testing.T) {
	for _, kind := range []string{"library-save", "library-change", "library-preview", "account-save", "account-change"} {
		for _, boundary := range []string{"validate", "fingerprint", "read"} {
			t.Run(kind+"/"+boundary, func(t *testing.T) {
				_, operation, _ := managementFailureFixture(t, kind)
				count := 0
				restore := installMutationFault(boundary, 0, &count)
				err := operation()
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, operation, marker := managementFailureFixture(t, kind)
						count := 0
						restore := installMutationFault(boundary, nth, &count)
						err := operation()
						restore()
						if count < nth {
							t.Fatal("failure boundary not reached")
						}
						// Some missing-state reads intentionally fall back to an empty inventory.
						// Regardless of the outcome, no unrelated data or malformed vault can remain.
						data, readErr := os.ReadFile(marker)
						if readErr != nil || string(data) != "keep" {
							t.Fatal("unrelated state changed", readErr)
						}
						if _, readErr = e.loadAccounts(); readErr != nil {
							t.Fatal("invalid account vault retained", err, readErr)
						}
						if _, readErr = e.loadLibrary(); readErr != nil {
							t.Fatal("invalid reusable vault retained", err, readErr)
						}
					})
				}
			})
		}
	}
}
