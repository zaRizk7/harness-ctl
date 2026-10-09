package setup

import (
	"errors"
	"io"
	"os"
	"testing"
)

func TestBuildFilesystemFailuresHaveNoPublication(t *testing.T) {
	for _, boundary := range []string{"validate", "linked", "lstat", "open", "stat", "read", "close", "fingerprint"} {
		t.Run(boundary, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			calls := 0
			inject := func(nth int) {
				platform = old
				calls = 0
				fail := func() bool { calls++; return nth > 0 && calls == nth }
				fault := errors.New("fixture IO failure")
				switch boundary {
				case "validate":
					platform.validate = func(a, b string) error {
						if fail() {
							return fault
						}
						return old.validate(a, b)
					}
				case "linked":
					platform.linked = func(p string) error {
						if fail() {
							return fault
						}
						return old.linked(p)
					}
				case "lstat":
					platform.lstat = func(p string) (os.FileInfo, error) {
						if fail() {
							return nil, fault
						}
						return old.lstat(p)
					}
				case "open":
					platform.open = func(p string) (*os.File, error) {
						if fail() {
							return nil, fault
						}
						return old.open(p)
					}
				case "stat":
					platform.stat = func(f *os.File) (os.FileInfo, error) {
						if fail() {
							return nil, fault
						}
						return old.stat(f)
					}
				case "read":
					platform.read = func(r io.Reader) ([]byte, error) {
						if fail() {
							return nil, fault
						}
						return old.read(r)
					}
				case "close":
					platform.close = func(f *os.File) error {
						err := old.close(f)
						if fail() {
							return fault
						}
						return err
					}
				case "fingerprint":
					platform.fingerprint = func(p string) (string, error) {
						if fail() {
							return "", fault
						}
						return old.fingerprint(p)
					}
				}
			}
			o := fixture(t)
			inject(0)
			if _, err := Build(o); err != nil {
				t.Fatal(err)
			}
			total := calls
			for nth := 1; nth <= total; nth++ {
				o := fixture(t)
				inject(nth)
				if _, err := Build(o); err == nil {
					t.Fatal("build ignored IO failure", nth)
				}
				if _, err := os.Stat(o.Root); !os.IsNotExist(err) {
					t.Fatal("failed preview published state", err)
				}
			}
		})
	}
}

func TestApplyFilesystemFailuresCleanPublishedPaths(t *testing.T) {
	for _, boundary := range []string{"linked", "fingerprint", "atomic", "mkdir", "symlink"} {
		t.Run(boundary, func(t *testing.T) {
			old := platform
			t.Cleanup(func() { platform = old })
			calls := 0
			inject := func(nth int) {
				platform = old
				calls = 0
				fail := func() bool { calls++; return nth > 0 && calls == nth }
				fault := errors.New("fixture apply failure")
				switch boundary {
				case "linked":
					platform.linked = func(p string) error {
						if fail() {
							return fault
						}
						return old.linked(p)
					}
				case "fingerprint":
					platform.fingerprint = func(p string) (string, error) {
						if fail() {
							return "", fault
						}
						return old.fingerprint(p)
					}
				case "atomic":
					platform.atomic = func(p string, data []byte, mode os.FileMode) error {
						if fail() {
							return fault
						}
						return old.atomic(p, data, mode)
					}
				case "mkdir":
					platform.mkdir = func(p string, mode os.FileMode) error {
						if fail() {
							return fault
						}
						return old.mkdir(p, mode)
					}
				case "symlink":
					platform.symlink = func(a, b string) error {
						if fail() {
							return fault
						}
						return old.symlink(a, b)
					}
				}
			}
			o := fixture(t)
			p, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			inject(0)
			if err = Apply(p, p.ID()); err != nil {
				t.Fatal(err)
			}
			total := calls
			for nth := 1; nth <= total; nth++ {
				platform = old
				o := fixture(t)
				p, err := Build(o)
				if err != nil {
					t.Fatal(err)
				}
				inject(nth)
				if err = Apply(p, p.ID()); err == nil {
					t.Fatal("apply ignored IO failure", nth)
				}
				for _, path := range p.Paths {
					if _, err = os.Lstat(path); !os.IsNotExist(err) {
						t.Fatal("failed install retained published path", path, err)
					}
				}
			}
		})
	}
}

func TestSetupDestinationAndSourceValidation(t *testing.T) {
	for _, change := range []func(*Options){func(o *Options) { o.Files = map[string][]byte{"../escape": nil} }, func(o *Options) { o.Prefix = "relative" }, func(o *Options) { o.Source = "missing" }, func(o *Options) { o.LinkDir = "relative" }, func(o *Options) { o.MaxBytes = 1 }, func(o *Options) { _ = os.Chmod(o.Source, 0600) }, func(o *Options) { o.Source = o.Home }, func(o *Options) {
		_ = os.MkdirAll(o.LinkDir, 0700)
		_ = os.WriteFile(o.LinkDir+"/harness-ctl", []byte("foreign"), 0600)
	}, func(o *Options) {
		_ = os.MkdirAll(o.Root, 0700)
		_ = os.WriteFile(o.Root+"/installation.json", []byte("foreign"), 0600)
	}} {
		o := fixture(t)
		change(&o)
		if _, err := Build(o); err == nil {
			t.Fatal("invalid setup accepted", o)
		}
	}
	o := fixture(t)
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	old := platform
	t.Cleanup(func() { platform = old })
	platform.symlink = func(string, string) error { return io.ErrClosedPipe }
	platform.remove = func(string) error { return io.ErrClosedPipe }
	if err = Apply(p, p.ID()); err == nil {
		t.Fatal("cleanup failure lost")
	}
}
