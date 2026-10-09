package component

import (
	"reflect"
	"testing"
)

func TestPointerMutationsRetainSiblings(t *testing.T) {
	node := map[string]any{"items": []any{map[string]any{"v": int64(1)}, "other"}, "keep": true}
	parts, err := PointerParts("/items/0/v")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := MutatePointer(node, parts, int64(2), false)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := PointerValue(updated, parts)
	if !ok || got != int64(2) || node["keep"] != true {
		t.Fatal(updated)
	}
	_, err = MutatePointer(node, []string{"items", "-"}, "last", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = MutatePointer(node, []string{"items", "1"}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(node["items"].([]any)[1], "last") {
		t.Fatal(node)
	}
	for _, parts := range [][]string{nil, {"items", "9"}, {"keep", "child"}, {"items", "invalid"}} {
		if _, err = MutatePointer(node, parts, nil, false); err == nil {
			t.Fatal(parts)
		}
	}
	if EscapePointer("a~/b") != "a~0~1b" {
		t.Fatal("escaping")
	}
}
func TestDecodeValueRejectsMalformedAndPreservesNumbers(t *testing.T) {
	for _, data := range []string{"bad", "{} {}", "{\"v\":1e999}"} {
		if _, err := DecodeValue([]byte(data)); err == nil {
			t.Fatal(data)
		}
	}
	v, err := DecodeValue([]byte(`{"values":[1,1.25],"keep":true}`))
	if err != nil {
		t.Fatal(err)
	}
	values := v.(map[string]any)["values"].([]any)
	if values[0] != int64(1) || values[1] != float64(1.25) {
		t.Fatal(values)
	}
}

func TestPointersHandleMissingMapsArraysAndNormalizationErrors(t *testing.T) {
	for _, p := range []string{"", "relative", "/"} {
		if _, err := PointerParts(p); err == nil {
			t.Fatal(p)
		}
	}
	parts, err := PointerParts("/a~1b/~0key")
	if err != nil || !reflect.DeepEqual(parts, []string{"a/b", "~key"}) {
		t.Fatal(parts, err)
	}
	for _, p := range [][]string{{"missing"}, {"array", "9"}, {"array", "bad"}, {"scalar", "v"}} {
		if _, ok := PointerValue(map[string]any{"array": []any{true}, "scalar": false}, p); ok {
			t.Fatal(p)
		}
	}
	node, err := MutatePointer(map[string]any{}, []string{"new", "entry"}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := PointerValue(node, []string{"new", "entry"}); !ok || v != true {
		t.Fatal(node)
	}
	for _, raw := range []string{`[1e999]`, `1e999`} {
		if _, err := DecodeValue([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestArrayValueReplacementPreservesOtherElements(t *testing.T) {
	node := []any{"old", "sibling"}
	updated, err := MutatePointer(node, []string{"0"}, "new", false)
	if err != nil || !reflect.DeepEqual(updated, []any{"new", "sibling"}) {
		t.Fatal(updated, err)
	}
}

func TestObjectRemovalPreservesSiblings(t *testing.T) {
	node := map[string]any{"remove": true, "keep": true}
	updated, err := MutatePointer(node, []string{"remove"}, nil, true)
	if err != nil || !reflect.DeepEqual(updated, map[string]any{"keep": true}) {
		t.Fatal(updated, err)
	}
}
