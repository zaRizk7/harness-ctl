// Package catalog owns configurable harness metadata and validates adapter contracts.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/launch"
	"github.com/zaRizk7/harness-ctl/internal/nativeauth"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// Spec defines one configured harness catalog entry. Native adapter code
// owns installation and state-format contracts selected by ID and Kind.
type Spec struct {
	Auth           nativeauth.Spec `json:"auth,omitempty"`
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Command        string          `json:"command"`
	Package        string          `json:"package"`
	LegacyPackages []string        `json:"legacy_packages"`
	BrewPackages   []string        `json:"brew_packages"`
	HomeEnv        string          `json:"home_env"`
	DefaultHome    string          `json:"default_home"`
	Kind           string          `json:"kind"`
	Docs           string          `json:"docs"`
	ConfigFiles    []string        `json:"config_files"`
	SharedClients  []string        `json:"shared_clients"`
	LaunchLabels   []string        `json:"launch_labels"`
	LaunchNote     string          `json:"launch_note,omitempty"`
	LaunchArgs     []string        `json:"launch_args,omitempty"`
	ApprovalFlags  []string        `json:"approval_flags,omitempty"`
	LaunchRules    []launch.Rule   `json:"launch_rules,omitempty"`
	ConfigFlags    []string        `json:"config_flags,omitempty"`
}

//go:embed catalog.json
var catalogData []byte

// Defaults returns fresh adapter defaults so callers can edit their catalog
// without changing another engine's inventory or ownership contracts.
func Defaults() []Spec {
	var specs []Spec
	if err := json.Unmarshal(catalogData, &specs); err != nil {
		panic(err)
	}
	return specs
}

// Validate rejects duplicate commands, unsafe paths and shell environment
// names before configuration can affect discovery, shims or lifecycle recipes.
func Validate(specs []Spec) error {
	if len(specs) == 0 {
		return fmt.Errorf("catalog must contain at least one harness")
	}
	ids, commands := map[string]bool{}, map[string]bool{}
	for _, s := range specs {
		if !identityPattern.MatchString(s.ID) || !identityPattern.MatchString(s.Command) || s.Name == "" || ids[s.ID] || commands[s.Command] {
			return fmt.Errorf("invalid or duplicate harness identity/command")
		}
		ids[s.ID], commands[s.Command] = true, true
		if err := nativeauth.Validate(s.Auth); err != nil {
			return err
		}
		if s.Kind != "npm" && s.Kind != "hermes" && s.Kind != "prime" {
			return fmt.Errorf("unsupported adapter kind for %s", s.ID)
		}
		if s.Kind == "npm" && (s.Package == "" || strings.HasPrefix(s.Package, "-") || strings.ContainsAny(s.Package, " \t\n")) {
			return fmt.Errorf("invalid npm package")
		}
		if s.HomeEnv != "" && !EnvironmentPattern.MatchString(s.HomeEnv) {
			return fmt.Errorf("invalid home environment name")
		}
		if filepath.IsAbs(s.DefaultHome) || s.DefaultHome == "" || s.DefaultHome == "." || strings.HasPrefix(filepath.Clean(s.DefaultHome), "..") {
			return fmt.Errorf("invalid relative state directory")
		}
		for _, f := range s.ConfigFiles {
			if filepath.Base(f) != f || f == "." || f == ".." {
				return fmt.Errorf("invalid configuration filename")
			}
		}
		if s.Docs != "" {
			u, err := url.Parse(s.Docs)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				return fmt.Errorf("documentation URL must use HTTPS")
			}
		}
	}
	return nil
}

// EnvironmentPattern accepts portable environment variable names before catalog
// metadata can select a native state root or generate shell assignments.
var EnvironmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
