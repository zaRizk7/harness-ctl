package manager

import (
	"archive/tar"
	"context"
	"crypto/hmac"
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

// keyStore persists the manager encryption identity at the native credential
// boundary. Tests substitute in-memory identities without touching OS secrets.
type keyStore interface {
	Get(string) (string, error)
	Set(string, string) error
}

// keychainStore binds platform credential operations for the manager identity.
type keychainStore struct {
	get    func(string, string) (string, error)
	set    func(string, string, string) error
	remove func(string, string) error
}

// newKeychainStore binds native credential operations at the platform boundary.
func newKeychainStore() keychainStore {
	return keychainStore{get: keyring.Get, set: keyring.Set, remove: keyring.Delete}
}

// Get returns the stored identity for account in the manager keychain service,
// or the native lookup error. It does not create an identity.
func (k keychainStore) Get(account string) (string, error) {
	return k.get(keychainService, account)
}

// Set stores value for account in the manager keychain service and returns the
// native write error. Callers persist the identity before publishing ciphertext.
func (k keychainStore) Set(account, value string) error {
	return k.set(keychainService, account, value)
}

// snapshotItem describes one captured or absent path and its restoration
// ownership. Metadata is authenticated before restoring any archive member.
type snapshotItem struct {
	Path       string     `json:"path"`
	Root       string     `json:"root"`
	Categories []category `json:"categories"`
	Directory  bool       `json:"directory"`
	Absent     bool       `json:"absent,omitempty"`
	Launcher   bool       `json:"launcher,omitempty"`
	Owners     []string   `json:"owners,omitempty"`
}

// snapshotMeta indexes an encrypted recovery archive, its retention and
// the registry/install coordinates needed to validate restoration.
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

// randomID returns a cryptographically random operation identity. Entropy
// failures terminate rather than weakening identity safety.
func randomID() string {
	var b [16]byte
	if _, err := archiveIO.entropy(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// identity returns the manager root's encryption identity. create permits
// generating and persisting one when absent, otherwise errors are returned.
func (e *engine) identity(create bool) (*age.X25519Identity, error) {
	account := installID("storage", e.cfg.Root)
	secret, err := e.keys.Get(account)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) || !create {
			return nil, fmt.Errorf("snapshot key unavailable in macOS Keychain: %w", err)
		}
		id, err := archiveIO.generate()
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

// snapshot returns authenticated metadata after encrypting p's rollback
// resources with ctx. No plaintext recovery payload is published.
func (e *engine) snapshot(ctx context.Context, p *plan) (snapshotMeta, error) {
	meta := snapshotMeta{ID: randomID(), Harness: p.Spec.ID, Action: p.Request.Action, Created: time.Now().UTC(), Registry: e.reg, Install: p.Install}
	meta.Expires = meta.Created.Add(time.Duration(e.cfg.BackupDays) * 24 * time.Hour)
	for _, r := range p.Resources {
		if r.Linked {
			continue
		}
		meta.Items = append(meta.Items, snapshotItem{Path: r.Path, Root: r.Root, Categories: resourceCategories(r), Owners: append([]string{}, r.Owners...)})
	}
	if p.Request.Action != "credentials" && within(e.cfg.Root, p.StateRoot) {
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
	if p.Install.Root != "" && p.Request.Action != "reset" && p.Request.Action != "manage" && p.Request.Action != "auth" && p.Request.Action != "credentials" && p.Install.Method != "brew" {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Install.Root, Root: filepath.Dir(p.Install.Root), Categories: []category{other}})
	}
	if p.Component != nil && p.Component.Native {
		state, _, err := e.componentEngine(p.Install, p.Component.Request.Scope)
		if err != nil {
			return meta, err
		}
		for _, root := range state.rootsFor(p.Spec) {
			owners := []string{p.Spec.ID}
			if !within(e.cfg.Root, root) {
				owners = append(owners, p.Spec.SharedClients...)
			}
			meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories, Owners: owners})
		}
	}
	if strings.HasPrefix(p.Install.Method, "native-") {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Install.Path, Root: filepath.Dir(p.Install.Path), Categories: []category{other}, Launcher: true})
	}
	shim := filepath.Join(e.cfg.BinDir, p.Spec.Command)
	meta.Items = append(meta.Items, snapshotItem{Path: shim, Root: e.cfg.BinDir, Categories: []category{other}})
	if prof, ok := e.reg.Profiles[p.Install.ID]; ok && p.Request.Action != "credentials" {
		meta.Items = append(meta.Items, snapshotItem{Path: prof.Root, Root: filepath.Dir(prof.Root), Categories: categories})
	}
	if p.Request.Action == "profile" {
		root := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
	}
	if p.Install.Managed && p.Request.Action != "credentials" {
		root := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		meta.Items = append(meta.Items, snapshotItem{Path: root, Root: filepath.Dir(root), Categories: categories})
	}
	for _, path := range p.Install.ServicePaths {
		meta.Items = append(meta.Items, snapshotItem{Path: path, Root: filepath.Dir(path), Categories: []category{other}})
	}
	if p.Credential != nil && p.Credential.Action == "purge" {
		meta.Items = append(meta.Items, snapshotItem{Path: p.Credential.Path, Root: e.cfg.Root, Categories: []category{auth}, Owners: []string{p.Spec.ID}})
	}
	meta.Items = dedupeItems(meta.Items)
	for i := range meta.Items {
		info, err := fileIO.lstat(meta.Items[i].Path)
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
	if err = fileIO.mkdir(dir, 0700); err != nil {
		return meta, err
	}
	f, err := fileIO.create(dir, ".snapshot-*")
	if err != nil {
		return meta, err
	}
	defer fileIO.remove(f.Name())
	if err = fileIO.chmod(f, 0600); err != nil {
		fileIO.close(f)
		return meta, err
	}
	encrypted, err := archiveIO.encrypt(f, identity.Recipient())
	if err != nil {
		fileIO.close(f)
		return meta, err
	}
	archive := tar.NewWriter(encrypted)
	var total int64
	manifest, err := marshalJSON(meta)
	if err != nil {
		archiveIO.tarClose(archive)
		archiveIO.cipherClose(encrypted)
		fileIO.close(f)
		return meta, err
	}
	if err = archiveIO.tarHeader(archive, &tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err == nil {
		_, err = archiveIO.tarWrite(archive, manifest)
	}
	if err != nil {
		archiveIO.tarClose(archive)
		archiveIO.cipherClose(encrypted)
		fileIO.close(f)
		return meta, err
	}
	for i := range meta.Items {
		item := &meta.Items[i]
		info, err := fileIO.lstat(item.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			archiveIO.tarClose(archive)
			archiveIO.cipherClose(encrypted)
			fileIO.close(f)
			return meta, err
		}
		item.Directory = info.IsDir()
		err = fileIO.walk(item.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := fileIO.info(d)
			if err != nil {
				return err
			}
			rel, err := fileIO.rel(item.Path, path)
			if err != nil {
				return err
			}
			var link string
			if info.Mode()&os.ModeSymlink != 0 {
				link, err = fileIO.readlink(path)
				if err != nil {
					return err
				}
			}
			header, err := archiveIO.header(info, link)
			if err != nil {
				return err
			}
			header.Name = fmt.Sprintf("%d/%s", i, filepath.ToSlash(rel))
			if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("unsupported state resource type: %s", path)
			}
			if err = archiveIO.tarHeader(archive, header); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				total += info.Size()
				if total > e.cfg.MaxSnapshotBytes {
					return fmt.Errorf("snapshot exceeds configured size limit")
				}
				f, err := fileIO.open(path)
				if err != nil {
					return err
				}
				_, err = archiveIO.copyN(archive, f, info.Size())
				closeErr := fileIO.close(f)
				if err != nil {
					return err
				}
				return closeErr
			}
			return nil
		})
		if err != nil {
			archiveIO.tarClose(archive)
			archiveIO.cipherClose(encrypted)
			fileIO.close(f)
			return meta, err
		}
	}
	if err = archiveIO.tarClose(archive); err != nil {
		archiveIO.cipherClose(encrypted)
		fileIO.close(f)
		return meta, err
	}
	if err = archiveIO.cipherClose(encrypted); err != nil {
		fileIO.close(f)
		return meta, err
	}
	if err = fileIO.sync(f); err != nil {
		fileIO.close(f)
		return meta, err
	}
	if err = fileIO.close(f); err != nil {
		return meta, err
	}
	if err = fileIO.rename(f.Name(), filepath.Join(dir, meta.ID+".age")); err != nil {
		return meta, err
	}
	archivePath := filepath.Join(dir, meta.ID+".age")
	mac, err := snapshotMAC(archivePath, identity)
	if err != nil {
		fileIO.remove(archivePath)
		return meta, err
	}
	if err = atomicWrite(filepath.Join(dir, meta.ID+".mac"), []byte(hex.EncodeToString(mac)), 0600); err != nil {
		fileIO.remove(archivePath)
		return meta, err
	}
	if err = writeJSON(filepath.Join(dir, meta.ID+".json"), meta); err != nil {
		e.purgeSnapshot(meta.ID)
		return meta, err
	}
	return meta, nil
}

// snapshotMAC returns the keyed authentication tag for meta using the root
// identity, or a key/encoding error.
func snapshotMAC(path string, identity *age.X25519Identity) ([]byte, error) {
	f, err := fileIO.open(path)
	if err != nil {
		return nil, err
	}
	defer fileIO.close(f)
	key := sha256.Sum256([]byte(keychainService + "\x00" + identity.String()))
	mac := hmac.New(sha256.New, key[:])
	if _, err = archiveIO.copy(mac, f); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

// dedupeItems returns items without redundant descendants, preserving required
// category and owner metadata.
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

// snapshots returns valid display indexes sorted by creation time. Restoration
// separately authenticates encrypted manifests.
func (e *engine) snapshots() ([]snapshotMeta, error) {
	var result []snapshotMeta
	dir := filepath.Join(e.cfg.Root, "snapshots")
	if err := validateOwnedPath(e.cfg.Root, dir); err != nil {
		return nil, err
	}
	entries, err := fileIO.readDir(dir)
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

// safeID reports whether s is a complete safe operation/snapshot identity,
// suitable for owned filenames.
func safeID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// purgeSnapshot permanently removes id's archive and display index after
// validating the identity and owned paths.
func (e *engine) purgeSnapshot(id string) error {
	if !safeID(id) {
		return fmt.Errorf("invalid snapshot ID")
	}
	for _, suffix := range []string{".age", ".json", ".mac"} {
		path := filepath.Join(e.cfg.Root, "snapshots", id+suffix)
		if err := validateOwnedPath(e.cfg.Root, path); err != nil {
			return err
		}
		if err := fileIO.remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// openSnapshot returns id's encrypted file and authenticated decryption reader.
// The caller closes the file, and decryption errors are returned.
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
	expected, err := fileIO.readFile(macPath)
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
	f, err := fileIO.open(path)
	if err != nil {
		return nil, nil, err
	}
	key := sha256.Sum256([]byte(keychainService + "\x00" + identity.String()))
	mac := hmac.New(sha256.New, key[:])
	if _, err = archiveIO.copy(mac, f); err != nil {
		fileIO.close(f)
		return nil, nil, err
	}
	if !hmac.Equal(mac.Sum(nil), signature) {
		fileIO.close(f)
		return nil, nil, fmt.Errorf("snapshot authentication failed")
	}
	if _, err = fileIO.seek(f, 0, io.SeekStart); err != nil {
		fileIO.close(f)
		return nil, nil, err
	}
	r, err := archiveIO.decrypt(f, identity)
	if err != nil {
		fileIO.close(f)
		return nil, nil, err
	}
	return f, r, nil
}

// readManifest returns the authenticated manifest decoded from tr, rejecting
// unexpected names, sizes and invalid metadata.
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

// authenticatedSnapshot returns id's verified encrypted metadata after checking
// manifest identity and complete archive authentication.
func (e *engine) authenticatedSnapshot(id string) (snapshotMeta, error) {
	f, decrypted, err := e.openSnapshot(id)
	if err != nil {
		return snapshotMeta{}, err
	}
	defer fileIO.close(f)
	meta, err := readManifest(tar.NewReader(decrypted))
	if err != nil {
		return meta, err
	}
	// Authenticate the complete age stream before relying on its manifest.
	if _, err = archiveIO.copy(io.Discard, decrypted); err != nil {
		return meta, err
	}
	if meta.ID != id {
		return meta, fmt.Errorf("snapshot ID mismatch")
	}
	return meta, nil
}

// validRestoreItem returns an error unless item's target matches s's documented
// state or inst's verified installation ownership.
func (e *engine) validRestoreItem(s harnessSpec, item snapshotItem, inst installation) error {
	allowed := (item.Path == filepath.Join(e.cfg.Root, "credentials.age") && item.Root == e.cfg.Root && len(item.Categories) == 1 && item.Categories[0] == auth) || within(filepath.Join(e.cfg.Root, "installs", s.ID), item.Path) || within(filepath.Join(e.cfg.Root, "states", s.ID), item.Path) || item.Path == filepath.Join(e.cfg.BinDir, s.Command)
	if within(filepath.Join(e.cfg.Root, "profiles"), item.Path) && filepath.Dir(item.Path) == filepath.Join(e.cfg.Root, "profiles") {
		allowed = true
	}
	for _, root := range e.rootsFor(s) {
		if within(root, item.Path) {
			allowed = true
		}
	}
	if item.Launcher && item.Path == inst.Path && filepath.Dir(item.Path) == filepath.Join(e.cfg.Home, ".local/bin") && strings.HasPrefix(inst.Method, "native-") {
		allowed = true
	}
	if s.ID == "claude" && item.Path == filepath.Join(e.cfg.Home, ".claude.json") {
		allowed = true
	}
	if s.ID == "codex" {
		root := e.codexSkillsRoot()
		if within(filepath.Join(root, "skills"), item.Path) || within(filepath.Join(root, disabledComponentsDir, string(skills)), item.Path) {
			allowed = true
		}
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

// restoreSnapshot restores id with ctx only after authenticating and validating
// every archive member. IO/service failures retain recovery evidence.
func (e *engine) restoreSnapshot(ctx context.Context, id string) error {
	f, decrypted, err := e.openSnapshot(id)
	if err != nil {
		return err
	}
	defer fileIO.close(f)
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
	s, err := e.specFor(meta.Harness)
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
	if err = fileIO.mkdir(stageRoot, 0700); err != nil {
		return err
	}
	stage, err := fileIO.mkdirTemp(stageRoot, "restore-*")
	if err != nil {
		return err
	}
	defer fileIO.removeAll(stage)
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
			if err = fileIO.mkdir(path, mode|0700); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if header.Size < 0 || total > e.cfg.MaxSnapshotBytes {
				return fmt.Errorf("snapshot size exceeds limit")
			}
			if err = fileIO.mkdir(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := fileIO.openFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := archiveIO.copyN(file, archive, header.Size)
			closeErr := fileIO.close(file)
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
	if _, err = archiveIO.copy(io.Discard, decrypted); err != nil {
		return err
	}
	for path, target := range links {
		if err = fileIO.mkdir(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err = fileIO.symlink(target, path); err != nil {
			return err
		}
	}
	for i, item := range meta.Items {
		prepared := filepath.Join(stage, strconv.Itoa(i))
		info, err := fileIO.lstat(prepared)
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
		if _, err = fileIO.lstat(prepared); os.IsNotExist(err) {
			if !item.Absent {
				return fmt.Errorf("snapshot member is missing for %s", item.Path)
			}
			continue
		} else if err != nil {
			return err
		}
		if err = fileIO.mkdir(filepath.Dir(item.Path), 0700); err != nil {
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
			if err = fileIO.removeAll(item.Path); err != nil {
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

// replaceTree replaces target with stage while retaining a rollback rename
// until replacement succeeds. It returns replacement or restoration failures.
func replaceTree(source, target string) error {
	parent := filepath.Dir(target)
	if err := fileIO.mkdir(parent, 0700); err != nil {
		return err
	}
	stage, err := fileIO.mkdirTemp(parent, ".harness-ctl-replace-*")
	if err != nil {
		return err
	}
	retainRollback := false
	defer func() {
		if !retainRollback {
			_ = fileIO.removeAll(stage)
		}
	}()
	ready := filepath.Join(stage, "ready")
	old := filepath.Join(stage, "old")
	if err = copyTree(source, ready); err != nil {
		return err
	}
	hadOld := false
	if _, err = fileIO.lstat(target); err == nil {
		if err = fileIO.rename(target, old); err != nil {
			return err
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = fileIO.rename(ready, target); err != nil {
		if hadOld {
			if restoreErr := fileIO.rename(old, target); restoreErr != nil {
				retainRollback = true
				return fmt.Errorf("replacement failed. Original payload retained at %s: %w", old, errors.Join(err, restoreErr))
			}
		}
		return err
	}
	return nil
}

// copyTree copies src to dest using the same confinement and file-type checks
// as cancellable recovery copying.
func copyTree(source, target string) error {
	return copyTreeContext(context.Background(), source, target)
}

// Component imports use cancellable copying so rollback cannot race a writer.
func copyTreeContext(ctx context.Context, source, target string) error {
	return fileIO.walk(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := fileIO.rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		info, err := fileIO.info(d)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fileIO.mkdir(dest, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := fileIO.readlink(path)
			if err != nil {
				return err
			}
			return fileIO.symlink(link, dest)
		}
		if err = fileIO.mkdir(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		in, err := fileIO.open(path)
		if err != nil {
			return err
		}
		defer fileIO.close(in)
		out, err := fileIO.openFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := archiveIO.copy(out, contextReader{ctx, in})
		closeErr := fileIO.close(out)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// contextReader checks cancellation before forwarding reads from its source.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read reads into data through the wrapped reader unless cancellation has been
// requested, returning the byte count and read/context error.
func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
