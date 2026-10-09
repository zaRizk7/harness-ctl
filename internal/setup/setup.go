// Package setup installs a verified local executable and initializes user-owned configuration.
package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/launch"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zaRizk7/harness-ctl/internal/storage"
)

// platform binds setup IO for deterministic failure and cleanup verification.
// Runtime configuration cannot replace these native operations.
var platform = struct {
	validate    func(string, string) error
	linked      func(string) error
	lstat       func(string) (os.FileInfo, error)
	open        func(string) (*os.File, error)
	stat        func(*os.File) (os.FileInfo, error)
	read        func(io.Reader) ([]byte, error)
	close       func(*os.File) error
	fingerprint func(string) (string, error)
	atomic      func(string, []byte, os.FileMode) error
	mkdir       func(string, os.FileMode) error
	symlink     func(string, string) error
	remove      func(string) error
}{storage.ValidateOwnedPath, storage.RejectLinkedAncestors, os.Lstat, os.Open, (*os.File).Stat, io.ReadAll, (*os.File).Close, storage.Fingerprint, storage.AtomicWrite, os.MkdirAll, os.Symlink, os.Remove}

// Options selects private installation paths. LinkDir is optional. SHA256 must
// match Source before any executable is published. Files seeds absent metadata.
type Options struct {
	Home, Root, Prefix, LinkDir, Source, SHA256 string
	MaxBytes                                    int64
	Files                                       map[string][]byte
	ShellFile                                   string
	BinDirs                                     []string
}

// Plan contains exact destinations and their fingerprints for explicit approval.
// Payloads are private and bound to the digest, preventing changed source reuse.
type Plan struct {
	Paths        []string
	Link, Binary string
	before       map[string]string
	data         map[string][]byte
	digest       string
	previous     map[string][]byte
	modes        map[string]os.FileMode
}

// Build returns a read-only installation/PATH plan for o, or an ownership,
// checksum, configuration or IO error. Existing configuration is preserved.
// Existing binaries/links are never replaced. ShellFile changes require approval.
func Build(o Options) (*Plan, error) {
	if err := platform.validate(o.Home, o.Root); err != nil {
		return nil, err
	}
	p := &Plan{before: map[string]string{}, data: map[string][]byte{}, previous: map[string][]byte{}, modes: map[string]os.FileMode{}}
	for name, data := range o.Files {
		path := filepath.Join(o.Root, name)
		if filepath.Base(name) != name {
			return nil, fmt.Errorf("setup metadata must use a direct filename")
		}
		if err := platform.validate(o.Root, path); err != nil {
			return nil, err
		}
		if _, err := platform.lstat(path); os.IsNotExist(err) {
			p.data[path] = data
		} else if err != nil {
			return nil, err
		}
	}
	if o.Source != "" {
		if err := platform.validate(o.Home, o.Prefix); err != nil {
			return nil, err
		}
		if err := platform.linked(o.Source); err != nil {
			return nil, err
		}
		f, err := platform.open(o.Source)
		if err != nil {
			return nil, err
		}
		info, err := platform.stat(f)
		if err != nil {
			platform.close(f)
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&0111 == 0 || o.MaxBytes < 1 {
			platform.close(f)
			return nil, fmt.Errorf("source must be a regular executable with a positive size limit")
		}
		data, err := platform.read(io.LimitReader(f, o.MaxBytes+1))
		closeErr := platform.close(f)
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		hash := sha256.Sum256(data)
		if int64(len(data)) > o.MaxBytes || len(o.SHA256) != 64 || hex.EncodeToString(hash[:]) != o.SHA256 {
			return nil, fmt.Errorf("source size or SHA-256 verification failed")
		}
		p.Binary = filepath.Join(o.Prefix, "harness-ctl")
		if err = platform.validate(o.Prefix, p.Binary); err != nil {
			return nil, err
		}
		if _, err = platform.lstat(p.Binary); !os.IsNotExist(err) {
			return nil, fmt.Errorf("binary destination already exists or is unreadable")
		}
		p.data[p.Binary] = data
		if o.LinkDir != "" {
			if err = platform.validate(o.Home, o.LinkDir); err != nil {
				return nil, err
			}
			p.Link = filepath.Join(o.LinkDir, "harness-ctl")
			if err = platform.validate(o.LinkDir, p.Link); err != nil {
				return nil, err
			}
			if _, err = platform.lstat(p.Link); !os.IsNotExist(err) {
				return nil, fmt.Errorf("launcher destination already exists or is unreadable")
			}
			p.Paths = append(p.Paths, p.Link)
		}
	}
	if p.Binary != "" {
		receipt := filepath.Join(o.Root, "installation.json")
		if err := platform.validate(o.Root, receipt); err != nil {
			return nil, err
		}
		if _, err := platform.lstat(receipt); !os.IsNotExist(err) {
			return nil, fmt.Errorf("installation receipt already exists or is unreadable")
		}
		// String-only receipt fields always encode as JSON.
		data, _ := json.Marshal(struct{ Binary, Link string }{p.Binary, p.Link})
		p.data[receipt] = data
	}
	if o.ShellFile != "" {
		if err := p.shellPath(o); err != nil {
			return nil, err
		}
	}
	for path := range p.data {
		p.Paths = append(p.Paths, path)
	}
	sort.Strings(p.Paths)
	for _, path := range p.Paths {
		before, err := platform.fingerprint(path)
		if err != nil {
			return nil, err
		}
		if initial, exists := p.before[path]; exists && before != initial {
			return nil, fmt.Errorf("shell file changed while preparing setup")
		}
		p.before[path] = before
	}
	p.digest = p.hash()
	return p, nil
}

// hash binds destinations, their fingerprints and data to approval.
func (p *Plan) hash() string {
	h := sha256.New()
	fmt.Fprintf(h, "%q %q\n", p.Binary, p.Link)
	for _, path := range p.Paths {
		fmt.Fprintf(h, "%q %q %d %d %d\n", path, p.before[path], p.modes[path], len(p.data[path]), len(p.previous[path]))
		h.Write(p.data[path])
		h.Write(p.previous[path])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ID returns the content-bound approval token for p.
func (p *Plan) ID() string { return p.digest }

// Apply publishes p only when approval equals its ID and its fingerprints still
// match, under the caller's mutation lock. On failure it restores a changed shell
// file and removes paths created by this invocation, returning publication and
// rollback errors. Preexisting configuration, binaries and links are retained.
func Apply(p *Plan, approval string) (err error) {
	if p == nil || approval != p.digest || p.hash() != p.digest {
		return fmt.Errorf("setup requires unchanged preview approval")
	}
	for _, path := range p.Paths {
		if err = platform.linked(path); err != nil {
			return err
		}
		before, e := platform.fingerprint(path)
		if e != nil {
			return e
		}
		if before != p.before[path] {
			return fmt.Errorf("setup destination changed after preview")
		}
	}
	var created []string
	defer func() {
		if err != nil {
			for _, path := range created {
				var e error
				if data, exists := p.previous[path]; exists {
					e = platform.atomic(path, data, p.modes[path])
				} else {
					e = platform.remove(path)
				}
				if e != nil {
					err = fmt.Errorf("%w; cleanup %s: %v", err, path, e)
				}
			}
		}
	}()
	for _, path := range p.Paths {
		if path == p.Link {
			continue
		}
		mode := os.FileMode(0600)
		if previousMode, exists := p.modes[path]; exists {
			mode = previousMode
		}
		if path == p.Binary {
			mode = 0700
		}
		if err = platform.atomic(path, p.data[path], mode); err != nil {
			return err
		}
		created = append(created, path)
	}
	if p.Link != "" {
		if err = platform.mkdir(filepath.Dir(p.Link), 0700); err != nil {
			return err
		}
		if err = platform.symlink(p.Binary, p.Link); err != nil {
			return err
		}
		created = append(created, p.Link)
	}
	return nil
}

// shellPath adds an explicitly selected POSIX startup file to p's preview.
// It preserves bytes and mode, rejects links/oversize files and records rollback.
func (p *Plan) shellPath(o Options) error {
	if err := platform.validate(o.Home, o.ShellFile); err != nil {
		return err
	}
	initial, err := platform.fingerprint(o.ShellFile)
	if err != nil {
		return err
	}
	p.before[o.ShellFile] = initial
	if _, collision := p.data[o.ShellFile]; collision || o.ShellFile == p.Link || o.ShellFile == o.Source {
		return fmt.Errorf("shell file overlaps setup payload")
	}
	parts := []string{}
	for _, dir := range o.BinDirs {
		if err := platform.validate(o.Home, dir); err != nil {
			return err
		}
		if strings.ContainsAny(dir, "\r\n") {
			return fmt.Errorf("PATH directory cannot contain line breaks")
		}
		parts = append(parts, launch.Quote(dir))
	}
	if len(parts) == 0 {
		return fmt.Errorf("shell setup requires binary directories")
	}
	line := "export PATH=" + strings.Join(parts, ":") + ":\"$PATH\"\n"
	data := []byte{}
	info, err := platform.lstat(o.ShellFile)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("shell file must be regular")
		}
		f, e := platform.open(o.ShellFile)
		if e != nil {
			return e
		}
		data, e = platform.read(io.LimitReader(f, o.MaxBytes+1))
		closeErr := platform.close(f)
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if o.MaxBytes < 1 || int64(len(data)) > o.MaxBytes {
			return fmt.Errorf("shell file exceeds setup byte limit")
		}
		p.previous[o.ShellFile] = data
		p.modes[o.ShellFile] = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	if bytes.Contains(data, []byte(line)) {
		delete(p.previous, o.ShellFile)
		delete(p.modes, o.ShellFile)
		return nil
	}
	updated := append([]byte{}, data...)
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	p.data[o.ShellFile] = append(updated, []byte("# harness-ctl PATH (explicit setup)\n"+line)...)
	return nil
}
