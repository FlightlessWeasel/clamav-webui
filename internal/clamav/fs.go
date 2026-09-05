package clamav

import "os"

// FS is the slice of the filesystem the ClamAV layer needs: reading signature
// DB metadata and the two ClamAV config files. Abstracted so the simulator and
// tests can supply their own.
type FS interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
}

type osFS struct{}

func (osFS) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }
func (osFS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
