package manager

import (
	"path/filepath"
	"testing"
)

func TestMarketplacePayloadNameIsVerified(t *testing.T) {
	_, _, inst := nativeComponentFixture(t, "claude")
	cache := filepath.Join(inst.StateRoot, "plugins", "marketplaces", "demo")
	if err := writeJSON(filepath.Join(cache, ".claude-plugin", "marketplace.json"), map[string]string{"name": "different"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(inst.StateRoot, "plugins", "known_marketplaces.json"), map[string]marketplaceRecord{"demo": {InstallLocation: cache}}); err != nil {
		t.Fatal(err)
	}
	p := &plan{StateRoot: inst.StateRoot, Component: &componentMutation{Request: componentRequest{Name: "demo", Operation: "add"}}}
	if verifyMarketplace(p) == nil {
		t.Fatal("unrelated marketplace payload accepted as verified")
	}
}
