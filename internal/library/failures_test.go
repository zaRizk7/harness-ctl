package library

import (
	"io"
	"io/fs"
	"os"
	"testing"
)

func TestCaptureFailureBoundariesRejectPartialAssets(t *testing.T) {
	for _, boundary := range []string{"info", "read", "rel", "growth"} {
		t.Run(boundary, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			source := t.TempDir()
			item := Item{ID: "a", Source: source, Targets: map[string]Target{"pi": {Path: "skills/a"}}}
			if err := os.WriteFile(source+"/SKILL.md", []byte("small"), 0600); err != nil {
				t.Fatal(err)
			}
			switch boundary {
			case "info":
				platform.info = func(fs.DirEntry) (fs.FileInfo, error) { return nil, io.ErrClosedPipe }
			case "read":
				platform.readFile = func(string) ([]byte, error) { return nil, io.ErrClosedPipe }
			case "rel":
				platform.rel = func(string, string) (string, error) { return "", io.ErrClosedPipe }
			case "growth":
				platform.readFile = func(string) ([]byte, error) { return make([]byte, 65), nil }
			}
			if _, err := Capture(item, 64); err == nil {
				t.Fatal("incomplete assets captured")
			}
		})
	}
}

func TestCaptureRejectsAmbiguousSourceAndLinkedAncestors(t *testing.T) {
	item := Item{ID: "a", Source: t.TempDir(), Files: map[string][]byte{"SKILL.md": nil}, Targets: map[string]Target{"pi": {Path: "skills/a"}}}
	if _, err := Capture(item, 64); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	item.Files = nil
	old := platform
	t.Cleanup(func() { platform = old })
	platform.linked = func(string) error { return io.ErrClosedPipe }
	if _, err := Capture(item, 64); err == nil {
		t.Fatal("unverified source accepted")
	}
	item.Source = ""
	item.Files = map[string][]byte{".": nil}
	if _, err := Capture(item, 64); err == nil {
		t.Fatal("invalid captured asset accepted")
	}
}
