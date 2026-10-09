package setup

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shellFixture(t *testing.T) Options {
	t.Helper()
	o := fixture(t)
	o.ShellFile = filepath.Join(o.Home, ".zshrc")
	o.BinDirs = []string{o.LinkDir, filepath.Join(o.Root, "bin")}
	return o
}

func TestExplicitShellSetupPreservesBytesModeAndRollback(t *testing.T) {
	for _, existing := range []bool{false, true} {
		o := shellFixture(t)
		if existing {
			if err := os.WriteFile(o.ShellFile, []byte("# user settings"), 0640); err != nil {
				t.Fatal(err)
			}
		}
		p, err := Build(o)
		if err != nil {
			t.Fatal(err)
		}
		if err = Apply(p, p.ID()); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(o.ShellFile)
		if err != nil || !strings.Contains(string(data), "export PATH=") {
			t.Fatal(string(data), err)
		}
		if existing {
			info, _ := os.Stat(o.ShellFile)
			if info.Mode().Perm() != 0640 || !strings.HasPrefix(string(data), "# user settings\n") {
				t.Fatal(string(data), info)
			}
		}
		o.Source = ""
		p, err = Build(o)
		if err != nil || len(p.Paths) != 0 {
			t.Fatal(p, err)
		}
	}
	o := shellFixture(t)
	if err := os.WriteFile(o.ShellFile, []byte("# original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	old := platform
	t.Cleanup(func() { platform = old })
	platform.symlink = func(string, string) error { return errors.New("synthetic symlink failure") }
	if Apply(p, p.ID()) == nil {
		t.Fatal("failure lost")
	}
	data, _ := os.ReadFile(o.ShellFile)
	if string(data) != "# original\n" {
		t.Fatal("shell rollback failed", string(data))
	}
}

func TestShellPreviewRejectsUnsafePathsChangesAndUnreadableSources(t *testing.T) {
	for _, scenario := range []string{"outside", "collision", "source", "empty-dirs", "bad-dir", "newline-dir", "directory", "linked", "oversize", "invalid-limit", "validate", "fingerprint", "open", "read", "close", "lstat", "changed"} {
		t.Run(scenario, func(t *testing.T) {
			o := shellFixture(t)
			o.Source = ""
			if err := os.WriteFile(o.ShellFile, []byte("# original\n"), 0600); err != nil {
				t.Fatal(err)
			}
			old := platform
			t.Cleanup(func() { platform = old })
			fault := errors.New("synthetic shell IO")
			switch scenario {
			case "outside":
				o.ShellFile = filepath.Dir(o.Home)
			case "collision":
				o.Files["shell"] = []byte("payload")
				o.ShellFile = filepath.Join(o.Root, "shell")
			case "source":
				o.Source = o.ShellFile
			case "empty-dirs":
				o.BinDirs = nil
			case "bad-dir":
				o.BinDirs = []string{filepath.Dir(o.Home)}
			case "newline-dir":
				o.BinDirs = []string{filepath.Join(o.Home, "bin\nunsafe")}
			case "directory":
				_ = os.Remove(o.ShellFile)
				_ = os.Mkdir(o.ShellFile, 0700)
			case "linked":
				_ = os.Remove(o.ShellFile)
				_ = os.Symlink(filepath.Join(o.Home, "foreign"), o.ShellFile)
			case "oversize":
				o.MaxBytes = 1
			case "invalid-limit":
				o.MaxBytes = 0
			case "validate":
				platform.validate = func(a, b string) error {
					if b == o.ShellFile {
						return fault
					}
					return old.validate(a, b)
				}
			case "fingerprint":
				platform.fingerprint = func(path string) (string, error) {
					if path == o.ShellFile {
						return "", fault
					}
					return old.fingerprint(path)
				}
			case "open":
				platform.open = func(path string) (*os.File, error) { return nil, fault }
			case "read":
				platform.read = func(io.Reader) ([]byte, error) { return nil, fault }
			case "close":
				platform.close = func(f *os.File) error { _ = f.Close(); return fault }
			case "lstat":
				platform.lstat = func(path string) (os.FileInfo, error) {
					if path == o.ShellFile {
						return nil, fault
					}
					return old.lstat(path)
				}
			case "changed":
				platform.read = func(r io.Reader) ([]byte, error) {
					data, err := old.read(r)
					_ = os.WriteFile(o.ShellFile, []byte("concurrent edit"), 0600)
					return data, err
				}
			}
			if _, err := Build(o); err == nil {
				t.Fatal("unsafe shell setup", scenario)
			}
		})
	}
}
