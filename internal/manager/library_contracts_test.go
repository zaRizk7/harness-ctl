package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/library"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

// reusableSkill returns an independent local recipe for synthetic managed state.
func reusableSkill(id string) library.Item {
	return library.Item{ID: id, Category: "skills", Enabled: true, Files: map[string][]byte{"SKILL.md": []byte("reusable")}, Targets: map[string]library.Target{"pi": {Path: "skills/" + id}}}
}

func TestLibraryCLIInputApprovalAndApplication(t *testing.T) {
	e, _ := testEngine(t)
	ctx := context.Background()
	var out strings.Builder
	for _, args := range [][]string{nil, {"list", "extra"}, {"set"}, {"set", "--unknown"}, {"apply", "entry"}, {"remove", "entry", "extra"}, {"unknown", "entry"}, {"set", "absent"}, {"apply", "entry", "pi"}} {
		if e.libraryCLI(ctx, args, strings.NewReader(""), &out) == nil {
			t.Fatal("invalid command accepted", args)
		}
	}
	item := reusableSkill("entry")
	path := filepath.Join(e.cfg.Home, "entry.json")
	if err := writeJSON(path, item); err != nil {
		t.Fatal(err)
	}
	if e.libraryCLI(ctx, []string{"set", path}, strings.NewReader("no\n"), &out) == nil {
		t.Fatal("missing approval accepted")
	}
	if err := e.libraryCLI(ctx, []string{"set", "--preview", path}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.cfg.Root); !os.IsNotExist(err) {
		t.Fatal("record preview wrote state", err)
	}
	if err := e.libraryCLI(ctx, []string{"set", "--yes", path}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"apply", "--preview", "--owners", "pi", "entry", "pi"}, {"apply", "entry", "pi"}, {"disable", "entry"}} {
		err := e.libraryCLI(ctx, args, strings.NewReader("no\n"), &out)
		if args[1] == "--preview" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("unapproved change executed", args)
		}
	}
	if err := e.libraryCLI(ctx, []string{"disable", "--preview", "entry"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if err := e.libraryCLI(ctx, []string{"apply", "--yes", "entry", "pi"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	s, _ := e.specFor("pi")
	if data, err := os.ReadFile(filepath.Join(e.stateRoot(s), "skills", "entry", "SKILL.md")); err != nil || string(data) != "reusable" {
		t.Fatal("application did not copy payload", err)
	}
	item.Source = "relative"
	item.Files = nil
	_ = writeJSON(path, item)
	if e.libraryCLI(ctx, []string{"set", "--yes", path}, strings.NewReader(""), &out) == nil {
		t.Fatal("unsafe capture accepted")
	}
	item = reusableSkill("entry")
	item.Category = "invalid"
	_ = writeJSON(path, item)
	if e.libraryCLI(ctx, []string{"set", "--yes", path}, strings.NewReader(""), &out) == nil {
		t.Fatal("invalid recipe accepted")
	}
	item = reusableSkill("entry")
	_ = writeJSON(path, item)
	if e.libraryCLI(ctx, []string{"set", "--yes", path}, strings.NewReader(""), brokenOutput{}) == nil {
		t.Fatal("unseen record preview approved")
	}
}

func TestLibraryVaultRejectsDuplicateInvalidAndStaleRecords(t *testing.T) {
	e, _ := testEngine(t)
	path, err := e.libraryPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]library.Item{{reusableSkill("a"), reusableSkill("a")}, {{ID: "unsafe"}}} {
		if err = vault.Write(path, e.cfg.MetadataBytes, e.identity, items); err != nil {
			t.Fatal(err)
		}
		if _, err = e.loadLibrary(); err == nil {
			t.Fatal("invalid vault accepted")
		}
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	item := reusableSkill("a")
	for _, change := range []func(*library.Item){func(i *library.Item) { i.Category = "invalid" }, func(i *library.Item) { i.Targets = map[string]library.Target{"missing": {Path: "skills/a"}} }, func(i *library.Item) {
		i.Targets = map[string]library.Target{"pi": {Path: "skills/a", HomeSkills: true}}
	}, func(i *library.Item) {
		i.Category = "plugins"
		i.Files = map[string][]byte{"gemini-extension.json": []byte(`{}`)}
	}} {
		bad := item
		change(&bad)
		if e.validateLibraryItem(bad) == nil {
			t.Fatal("invalid compatibility accepted")
		}
	}
	if err = e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	if e.changeLibraryItem("a", "unknown", "") == nil || e.changeLibraryItem("missing", "remove", "") == nil {
		t.Fatal("invalid change accepted")
	}
	if e.saveLibraryItem(item, "stale") == nil || e.changeLibraryItem("a", "disable", "stale") == nil {
		t.Fatal("stale record edited")
	}
	if err = e.changeLibraryItem("a", "disable", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil); err == nil {
		t.Fatal("disabled recipe applied")
	}
	if err = e.changeLibraryItem("a", "enable", ""); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Before = "altered"
	if e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil) == nil {
		t.Fatal("altered preview executed")
	}
	p.Digest = valueDigest([]any{p.Batch.ID, p.Batch.Digest, p.Path, p.Before})
	p.Path = filepath.Join(e.cfg.Root, "other.age")
	p.Digest = valueDigest([]any{p.Batch.ID, p.Batch.Digest, p.Path, p.Before})
	if e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil) == nil {
		t.Fatal("wrong vault applied")
	}
}

func TestLibraryStorageFailuresRemainReadOnly(t *testing.T) {
	for _, operation := range []string{"save", "change", "preview", "list"} {
		t.Run(operation, func(t *testing.T) {
			e, _ := testEngine(t)
			item := reusableSkill("a")
			if err := e.saveLibraryItem(item, ""); err != nil {
				t.Fatal(err)
			}
			path, _ := e.libraryPath()
			if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			var err error
			switch operation {
			case "save":
				err = e.saveLibraryItem(item, "")
			case "change":
				err = e.changeLibraryItem("a", "remove", "")
			case "preview":
				_, err = e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil)
			case "list":
				err = e.libraryCLI(context.Background(), []string{"list"}, strings.NewReader(""), &strings.Builder{})
			}
			after, _ := os.ReadFile(path)
			if err == nil || string(before) != string(after) {
				t.Fatal("corrupt vault changed", err)
			}
		})
	}
	e, _ := testEngine(t)
	item := reusableSkill("a")
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	unlock, err := e.lock()
	if err != nil {
		t.Fatal(err)
	}
	if e.saveLibraryItem(item, "") == nil || e.changeLibraryItem("a", "remove", "") == nil {
		t.Fatal("concurrent mutation accepted")
	}
	unlock()
	record := operationRecord{ID: randomID(), Status: "preparing"}
	if err = e.saveRecord(record); err != nil {
		t.Fatal(err)
	}
	if e.saveLibraryItem(item, "") == nil || e.changeLibraryItem("a", "remove", "") == nil {
		t.Fatal("pending transaction ignored")
	}
	// An injected inventory failure must stop compatible fan-out construction.
	old := readJSON
	t.Cleanup(func() { readJSON = old })
	readJSON = func(string, any) error { return errors.New("fixture read") }
	if _, err = e.buildLibraryApply(context.Background(), "a", []string{"pi"}, nil); err == nil {
		t.Fatal("inventory failure ignored")
	}
}

func TestLibraryRegistrationRecipeValidation(t *testing.T) {
	e, _ := testEngine(t)
	i := library.Item{ID: "a", Category: "mcp", Targets: map[string]library.Target{"pi": {Path: "settings.json", Field: "/mcpServers/a", Value: json.RawMessage(`{}`)}}}
	if err := e.validateLibraryItem(i); err != nil {
		t.Fatal(err)
	}
	i.Targets["pi"] = library.Target{Path: "settings.json", Field: "/mcpServers/a", Value: json.RawMessage("bad")}
	if e.validateLibraryItem(i) == nil {
		t.Fatal("invalid registration accepted")
	}
}
