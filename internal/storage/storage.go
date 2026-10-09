// Package storage owns safe paths, content fingerprints and atomic local publication.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// platform binds native filesystem calls for deterministic failure testing. It
// stays private and is never changed by runtime configuration.
var platform = struct {
	lstat      func(string) (os.FileInfo, error)
	readlink   func(string) (string, error)
	evalLinks  func(string) (string, error)
	rel        func(string, string) (string, error)
	walk       func(string, fs.WalkDirFunc) error
	open       func(string) (*os.File, error)
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (*os.File, error)
	chmod      func(*os.File, os.FileMode) error
	write      func(*os.File, []byte) (int, error)
	sync       func(*os.File) error
	close      func(*os.File) error
	rename     func(string, string) error
}{os.Lstat, os.Readlink, filepath.EvalSymlinks, filepath.Rel, filepath.WalkDir, os.Open, os.MkdirAll, os.CreateTemp, (*os.File).Chmod, (*os.File).Write, (*os.File).Sync, (*os.File).Close, os.Rename}

// ValidateOwnedPath returns an error unless target is a strict child of
// absolute root with no user-controlled linked ancestors or descendants.
func ValidateOwnedPath(root, target string) error {
	root, target = filepath.Clean(root), filepath.Clean(target)
	if err := RejectLinkedAncestors(root); err != nil {
		return err
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(target) || root == string(filepath.Separator) {
		return errors.New("ownership requires an absolute, non-root directory")
	}
	rel, err := platform.rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path is outside its declared owner: %s", target)
	}
	// Resolve the owner, but reject links below it. System-level /var and /tmp
	// aliases are harmless. A linked state or installation child is not owned.
	if info, err := platform.lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("owner is a shared symlink: %s", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := platform.lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing a symlinked resource: %s", path)
		}
	}
	return nil
}

// RejectLinkedAncestors returns an error for linked user-controlled ancestors
// of path, while accepting canonical macOS system aliases.
func RejectLinkedAncestors(path string) error {
	// macOS exposes /tmp and /var through system aliases. Canonicalize those
	// aliases only, and reject every user-controlled linked ancestor.
	for p := filepath.Clean(path); p != "/" && p != "."; p = filepath.Dir(p) {
		info, err := platform.lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 && p != "/tmp" && p != "/var" {
			return fmt.Errorf("linked ownership ancestor: %s", p)
		}
	}
	return nil
}

// Within reports whether target is root or its descendant after canonicalizing
// only supported system aliases.
func Within(root, target string) bool {
	root = CanonicalSystemPath(root)
	target = CanonicalSystemPath(target)
	rel, err := platform.rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// CanonicalSystemPath returns path cleaned and normalized through macOS system
// aliases, without following user-controlled links.
func CanonicalSystemPath(path string) string {
	path = filepath.Clean(path)
	for _, alias := range []string{"/var", "/tmp"} {
		if path == alias || strings.HasPrefix(path, alias+"/") {
			if info, err := platform.lstat(alias); err == nil && info.Mode()&os.ModeSymlink != 0 {
				if canonical, err := platform.evalLinks(alias); err == nil {
					rel, _ := platform.rel(alias, path)
					return filepath.Join(canonical, rel)
				}
			}
		}
	}
	return path
}

// Fingerprint returns a recursive digest of path content, metadata and link
// identities. An absent root has a distinct Fingerprint, and IO failures return
// errors.
func Fingerprint(path string) (string, error) {
	h := sha256.New()
	err := platform.walk(path, func(p string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) && p == path {
			_, e := h.Write([]byte("absent"))
			return e
		}
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := platform.rel(path, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\n", rel, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := platform.readlink(p)
			if err != nil {
				return err
			}
			_, err = h.Write([]byte(target))
			return err
		}
		if info.Mode().IsRegular() {
			data, err := platform.open(p)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(h, data)
			closeErr := platform.close(data)
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// AtomicWrite publishes data at path with mode using a private synchronized
// temporary file and rename. Every write/sync/rename failure is returned.
func AtomicWrite(path string, data []byte, mode fs.FileMode) error {
	if err := platform.mkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := platform.createTemp(filepath.Dir(path), ".harness-ctl-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = platform.chmod(f, mode); err != nil {
		platform.close(f)
		return err
	}
	if _, err = platform.write(f, data); err != nil {
		platform.close(f)
		return err
	}
	if err = platform.sync(f); err != nil {
		platform.close(f)
		return err
	}
	if err = platform.close(f); err != nil {
		return err
	}
	return platform.rename(name, path)
}

// WriteJSON atomically writes value as indented private JSON to path. Encoding
// and publication errors are returned.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(data, '\n'), 0600)
}

// ReadJSON decodes path into value, returning the original read or JSON
// decoding error.
func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
