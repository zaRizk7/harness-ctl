package manager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestCancellationAfterInstallerGroupExited(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := stopProcessGroup(cmd.Process.Pid); !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
}

func TestCancellationStopsInstallerChildrenBeforeReturning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "late-write")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := (systemRunner{}).Run(ctx, command{Path: "/bin/sh", Args: []string{"-c", "(/bin/sleep 0.4; printf late > " + shellQuote(path) + ") & wait"}, Description: "synthetic installer"})
	if err == nil {
		t.Fatal("cancelled installer reported success")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("installer child continued writing after cancellation")
	}
}
