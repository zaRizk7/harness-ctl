package component

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"github.com/zaRizk7/harness-ctl/internal/storage"
)

func localFixture(t *testing.T) (LocalInput, LocalIO) {
	t.Helper()
	root := t.TempDir()
	input := LocalInput{Harness: "example", Path: filepath.Join(root, "settings.json"), Root: root, StateRoot: root, ParkRoot: root, ParkedPath: filepath.Join(root, "disabled", "entry"), Format: "json", Request: Request{Category: stateconfig.MCP, Field: "/mcpServers/demo"}}
	readJSON := func(path string, value any) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, value)
	}
	io := LocalIO{Lstat: os.Lstat, ReadDir: os.ReadDir, ReadJSON: readJSON, ReadConfig: func(path, format string) (map[string]any, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return stateconfig.Decode(data, format)
	}, Encode: stateconfig.Encode, Marshal: json.Marshal, ValidatePath: storage.ValidateOwnedPath, ValidateTree: storage.RejectLinkedAncestors, Fingerprint: storage.Fingerprint, ContentDigest: storage.Fingerprint, Decode: DecodeValue, Mutate: MutatePointer}
	return input, io
}

func applyFixture(t *testing.T, result LocalResult) {
	t.Helper()
	for _, w := range result.Writes {
		var err error
		if w.Remove {
			err = os.RemoveAll(w.Path)
		} else {
			err = storage.AtomicWrite(w.Path, w.Data, fs.FileMode(w.Mode))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalFieldLifecyclePreservesSiblingsAndDisabledData(t *testing.T) {
	input, io := localFixture(t)
	if err := storage.AtomicWrite(input.Path, []byte(`{"untouched":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"add", "edit", "disable", "enable", "remove"} {
		input.Request.Operation = operation
		input.Request.Value = json.RawMessage(`{"command":"example"}`)
		result, err := PlanLocal(input, io)
		if err != nil {
			t.Fatal(operation, err)
		}
		applyFixture(t, result)
		value, err := io.ReadConfig(input.Path, input.Format)
		if err != nil {
			t.Fatal(err)
		}
		if value["untouched"] != true {
			t.Fatal("unrelated setting lost", operation, value)
		}
		_, exists := PointerValue(value, []string{"mcpServers", "demo"})
		want := operation != "disable" && operation != "remove"
		if exists != want {
			t.Fatal(operation, value)
		}
	}
}

func TestLocalArrayRestorationRetainsOriginalOrder(t *testing.T) {
	input, io := localFixture(t)
	input.Request.Category = stateconfig.Hooks
	input.Request.Field = "/hooks/0"
	input.Request.Operation = "disable"
	if err := storage.AtomicWrite(input.Path, []byte(`{"hooks":["first","second"],"untouched":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := PlanLocal(input, io)
	if err != nil {
		t.Fatal(err)
	}
	applyFixture(t, result)
	input.Request.Operation = "enable"
	result, err = PlanLocal(input, io)
	if err != nil {
		t.Fatal(err)
	}
	applyFixture(t, result)
	value, err := io.ReadConfig(input.Path, input.Format)
	if err != nil {
		t.Fatal(err)
	}
	values := value["hooks"].([]any)
	if len(values) != 2 || values[0] != "first" || values[1] != "second" {
		t.Fatal(value)
	}
}
