// Package library owns reusable component records and per-harness application recipes.
package library

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/zaRizk7/harness-ctl/internal/storage"
)

// platform binds capture IO so partial reads and concurrent growth are testable.
// Runtime configuration cannot replace these native operations.
var platform = struct {
	linked   func(string) error
	info     func(fs.DirEntry) (fs.FileInfo, error)
	readFile func(string) ([]byte, error)
	rel      func(string, string) (string, error)
}{storage.RejectLinkedAncestors, fs.DirEntry.Info, os.ReadFile, filepath.Rel}

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// Target describes a native registration or asset destination relative to the
// selected state root. HomeSkills selects a harness's documented HOME skills.
type Target struct {
	Path       string          `json:"path,omitempty"`
	Field      string          `json:"field,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Native     bool            `json:"native,omitempty"`
	Name       string          `json:"name,omitempty"`
	Source     string          `json:"source,omitempty"`
	HomeSkills bool            `json:"home_skills,omitempty"`
}

// Item stores one reusable component. Targets explicitly declare compatibility.
// Files are captured bytes, never shared symlinks. Source is consumed on import.
type Item struct {
	ID       string            `json:"id"`
	Category string            `json:"category"`
	Enabled  bool              `json:"enabled"`
	Targets  map[string]Target `json:"targets"`
	Files    map[string][]byte `json:"files,omitempty"`
	Source   string            `json:"source,omitempty"`
}

// Validate returns an error for unsafe identities/destinations or ambiguous asset
// recipes in item. It does not establish native plugin compatibility or mutate item.
func Validate(item Item) error {
	if !identityPattern.MatchString(item.ID) || len(item.Targets) == 0 {
		return fmt.Errorf("library requires an ID and compatible target recipes")
	}
	for id, target := range item.Targets {
		if !identityPattern.MatchString(id) {
			return fmt.Errorf("invalid library harness identity")
		}
		if !target.Native && !filepath.IsLocal(target.Path) {
			return fmt.Errorf("library destination must be relative to selected native state")
		}
		if target.Native && len(item.Files) > 0 {
			return fmt.Errorf("native recipes cannot contain local files")
		}
		if target.Field != "" && len(item.Files) > 0 {
			return fmt.Errorf("registration recipes cannot contain asset files")
		}
		if target.Field != "" && !json.Valid(target.Value) {
			return fmt.Errorf("library registration requires JSON")
		}
	}
	for name := range item.Files {
		if !filepath.IsLocal(name) || name == "." {
			return fmt.Errorf("library asset path escapes its root")
		}
	}
	return nil
}

// Capture returns item with its Source captured into independent Files under
// limit bytes, or an IO/validation error. Symlinks/nonregular files are rejected.
// A successful capture clears Source. Failure can return partially captured Files.
func Capture(item Item, limit int64) (Item, error) {
	if item.Source == "" {
		return item, Validate(item)
	}
	if len(item.Files) > 0 {
		return item, fmt.Errorf("choose source or captured files")
	}
	source := item.Source
	if !filepath.IsAbs(source) {
		return item, fmt.Errorf("library source must be absolute")
	}
	if err := platform.linked(source); err != nil {
		return item, err
	}
	item.Files = map[string][]byte{}
	var size int64
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := platform.info(entry)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("library assets must be regular files")
		}
		if info.Size() > limit-size {
			return fmt.Errorf("library assets exceed size limit")
		}
		data, err := platform.readFile(path)
		if err != nil {
			return err
		}
		size += int64(len(data))
		if size > limit {
			return fmt.Errorf("library assets exceed size limit")
		}
		name, err := platform.rel(source, path)
		if err != nil {
			return err
		}
		if name == "." {
			name = filepath.Base(source)
		}
		item.Files[name] = data
		return nil
	})
	if err != nil {
		return item, err
	}
	item.Source = ""
	return item, Validate(item)
}

// View contains inventory metadata without reusable configuration or asset contents.
type View struct {
	ID, Category string
	Enabled      bool
	Harnesses    []string
}
