package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
