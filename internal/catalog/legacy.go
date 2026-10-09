package catalog

import (
	"slices"

	"github.com/zaRizk7/harness-ctl/internal/launch"
)

// NormalizeLegacy returns a catalog copy with unchanged shipped legacy launch
// policies expressed as independent rules. It preserves customized policies and
// metadata and never writes installed catalog files. Returned policies come from
// fresh default data, so normalization does not mutate the supplied specs.
func NormalizeLegacy(specs []Spec) []Spec {
	result := append([]Spec(nil), specs...)
	defaults := Defaults()
	for i, spec := range result {
		if spec.Auth.Commands == nil && spec.Auth.Notes == nil && spec.Auth.CredentialFiles == nil && spec.Auth.RequiredArgument == nil {
			for _, current := range defaults {
				if current.ID == spec.ID {
					result[i].Auth = current.Auth
				}
			}
		}
		if len(spec.LaunchRules) > 0 || len(spec.LaunchArgs) == 0 {
			continue
		}
		for _, current := range defaults {
			if spec.ID != current.ID || len(current.LaunchRules) == 0 || !slices.Equal(spec.LaunchArgs, (launch.Policy{Rules: current.LaunchRules}).DefaultArgs()) {
				continue
			}
			flags := append([]string{}, current.ConfigFlags...)
			for _, rule := range current.LaunchRules {
				flags = append(flags, rule.Flags...)
			}
			slices.Sort(flags)
			flags = slices.Compact(flags)
			legacy := append([]string{}, spec.ApprovalFlags...)
			slices.Sort(legacy)
			legacy = slices.Compact(legacy)
			if slices.Equal(flags, legacy) {
				result[i].LaunchRules = current.LaunchRules
				result[i].ConfigFlags = current.ConfigFlags
			}
		}
	}
	return result
}
