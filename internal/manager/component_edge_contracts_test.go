package manager

import (
	"context"
	"errors"
	"github.com/zaRizk7/harness-ctl/internal/component"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestParkedInventoryValidatesCategoryAndNativeAssetCompatibility(t *testing.T) {
	e, inst, _ := componentFixture(t)
	root := inst.StateRoot
	parked := filepath.Join(root, disabledComponentsDir, string(plugins), "demo")
	_ = writeJSON(filepath.Join(parked, "meta.json"), parkedComponent{Path: filepath.Join(root, "plugins/demo"), Category: skills})
	if _, err := e.components(inst, "base", plugins); err == nil {
		t.Fatal("parked category mismatch accepted")
	}
	_ = writeJSON(filepath.Join(parked, "meta.json"), parkedComponent{Path: filepath.Join(root, "plugins/demo"), Category: plugins})
	_ = writeJSON(filepath.Join(parked, "payload/package.json"), map[string]any{"pi": map[string]any{}})
	items, err := e.components(inst, "base", plugins)
	if err != nil || len(items) != 2 || !contains(items[0].BuiltFor, "pi") {
		t.Fatal(items, err)
	}
	old := fileIO
	defer func() { fileIO = old }()
	fileIO.stat = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, ".claude-plugin/plugin.json") {
			return nil, errors.New("asset manifest probe")
		}
		return old.stat(path)
	}
	if _, err = e.components(inst, "base", plugins); err == nil {
		t.Fatal("unreadable compatibility ignored")
	}
	fileIO = old
	if _, err = e.components(inst, "base", hooks); err != nil {
		t.Fatal("other parked categories blocked listing", err)
	}
	_ = atomicWrite(filepath.Join(root, "skill.md"), []byte("single skill"), 0600)
	if items, err = e.components(inst, "base", skills); err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	s, _ := e.specFor("claude")
	native := installation{Harness: s.ID}
	if _, err = e.components(native, "base", plugins); err != nil {
		t.Fatal(err)
	}
}

func TestArrayRestoreAndRemovalRetainSiblingIdentity(t *testing.T) {
	for _, scenario := range []string{"position", "decode", "mutate-enable", "mutate-remove", "remove-retained", "rebase-read", "indices-read", "indices-validate"} {
		t.Run(scenario, func(t *testing.T) {
			e, inst, path := componentFixture(t)
			_ = writeJSON(path, map[string]any{"hooks": []any{map[string]any{"name": "one"}, map[string]any{"name": "two"}, map[string]any{"name": "three"}}})
			applyComponentRequest(t, e, inst, componentRequest{Category: hooks, Operation: "disable", Path: path, Field: "/hooks/1"})
			items, err := e.components(inst, "base", hooks)
			if err != nil {
				t.Fatal(err)
			}
			var item componentItem
			for _, candidate := range items {
				if candidate.Parked != "" {
					item = candidate
				}
			}
			change := componentRequest{Category: hooks, Operation: "enable", Path: path, Field: item.Field, Parked: item.Parked}
			oldMutate, oldDecode, oldRead, oldValidate := mutatePointer, decodeComponentValue, readJSON, validateOwnedPath
			defer func() {
				mutatePointer, decodeComponentValue, readJSON, validateOwnedPath = oldMutate, oldDecode, oldRead, oldValidate
			}()
			fault := errors.New("array boundary")
			switch scenario {
			case "position":
				_ = writeJSON(path, map[string]any{"hooks": []any{}})
			case "decode":
				decodeComponentValue = func([]byte) (any, error) { return nil, fault }
			case "mutate-enable":
				mutatePointer = func(any, []string, any, bool) (any, error) { return nil, fault }
			case "mutate-remove":
				change.Operation = "remove"
				change.Field = "/hooks/0"
				change.Parked = ""
				mutatePointer = func(any, []string, any, bool) (any, error) { return nil, fault }
			case "remove-retained":
				change.Operation = "remove"
				change.Parked = "" // Remove the active entry while its original slot also has parking metadata.
			case "rebase-read", "indices-read":
				change.Operation = "remove"
				change.Field = "/hooks/0"
				change.Parked = ""
				readJSON = func(path string, value any) error {
					if strings.Contains(path, item.Parked) {
						return fault
					}
					return oldRead(path, value)
				}
			case "indices-validate":
				change.Operation = "remove"
				change.Field = "/hooks/0"
				change.Parked = ""
				validateOwnedPath = func(root, path string) error {
					if strings.Contains(path, item.Parked) {
						return fault
					}
					return oldValidate(root, path)
				}
			}
			p, err := e.buildPlan(context.Background(), request{Harness: inst.Harness, InstallID: inst.ID, Action: "manage", Component: &change})
			if scenario == "remove-retained" {
				if err != nil {
					t.Fatal(err)
				}
				if err = e.execute(context.Background(), p, p.ID, nil); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid array transition accepted")
			}
		})
	}
}

func TestRelocationRebaseAndDigestReturnFileFailures(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	park := filepath.Join(root, disabledComponentsDir, string(skills), "a")
	file := filepath.Join(park, "meta.json")
	_ = writeJSON(file, parkedComponent{Path: filepath.Join(root, "skills/a"), Category: skills})
	_ = atomicWrite(filepath.Join(root, "unrelated/meta.json"), []byte("unrelated"), 0600)
	oldIO, oldRead, oldValidate := fileIO, readJSON, validateOwnedPath
	defer func() { fileIO, readJSON, validateOwnedPath = oldIO, oldRead, oldValidate }()
	fault := errors.New("component file boundary")
	for _, boundary := range []string{"walk", "read", "relative"} {
		switch boundary {
		case "walk":
			fileIO.walk = func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, fault) }
		case "read":
			readJSON = func(string, any) error { return fault }
		case "relative":
			fileIO.rel = func(string, string) (string, error) { return "", fault }
		}
		if err := relocateParkedComponents(root, target, root); err == nil {
			t.Fatal(boundary)
		}
		fileIO, readJSON = oldIO, oldRead
	}
	p := &plan{StateRoot: root, Component: &componentMutation{}}
	if err := component.RebaseDisabledArray(&component.LocalResult{}, componentIO(), p.StateRoot, filepath.Join(root, "missing"), "", nil, 0, ""); err != nil {
		t.Fatal(err)
	}
	_ = writeJSON(file, parkedComponent{Path: "different", Field: "/hooks/2"})
	if err := component.RebaseDisabledArray(&component.LocalResult{}, componentIO(), p.StateRoot, filepath.Dir(park), "selected", []string{"hooks"}, 0, ""); err != nil {
		t.Fatal(err)
	}
	for _, boundary := range []string{"validate", "read"} {
		if boundary == "validate" {
			validateOwnedPath = func(string, string) error { return fault }
		} else {
			readJSON = func(string, any) error { return fault }
		}
		if err := component.RebaseDisabledArray(&component.LocalResult{}, componentIO(), p.StateRoot, filepath.Dir(park), "", nil, 0, ""); err == nil {
			t.Fatal(boundary)
		}
		if _, err := component.DisabledArrayIndices(componentIO(), filepath.Dir(park), "", nil, ""); err == nil {
			t.Fatal(boundary)
		}
		readJSON, validateOwnedPath = oldRead, oldValidate
	}
	for _, boundary := range []string{"walk", "info", "read"} {
		switch boundary {
		case "walk":
			fileIO.walk = func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, fault) }
		case "info":
			fileIO.info = func(fs.DirEntry) (fs.FileInfo, error) { return nil, fault }
		case "read":
			fileIO.open = func(path string) (*os.File, error) {
				f, err := oldIO.open(path)
				if err == nil {
					_ = f.Close()
				}
				return f, err
			}
		}
		if _, err := componentContentDigest(file); err == nil {
			t.Fatal(boundary)
		}
		fileIO = oldIO
	}
	fifo := filepath.Join(root, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateComponentTree(fifo); err == nil {
		t.Fatal("special component payload accepted")
	}
}

func TestComponentWriteVerificationAndCancellationAfterPublication(t *testing.T) {
	e, _ := testEngine(t)
	root := filepath.Join(e.cfg.Home, "assets")
	path := filepath.Join(root, "skill.md")
	p := &plan{Component: &componentMutation{Writes: []componentWrite{{Root: root, Path: path, Data: []byte("approved"), Mode: 0600}}}}
	old := atomicWrite
	defer func() { atomicWrite = old }()
	atomicWrite = func(path string, data []byte, mode fs.FileMode) error { return old(path, []byte("changed"), mode) }
	if err := e.applyComponent(context.Background(), p); err == nil {
		t.Fatal("changed component write verified")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	atomicWrite = func(path string, data []byte, mode fs.FileMode) error {
		err := old(path, data, mode)
		cancel()
		return err
	}
	if err := e.applyComponent(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
