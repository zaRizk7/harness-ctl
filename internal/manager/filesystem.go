package manager

import (
	"io/fs"
	"os"
	"path/filepath"
)

// fileIO binds native filesystem operations shared by inventory, transactions
// and recovery. Keeping this private permits deterministic failure tests without
// making ownership rules or native operations configurable by users.
var fileIO = struct {
	lstat     func(string) (os.FileInfo, error)
	stat      func(string) (os.FileInfo, error)
	mkdir     func(string, os.FileMode) error
	create    func(string, string) (*os.File, error)
	mkdirTemp func(string, string) (string, error)
	chmod     func(*os.File, os.FileMode) error
	close     func(*os.File) error
	sync      func(*os.File) error
	seek      func(*os.File, int64, int) (int64, error)
	walk      func(string, fs.WalkDirFunc) error
	info      func(fs.DirEntry) (fs.FileInfo, error)
	rel       func(string, string) (string, error)
	readlink  func(string) (string, error)
	eval      func(string) (string, error)
	open      func(string) (*os.File, error)
	openFile  func(string, int, os.FileMode) (*os.File, error)
	rename    func(string, string) error
	remove    func(string) error
	removeAll func(string) error
	readFile  func(string) ([]byte, error)
	readDir   func(string) ([]os.DirEntry, error)
	symlink   func(string, string) error
}{os.Lstat, os.Stat, os.MkdirAll, os.CreateTemp, os.MkdirTemp, (*os.File).Chmod, (*os.File).Close, (*os.File).Sync, (*os.File).Seek, filepath.WalkDir, fs.DirEntry.Info, filepath.Rel, os.Readlink, filepath.EvalSymlinks, os.Open, os.OpenFile, os.Rename, os.Remove, os.RemoveAll, os.ReadFile, os.ReadDir, os.Symlink}
