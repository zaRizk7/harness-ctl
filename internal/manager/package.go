package manager

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (e *engine) stagePackage(ctx context.Context, p *plan) error {
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
	response, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("package download returned HTTP %d", response.StatusCode)
	}
	if err = validateOwnedPath(e.cfg.Root, p.Artifact); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p.Artifact), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p.Artifact), ".package-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	count, err := io.Copy(io.MultiWriter(f, digest), io.LimitReader(response.Body, e.cfg.PackageBytes+1))
	if err != nil {
		f.Close()
		return err
	}
	if count > e.cfg.PackageBytes {
		f.Close()
		return fmt.Errorf("package exceeds configured size limit")
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expected) != 1 {
		f.Close()
		return fmt.Errorf("package integrity differs from preview")
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p.Artifact)
}
