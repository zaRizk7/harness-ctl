package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComponentPlanningRejectsInvalidIdentitiesAndOperations(t *testing.T) {
	for _, scenario := range []string{"missing", "category", "operation", "subpath", "scope", "park-scope", "park-missing", "identity", "add-existing", "edit-missing", "disable-missing", "enable-active", "remove-missing", "bad-value", "bad-parked-value", "bad-file", "escape-file", "relative-source", "nested-source", "escape-child"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, path := componentFixture(t)
			change := componentRequest{Operation: "disable", Category: mcp, Path: path, Field: "/mcpServers/one"}
			switch scenario {
			case "category":
				change.Category = "invalid"
			case "operation":
				change.Operation = "invalid"
			case "subpath":
				change.Subpath = "child"
			case "scope":
				change.Scope = "invalid"
			case "park-scope":
				change.Parked = "/foreign"
			case "park-missing":
				change.Parked = filepath.Join(inst.StateRoot, disabledComponentsDir, string(mcp), "missing")
			case "identity":
				park := filepath.Join(inst.StateRoot, disabledComponentsDir, string(mcp), installID("component", path+"\x00"+change.Field))
				_ = writeJSON(filepath.Join(park, "meta.json"), parkedComponent{Path: "/foreign", Category: mcp})
			case "add-existing":
				change.Operation = "add"
				change.Value = json.RawMessage(`{}`)
			case "edit-missing", "disable-missing", "remove-missing":
				change.Field = "/mcpServers/absent"
				change.Operation = map[string]string{"edit-missing": "edit", "disable-missing": "disable", "remove-missing": "remove"}[scenario]
			case "enable-active":
				change.Operation = "enable"
			case "bad-value":
				change.Operation = "edit"
				change.Value = json.RawMessage("invalid")
			case "bad-parked-value":
				applyComponentRequest(t, e, inst, change)
				items, _ := e.components(inst, "base", mcp)
				for _, item := range items {
					if item.Parked != "" {
						change.Parked = item.Parked
					}
				}
				change.Operation = "edit"
				change.Value = json.RawMessage("invalid")
			case "bad-file", "escape-file", "relative-source", "nested-source":
				change = componentRequest{Operation: "install", Category: skills, Path: filepath.Join(inst.StateRoot, "skills", "new")}
				if scenario == "bad-file" {
					change.Files = map[string][]byte{"settings.json": []byte("invalid")}
				}
				if scenario == "escape-file" {
					change.Files = map[string][]byte{"../escape": []byte("x")}
				}
				if scenario == "relative-source" {
					change.Source = "relative"
				}
				if scenario == "nested-source" {
					change.Source = change.Path
				}
			case "escape-child":
				asset := filepath.Join(inst.StateRoot, "skills", "demo")
				_ = atomicWrite(filepath.Join(asset, "SKILL.md"), []byte("skill"), 0600)
				applyComponentRequest(t, e, inst, componentRequest{Operation: "disable", Category: skills, Path: asset})
				items, _ := e.components(inst, "base", skills)
				change = componentRequest{Operation: "edit", Category: skills, Path: asset, Parked: items[0].Parked, Subpath: "../escape"}
			}
			if scenario == "missing" {
				if err := e.planComponent(&plan{}); err == nil {
					t.Fatal("missing request accepted")
				}
				return
			}
			if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
				t.Fatal("invalid component plan accepted")
			}
		})
	}
}

func TestComponentAssetExistenceAndTextValidation(t *testing.T) {
	for _, op := range []string{"add", "edit", "disable", "enable", "remove"} {
		e, inst, _ := componentFixture(t)
		path := filepath.Join(inst.StateRoot, "skills", "demo.json")
		if op == "add" {
			_ = atomicWrite(path, []byte(`{}`), 0600)
		}
		change := componentRequest{Operation: op, Category: skills, Path: path, Text: `{}`}
		if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
			t.Fatal(op, "invalid existence state accepted")
		}
	}
	e, inst, _ := componentFixture(t)
	path := filepath.Join(inst.StateRoot, "skills", "demo.json")
	change := componentRequest{Operation: "add", Category: skills, Path: path, Text: "bad"}
	if _, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change}); err == nil {
		t.Fatal("invalid text accepted")
	}
	change.Text = `{}`
	applyComponentRequest(t, e, inst, change)
	change.Operation = "remove"
	applyComponentRequest(t, e, inst, change)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("asset remains", err)
	}
}

func TestComponentEditorReadsActiveAndParkedSources(t *testing.T) {
	e, inst, path := componentFixture(t)
	item := componentItem{Path: path, Field: "/mcpServers/one", Category: mcp}
	if data, err := e.componentEditData(inst, "base", item); err != nil || !json.Valid(data) {
		t.Fatal("active field unreadable", err)
	}
	for _, scenario := range []string{"scope", "field", "missing", "parked", "child"} {
		copy := item
		scope := "base"
		switch scenario {
		case "scope":
			scope = "invalid"
		case "field":
			copy.Path = filepath.Join(inst.StateRoot, "unknown.json")
		case "missing":
			copy.Field = "/mcpServers/missing"
		case "parked":
			copy.Parked = filepath.Join(inst.StateRoot, disabledComponentsDir, "missing")
		case "child":
			copy.Field = ""
			copy.Parked = filepath.Join(inst.StateRoot, disabledComponentsDir, "missing")
			copy.Subpath = "../escape"
		}
		if _, err := e.componentEditData(inst, scope, copy); err == nil {
			t.Fatal(scenario, "invalid edit source accepted")
		}
	}
	asset := filepath.Join(inst.StateRoot, "skills", "demo")
	_ = atomicWrite(filepath.Join(asset, "SKILL.md"), []byte("skill"), 0600)
	applyComponentRequest(t, e, inst, componentRequest{Operation: "disable", Category: skills, Path: asset})
	items, _ := e.components(inst, "base", skills)
	items[0].Subpath = "SKILL.md"
	if data, err := e.componentEditData(inst, "base", items[0]); err != nil || string(data) != "skill" {
		t.Fatal("parked child unreadable", err)
	}
}

func TestComponentScopeAndResourceContracts(t *testing.T) {
	e, inst, path := componentFixture(t)
	if _, _, err := e.componentEngine(installation{Harness: "missing"}, "base"); err == nil {
		t.Fatal("unknown harness accepted")
	}
	e.cfg.StateRoots = map[string]string{"pi": inst.StateRoot}
	if _, _, err := e.componentEngine(inst, "bad"); err == nil {
		t.Fatal("unknown scope accepted")
	}
	s, _ := e.specFor("pi")
	for _, change := range []componentRequest{{Path: "/foreign", Category: skills}, {Path: path, Field: "bad", Category: mcp}, {Path: path, Field: "/mcpServers/one", Category: skills}} {
		if _, err := e.componentResource(s, change.Path, change.Field, change.Category); err == nil {
			t.Fatal("foreign resource accepted", change)
		}
	}
	s.ConfigFiles = []string{"opaque.env"}
	if _, err := e.componentResource(s, filepath.Join(inst.StateRoot, "opaque.env"), "/hooks/one", hooks); err == nil {
		t.Fatal("opaque selective edit accepted")
	}
	claude, _ := e.specFor("claude")
	p := filepath.Join(e.cfg.Home, ".claude.json")
	_ = atomicWrite(p, []byte(`{"mcpServers":{"one":{}}}`), 0600)
	if _, err := e.componentResource(claude, p, "/mcpServers/one", mcp); err != nil {
		t.Fatal(err)
	}
}
