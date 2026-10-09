package catalog

import "testing"

func TestEmbeddedContractCorruptionFailsClosed(t *testing.T) {
	old := catalogData
	defer func() { catalogData = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("corrupt bundled contract accepted")
		}
	}()
	catalogData = []byte("invalid-json")
	Defaults()
}

func TestContractValidationRejectsNativeTrustViolations(t *testing.T) {
	for _, change := range []func(*Spec){func(s *Spec) { s.Kind = "shell" }, func(s *Spec) { s.Package = "-bad" }, func(s *Spec) { s.Package = "bad package" }, func(s *Spec) { s.Docs = "http://example.com" }} {
		specs := Defaults()
		change(&specs[0])
		if Validate(specs) == nil {
			t.Fatal("unsafe contract accepted")
		}
	}
	if Validate(nil) == nil {
		t.Fatal("empty catalog accepted")
	}
}

func TestRejectsInvalidNativeAuthContract(t *testing.T) {
	specs := Defaults()
	specs[0].Auth.Commands["unverified"] = nil
	if Validate(specs) == nil {
		t.Fatal("unknown native auth operation")
	}
}
