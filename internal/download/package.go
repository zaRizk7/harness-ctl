package download

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/storage"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Options binds an artifact destination, public HTTPS URL, publisher integrity
// and maximum bytes to the caller's approved plan. Root owns Artifact.
type Options struct {
	Root, Artifact, PackageURL, Integrity string
	MaxBytes                              int64
}

// platform isolates publication failures for deterministic integrity/cleanup proof.
var platform = struct {
	validate func(string, string) error
	mkdir    func(string, os.FileMode) error
	create   func(string, string) (*os.File, error)
	copy     func(io.Writer, io.Reader) (int64, error)
	sync     func(*os.File) error
	close    func(*os.File) error
	rename   func(string, string) error
}{storage.ValidateOwnedPath, os.MkdirAll, os.CreateTemp, io.Copy, (*os.File).Sync, (*os.File).Close, os.Rename}

// Stage downloads p's package with ctx and client, verifying the exact previewed
// integrity before publishing its private artifact.
func Stage(ctx context.Context, client Client, p Options) error {
	algorithm, encoded, ok := strings.Cut(p.Integrity, "-")
	if !ok {
		return fmt.Errorf("invalid package integrity")
	}
	var digest hash.Hash
	switch algorithm {
	case "sha512":
		digest = sha512.New()
	case "sha256":
		digest = sha256.New()
	default:
		return fmt.Errorf("unsupported package integrity algorithm")
	}
	expected, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.PackageURL, nil)
	if err != nil {
		return err
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("package downloads require HTTPS")
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("package download returned HTTP %d", response.StatusCode)
	}
	if err = platform.validate(p.Root, p.Artifact); err != nil {
		return err
	}
	if err = platform.mkdir(filepath.Dir(p.Artifact), 0700); err != nil {
		return err
	}
	f, err := platform.create(filepath.Dir(p.Artifact), ".package-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	count, err := platform.copy(io.MultiWriter(f, digest), io.LimitReader(response.Body, p.MaxBytes+1))
	if err != nil {
		platform.close(f)
		return err
	}
	if count > p.MaxBytes {
		platform.close(f)
		return fmt.Errorf("package exceeds configured size limit")
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expected) != 1 {
		platform.close(f)
		return fmt.Errorf("package integrity differs from preview")
	}
	if err = platform.sync(f); err != nil {
		platform.close(f)
		return err
	}
	if err = platform.close(f); err != nil {
		return err
	}
	return platform.rename(f.Name(), p.Artifact)
}
