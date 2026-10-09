package component

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ManifestPaths lists native plugin manifests recognized by Compatibility.
var ManifestPaths = []string{".claude-plugin/plugin.json", "gemini-extension.json", "package.json"}

// Compatibility returns harness identities asserted by files' native manifests.
// Unknown formats return no compatibility claim. Malformed package JSON is an error.
func Compatibility(files map[string][]byte) ([]string, error) {
	var ids []string
	for _, m := range []struct{ path, id string }{{ManifestPaths[0], "claude"}, {ManifestPaths[1], "gemini"}} {
		if _, exists := files[m.path]; exists {
			ids = append(ids, m.id)
		}
	}
	if data, exists := files[ManifestPaths[2]]; exists {
		var manifest map[string]json.RawMessage
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("invalid package manifest")
		}
		if _, exists := manifest["pi"]; exists {
			ids = append(ids, "pi")
		}
	}
	return ids, nil
}

// CheckCompatibility rejects id unless it satisfies both explicit builtFor and
// detected manifest identities. Unknown manifest formats rely on builtFor.
func CheckCompatibility(id string, builtFor, detected []string) error {
	if len(builtFor) > 0 && !slices.Contains(builtFor, id) {
		return fmt.Errorf("component is not compatible with selected harness %s", id)
	}
	if len(detected) > 0 && !slices.Contains(detected, id) {
		return fmt.Errorf("native extension is built for %s, not %s", strings.Join(detected, ", "), id)
	}
	return nil
}
