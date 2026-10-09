package manager

import (
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/component"
	"os"
	"path/filepath"
	"strings"
)

// assetHarnesses identifies incompatible native extension formats from local
// manifests. An unrecognized format returns no compatibility claim. Explicit
// built_for declarations can restrict imports of such assets.
func assetHarnesses(source string) ([]string, error) {
	if source == "" {
		return nil, nil
	}
	info, err := fileIO.stat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	files := map[string][]byte{}
	for _, name := range component.ManifestPaths[:2] {
		if _, err := fileIO.stat(filepath.Join(source, name)); err == nil {
			files[name] = nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	var manifest json.RawMessage
	err = readJSON(filepath.Join(source, "package.json"), &manifest)
	if err == nil {
		files["package.json"] = manifest
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("invalid package manifest")
	}
	return component.Compatibility(files)
}

// validateCompatibility combines explicit compatibility and detected native
// manifests, rejecting a source that was built for a different harness.
func validateCompatibility(id string, change componentRequest) error {
	var detected []string
	var err error
	if change.Category == plugins {
		if len(change.Files) > 0 {
			detected, err = component.Compatibility(change.Files)
		} else if filepath.IsAbs(change.Source) {
			detected, err = assetHarnesses(change.Source)
		}
	}
	if err != nil {
		return err
	}
	return component.CheckCompatibility(id, change.BuiltFor, detected)
}

// piPackageName resolves a safe npm source to its package and optional exact
// version. Git/local native side effects need a separate verified contract.
func piPackageName(source string) (string, string, error) {
	if !strings.HasPrefix(source, "npm:") {
		return "", "", fmt.Errorf("verified Pi native install/remove supports npm: sources. Import local extensions as Pi-specific assets")
	}
	spec := strings.TrimPrefix(source, "npm:")
	name, version := spec, ""
	if i := strings.LastIndex(spec, "@"); i > 0 {
		name, version = spec[:i], spec[i+1:]
		if !targetPattern.MatchString(version) {
			return "", "", fmt.Errorf("use an exact npm version")
		}
	}
	parts := strings.Split(name, "/")
	if len(parts) > 2 || len(parts) == 2 && !strings.HasPrefix(parts[0], "@") {
		return "", "", fmt.Errorf("invalid npm package source")
	}
	for i, part := range parts {
		if i == 0 && len(parts) == 2 {
			part = strings.TrimPrefix(part, "@")
		}
		if !targetPattern.MatchString(part) || part == "." || part == ".." {
			return "", "", fmt.Errorf("invalid npm package source")
		}
	}
	return name, version, nil
}

// verifyPiPackage checks registration and scoped payload manifest. Successful
// native exit alone does not prove compatibility, installation or removal.
func verifyPiPackage(root, source, operation string) error {
	name, version, err := piPackageName(source)
	if err != nil {
		return err
	}
	value, err := readConfig(filepath.Join(root, "settings.json"), "json")
	if err != nil {
		return err
	}
	entries, _ := value["packages"].([]any)
	found := false
	for _, entry := range entries {
		if piPackageSource(entry) == source {
			found = true
		}
	}
	if operation == "remove" {
		if found {
			return fmt.Errorf("native Pi package registration remains")
		}
		return nil
	}
	if !found {
		return fmt.Errorf("native Pi package registration is missing")
	}
	path := filepath.Join(root, "npm", "node_modules", filepath.FromSlash(name), "package.json")
	if err = validateOwnedPath(root, path); err != nil {
		return err
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Pi      any    `json:"pi"`
	}
	if readJSON(path, &manifest) != nil || manifest.Name != name || version != "" && manifest.Version != version {
		return fmt.Errorf("native Pi payload manifest does not match requested package")
	}
	if manifest.Pi == nil {
		valid := false
		for _, dir := range []string{"extensions", "skills", "prompts", "themes"} {
			if info, err := fileIO.stat(filepath.Join(filepath.Dir(path), dir)); err == nil && info.IsDir() {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("package has no Pi manifest or native resource directories")
		}
	}
	return nil
}

// piPackageSource extracts the source identity from Pi's string/object declaration.
func piPackageSource(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	if m, ok := value.(map[string]any); ok {
		s, _ := m["source"].(string)
		return s
	}
	return ""
}
