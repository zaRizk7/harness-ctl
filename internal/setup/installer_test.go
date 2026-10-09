package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellInstallerVerifiesBeforeExecutingAndPassesSetupOptions(t *testing.T) {
	home := t.TempDir()
	tools := filepath.Join(home, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_LOG\"\n")
	source := filepath.Join(home, "payload")
	if err := os.WriteFile(source, payload, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"uname": "#!/bin/sh\ncase \"$1\" in -s) echo Darwin ;; -m) echo arm64 ;; esac\n", "curl": "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = --output ]; then cp \"$TEST_SOURCE\" \"$2\"; exit 0; fi; shift; done\nexit 1\n"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(home, "executed")
	hash := sha256.Sum256(payload)
	run := func(sum string) error {
		cmd := exec.Command("/bin/bash", "../../scripts/install.sh", "--url", "https://example.invalid/binary", "--sha256", sum, "--headless", "--yes", "--direct", "--root", filepath.Join(home, "manager"))
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+tools+":"+os.Getenv("PATH"), "TEST_SOURCE="+source, "TEST_LOG="+log)
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Log(string(data))
		}
		return err
	}
	if run(strings.Repeat("0", 64)) == nil {
		t.Fatal("bad checksum executed")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("binary ran before verification")
	}
	if err := run(hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "setup\n--binary\n") || !strings.Contains(string(data), "--headless\n--yes") || strings.Contains(string(data), "--link-dir") {
		t.Fatal(string(data))
	}
}

func TestPOSIXInstallerDiscoversVerifiedRelease(t *testing.T) {
	home := t.TempDir()
	tools := filepath.Join(home, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_LOG\"\n")
	source := filepath.Join(home, "payload")
	if err := os.WriteFile(source, payload, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	sums := filepath.Join(home, "sums")
	if err := os.WriteFile(sums, []byte(hex.EncodeToString(hash[:])+"  harness-ctl-darwin-amd64\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"uname": "#!/bin/sh\ncase \"$1\" in -s) echo Darwin ;; -m) echo x86_64 ;; esac\n",
		"curl":  "#!/bin/sh\nfor arg do url=$arg; done\nprintf '%s\\n' \"$url\" >> \"$TEST_URLS\"\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = --output ]; then case \"$url\" in */SHA256SUMS) cp \"$TEST_SUMS\" \"$2\" ;; *) cp \"$TEST_SOURCE\" \"$2\" ;; esac; exit 0; fi; shift; done\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	log, urls := filepath.Join(home, "executed"), filepath.Join(home, "urls")
	run := func(extra ...string) error {
		args := append([]string{"../../scripts/install.sh", "--headless", "--yes", "--prefix", filepath.Join(home, "private prefix")}, extra...)
		cmd := exec.Command("/bin/sh", args...)
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+tools+":"+os.Getenv("PATH"), "TEST_SOURCE="+source, "TEST_SUMS="+sums, "TEST_LOG="+log, "TEST_URLS="+urls, "HARNESS_CTL_BINARY_URL=", "HARNESS_CTL_SHA256=", "HARNESS_CTL_VERSION=", "HARNESS_CTL_REPOSITORY=")
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Log(string(data))
		}
		return err
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(urls)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "releases/latest/download/SHA256SUMS") || !strings.Contains(string(data), "harness-ctl-darwin-amd64") {
		t.Fatal(string(data))
	}
	data, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "--prefix\n"+filepath.Join(home, "private prefix")) || !strings.Contains(string(data), "--link-dir\n"+filepath.Join(home, ".local/bin")) {
		t.Fatal(string(data))
	}
	if err := run("--version", "v0.1.0"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(urls)
	if !strings.Contains(string(data), "releases/download/v0.1.0/SHA256SUMS") {
		t.Fatal(string(data))
	}
	if err := run("--version", "../../invalid"); err == nil {
		t.Fatal("unsafe release tag accepted")
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sums, []byte(strings.Repeat("0", 64)+"  harness-ctl-darwin-amd64\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Fatal("mismatched release executed")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("binary executed before verification")
	}
}
