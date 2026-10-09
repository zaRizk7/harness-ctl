package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/library"
)

func TestLibraryApprovedFanoutAndStaleEntry(t *testing.T) {
	e, _ := testEngine(t)
	item := library.Item{ID: "demo", Category: "mcp", Enabled: true, Targets: map[string]library.Target{"codex": {Path: "config.toml", Field: "/mcp_servers/demo", Value: json.RawMessage(`{"command":"demo"}`)}, "claude": {Path: "settings.json", Field: "/mcpServers/demo", Value: json.RawMessage(`{"command":"demo"}`)}}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildLibraryApply(context.Background(), "demo", []string{"codex", "claude"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batch.Plans) != 2 {
		t.Fatal(p)
	}
	if err = e.executeLibraryApply(context.Background(), p, "wrong", nil); err == nil {
		t.Fatal("unapproved fanout")
	}
	item.Enabled = false
	if err = e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	if err = e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil); err == nil {
		t.Fatal("stale entry executed")
	}
	item.Enabled = true
	if err = e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	var owners []string
	for _, plan := range p.Batch.Plans {
		for _, r := range plan.Resources {
			for _, owner := range r.Owners {
				if !contains(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
		for _, owner := range plan.Spec.SharedClients {
			if !contains(owners, owner) {
				owners = append(owners, owner)
			}
		}
	}
	p, err = e.buildLibraryApply(context.Background(), "demo", []string{"codex", "claude"}, owners)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil); err != nil {
		t.Fatal(err)
	}
	for _, plan := range p.Batch.Plans {
		if _, err = os.Stat(plan.Request.Component.Path); err != nil {
			t.Fatal(err)
		}
	}
}
func TestLibraryAssetsAreCapturedAndCompatible(t *testing.T) {
	e, inst, _ := componentFixture(t)
	item := library.Item{ID: "skill", Category: "skills", Enabled: true, Files: map[string][]byte{"SKILL.md": []byte("original")}, Targets: map[string]library.Target{inst.Harness: {Path: "skills/demo"}}}
	if err := e.saveLibraryItem(item, ""); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildLibraryApply(context.Background(), item.ID, []string{inst.Harness}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.executeLibraryApply(context.Background(), p, p.Batch.ID, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(inst.StateRoot, "skills", "demo", "SKILL.md"))
	if err != nil || string(data) != "original" {
		t.Fatal(string(data), err)
	}
	if _, err = e.buildLibraryApply(context.Background(), item.ID, []string{"pi"}, nil); err == nil {
		t.Fatal("incompatible target accepted")
	}
}

func TestCapturedPiManifestRejectsOtherHarnesses(t *testing.T) {
	e, _ := testEngine(t)
	for _, id := range []string{"claude", "gemini", "pi"} {
		item := library.Item{ID: "pi-asset", Category: "plugins", Enabled: true, Files: map[string][]byte{"package.json": []byte(`{"name":"pi-asset","pi":{"extensions":["extension.ts"]}}`), "extension.ts": []byte("export default function() {}")}, Targets: map[string]library.Target{id: {Path: "extensions/pi-asset"}}}
		err := e.validateLibraryItem(item)
		if (err == nil) != (id == "pi") {
			t.Fatalf("%s: %v", id, err)
		}
		change := componentRequest{Category: plugins, Files: item.Files}
		err = validateCompatibility(id, change)
		if (err == nil) != (id == "pi") {
			t.Fatalf("component %s: %v", id, err)
		}
	}
}
