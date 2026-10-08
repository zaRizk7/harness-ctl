package manager

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

func validateOwnedPath(root, target string) error {
	root, target = filepath.Clean(root), filepath.Clean(target)
	if err := rejectLinkedAncestors(root); err != nil {
		return err
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(target) || root == string(filepath.Separator) {
		return errors.New("ownership requires an absolute, non-root directory")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path is outside its declared owner: %s", target)
	}
	// Resolve the owner, but reject links below it. System-level /var and /tmp
	// aliases are harmless. A linked state or installation child is not owned.
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("owner is a shared symlink: %s", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
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

func rejectLinkedAncestors(path string) error {
	// macOS exposes /tmp and /var through system aliases. Canonicalize those
	// aliases only, and reject every user-controlled linked ancestor.
	for p := filepath.Clean(path); p != "/" && p != "."; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
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

func within(root, target string) bool {
	root = canonicalSystemPath(root)
	target = canonicalSystemPath(target)
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalSystemPath(path string) string {
	path = filepath.Clean(path)
	for _, alias := range []string{"/var", "/tmp"} {
		if path == alias || strings.HasPrefix(path, alias+"/") {
			if info, err := os.Lstat(alias); err == nil && info.Mode()&os.ModeSymlink != 0 {
				if canonical, err := filepath.EvalSymlinks(alias); err == nil {
					rel, _ := filepath.Rel(alias, path)
					return filepath.Join(canonical, rel)
				}
			}
		}
	}
	return path
}

func fingerprint(path string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
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
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\n", rel, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			_, err = h.Write([]byte(target))
			return err
		}
		if info.Mode().IsRegular() {
			data, err := os.Open(p)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(h, data)
			closeErr := data.Close()
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

func atomicWrite(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".harness-ctl-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0600)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
