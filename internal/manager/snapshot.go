package manager

import (
	"archive/tar"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/zalando/go-keyring"
)

const keychainService = "harness-ctl.snapshot-key"

type keyStore interface {
	Get(string) (string, error)
	Set(string, string) error
}
type keychainStore struct{}

func (keychainStore) Get(account string) (string, error) {
	return keyring.Get(keychainService, account)
}
func (keychainStore) Set(account, value string) error {
	return keyring.Set(keychainService, account, value)
}

type snapshotItem struct {
	Path       string     `json:"path"`
	Root       string     `json:"root"`
	Categories []category `json:"categories"`
	Directory  bool       `json:"directory"`
	Absent     bool       `json:"absent,omitempty"`
	Launcher   bool       `json:"launcher,omitempty"`
	Owners     []string   `json:"owners,omitempty"`
}
type snapshotMeta struct {
	ID       string         `json:"id"`
	Harness  string         `json:"harness"`
	Action   string         `json:"action"`
	Created  time.Time      `json:"created"`
	Expires  time.Time      `json:"expires"`
	Items    []snapshotItem `json:"items"`
	Registry registry       `json:"registry"`
	Install  installation   `json:"installation"`
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (e *engine) identity(create bool) (*age.X25519Identity, error) {
	account := installID("storage", e.cfg.Root)
	secret, err := e.keys.Get(account)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) || !create {
			return nil, fmt.Errorf("snapshot key unavailable in macOS Keychain: %w", err)
		}
		id, err := age.GenerateX25519Identity()
		if err != nil {
			return nil, err
		}
		if err := e.keys.Set(account, id.String()); err != nil {
			return nil, fmt.Errorf("cannot save snapshot key in Keychain: %w", err)
		}
		return id, nil
	}
	return age.ParseX25519Identity(secret)
}

func (e *engine) snapshot(ctx context.Context, p *plan) (snapshotMeta, error) {
	meta := snapshotMeta{ID: randomID(), Harness: p.Spec.ID, Action: p.Request.Action, Created: time.Now().UTC(), Registry: e.reg, Install: p.Install}
	meta.Expires = meta.Created.Add(time.Duration(e.cfg.BackupDays) * 24 * time.Hour)
	for _, r := range p.Resources {
		if r.Linked {
			continue
		}
		meta.Items = append(meta.Items, snapshotItem{Path: r.Path, Root: r.Root, Categories: resourceCategories(r), Owners: append([]string{}, r.Owners...)})
	}
	if within(e.cfg.Root, p.StateRoot) {
		stateEngine := *e
		stateEngine.cfg.StateRoots = map[string]string{p.Spec.ID: p.StateRoot}
		for _, root := range stateEngine.rootsFor(p.Spec) {
			meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
		}
	}
	if p.Request.Action == "migrate" {
		stateEngine := *e
		stateEngine.cfg.StateRoots = map[string]string{p.Spec.ID: e.managedStateRoot(p.Spec)}
		for _, root := range stateEngine.rootsFor(p.Spec) {
			meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
		}
	}
	if p.Request.Model == "isolated" && p.Destination != "" && p.Destination != p.Install.Root {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Destination, Root: filepath.Dir(p.Destination), Categories: []category{other}})
	}
	if p.Install.Root != "" && p.Request.Action != "reset" && p.Install.Method != "brew" {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Install.Root, Root: filepath.Dir(p.Install.Root), Categories: []category{other}})
	}
	if strings.HasPrefix(p.Install.Method, "native-") {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Install.Path, Root: filepath.Dir(p.Install.Path), Categories: []category{other}, Launcher: true})
	}
	shim := filepath.Join(e.cfg.BinDir, p.Spec.Command)
	meta.Items = append(meta.Items, snapshotItem{Path: shim, Root: e.cfg.BinDir, Categories: []category{other}})
	if prof, ok := e.reg.Profiles[p.Install.ID]; ok {
		meta.Items = append(meta.Items, snapshotItem{Path: prof.Root, Root: filepath.Dir(prof.Root), Categories: categories})
	}
	if p.Request.Action == "profile" {
		root := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
	}
	if p.Install.Managed {
		root := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
	}
	for _, path := range p.Install.ServicePaths {
		meta.Items = append(meta.Items, snapshotItem{Path: path, Root: filepath.Dir(path), Categories: []category{other}})
	}
	meta.Items = dedupeItems(meta.Items)
	for i := range meta.Items {
		info, err := os.Lstat(meta.Items[i].Path)
		if err == nil {
			meta.Items[i].Directory = info.IsDir()
		} else if os.IsNotExist(err) {
			meta.Items[i].Absent = true
		} else {
			return meta, err
		}
	}
	identity, err := e.identity(true)
	if err != nil {
		return meta, err
	}
	dir := filepath.Join(e.cfg.Root, "snapshots")
	if err = validateOwnedPath(e.cfg.Root, dir); err != nil {
		return meta, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return meta, err
	}
	f, err := os.CreateTemp(dir, ".snapshot-*")
	if err != nil {
		return meta, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return meta, err
	}
	encrypted, err := age.Encrypt(f, identity.Recipient())
	if err != nil {
		f.Close()
		return meta, err
	}
	archive := tar.NewWriter(encrypted)
	var total int64
	manifest, err := json.Marshal(meta)
	if err != nil {
		archive.Close()
		encrypted.Close()
		f.Close()
		return meta, err
	}
	if err = archive.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err == nil {
		_, err = archive.Write(manifest)
	}
	if err != nil {
		archive.Close()
		encrypted.Close()
		f.Close()
		return meta, err
	}
	for i := range meta.Items {
		item := &meta.Items[i]
		info, err := os.Lstat(item.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			archive.Close()
			encrypted.Close()
			f.Close()
			return meta, err
		}
		item.Directory = info.IsDir()
		err = filepath.WalkDir(item.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(item.Path, path)
			if err != nil {
				return err
			}
			var link string
			if info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			header.Name = fmt.Sprintf("%d/%s", i, filepath.ToSlash(rel))
			if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("unsupported state resource type: %s", path)
			}
			if err = archive.WriteHeader(header); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				total += info.Size()
				if total > e.cfg.MaxSnapshotBytes {
					return fmt.Errorf("snapshot exceeds configured size limit")
				}
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				_, err = io.CopyN(archive, f, info.Size())
				closeErr := f.Close()
				if err != nil {
					return err
				}
				return closeErr
			}
			return nil
		})
		if err != nil {
			archive.Close()
			encrypted.Close()
			f.Close()
			return meta, err
		}
	}
	if err = archive.Close(); err != nil {
		encrypted.Close()
		f.Close()
		return meta, err
	}
	if err = encrypted.Close(); err != nil {
		f.Close()
		return meta, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return meta, err
	}
	if err = f.Close(); err != nil {
		return meta, err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, meta.ID+".age")); err != nil {
		return meta, err
	}
	archivePath := filepath.Join(dir, meta.ID+".age")
	mac, err := snapshotMAC(archivePath, identity)
	if err != nil {
		os.Remove(archivePath)
		return meta, err
	}
	if err = atomicWrite(filepath.Join(dir, meta.ID+".mac"), []byte(hex.EncodeToString(mac)), 0600); err != nil {
		os.Remove(archivePath)
		return meta, err
	}
	if err = writeJSON(filepath.Join(dir, meta.ID+".json"), meta); err != nil {
		e.purgeSnapshot(meta.ID)
		return meta, err
	}
	return meta, nil
}

func snapshotMAC(path string, identity *age.X25519Identity) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	key := sha256.Sum256([]byte(keychainService + "\x00" + identity.String()))
	mac := hmac.New(sha256.New, key[:])
	if _, err = io.Copy(mac, f); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

func dedupeItems(items []snapshotItem) []snapshotItem {
	var out []snapshotItem
	for _, item := range items {
		covered := false
		for _, other := range items {
			if item.Path != other.Path && within(other.Path, item.Path) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		duplicate := false
		for _, other := range out {
			if item.Path == other.Path {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, item)
		}
	}
	return out
}

func (e *engine) snapshots() ([]snapshotMeta, error) {
	var result []snapshotMeta
	dir := filepath.Join(e.cfg.Root, "snapshots")
	if err := validateOwnedPath(e.cfg.Root, dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".age")
		if !safeID(id) {
			return nil, fmt.Errorf("invalid snapshot filename")
		}
		meta := snapshotMeta{ID: id, Harness: "unindexed"}
		if err := validateOwnedPath(e.cfg.Root, filepath.Join(dir, entry.Name())); err != nil {
			return nil, err
		}
		indexPath := filepath.Join(dir, id+".json")
		if err := validateOwnedPath(e.cfg.Root, indexPath); err != nil {
			return nil, err
		}
		if err := readJSON(indexPath, &meta); err != nil && !os.IsNotExist(err) {
			meta = snapshotMeta{ID: id, Harness: "unindexed"}
		}
		if meta.ID != id {
			meta = snapshotMeta{ID: id, Harness: "unindexed"}
		}
		result = append(result, meta)
	}
	return result, nil
}

func safeID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (e *engine) purgeSnapshot(id string) error {
	if !safeID(id) {
		return fmt.Errorf("invalid snapshot ID")
	}
	for _, suffix := range []string{".age", ".json", ".mac"} {
		path := filepath.Join(e.cfg.Root, "snapshots", id+suffix)
		if err := validateOwnedPath(e.cfg.Root, path); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (e *engine) openSnapshot(id string) (*os.File, io.Reader, error) {
	if !safeID(id) {
		return nil, nil, fmt.Errorf("invalid snapshot ID")
	}
	identity, err := e.identity(false)
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(e.cfg.Root, "snapshots", id+".age")
	if err = validateOwnedPath(e.cfg.Root, path); err != nil {
		return nil, nil, err
	}
	macPath := filepath.Join(e.cfg.Root, "snapshots", id+".mac")
	if err = validateOwnedPath(e.cfg.Root, macPath); err != nil {
		return nil, nil, err
	}
	expected, err := os.ReadFile(macPath)
	if err != nil {
		return nil, nil, err
	}
	if len(expected) != sha256.Size*2 {
		return nil, nil, fmt.Errorf("invalid snapshot authenticator")
	}
	signature, err := hex.DecodeString(string(expected))
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	key := sha256.Sum256([]byte(keychainService + "\x00" + identity.String()))
	mac := hmac.New(sha256.New, key[:])
	if _, err = io.Copy(mac, f); err != nil {
		f.Close()
		return nil, nil, err
	}
	if !hmac.Equal(mac.Sum(nil), signature) {
		f.Close()
		return nil, nil, fmt.Errorf("snapshot authentication failed")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, nil, err
	}
	r, err := age.Decrypt(f, identity)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, r, nil
}

func readManifest(archive *tar.Reader) (snapshotMeta, error) {
	var meta snapshotMeta
	header, err := archive.Next()
	if err != nil {
		return meta, err
	}
	if header.Name != "manifest.json" || header.Typeflag != tar.TypeReg || header.Size > 8<<20 {
		return meta, fmt.Errorf("snapshot manifest is invalid")
	}
	if err := json.NewDecoder(archive).Decode(&meta); err != nil {
		return meta, err
	}
	if !safeID(meta.ID) {
		return meta, fmt.Errorf("snapshot manifest ID is invalid")
	}
	return meta, nil
}

func (e *engine) authenticatedSnapshot(id string) (snapshotMeta, error) {
	f, decrypted, err := e.openSnapshot(id)
	if err != nil {
		return snapshotMeta{}, err
	}
	defer f.Close()
	meta, err := readManifest(tar.NewReader(decrypted))
	if err != nil {
		return meta, err
	}
	// Authenticate the complete age stream before relying on its manifest.
	if _, err = io.Copy(io.Discard, decrypted); err != nil {
		return meta, err
	}
	if meta.ID != id {
		return meta, fmt.Errorf("snapshot ID mismatch")
	}
	return meta, nil
}

func (e *engine) validRestoreItem(s harnessSpec, item snapshotItem, inst installation) error {
	allowed := within(filepath.Join(e.cfg.Root, "installs", s.ID), item.Path) || within(filepath.Join(e.cfg.Root, "states", s.ID), item.Path) || item.Path == filepath.Join(e.cfg.BinDir, s.Command)
	if within(filepath.Join(e.cfg.Root, "profiles"), item.Path) && filepath.Dir(item.Path) == filepath.Join(e.cfg.Root, "profiles") {
		allowed = true
	}
	for _, root := range e.rootsFor(s) {
		if within(root, item.Path) && item.Path != root {
			allowed = true
		}
	}
	if item.Launcher && item.Path == inst.Path && filepath.Dir(item.Path) == filepath.Join(e.cfg.Home, ".local/bin") && strings.HasPrefix(inst.Method, "native-") {
		allowed = true
	}
	if s.ID == "claude" && item.Path == filepath.Join(e.cfg.Home, ".claude.json") {
		allowed = true
	}
	for _, root := range []string{filepath.Join(e.cfg.Home, ".local/share/claude"), filepath.Join(e.cfg.Home, ".local/share/prime-agent"), filepath.Join(e.stateRoot(s), "packages/standalone"), filepath.Join(e.stateRoot(s), "hermes-agent")} {
		if item.Path == root && item.Path == inst.Root {
			allowed = true
		}
	}
	if inst.Method == "npm" && item.Path == inst.Root && strings.HasSuffix(inst.Root, "/lib/node_modules/"+inst.Package) && (inst.Package == s.Package || contains(s.LegacyPackages, inst.Package)) {
		allowed = true
	}
	for _, label := range s.LaunchLabels {
		if item.Path == filepath.Join(e.cfg.Home, "Library/LaunchAgents", label+".plist") {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("snapshot target is outside documented ownership: %s", item.Path)
	}
	if item.Launcher {
		if item.Path != inst.Path || filepath.Dir(item.Path) != filepath.Join(e.cfg.Home, ".local/bin") {
			return fmt.Errorf("invalid snapshot launcher")
		}
		return rejectLinkedAncestors(filepath.Dir(item.Path))
	}
	return validateOwnedPath(item.Root, item.Path)
}

func (e *engine) restoreSnapshot(ctx context.Context, id string) error {
	f, decrypted, err := e.openSnapshot(id)
	if err != nil {
		return err
	}
	defer f.Close()
	archive := tar.NewReader(decrypted)
	meta, err := readManifest(archive)
	if err != nil {
		return err
	}
	if meta.ID != id {
		return fmt.Errorf("snapshot ID mismatch")
	}
	if err = e.validateRegistry(meta.Registry); err != nil {
		return err
	}
	s, err := specFor(meta.Harness)
	if err != nil {
		return err
	}
	for _, item := range meta.Items {
		if err = e.validRestoreItem(s, item, meta.Install); err != nil {
			return err
		}
	}
	stageRoot := filepath.Join(e.cfg.Root, "restore-staging")
	if err = validateOwnedPath(e.cfg.Root, stageRoot); err != nil {
		return err
	}
	if err = os.MkdirAll(stageRoot, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(stageRoot, "restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	links := map[string]string{}
	var total int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		index, rel, ok := strings.Cut(header.Name, "/")
		n, parseErr := strconv.Atoi(index)
		if !ok || parseErr != nil || n < 0 || n >= len(meta.Items) || filepath.IsAbs(rel) || strings.Contains(rel, "\\") {
			return fmt.Errorf("invalid snapshot member")
		}
		clean := filepath.Clean(filepath.FromSlash(rel))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("snapshot path traversal")
		}
		path := filepath.Join(stage, index, clean)
		if err = validateOwnedPath(stage, path); err != nil {
			return err
		}
		for link := range links {
			if within(link, path) {
				return fmt.Errorf("snapshot member traverses a link")
			}
		}
		mode := fs.FileMode(header.Mode) & 0777
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(path, mode|0700); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if header.Size < 0 || total > e.cfg.MaxSnapshotBytes {
				return fmt.Errorf("snapshot size exceeds limit")
			}
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			links[path] = header.Linkname
		default:
			return fmt.Errorf("unsupported snapshot member type")
		}
	}
	if _, err = io.Copy(io.Discard, decrypted); err != nil {
		return err
	}
	for path, target := range links {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err = os.Symlink(target, path); err != nil {
			return err
		}
	}
	for i, item := range meta.Items {
		prepared := filepath.Join(stage, strconv.Itoa(i))
		info, err := os.Lstat(prepared)
		if item.Absent {
			if err == nil {
				return fmt.Errorf("absent snapshot item contains payload")
			}
			if !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("snapshot payload is incomplete: %w", err)
		}
		if info.IsDir() != item.Directory {
			return fmt.Errorf("snapshot payload type disagrees with manifest")
		}
	}
	// Stage and validate the entire archive before touching any live target.
	for i, item := range meta.Items {
		if err = e.validRestoreItem(s, item, meta.Install); err != nil {
			return err
		}
		prepared := filepath.Join(stage, strconv.Itoa(i))
		if _, err = os.Lstat(prepared); os.IsNotExist(err) {
			if !item.Absent {
				return fmt.Errorf("snapshot member is missing for %s", item.Path)
			}
			continue
		} else if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(item.Path), 0700); err != nil {
			return err
		}
		if err = replaceTree(prepared, item.Path); err != nil {
			return err
		}
	}
	for _, item := range meta.Items {
		if item.Absent {
			if err = e.validRestoreItem(s, item, meta.Install); err != nil {
				return err
			}
			if err = os.RemoveAll(item.Path); err != nil {
				return err
			}
		}
	}
	// Recover this harness without dropping installations registered later
	// for other harnesses.
	var installs []installation
	for _, inst := range e.reg.Installs {
		if inst.Harness != meta.Harness {
			installs = append(installs, inst)
		} else {
			delete(e.reg.Profiles, inst.ID)
		}
	}
	for _, inst := range meta.Registry.Installs {
		if inst.Harness == meta.Harness {
			installs = append(installs, inst)
			if prof, ok := meta.Registry.Profiles[inst.ID]; ok {
				e.reg.Profiles[inst.ID] = prof
			}
		}
	}
	e.reg.Installs = installs
	if err = writeJSON(e.statePath, e.reg); err != nil {
		return err
	}
	for _, inst := range e.reg.Installs {
		if inst.Harness == meta.Harness && inst.Managed {
			if err = e.writeShim(inst); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceTree(source, target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".harness-ctl-replace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	ready := filepath.Join(stage, "ready")
	old := filepath.Join(stage, "old")
	if err = copyTree(source, ready); err != nil {
		return err
	}
	hadOld := false
	if _, err = os.Lstat(target); err == nil {
		if err = os.Rename(target, old); err != nil {
			return err
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(ready, target); err != nil {
		if hadOld {
			if restoreErr := os.Rename(old, target); restoreErr != nil {
				return errors.Join(err, restoreErr)
			}
		}
		return err
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, dest)
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
