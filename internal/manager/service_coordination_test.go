package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serviceFixture creates an owned synthetic plist, without touching launchd.
func serviceFixture(t *testing.T) (*engine, installation, string) {
	t.Helper()
	e, _ := testEngine(t)
	s, _ := e.specFor("hermes")
	inst := syntheticInstall(t, e, s, "1")
	path := filepath.Join(e.cfg.Home, "Library/LaunchAgents", s.LaunchLabels[0]+".plist")
	body := "<plist><dict><key>Label</key><string>" + s.LaunchLabels[0] + "</string><key>Program</key><string>" + inst.Path + "</string></dict></plist>"
	if err := atomicWrite(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	inst.ServicePaths = []string{path}
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	return e, inst, path
}

func TestServiceProgramRejectsAmbiguousXMLAndTrailingContent(t *testing.T) {
	valid := `<plist><dict><key>Label</key><string>demo</string><key>Program</key><string>/owned</string></dict></plist>`
	for _, data := range []string{"invalid", valid + "<", valid + "text", valid + "<other/>", `<dict/>`, `<plist><dict><key>Label</key></dict></plist>`, `<plist><dict><string>Label</string><string>demo</string></dict></plist>`, `<plist><dict><key>Label</key><string>other</string></dict></plist>`, `<plist><dict><key>Label</key><string>demo</string><key>Program</key><array/></dict></plist>`, `<plist><dict><key>Label</key><string>demo</string><key>ProgramArguments</key><array><integer>1</integer></array></dict></plist>`} {
		if _, err := serviceProgram([]byte(data), "demo"); err == nil {
			t.Fatal("invalid service accepted", data)
		}
	}
	if program, err := serviceProgram([]byte(valid+" \n<!--tail--><?fixture done?>"), "demo"); err != nil || program != "/owned" {
		t.Fatal(program, err)
	}
}

func TestServiceStopJournalsBeforeBootoutAndReportsFailures(t *testing.T) {
	for _, scenario := range []string{"loaded", "missing", "print-error", "journal-error", "bootout-error"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, _ := serviceFixture(t)
			r := operationRecord{ID: randomID(), Harness: inst.Harness, Status: "preparing"}
			calls := 0
			e.run = commandRunner(func(_ context.Context, c command) (string, error) {
				calls++
				if c.Args[0] == "print" {
					if scenario == "missing" {
						return "Could not find service", errors.New("absent")
					}
					if scenario == "print-error" {
						return "", errors.New("fixture")
					}
					return "loaded", nil
				}
				records, err := e.records()
				if err != nil || len(records) != 1 || len(records[0].Services) != 1 {
					t.Fatal("service not journaled before stop", records, err)
				}
				if scenario == "bootout-error" {
					return "", errors.New("fixture")
				}
				return "", nil
			})
			restore := func() {}
			if scenario == "journal-error" {
				count := 0
				restore = installMutationFault("json", 1, &count)
			}
			err := e.stopServices(context.Background(), inst, &r)
			restore()
			if (err != nil) != strings.HasSuffix(scenario, "error") {
				t.Fatal(scenario, err)
			}
			if scenario == "loaded" && calls != 2 || scenario == "missing" && len(r.Services) != 0 {
				t.Fatal(calls, r.Services)
			}
		})
	}
}

func TestServiceValidationNativeReadFailuresAndLimits(t *testing.T) {
	for _, scenario := range []string{"unknown", "foreign", "linked", "missing", "directory", "large", "open-failure"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, path := serviceFixture(t)
			switch scenario {
			case "unknown":
				inst.Harness = "unknown"
			case "foreign":
				path = filepath.Join(e.cfg.Home, "foreign.plist")
			case "linked":
				_ = os.Remove(path)
				_ = os.Symlink(filepath.Join(e.cfg.Home, "absent"), path)
			case "missing":
				_ = os.Remove(path)
			case "directory":
				_ = os.Remove(path)
				_ = os.Mkdir(path, 0700)
			case "large":
				e.cfg.MetadataBytes = 1
			case "open-failure":
				count := 0
				restore := injectArchiveFailure("open", 1, &count)
				defer restore()
			}
			if err := e.validateService(inst, path); err == nil {
				t.Fatal("invalid service accepted")
			}
		})
	}
}

func TestRecoveryRecordsRejectSettledMissingAndMismatchedOperations(t *testing.T) {
	for _, scenario := range []string{"missing", "complete", "rolled-back", "failed", "executing", "mismatch", "process-error", "plan-error"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, _ := archiveFaultFixture(t)
			r := operationRecord{ID: randomID(), Harness: "pi", Action: "reset", Status: scenario}
			ctx := context.Background()
			if scenario == "mismatch" || scenario == "process-error" || scenario == "plan-error" {
				meta, err := e.snapshot(ctx, p)
				if err != nil {
					t.Fatal(err)
				}
				r.Snapshot = meta.ID
				r.Status = "executing"
				if scenario == "mismatch" {
					r.Harness = "gemini"
				}
				if scenario == "process-error" {
					e.run = commandRunner(func(context.Context, command) (string, error) { return "", errors.New("fixture") })
				}
				if scenario == "plan-error" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
			}
			if scenario != "missing" {
				if err := e.saveRecord(r); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.recoverOperation(ctx, r.ID); err == nil {
				t.Fatal("invalid recovery accepted")
			}
		})
	}
	e, _, path := serviceFixture(t)
	for _, cause := range []error{nil, errors.New("original")} {
		err := e.finishRecovery(operationRecord{ID: randomID(), Services: []string{path + ".foreign"}}, cause)
		if err == nil || cause != nil && !errors.Is(err, cause) {
			t.Fatal("restart failure lost original cause", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.recoveryPlan(ctx, snapshotMeta{Harness: "pi"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.recoveryPlan(context.Background(), snapshotMeta{Harness: "unknown"}); err == nil {
		t.Fatal("unknown recovery accepted")
	}
}

func TestAffectedProcessInspectionIncludesSelectedSharedOwners(t *testing.T) {
	e, inst, _ := serviceFixture(t)
	p := &plan{Install: inst, Spec: harnessSpec{ID: inst.Harness}, Request: request{Owners: []string{inst.Harness}}}
	e.run = commandRunner(func(context.Context, command) (string, error) {
		return fmt.Sprintf("bad ignored\n%d %s\n", os.Getpid(), inst.Path), nil
	})
	if err := e.checkProcesses(context.Background(), p); err != nil {
		t.Fatal("own process blocked", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.checkProcesses(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
	if err := e.restartServices(ctx, []string{"/foreign"}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
}
