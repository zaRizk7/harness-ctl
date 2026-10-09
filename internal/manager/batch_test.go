package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBatchUninstallApprovalAndStaleCheck(t *testing.T) {
	e, r := testEngine(t)
	for _, id := range []string{"pi", "codex"} {
		s, _ := e.specFor(id)
		e.reg.Installs = append(e.reg.Installs, syntheticInstall(t, e, s, "1.0.0"))
	}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	reqs := []request{{Harness: "pi", Action: "uninstall"}, {Harness: "codex", Action: "uninstall"}}
	b, err := e.buildBatch(context.Background(), reqs)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeBatch(context.Background(), b, "", nil); err == nil {
		t.Fatal("unapproved batch executed")
	}
	if err = atomicWrite(filepath.Join(b.Plans[1].Install.Root, "new"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.executeBatch(context.Background(), b, b.ID, nil); err == nil {
		t.Fatal("stale batch executed")
	}
	if len(r.calls) != 0 || len(e.reg.Installs) != 2 {
		t.Fatal("stale batch partly applied")
	}
	b, err = e.buildBatch(context.Background(), reqs)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeBatch(context.Background(), b, b.ID, nil); err != nil {
		t.Fatal(err)
	}
	if len(e.reg.Installs) != 0 {
		t.Fatal("batch did not uninstall both")
	}
	for _, p := range b.Plans {
		if _, err = os.Stat(p.Install.Path); !os.IsNotExist(err) {
			t.Fatal("batch retained binary")
		}
	}
}

func TestBatchRejectsDuplicateAndChangedPlans(t *testing.T) {
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1.0.0")
	e.reg.Installs = []installation{inst}
	_ = writeJSON(e.statePath, e.reg)
	if _, err := e.buildBatch(context.Background(), []request{{Harness: "pi", Action: "uninstall"}, {Harness: "pi", Action: "reset"}}); err == nil {
		t.Fatal("duplicate harness accepted")
	}
	b, err := e.buildBatch(context.Background(), []request{{Harness: "pi", Action: "uninstall"}})
	if err != nil {
		t.Fatal(err)
	}
	b.Plans[0].Request.Permanent = true
	if err = e.executeBatch(context.Background(), b, b.ID, nil); err == nil {
		t.Fatal("changed approved batch accepted")
	}
}
