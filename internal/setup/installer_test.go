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
