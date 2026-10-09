package catalog

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/zaRizk7/harness-ctl/internal/launch"
	"github.com/zaRizk7/harness-ctl/internal/nativeauth"
)

func TestUnchangedLegacyPolicyKeepsIndependentOverrides(t *testing.T) {
	specs := Defaults()
	current := specs[0]
	specs[0].LaunchArgs = launch.Policy{Rules: current.LaunchRules}.DefaultArgs()
	specs[0].ApprovalFlags = []string{"--sandbox", "-s", "--ask-for-approval", "-a", "--config", "-c", "--full-auto", "--yolo", "--dangerously-bypass-approvals-and-sandbox"}
	specs[0].LaunchRules = nil
	specs[0].ConfigFlags = nil
	// Metadata edits survive migration. Security policy edits remain explicit.
	specs[0].Name = "Personal name"
	before, _ := json.Marshal(specs)
	c := NormalizeLegacy(specs)
	want := launch.Policy{Rules: current.LaunchRules, ConfigFlags: current.ConfigFlags}.Args([]string{"-s", "read-only"}, false)
	got := launch.Policy{Rules: c[0].LaunchRules, ConfigFlags: c[0].ConfigFlags, LegacyArgs: c[0].LaunchArgs, LegacyFlags: c[0].ApprovalFlags}.Args([]string{"-s", "read-only"}, false)
	if !reflect.DeepEqual(got, want) || c[0].Name != "Personal name" {
		t.Fatal(got, c[0])
	}
	after, _ := json.Marshal(specs)
	if string(before) != string(after) {
		t.Fatal("normalization changed caller catalog")
	}
	for _, change := range []string{"args", "flags", "explicit", "unknown"} {
		clone := append([]Spec{}, specs...)
		switch change {
		case "args":
			clone[0].LaunchArgs = []string{"--custom"}
		case "flags":
			clone[0].ApprovalFlags = []string{"--custom"}
		case "explicit":
			clone[0].LaunchRules = []launch.Rule{{Args: []string{"--custom"}}}
		case "unknown":
			clone[0].ID = "custom"
		}
		if !reflect.DeepEqual(NormalizeLegacy(clone), clone) {
			t.Fatal("rewrote custom policy", change)
		}
	}
}

func TestLegacyNativeAuthDefaultsPreserveExplicitCustomization(t *testing.T) {
	specs := Defaults()
	want := specs[0].Auth
	specs[0].Auth = nativeauth.Spec{}
	before, _ := json.Marshal(specs)
	got := NormalizeLegacy(specs)
	if !reflect.DeepEqual(got[0].Auth, want) {
		t.Fatal("legacy native auth metadata was not supplied")
	}
	after, _ := json.Marshal(specs)
	if string(before) != string(after) {
		t.Fatal("normalization rewrote the user's catalog")
	}
	specs[0].Auth.Commands = map[string][]string{}
	if !reflect.DeepEqual(NormalizeLegacy(specs)[0].Auth, specs[0].Auth) {
		t.Fatal("explicit native auth disabling was overwritten")
	}
}
