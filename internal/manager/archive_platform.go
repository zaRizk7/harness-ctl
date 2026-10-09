package manager

import (
	"archive/tar"
	"crypto/rand"
	"io"
	"io/fs"

	"filippo.io/age"
)

// archiveIO isolates native archive IO for deterministic failure and rollback tests.
// These operations stay private and cannot be replaced by user configuration.
var archiveIO = struct {
	entropy     func([]byte) (int, error)
	generate    func() (*age.X25519Identity, error)
	encrypt     func(io.Writer, ...age.Recipient) (io.WriteCloser, error)
	decrypt     func(io.Reader, ...age.Identity) (io.Reader, error)
	header      func(fs.FileInfo, string) (*tar.Header, error)
	tarHeader   func(*tar.Writer, *tar.Header) error
	tarWrite    func(*tar.Writer, []byte) (int, error)
	tarClose    func(*tar.Writer) error
	cipherClose func(io.WriteCloser) error
	copyN       func(io.Writer, io.Reader, int64) (int64, error)
	copy        func(io.Writer, io.Reader) (int64, error)
}{rand.Read, age.GenerateX25519Identity, age.Encrypt, age.Decrypt, tar.FileInfoHeader, (*tar.Writer).WriteHeader, (*tar.Writer).Write, (*tar.Writer).Close, io.WriteCloser.Close, io.CopyN, io.Copy}
