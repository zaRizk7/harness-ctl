package manager

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

// signedArchive publishes an authenticated fixture with arbitrary tar members.
func signedArchive(t *testing.T, e *engine, id string, payload []byte) {
	t.Helper()
	identity, err := e.identity(true)
	if err != nil {
		t.Fatal(err)
	}
	var encrypted bytes.Buffer
	w, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(e.cfg.Root, "snapshots", id+".age")
	if err = atomicWrite(p, encrypted.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	mac, err := snapshotMAC(p, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = atomicWrite(strings.TrimSuffix(p, ".age")+".mac", []byte(hex.EncodeToString(mac)), 0600); err != nil {
		t.Fatal(err)
	}
}

// archivePayload returns a tar fixture whose manifest and members are test owned.
func archivePayload(t *testing.T, meta snapshotMeta, headers ...tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	manifest, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(manifest); err != nil {
		t.Fatal(err)
	}
	for _, h := range headers {
		if err = w.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err = w.Write(bytes.Repeat([]byte{'x'}, int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestManifestRejectsInvalidShapeAndIdentity(t *testing.T) {
	for _, scenario := range []string{"empty", "name", "type", "size", "json", "identity"} {
		t.Run(scenario, func(t *testing.T) {
			var b bytes.Buffer
			if scenario != "empty" {
				w := tar.NewWriter(&b)
				h := tar.Header{Name: "manifest.json", Mode: 0600}
				data := []byte(`{"id":"bad"}`)
				switch scenario {
				case "name":
					h.Name = "other"
				case "type":
					h.Typeflag = tar.TypeDir
					data = nil
				case "size":
					data = make([]byte, (8<<20)+1)
				case "json":
					data = []byte("invalid")
				}
				h.Size = int64(len(data))
				if err := w.WriteHeader(&h); err != nil {
					t.Fatal(err)
				}
				_, _ = w.Write(data)
				_ = w.Close()
			}
			if _, err := readManifest(tar.NewReader(&b)); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestAuthenticatedRestoreValidationNeverWritesTargets(t *testing.T) {
	for _, scenario := range []string{"manifest", "id", "registry", "harness", "target", "staging", "member", "type", "limit", "absent", "missing", "link-child", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, marker := archiveFaultFixture(t)
			id := randomID()
			meta := snapshotMeta{ID: id, Harness: "pi", Registry: e.reg, Install: p.Install, Items: []snapshotItem{{Path: p.StateRoot, Root: filepath.Dir(p.StateRoot), Directory: true}}}
			headers := []tar.Header{{Name: "0/.", Typeflag: tar.TypeDir, Mode: 0700}}
			switch scenario {
			case "id":
				meta.ID = randomID()
			case "registry":
				meta.Registry.Installs[0].Root = "/foreign"
			case "harness":
				meta.Harness = "unknown"
			case "target":
				meta.Items[0].Path = filepath.Join(e.cfg.Home, "foreign")
			case "staging":
				_ = atomicWrite(filepath.Join(e.cfg.Root, "restore-staging"), []byte("block"), 0600)
			case "member":
				headers = []tar.Header{{Name: "0/child", Size: 1, Mode: 0600}}
				headers[0].Size = 10
			case "type":
				headers = []tar.Header{{Name: "0/.", Typeflag: tar.TypeFifo}}
			case "limit":
				e.cfg.MaxSnapshotBytes = 1
				headers = append(headers, tar.Header{Name: "0/file", Size: 2, Mode: 0600})
			case "absent":
				meta.Items[0].Absent = true
			case "missing":
				headers = nil
			case "link-child":
				headers = append(headers, tar.Header{Name: "0/link", Typeflag: tar.TypeSymlink, Linkname: "."}, tar.Header{Name: "0/link/file", Mode: 0600})
			}
			payload := archivePayload(t, meta, headers...)
			if scenario == "manifest" {
				payload = []byte("invalid tar")
			}
			if scenario == "member" {
				payload = payload[:len(payload)-1531]
			}
			signedArchive(t, e, id, payload)
			ctx := context.Background()
			if scenario == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before, _ := fingerprint(p.StateRoot)
			if err := e.restoreSnapshot(ctx, id); err == nil {
				t.Fatal("invalid restore accepted")
			}
			after, _ := fingerprint(p.StateRoot)
			data, err := os.ReadFile(marker)
			if before != after || err != nil || string(data) != "keep" {
				t.Fatal("invalid restore changed live state", err)
			}
		})
	}
}

func TestSnapshotAuthenticatorAndCryptoFailures(t *testing.T) {
	for _, scenario := range []string{"id", "short-mac", "hex-mac", "decrypt", "encrypt", "generate", "key-set", "entropy", "linked", "size", "walk-error", "cancel", "copy-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			e, p, _ := archiveFaultFixture(t)
			old := archiveIO
			oldFiles := fileIO
			defer func() { archiveIO = old; fileIO = oldFiles }()
			fault := errors.New("fixture crypto failure")
			switch scenario {
			case "id":
				if _, _, err := e.openSnapshot("bad"); err == nil {
					t.Fatal("bad identity accepted")
				}
				return
			case "short-mac", "hex-mac", "decrypt":
				meta, err := e.snapshot(context.Background(), p)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "decrypt" {
					archiveIO.decrypt = func(io.Reader, ...age.Identity) (io.Reader, error) { return nil, fault }
				} else {
					mac := []byte("short")
					if scenario == "hex-mac" {
						mac = []byte(strings.Repeat("z", 64))
					}
					_ = atomicWrite(filepath.Join(e.cfg.Root, "snapshots", meta.ID+".mac"), mac, 0600)
				}
				if _, err = e.authenticatedSnapshot(meta.ID); err == nil {
					t.Fatal("invalid authenticator accepted")
				}
				return
			case "encrypt":
				archiveIO.encrypt = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) { return nil, fault }
			case "generate":
				archiveIO.generate = func() (*age.X25519Identity, error) { return nil, fault }
			case "key-set":
				e.keys = &failingKeyStore{}
			case "entropy":
				archiveIO.entropy = func([]byte) (int, error) { return 0, fault }
				defer func() {
					if recover() == nil {
						t.Fatal("entropy failure did not terminate")
					}
				}()
				_ = randomID()
				return
			case "linked":
				p.Resources = append(p.Resources, resource{Path: "/ignored", Linked: true})
			case "size":
				e.cfg.MaxSnapshotBytes = 1
			case "walk-error":
				fileIO.walk = func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, fault) }
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err := e.snapshot(ctx, p); err == nil {
					t.Fatal("cancel ignored")
				}
				return
			case "copy-cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if err := copyTreeContext(ctx, p.StateRoot, filepath.Join(t.TempDir(), "copy")); err == nil {
					t.Fatal("cancel ignored")
				}
				if _, err := (contextReader{ctx, strings.NewReader("a")}).Read(make([]byte, 1)); err == nil {
					t.Fatal("reader cancel ignored")
				}
				return
			}
			_, err := e.snapshot(context.Background(), p)
			if scenario == "linked" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("crypto/size failure ignored")
			}
		})
	}
}

// failingKeyStore models a credential store that cannot persist a newly generated identity.
type failingKeyStore struct{ memoryKeys }

func (failingKeyStore) Set(string, string) error { return errors.New("fixture key write failed") }
