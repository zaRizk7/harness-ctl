package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureOwnsBytesAndRejectsUnsafeAssets(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(source, "SKILL.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	item := Item{ID: "skill", Category: "skills", Enabled: true, Source: source, Targets: map[string]Target{"custom": {Path: "skills/demo"}}}
	captured, err := Capture(item, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if string(captured.Files["SKILL.md"]) != "original" || captured.Source != "" {
		t.Fatal(captured)
	}
	if _, err = Capture(item, 1); err == nil {
		t.Fatal("unbounded source")
	}
	if err = os.Symlink(path, filepath.Join(source, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err = Capture(item, 1024); err == nil {
		t.Fatal("linked asset")
	}
	item.Source = "relative"
	if _, err = Capture(item, 1024); err == nil {
		t.Fatal("relative source")
	}
	item.Source = filepath.Join(source, "missing")
	if _, err = Capture(item, 1024); err == nil {
		t.Fatal("missing source")
	}
	item.Source = path
	captured, err = Capture(item, 1024)
	if err != nil || string(captured.Files["SKILL.md"]) != "changed" {
		t.Fatal(captured, err)
	}
}
func TestValidateRejectsAmbiguousAndUnsafeRecipes(t *testing.T) {
	base := Item{ID: "demo", Category: "mcp", Targets: map[string]Target{"custom": {Path: "config.json", Field: "/mcp/demo", Value: json.RawMessage(`{}`)}}}
	for _, change := range []func(*Item){func(i *Item) { i.ID = "../bad" }, func(i *Item) { i.Targets = nil }, func(i *Item) { i.Targets = map[string]Target{"../id": {Path: "config"}} }, func(i *Item) { i.Targets = map[string]Target{"custom": {Path: "../config"}} }, func(i *Item) { i.Files = map[string][]byte{"../bad": nil} }, func(i *Item) { i.Files = map[string][]byte{"file": nil} }, func(i *Item) {
		i.Targets = map[string]Target{"custom": {Native: true}}
		i.Files = map[string][]byte{"file": nil}
	}, func(i *Item) {
		i.Targets = map[string]Target{"custom": {Path: "config", Field: "/mcp", Value: json.RawMessage("bad")}}
	}} {
		item := base
		change(&item)
		if Validate(item) == nil {
			t.Fatal(item)
		}
	}
	if err := Validate(base); err != nil {
		t.Fatal(err)
	}
}
