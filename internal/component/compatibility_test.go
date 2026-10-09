package component

import "testing"

func TestManifestCompatibilityAppliesToCapturedAssets(t *testing.T) {
	for _, tc := range []struct {
		files  map[string][]byte
		id     string
		accept bool
	}{
		{map[string][]byte{"package.json": []byte(`{"pi":{}}`)}, "pi", true},
		{map[string][]byte{"package.json": []byte(`{"pi":{}}`)}, "claude", false},
		{map[string][]byte{"package.json": []byte(`{}`)}, "custom", true},
		{map[string][]byte{"gemini-extension.json": nil}, "gemini", true},
		{map[string][]byte{".claude-plugin/plugin.json": nil}, "pi", false},
	} {
		ids, err := Compatibility(tc.files)
		if err != nil {
			t.Fatal(err)
		}
		if (CheckCompatibility(tc.id, nil, ids) == nil) != tc.accept {
			t.Fatal(tc)
		}
	}
	if _, err := Compatibility(map[string][]byte{"package.json": []byte("bad")}); err == nil {
		t.Fatal("malformed manifest")
	}
	if CheckCompatibility("pi", []string{"claude"}, nil) == nil {
		t.Fatal("explicit compatibility bypassed")
	}
}
