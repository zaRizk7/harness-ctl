package nativeauth

import (
	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"reflect"
	"testing"
)

func TestContractsAndNativeRouting(t *testing.T) {
	s := Spec{Commands: map[string][]string{"login": {"login"}, "status": {"login", "status"}, "manage": {}}}
	if err := Validate(s); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Spec{{Commands: map[string][]string{"unknown": {}}}, {Commands: map[string][]string{"login": {"bad\n"}}}} {
		if Validate(bad) == nil {
			t.Fatal("unsafe contract")
		}
	}
	args, err := Args(s, "login", []string{"--device-auth"})
	if err != nil || !reflect.DeepEqual(args, []string{"login", "--device-auth"}) {
		t.Fatal(args, err)
	}
	args[0] = "changed"
	if s.Commands["login"][0] != "login" {
		t.Fatal("aliased catalog")
	}
	if _, err := Args(s, "logout", nil); err == nil {
		t.Fatal("missing contract")
	}
	for _, extra := range []string{"bad\n", "--token=secret", "--api-key=secret", "--api-key"} {
		if _, err := Args(s, "login", []string{extra}); err == nil {
			t.Fatal("unsafe extra")
		}
	}
	op, extra := Match(s, []string{"login", "status", "--help"})
	if op != "status" || !reflect.DeepEqual(extra, []string{"--help"}) {
		t.Fatal(op, extra)
	}
	if op, _ := Match(s, []string{"hello"}); op != "" {
		t.Fatal("normal session routed")
	}
}

func TestNativeCredentialsRequireConfinedFileLocations(t *testing.T) {
	s := Spec{CredentialFiles: []credentials.Location{{Root: 0, Path: "../outside"}}}
	if Validate(s) == nil {
		t.Fatal("unsafe native credentials")
	}
}

func TestNativeProviderArgumentsRemainExplicit(t *testing.T) {
	s := Spec{Commands: map[string][]string{"login": {"auth", "add"}}, RequiredArgument: map[string]string{"login": "provider ID"}}
	if _, err := Args(s, "login", nil); err == nil {
		t.Fatal("native provider was guessed")
	}
	if _, err := Args(s, "login", []string{"openai-codex"}); err != nil {
		t.Fatal(err)
	}
}
