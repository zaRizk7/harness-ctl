// Package component owns structured component values and pointer mutations.
package component

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// EscapePointer returns value escaped as one JSON pointer segment.
func EscapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

// PointerParts returns decoded segments for pointer, rejecting an
// empty/non-absolute field pointer.
func PointerParts(pointer string) ([]string, error) {
	if !strings.HasPrefix(pointer, "/") || pointer == "/" {
		return nil, fmt.Errorf("a component field requires a non-empty JSON pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

// PointerValue returns the nested object/array value addressed by parts, with
// false for missing or invalid traversal.
func PointerValue(node any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return node, true
	}
	switch value := node.(type) {
	case map[string]any:
		next, exists := value[parts[0]]
		if exists {
			return PointerValue(next, parts[1:])
		}
	case []any:
		index, err := strconv.Atoi(parts[0])
		if err == nil && index >= 0 && index < len(value) {
			return PointerValue(value[index], parts[1:])
		}
	}
	return nil, false
}

// MutatePointer replaces the value at decoded parts with replacement, or removes
// it when remove is true. It mutates containers in place and returns the updated
// root because array removal/appending can change its backing slice. Empty parts,
// invalid array positions and traversal through scalars return errors.
func MutatePointer(node any, parts []string, replacement any, remove bool) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("cannot replace the complete configuration through a field edit")
	}
	key := parts[0]
	switch value := node.(type) {
	case map[string]any:
		if len(parts) == 1 {
			if remove {
				delete(value, key)
			} else {
				value[key] = replacement
			}
			return value, nil
		}
		next, exists := value[key]
		if !exists && !remove {
			next = map[string]any{}
		}
		updated, err := MutatePointer(next, parts[1:], replacement, remove)
		if err != nil {
			return nil, err
		}
		value[key] = updated
		return value, nil
	case []any:
		if key == "-" && len(parts) == 1 && !remove {
			return append(value, replacement), nil
		}
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(value) {
			return nil, fmt.Errorf("invalid component array index")
		}
		if len(parts) == 1 {
			if remove {
				return append(value[:index], value[index+1:]...), nil
			}
			value[index] = replacement
			return value, nil
		}
		updated, err := MutatePointer(value[index], parts[1:], replacement, remove)
		value[index] = updated
		return value, err
	}
	return nil, fmt.Errorf("component field crosses a non-container value")
}

// DecodeValue returns a single structured JSON component value from
// data. Invalid/trailing/unsupported values return errors.
func DecodeValue(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing component value")
	}
	var normalize func(any) (any, error)
	normalize = func(value any) (any, error) {
		switch v := value.(type) {
		case json.Number:
			if integer, err := v.Int64(); err == nil {
				return integer, nil
			}
			return v.Float64()
		case map[string]any:
			for key, child := range v {
				result, err := normalize(child)
				if err != nil {
					return nil, err
				}
				v[key] = result
			}
		case []any:
			for i, child := range v {
				result, err := normalize(child)
				if err != nil {
					return nil, err
				}
				v[i] = result
			}
		}
		return value, nil
	}
	return normalize(value)
}
