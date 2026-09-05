// Package quarantine manages the on-disk store of infected files: it moves a
// file out of harm's way (unreadable, under the app's state dir), can move it
// back, and can destroy it. Each blob has a JSON sidecar recording where it
// came from.
package quarantine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrOriginExists is returned by Restore when the original path is occupied.
var ErrOriginExists = errors.New("quarantine: original path already exists")

// Store is a directory holding quarantined blobs and their sidecars.
type Store struct {
	dir string
}

// NewStore returns a Store rooted at dir, creating it (0700) if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("quarantine: create %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Sidecar is the metadata stored next to each blob as <name>.json.
type Sidecar struct {
	Name      string `json:"name"`
	OrigPath  string `json:"orig_path"`
	OrigMode  uint32 `json:"orig_mode"`
	Signature string `json:"signature"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	MovedAt   string `json:"moved_at"`
}

// Hold moves origPath into the store. The original is chmod 0000 first so it is
// inert even if the move degrades to a copy across filesystems. Returns the
// sidecar describing the stored blob.
func (s *Store) Hold(origPath, signature string) (Sidecar, error) {
	abs, err := filepath.Abs(origPath)
	if err != nil {
		return Sidecar{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Sidecar{}, fmt.Errorf("quarantine: stat %s: %w", abs, err)
	}
	if !info.Mode().IsRegular() {
		return Sidecar{}, fmt.Errorf("quarantine: %s is not a regular file", abs)
	}

	sum, err := fileSHA256(abs)
	if err != nil {
		return Sidecar{}, err
	}

	name, err := randomName()
	if err != nil {
		return Sidecar{}, err
	}
	dst := filepath.Join(s.dir, name)

	// Neutralise before moving.
	_ = os.Chmod(abs, 0)

	if err := moveFile(abs, dst); err != nil {
		_ = os.Chmod(abs, info.Mode().Perm()) // best-effort undo
		return Sidecar{}, err
	}
	_ = os.Chmod(dst, 0o600)

	sc := Sidecar{
		Name:      name,
		OrigPath:  abs,
		OrigMode:  uint32(info.Mode().Perm()),
		Signature: signature,
		SHA256:    sum,
		Size:      info.Size(),
		MovedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.writeSidecar(sc); err != nil {
		return Sidecar{}, err
	}
	return sc, nil
}

// Restore moves the blob back to its original path and restores its mode. It
// refuses to overwrite an existing file.
func (s *Store) Restore(name, origPath string, mode uint32) error {
	src := filepath.Join(s.dir, name)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("quarantine: %s not in store: %w", name, err)
	}
	if _, err := os.Lstat(origPath); err == nil {
		return ErrOriginExists
	}
	if err := os.MkdirAll(filepath.Dir(origPath), 0o755); err != nil {
		return err
	}
	if err := moveFile(src, origPath); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	_ = os.Chmod(origPath, os.FileMode(mode))
	_ = os.Remove(s.sidecarPath(name))
	return nil
}

// Purge overwrites the blob once and removes it along with its sidecar.
func (s *Store) Purge(name string) error {
	blob := filepath.Join(s.dir, name)
	if info, err := os.Stat(blob); err == nil {
		if f, err := os.OpenFile(blob, os.O_WRONLY, 0); err == nil {
			_, _ = io.CopyN(f, zeroReader{}, info.Size())
			_ = f.Sync()
			_ = f.Close()
		}
	}
	err := os.Remove(blob)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(s.sidecarPath(name))
	return nil
}

// BlobPath returns the absolute path of a stored blob (used when offering a
// download later).
func (s *Store) BlobPath(name string) string { return filepath.Join(s.dir, name) }

func (s *Store) sidecarPath(name string) string { return filepath.Join(s.dir, name+".json") }

func (s *Store) writeSidecar(sc Sidecar) error {
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.sidecarPath(sc.Name), b, 0o600)
}

func randomName() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// moveFile renames src to dst, falling back to copy+remove across filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
