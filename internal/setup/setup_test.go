package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) Options {
	t.Helper()
	home := t.TempDir()
	source := filepath.Join(home, "download")
	data := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(source, data, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return Options{Home: home, Root: filepath.Join(home, "manager"), Prefix: filepath.Join(home, "app"), LinkDir: filepath.Join(home, "bin"), Source: source, SHA256: hex.EncodeToString(hash[:]), MaxBytes: 1024, Files: map[string][]byte{"catalog.json": []byte("[]")}}
}

func TestVerifiedInstallAndApproval(t *testing.T) {
	o := fixture(t)
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	if err = Apply(p, "wrong"); err == nil {
		t.Fatal("unapproved install")
	}
	if _, err = os.Stat(o.Root); !os.IsNotExist(err) {
		t.Fatal("preview wrote storage")
	}
	if err = Apply(p, p.ID()); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(p.Link)
	if err != nil || target != p.Binary {
		t.Fatal(target, err)
	}
	if _, err = Build(o); err == nil {
		t.Fatal("overwrote installed binary")
	}
}
func TestSetupRejectsTamperingAndLinks(t *testing.T) {
	o := fixture(t)
	o.SHA256 = "wrong"
	if _, err := Build(o); err == nil {
		t.Fatal("bad checksum")
	}
	o = fixture(t)
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	p.Binary = filepath.Join(o.Home, "escaped")
	if Apply(p, p.ID()) == nil {
		t.Fatal("modified plan")
	}
	o = fixture(t)
	p, err = Build(o)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(o.Root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(o.Root, "catalog.json"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if Apply(p, p.ID()) == nil {
		t.Fatal("stale preview")
	}
	o = fixture(t)
	if err = os.Symlink(o.Home, o.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err = Build(o); err == nil {
		t.Fatal("linked prefix")
	}
}
