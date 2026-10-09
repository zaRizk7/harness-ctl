package manager

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// mutationFaultFixture prepares a valid transaction before filesystem faults are injected.
func mutationFaultFixture(t *testing.T, kind string) (*engine, *plan, string) {
	t.Helper()
	e, _, _ := componentFixture(t)
	var p *plan
	var err error
	marker := filepath.Join(e.cfg.Home, "unrelated", "keep")
	if err := atomicWrite(marker, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	inst := e.reg.Installs[0]
	switch kind {
	case "reset":
		p, err = e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "reset", Preserve: map[category]bool{}})
	case "component":
		p, err = e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &componentRequest{Category: mcp, Operation: "remove", Path: filepath.Join(inst.StateRoot, "settings.json"), Field: "/mcpServers/one"}})
	case "profile":
		p, err = e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "profile", Preserve: keepAll(), Disabled: map[category]bool{mcp: true}})
	}
	if err != nil {
		t.Fatal(err)
	}
	return e, p, marker
}

func TestTransactionsKeepUnrelatedStateAcrossFilesystemFailures(t *testing.T) {
	for _, kind := range []string{"reset", "component", "profile"} {
		for _, boundary := range []string{"fingerprint", "validate", "atomic", "json", "read"} {
			t.Run(kind+"/"+boundary, func(t *testing.T) {
				// Count the real boundary calls in one successful execution.
				baselineCount := 0
				e, p, _ := mutationFaultFixture(t, kind)
				restore := installMutationFault(boundary, 0, &baselineCount)
				err := e.execute(context.Background(), p, p.ID, nil)
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				for nth := 1; nth <= baselineCount; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, p, marker := mutationFaultFixture(t, kind)
						count := 0
						restore := installMutationFault(boundary, nth, &count)
						err := e.execute(context.Background(), p, p.ID, nil)
						restore()
						if count < nth {
							t.Fatal("fault point was not reached")
						}
						data, readErr := os.ReadFile(marker)
						if readErr != nil || string(data) != "untouched" {
							t.Fatal("unrelated state changed", readErr)
						}
						if err == nil {
							t.Log("boundary failure was handled before verified completion")
						}
						if registryErr := e.refreshRegistry(); registryErr != nil {
							t.Fatal("invalid registry retained", registryErr)
						}
					})
				}
			})
		}
	}
}

// installMutationFault injects one deterministic IO failure and returns restoration.
func installMutationFault(boundary string, nth int, count *int) func() {
	oldFingerprint, oldValidate, oldAtomic, oldJSON, oldRead := fingerprint, validateOwnedPath, atomicWrite, writeJSON, readJSON
	fail := func() bool { *count++; return nth > 0 && *count == nth }
	fault := errors.New("fixture filesystem failure")
	switch boundary {
	case "fingerprint":
		fingerprint = func(path string) (string, error) {
			if fail() {
				return "", fault
			}
			return oldFingerprint(path)
		}
	case "validate":
		validateOwnedPath = func(root, path string) error {
			if fail() {
				return fault
			}
			return oldValidate(root, path)
		}
	case "atomic":
		atomicWrite = func(path string, data []byte, mode fs.FileMode) error {
			if fail() {
				return fault
			}
			return oldAtomic(path, data, mode)
		}
	case "json":
		writeJSON = func(path string, value any) error {
			if fail() {
				return fault
			}
			return oldJSON(path, value)
		}
	case "read":
		readJSON = func(path string, value any) error {
			if fail() {
				return fault
			}
			return oldRead(path, value)
		}
	}
	return func() {
		fingerprint, validateOwnedPath, atomicWrite, writeJSON, readJSON = oldFingerprint, oldValidate, oldAtomic, oldJSON, oldRead
	}
}
