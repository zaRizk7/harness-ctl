package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedPathRejectsEscapesAndSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "owned")
	outside := filepath.Join(base, "other")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{base, outside, filepath.Join(root, "linked", "credentials"), root, "/"} {
		t.Run(target, func(t *testing.T) {
			if err := ValidateOwnedPath(root, target); err == nil {
				t.Fatalf("expected ownership rejection for %q", target)
			}
		})
	}
}

func TestOwnedPathAcceptsAbsentOwnedDescendant(t *testing.T) {
	root := t.TempDir()
	if err := ValidateOwnedPath(root, filepath.Join(root, "versions", "new")); err != nil {
		t.Fatal(err)
	}
}

func TestNativePublicationFailuresAreReturned(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "file")
	errFixture := errors.New("fixture filesystem failure")
	for _, boundary := range []string{"mkdir", "create", "chmod", "write", "sync", "close", "rename"} {
		t.Run(boundary, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			switch boundary {
			case "mkdir":
				platform.mkdirAll = func(string, os.FileMode) error { return errFixture }
			case "create":
				platform.createTemp = func(string, string) (*os.File, error) { return nil, errFixture }
			case "chmod":
				platform.chmod = func(*os.File, os.FileMode) error { return errFixture }
			case "write":
				platform.write = func(*os.File, []byte) (int, error) { return 0, errFixture }
			case "sync":
				platform.sync = func(*os.File) error { return errFixture }
			case "close":
				platform.close = func(f *os.File) error { _ = f.Close(); return errFixture }
			case "rename":
				platform.rename = func(string, string) error { return errFixture }
			}
			if err := AtomicWrite(path, []byte("private"), 0600); !errors.Is(err, errFixture) {
				t.Fatal(boundary, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("partial target published", err)
			}
		})
	}
}

func TestOwnershipNativeFailuresAndConcurrentLinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "owner")
	target := filepath.Join(root, "file")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	fail := errors.New("fixture path error")
	for _, scenario := range []string{"relative", "rel", "ancestor", "root-link", "root-error", "child-error", "child-link"} {
		t.Run(scenario, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			rootCalls := 0
			switch scenario {
			case "relative":
				if ValidateOwnedPath("relative", "relative/file") == nil {
					t.Fatal("relative owner")
				}
				return
			case "rel":
				platform.rel = func(string, string) (string, error) { return "", fail }
				if Within(root, target) {
					t.Fatal("failed relation accepted")
				}
			case "ancestor":
				platform.lstat = func(string) (os.FileInfo, error) { return nil, fail }
			default:
				platform.lstat = func(path string) (os.FileInfo, error) {
					if path == root {
						rootCalls++
						if rootCalls == 2 {
							if scenario == "root-link" {
								return linkInfo, nil
							}
							if scenario == "root-error" {
								return nil, fail
							}
						}
					}
					if path == target {
						if scenario == "child-error" {
							return nil, fail
						}
						if scenario == "child-link" {
							return linkInfo, nil
						}
					}
					return old.lstat(path)
				}
			}
			if ValidateOwnedPath(root, target) == nil {
				t.Fatal("unsafe/native failure accepted", scenario)
			}
		})
	}
}

type badEntry struct{ fs.DirEntry }

func (badEntry) Info() (fs.FileInfo, error) { return nil, errors.New("fixture entry error") }

func TestFingerprintErrorsAndSystemAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("fixture IO error")
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"walk", "info", "rel", "open", "copy", "close", "link"} {
		t.Run(scenario, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			switch scenario {
			case "walk":
				platform.walk = func(root string, walk fs.WalkDirFunc) error { return walk(root, nil, fail) }
			case "info":
				platform.walk = func(root string, walk fs.WalkDirFunc) error { return walk(root, badEntry{entries[0]}, nil) }
			case "rel":
				platform.rel = func(string, string) (string, error) { return "", fail }
			case "open":
				platform.open = func(string) (*os.File, error) { return nil, fail }
			case "copy":
				platform.open = func(name string) (*os.File, error) {
					f, err := os.Open(name)
					if err == nil {
						_ = f.Close()
					}
					return f, err
				}
			case "close":
				platform.close = func(f *os.File) error { _ = f.Close(); return fail }
			case "link":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing", path); err != nil {
					t.Fatal(err)
				}
				platform.readlink = func(string) (string, error) { return "", fail }
			}
			if _, err := Fingerprint(path); err == nil {
				t.Fatal("fingerprint failure ignored", scenario)
			}
		})
	}
	old := platform
	t.Cleanup(func() { platform = old })
	linkInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	platform.lstat = func(name string) (os.FileInfo, error) {
		if name == "/tmp" {
			return linkInfo, nil
		}
		return old.lstat(name)
	}
	platform.evalLinks = func(string) (string, error) { return "/private/tmp", nil }
	if got := CanonicalSystemPath("/tmp/demo"); got != "/private/tmp/demo" {
		t.Fatal(got)
	}
	platform.evalLinks = func(string) (string, error) { return "", fail }
	if got := CanonicalSystemPath("/tmp/demo"); got != "/tmp/demo" {
		t.Fatal(got)
	}
}

func TestJSONAndAbsentFingerprints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	before, err := Fingerprint(path)
	if err != nil || before == "" {
		t.Fatal(before, err)
	}
	value := map[string]string{"key": "value"}
	if err = WriteJSON(path, value); err != nil {
		t.Fatal(err)
	}
	after, err := Fingerprint(path)
	if err != nil || before == after {
		t.Fatal(after, err)
	}
	var read map[string]string
	if err = ReadJSON(path, &read); err != nil || read["key"] != "value" {
		t.Fatal(read, err)
	}
	if WriteJSON(path, func() {}) == nil {
		t.Fatal("marshal failure")
	}
	if ReadJSON(filepath.Join(path, "missing"), &read) == nil {
		t.Fatal("read failure")
	}
}

func TestLinkedAncestorAndLinkFingerprint(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "shared")
	if err := os.Symlink("missing", link); err != nil {
		t.Fatal(err)
	}
	if RejectLinkedAncestors(filepath.Join(link, "file")) == nil {
		t.Fatal("linked ancestor accepted")
	}
	if digest, err := Fingerprint(link); err != nil || digest == "" {
		t.Fatal(digest, err)
	}
}
