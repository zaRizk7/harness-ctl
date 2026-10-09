package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageGateCombinesDuplicatePackageBlocks(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "partial"}[complete], func(t *testing.T) {
			root := t.TempDir()
			fixture := filepath.Join(root, "fixture.cover")
			count := "1"
			if !complete {
				count = "0"
			}
			profile := "mode: atomic\nexample/file.go:1.1,2.2 2 1\nexample/file.go:3.1,4.2 3 0\nexample/file.go:1.1,2.2 2 0\nexample/file.go:3.1,4.2 3 " + count + "\n"
			if err := os.WriteFile(fixture, []byte(profile), 0600); err != nil {
				t.Fatal(err)
			}
			fakeGo := "#!/bin/sh\nif [ \"$1\" = test ]; then\nfor arg in \"$@\"; do case \"$arg\" in -coverprofile=*) cp \"$COVERAGE_FIXTURE\" \"${arg#-coverprofile=}\" ;; esac; done\nfi\n"
			if err := os.WriteFile(filepath.Join(root, "go"), []byte(fakeGo), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", filepath.Join("..", "..", "scripts", "coverage-go.sh"))
			cmd.Env = append(os.Environ(), "PATH="+root+":/usr/bin:/bin", "COVERAGE_FIXTURE="+fixture)
			out, err := cmd.CombinedOutput()
			if (err == nil) != complete {
				t.Fatal(string(out), err)
			}
			if complete && !strings.Contains(string(out), "Covered 5 of 5 statements") {
				t.Fatal(string(out))
			}
			if !complete && !strings.Contains(string(out), "Covered 2 of 5 statements") {
				t.Fatal(string(out))
			}
		})
	}
}
