// Package vault authenticates bounded encrypted JSON storage before decoding it.
package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"github.com/zaRizk7/harness-ctl/internal/storage"
)

// Identity resolves the local encryption identity, creating it only on writes.
type Identity func(bool) (*age.X25519Identity, error)

// cryptography keeps native failures testable without exposing runtime overrides.
var cryptography = struct {
	encrypt func(io.Writer, ...age.Recipient) (io.WriteCloser, error)
	decrypt func(io.Reader, ...age.Identity) (io.Reader, error)
}{age.Encrypt, age.Decrypt}

// Read decrypts path into value. Missing storage leaves value untouched.
// Authentication errors reveal no ciphertext, plaintext or identity material.
func Read(path string, limit int64, identity Identity, value any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("vault exceeds configured size limit")
	}
	id, err := identity(false)
	if err != nil {
		return err
	}
	reader, err := cryptography.decrypt(bytes.NewReader(data), id)
	if err != nil {
		return fmt.Errorf("vault authentication failed")
	}
	plain, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return fmt.Errorf("vault authentication failed")
	}
	if int64(len(plain)) > limit {
		return fmt.Errorf("vault exceeds configured size limit")
	}
	if json.Unmarshal(plain, value) != nil {
		return fmt.Errorf("invalid vault document")
	}
	return nil
}

// Write encrypts value before atomically publishing private storage. The caller
// owns path validation and the mutation lock shared by all vault writers.
func Write(path string, limit int64, identity Identity, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("vault exceeds configured size limit")
	}
	id, err := identity(true)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	writer, err := cryptography.encrypt(&buf, id.Recipient())
	if err != nil {
		return err
	}
	if _, err = writer.Write(data); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	if int64(buf.Len()) > limit {
		return fmt.Errorf("encrypted vault exceeds configured size limit")
	}
	return storage.AtomicWrite(path, buf.Bytes(), 0600)
}
