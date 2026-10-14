package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeDefaultAuthDoesNotChangeCredentialNamespace(t *testing.T) {
	e, _ := testEngine(t)
	t.Setenv("HOME", e.cfg.Home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	s, _ := e.specFor("claude")
	inst := installation{ID: "native-default", Harness: s.ID, Path: filepath.Join(e.cfg.Home, ".local/bin", s.Command)}
	root := e.stateRoot(s)
	env := e.launchEnvironment(inst, root)
	if _, forced := env[s.HomeEnv]; forced {
		t.Fatal("default native auth was moved to an explicit credential namespace")
	}
	// A native override already inherited from the caller remains byte-for-byte
	// intact, rather than being reconstructed and potentially changing its scope.
	t.Setenv(s.HomeEnv, root+string(os.PathSeparator))
	if _, forced := e.launchEnvironment(inst, e.stateRoot(s))[s.HomeEnv]; forced {
		t.Fatal("inherited native auth scope was rewritten")
	}
}

func TestExplicitNativeScopesStillBindTheSelectedState(t *testing.T) {
	for _, change := range []string{"other-user-home", "managed", "installation-root", "profile", "configured-root", "different-root", "custom-catalog-directory", "custom-catalog-environment", "other-harness"} {
		t.Run(change, func(t *testing.T) {
			e, _ := testEngine(t)
			t.Setenv("HOME", e.cfg.Home)
			s, _ := e.specFor("claude")
			inst := installation{ID: "explicit", Harness: s.ID}
			root := e.stateRoot(s)
			switch change {
			case "other-user-home":
				t.Setenv("HOME", filepath.Join(e.cfg.Home, "other"))
			case "managed":
				inst.Managed = true
			case "installation-root":
				inst.StateRoot = root
			case "profile":
				e.reg.Profiles[inst.ID] = profile{Root: root}
			case "configured-root":
				e.cfg.StateRoots[s.ID] = root
			case "different-root":
				root = filepath.Join(e.cfg.Root, "states", s.ID)
			case "custom-catalog-directory":
				s.DefaultHome = ".custom-native-state"
				for i := range e.cfg.Harnesses {
					if e.cfg.Harnesses[i].ID == s.ID {
						e.cfg.Harnesses[i] = s
					}
				}
				root = e.stateRoot(s)
			case "custom-catalog-environment":
				s.HomeEnv = "CUSTOM_NATIVE_HOME"
				for i := range e.cfg.Harnesses {
					if e.cfg.Harnesses[i].ID == s.ID {
						e.cfg.Harnesses[i] = s
					}
				}
			case "other-harness":
				s, _ = e.specFor("codex")
				inst.Harness = s.ID
				root = e.stateRoot(s)
			}
			if env := e.launchEnvironment(inst, root); env[s.HomeEnv] != root {
				t.Fatal("explicit native state selection was lost")
			}
		})
	}
}

func TestUnknownNativeDefaultCannotImplicitlySelectAuth(t *testing.T) {
	e, _ := testEngine(t)
	t.Setenv("HOME", e.cfg.Home)
	s := harnessSpec{ID: "custom", HomeEnv: "CUSTOM_AUTH_HOME", DefaultHome: ".custom"}
	if e.inheritsNativeHome(s, installation{}, filepath.Join(e.cfg.Home, s.DefaultHome)) {
		t.Fatal("unknown native credential namespace inferred")
	}
}
