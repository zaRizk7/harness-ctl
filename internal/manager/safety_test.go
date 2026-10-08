package manager

import (
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
			if err := validateOwnedPath(root, target); err == nil {
				t.Fatalf("expected ownership rejection for %q", target)
			}
		})
	}
}

func TestOwnedPathAcceptsAbsentOwnedDescendant(t *testing.T) {
	root := t.TempDir()
	if err := validateOwnedPath(root, filepath.Join(root, "versions", "new")); err != nil {
		t.Fatal(err)
	}
}
