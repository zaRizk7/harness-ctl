package vault

import (
	"errors"
	"filippo.io/age"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticatedVaultAndFailureBoundaries(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	identity := func(bool) (*age.X25519Identity, error) { return id, nil }
	path := filepath.Join(t.TempDir(), "vault.age")
	var loaded []string
	if err = Read(path, 4096, identity, &loaded); err != nil {
		t.Fatal(err)
	}
	if err = Write(path, 4096, identity, []string{"secret"}); err != nil {
		t.Fatal(err)
	}
	if err = Read(path, 4096, identity, &loaded); err != nil || len(loaded) != 1 || loaded[0] != "secret" {
		t.Fatal(loaded, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") {
		t.Fatal("plaintext written")
	}
	broken := func(bool) (*age.X25519Identity, error) { return nil, errors.New("identity unavailable") }
	if Read(path, 4096, broken, &loaded) == nil || Write(path, 4096, broken, loaded) == nil {
		t.Fatal("identity failure ignored")
	}
	if Read(path, 1, identity, &loaded) == nil || Write(path, 1, identity, loaded) == nil {
		t.Fatal("size limit ignored")
	}
	if Write(path, 4096, identity, func() {}) == nil {
		t.Fatal("marshal failure ignored")
	}
	if err = os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if Read(path, 4096, identity, &loaded) == nil {
		t.Fatal("tampering accepted")
	}
	if err = Write(path, 4096, identity, map[string]string{"wrong": "shape"}); err != nil {
		t.Fatal(err)
	}
	if Read(path, 4096, identity, &loaded) == nil {
		t.Fatal("invalid shape accepted")
	}
}

type failingCipherWriter struct{ failWrite bool }

func (w failingCipherWriter) Write(p []byte) (int, error) {
	if w.failWrite {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}
func (failingCipherWriter) Close() error { return io.ErrClosedPipe }

func TestVaultNativeCryptoFailuresNeverPublishPlaintext(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	identity := func(bool) (*age.X25519Identity, error) { return id, nil }
	path := filepath.Join(t.TempDir(), "vault.age")
	if err = os.WriteFile(path, []byte("existing ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"encrypt", "write", "close", "decrypt-read", "plain-limit"} {
		t.Run(scenario, func(t *testing.T) {
			old := cryptography
			t.Cleanup(func() { cryptography = old })
			switch scenario {
			case "encrypt":
				cryptography.encrypt = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) { return nil, io.ErrClosedPipe }
			case "write", "close":
				cryptography.encrypt = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) {
					return failingCipherWriter{scenario == "write"}, nil
				}
			case "decrypt-read":
				cryptography.decrypt = func(io.Reader, ...age.Identity) (io.Reader, error) { return badCipherReader{}, nil }
			case "plain-limit":
				cryptography.decrypt = func(io.Reader, ...age.Identity) (io.Reader, error) {
					return strings.NewReader(strings.Repeat("x", 100)), nil
				}
			}
			var err error
			if strings.HasPrefix(scenario, "decrypt") || scenario == "plain-limit" {
				var value any
				err = Read(path, 64, identity, &value)
			} else {
				err = Write(path, 64, identity, []string{"private"})
			}
			if err == nil {
				t.Fatal("crypto failure ignored")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "existing ciphertext" {
				t.Fatal("failed vault operation published data", err)
			}
		})
	}
	if Write(path, 32, identity, []string{"small"}) == nil {
		t.Fatal("ciphertext limit ignored")
	}
	var value any
	if Read(filepath.Dir(path), 64, identity, &value) == nil {
		t.Fatal("native read failure ignored")
	}
}

type badCipherReader struct{}

func (badCipherReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }
