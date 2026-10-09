package manager

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// injectArchiveFailure replaces one private IO operation for the duration of a test.
func injectArchiveFailure(boundary string, nth int, count *int) func() {
	old := archiveIO
	oldFiles := fileIO
	fail := func() bool { *count++; return nth > 0 && *count == nth }
	fault := errors.New("fixture archive IO failure")
	switch boundary {
	case "stat":
		fileIO.stat = func(path string) (os.FileInfo, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.stat(path)
		}
	case "eval":
		fileIO.eval = func(path string) (string, error) {
			if fail() {
				return "", fault
			}
			return oldFiles.eval(path)
		}
	case "lstat":
		fileIO.lstat = func(path string) (os.FileInfo, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.lstat(path)
		}
	case "mkdir":
		fileIO.mkdir = func(path string, mode os.FileMode) error {
			if fail() {
				return fault
			}
			return oldFiles.mkdir(path, mode)
		}
	case "create":
		fileIO.create = func(dir, pattern string) (*os.File, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.create(dir, pattern)
		}
	case "mkdirTemp":
		fileIO.mkdirTemp = func(dir, pattern string) (string, error) {
			if fail() {
				return "", fault
			}
			return oldFiles.mkdirTemp(dir, pattern)
		}
	case "chmod":
		fileIO.chmod = func(f *os.File, mode os.FileMode) error {
			if fail() {
				return fault
			}
			return oldFiles.chmod(f, mode)
		}
	case "close":
		fileIO.close = func(f *os.File) error {
			err := oldFiles.close(f)
			if fail() {
				return fault
			}
			return err
		}
	case "sync":
		fileIO.sync = func(f *os.File) error {
			if fail() {
				return fault
			}
			return oldFiles.sync(f)
		}
	case "seek":
		fileIO.seek = func(f *os.File, offset int64, whence int) (int64, error) {
			if fail() {
				return 0, fault
			}
			return oldFiles.seek(f, offset, whence)
		}
	case "walk":
		fileIO.walk = func(path string, visit fs.WalkDirFunc) error {
			if fail() {
				return fault
			}
			return oldFiles.walk(path, visit)
		}
	case "info":
		fileIO.info = func(entry fs.DirEntry) (fs.FileInfo, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.info(entry)
		}
	case "rel":
		fileIO.rel = func(base, path string) (string, error) {
			if fail() {
				return "", fault
			}
			return oldFiles.rel(base, path)
		}
	case "readlink":
		fileIO.readlink = func(path string) (string, error) {
			if fail() {
				return "", fault
			}
			return oldFiles.readlink(path)
		}
	case "header":
		archiveIO.header = func(info fs.FileInfo, link string) (*tar.Header, error) {
			if fail() {
				return nil, fault
			}
			return old.header(info, link)
		}
	case "tarHeader":
		archiveIO.tarHeader = func(w *tar.Writer, h *tar.Header) error {
			if fail() {
				return fault
			}
			return old.tarHeader(w, h)
		}
	case "tarWrite":
		archiveIO.tarWrite = func(w *tar.Writer, data []byte) (int, error) {
			if fail() {
				return 0, fault
			}
			return old.tarWrite(w, data)
		}
	case "tarClose":
		archiveIO.tarClose = func(w *tar.Writer) error {
			err := old.tarClose(w)
			if fail() {
				return fault
			}
			return err
		}
	case "cipherClose":
		archiveIO.cipherClose = func(w io.WriteCloser) error {
			err := old.cipherClose(w)
			if fail() {
				return fault
			}
			return err
		}
	case "open":
		fileIO.open = func(path string) (*os.File, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.open(path)
		}
	case "openFile":
		fileIO.openFile = func(path string, flags int, mode os.FileMode) (*os.File, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.openFile(path, flags, mode)
		}
	case "copyN":
		archiveIO.copyN = func(w io.Writer, r io.Reader, n int64) (int64, error) {
			if fail() {
				return 0, fault
			}
			return old.copyN(w, r, n)
		}
	case "copy":
		archiveIO.copy = func(w io.Writer, r io.Reader) (int64, error) {
			if fail() {
				return 0, fault
			}
			return old.copy(w, r)
		}
	case "rename":
		fileIO.rename = func(source, target string) error {
			if fail() {
				return fault
			}
			return oldFiles.rename(source, target)
		}
	case "remove":
		fileIO.remove = func(path string) error {
			if fail() {
				return fault
			}
			return oldFiles.remove(path)
		}
	case "removeAll":
		fileIO.removeAll = func(path string) error {
			if fail() {
				return fault
			}
			return oldFiles.removeAll(path)
		}
	case "readFile":
		fileIO.readFile = func(path string) ([]byte, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.readFile(path)
		}
	case "readDir":
		fileIO.readDir = func(path string) ([]os.DirEntry, error) {
			if fail() {
				return nil, fault
			}
			return oldFiles.readDir(path)
		}
	case "symlink":
		fileIO.symlink = func(source, target string) error {
			if fail() {
				return fault
			}
			return oldFiles.symlink(source, target)
		}
	}
	return func() { archiveIO = old; fileIO = oldFiles }
}

// archiveFaultFixture creates only synthetic owned recovery data, including a link.
func archiveFaultFixture(t *testing.T) (*engine, *plan, string) {
	t.Helper()
	e, _ := testEngine(t)
	s, _ := e.specFor("pi")
	inst := syntheticInstall(t, e, s, "1")
	e.reg.Installs = []installation{inst}
	if err := writeJSON(e.statePath, e.reg); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(inst.StateRoot, "nested", "auth.json")
	if err := atomicWrite(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("auth.json", filepath.Join(filepath.Dir(file), "linked")); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(e.cfg.Home, "unrelated", "keep")
	if err := atomicWrite(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := e.buildPlan(context.Background(), request{Harness: "pi", InstallID: inst.ID, Action: "reset", Preserve: keepAll()})
	if err != nil {
		t.Fatal(err)
	}
	return e, p, marker
}

func TestArchiveIOFailuresRetainUnrelatedStateAndRecovery(t *testing.T) {
	for _, kind := range []string{"capture", "restore"} {
		for _, boundary := range []string{"lstat", "mkdir", "create", "mkdirTemp", "chmod", "close", "sync", "seek", "walk", "info", "rel", "readlink", "header", "tarHeader", "tarWrite", "tarClose", "cipherClose", "open", "openFile", "copyN", "copy", "rename", "remove", "removeAll", "readFile", "readDir", "symlink"} {
			t.Run(kind+"/"+boundary, func(t *testing.T) {
				prepare := func() (*engine, func() error, string, string) {
					e, p, marker := archiveFaultFixture(t)
					id := ""
					if kind == "capture" {
						return e, func() error { _, err := e.snapshot(context.Background(), p); return err }, marker, id
					}
					meta, err := e.snapshot(context.Background(), p)
					if err != nil {
						t.Fatal(err)
					}
					id = meta.ID
					_ = atomicWrite(filepath.Join(p.StateRoot, "nested", "auth.json"), []byte("changed"), 0600)
					return e, func() error { return e.restoreSnapshot(context.Background(), id) }, marker, id
				}
				_, operation, _, _ := prepare()
				count := 0
				restore := injectArchiveFailure(boundary, 0, &count)
				err := operation()
				restore()
				if err != nil {
					t.Fatal("baseline", err)
				}
				total := count
				for nth := 1; nth <= total; nth++ {
					t.Run(fmt.Sprint(nth), func(t *testing.T) {
						e, operation, marker, id := prepare()
						count := 0
						restore := injectArchiveFailure(boundary, nth, &count)
						err := operation()
						restore()
						if count < nth {
							t.Fatal("failure boundary not reached")
						}
						data, readErr := os.ReadFile(marker)
						if readErr != nil || string(data) != "keep" {
							t.Fatal("unrelated state changed", readErr)
						}
						if err = e.refreshRegistry(); err != nil {
							t.Fatal("invalid registry retained", err)
						}
						if id != "" {
							if _, authErr := e.authenticatedSnapshot(id); authErr != nil {
								t.Fatal("IO failure destroyed authenticated recovery", authErr)
							}
						}
					})
				}
			})
		}
	}
}
