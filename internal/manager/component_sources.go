package manager

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateComponentSources validates explicitly selected native-format roots.
// It never discovers project/system roots or claims their mutation ownership.
func (c config) validateComponentSources() error {
	for harness, sources := range c.ComponentSources {
		found := false
		for _, s := range c.Harnesses {
			if s.ID == harness {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown component source harness")
		}
		for name, root := range sources {
			if !targetPattern.MatchString(name) || !filepath.IsAbs(root) || filepath.Clean(root) == "/" || filepath.Clean(root) == c.Home {
				return fmt.Errorf("component sources need a named absolute native state directory")
			}
		}
	}
	return nil
}

// componentScopes returns mutable native scopes followed by explicit read-only
// source roots. Their names keep project/system ownership visible in the UI.
func (m tuiModel) componentScopes() []string {
	scopes := []string{"base"}
	if _, ok := m.e.reg.Profiles[m.componentInstallation().ID]; ok {
		scopes = append(scopes, "profile")
	}
	for _, name := range sortedKeys(m.e.cfg.ComponentSources[m.e.cfg.Harnesses[m.harness].ID]) {
		scopes = append(scopes, "source:"+name)
	}
	return scopes
}

// sourceScope identifies external inventory scopes, which cannot mutate assets.
func sourceScope(scope string) bool { return strings.HasPrefix(scope, "source:") }
