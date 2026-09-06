package clamav

import "os"

// FS is the slice of the filesystem the ClamAV layer needs: reading signature
// DB metadata and reading/patching the two ClamAV config files. Abstracted so
// the simulator and tests can supply their own.
type FS interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
	RemoveAll(path string) error
}

type osFS struct{}

func (osFS) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }
func (osFS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osFS) Remove(name string) error                     { return os.Remove(name) }
func (osFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
