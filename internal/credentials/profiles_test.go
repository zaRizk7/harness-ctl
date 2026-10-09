package credentials

import "testing"

func TestCredentialProfilesValidateCompatibilityAndPaths(t *testing.T) {
	allowed := []Location{{0, "auth.json"}, {1, "auth.json"}}
	if err := ValidateLocations(allowed); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]Location{{{-1, "auth.json"}}, {{0, "../outside"}}, {{0, "."}}, {{0, "auth.json"}, {0, "auth.json"}}} {
		if ValidateLocations(bad) == nil {
			t.Fatal("unsafe location")
		}
	}
	p := Profile{ID: "work", Harness: "native", Files: []File{{Location: allowed[0], Data: []byte("secret")}}}
	if err := Validate(p, "native", allowed); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Profile{{ID: "../bad", Harness: "native", Files: p.Files}, {ID: "work", Harness: "other", Files: p.Files}, {ID: "work", Harness: "native"}, {ID: "work", Harness: "native", Files: []File{{Location: Location{0, "unknown"}, Data: []byte("secret")}}}, {ID: "work", Harness: "native", Files: []File{{Location: allowed[0]}}}} {
		if Validate(bad, "native", allowed) == nil {
			t.Fatal("unsafe profile")
		}
	}
	views := Views([]Profile{p})
	if len(views) != 1 || views[0].FileCount != 1 || views[0].ID != "work" {
		t.Fatal(views)
	}
}
