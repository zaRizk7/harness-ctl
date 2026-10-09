package manager

import (
	"context"
	"errors"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/component"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestSerializationFailureStopsComponentAndArchivePublication(t *testing.T) {
	for _, scenario := range []string{"field-disable", "field-parked-edit", "field-parked-enable", "array-parked-remove", "asset-disable", "snapshot"} {
		for _, boundary := range []string{"marshal", "encode"} {
			t.Run(scenario+"/"+boundary, func(t *testing.T) {
				fixture := func(t *testing.T) (*engine, func() error, string) {
					if scenario == "snapshot" {
						e, p, marker := archiveFaultFixture(t)
						return e, func() error { _, err := e.snapshot(context.Background(), p); return err }, marker
					}
					return componentFaultFixture(t, scenario)
				}
				inject := func(n int, count *int) func() {
					oldMarshal, oldEncode := marshalJSON, encodeConfig
					fault := errors.New("serialization boundary")
					if boundary == "marshal" {
						marshalJSON = func(value any) ([]byte, error) {
							*count++
							if n > 0 && *count == n {
								return nil, fault
							}
							return oldMarshal(value)
						}
					} else {
						encodeConfig = func(format string, value map[string]any) ([]byte, error) {
							*count++
							if n > 0 && *count == n {
								return nil, fault
							}
							return oldEncode(format, value)
						}
					}
					return func() { marshalJSON, encodeConfig = oldMarshal, oldEncode }
				}
				_, operation, _ := fixture(t)
				count := 0
				restore := inject(0, &count)
				err := operation()
				restore()
				if err != nil {
					t.Fatal(err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, operation, _ := fixture(t)
						count := 0
						restore := inject(nth, &count)
						_ = operation()
						restore()
						if count < nth {
							t.Fatal("serialization boundary missed")
						}
						if err := e.refreshRegistry(); err != nil {
							t.Fatal("invalid registry published", err)
						}
					})
				}
			})
		}
	}
}

func TestRequestEncodingFailuresDoNotStartEditorsOrSetup(t *testing.T) {
	m := componentModel(t)
	t.Setenv("TMPDIR", t.TempDir())
	old := marshalJSONIndent
	defer func() { marshalJSONIndent = old }()
	fault := errors.New("request serialization")
	marshalJSONIndent = func(any, string, string) ([]byte, error) { return nil, fault }
	if msg := m.editAccount("")().(accountEditMsg); !errors.Is(msg.err, fault) {
		t.Fatal(msg.err)
	}
	if msg := m.editLibrary("")().(libraryEditMsg); !errors.Is(msg.err, fault) {
		t.Fatal(msg.err)
	}
	item := componentItem{Category: mcp, Path: filepath.Join(m.componentInstallation().StateRoot, "settings.json"), Field: "/mcpServers/one"}
	if msg := m.editComponent(&item)().(componentEditorMsg); !errors.Is(msg.err, fault) {
		t.Fatal(msg.err)
	}
	for nth := 1; nth <= 2; nth++ {
		count := 0
		marshalJSONIndent = func(value any, prefix, indent string) ([]byte, error) {
			count++
			if count == nth {
				return nil, fault
			}
			return old(value, prefix, indent)
		}
		if err := m.e.setupCLI(context.Background(), []string{"--yes"}, strings.NewReader(""), io.Discard); !errors.Is(err, fault) {
			t.Fatal(err)
		}
	}
}

func TestArrayRebaseAndSelectiveStateEncodingFailures(t *testing.T) {
	e, inst, path := componentFixture(t)
	root := filepath.Join(inst.StateRoot, disabledComponentsDir, string(hooks))
	_ = writeJSON(filepath.Join(root, "later", "meta.json"), parkedComponent{Path: path, Field: "/hooks/2", Category: hooks, Value: []byte(`{}`)})
	p := &plan{StateRoot: inst.StateRoot, Component: &componentMutation{}}
	old := marshalJSON
	defer func() { marshalJSON = old }()
	fault := errors.New("rebase serialization")
	marshalJSON = func(any) ([]byte, error) { return nil, fault }
	if err := component.RebaseDisabledArray(&component.LocalResult{}, componentIO(), p.StateRoot, root, path, []string{"hooks"}, 0, ""); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	oldEncode := encodeConfig
	defer func() { encodeConfig = oldEncode }()
	encodeConfig = func(string, map[string]any) ([]byte, error) { return nil, fault }
	r := resource{Path: path, Root: inst.StateRoot, Format: "json", Fields: map[string]category{"/mcpServers": mcp, "/theme": settings}}
	if err := applyState(r, request{Preserve: map[category]bool{settings: true}}); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	_ = e
}
