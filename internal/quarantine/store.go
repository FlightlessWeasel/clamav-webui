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

// ErrOriginParentGone is returned by Restore when the original directory no
// longer exists (we won't recreate it — wrong owner/mode is worse than failing).
var ErrOriginParentGone = errors.New("quarantine: original directory no longer exists")

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
	ScanID    int64  `json:"scan_id,omitempty"`
	Size      int64  `json:"size"`
	MovedAt   string `json:"moved_at"`
}

// Hold moves origPath into the store. It Lstats (rejecting a symlink leaf),
// opens, and confirms the opened descriptor still refers to that same inode —
// so a file swapped for a symlink between the checks is caught — then hashes
// and neutralises (fchmod 0) through that descriptor before the move.
func (s *Store) Hold(origPath, signature string, scanID int64) (Sidecar, error) {
	abs, err := filepath.Abs(origPath)
	if err != nil {
		return Sidecar{}, err
	}

	lst, err := os.Lstat(abs)
	if err != nil {
		return Sidecar{}, fmt.Errorf("quarantine: stat %s: %w", abs, err)
	}
	if !lst.Mode().IsRegular() {
		return Sidecar{}, fmt.Errorf("quarantine: %s is not a regular file", abs)
	}

	f, err := os.Open(abs)
	if err != nil {
		return Sidecar{}, fmt.Errorf("quarantine: open %s: %w", abs, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return Sidecar{}, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(lst, info) {
		f.Close()
		return Sidecar{}, fmt.Errorf("quarantine: %s changed while being read", abs)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return Sidecar{}, err
	}
	_ = f.Chmod(0) // fchmod: no path re-resolution
	f.Close()

	name, err := randomName()
	if err != nil {
		return Sidecar{}, err
	}
	dst := filepath.Join(s.dir, name)

	if err := moveFile(abs, dst); err != nil {
		_ = os.Chmod(abs, info.Mode().Perm()) // best-effort undo of the fchmod
		return Sidecar{}, err
	}
	_ = os.Chmod(dst, 0o600)

	sc := Sidecar{
		Name:      name,
		OrigPath:  abs,
		OrigMode:  uint32(info.Mode().Perm()),
		Signature: signature,
		SHA256:    hex.EncodeToString(h.Sum(nil)),
		ScanID:    scanID,
		Size:      info.Size(),
		MovedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.writeSidecar(sc); err != nil {
		return Sidecar{}, err
	}
	return sc, nil
}

// Restore moves the blob described by sc back to its original path and restores
// its mode. It refuses to overwrite an existing file and will not recreate a
// missing parent directory.
func (s *Store) Restore(sc Sidecar) error {
	src := filepath.Join(s.dir, sc.Name)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("quarantine: %s not in store: %w", sc.Name, err)
	}
	parent := filepath.Dir(sc.OrigPath)
	if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
		return ErrOriginParentGone
	}
	if _, err := os.Lstat(sc.OrigPath); err == nil {
		return ErrOriginExists
	}
	if err := moveFile(src, sc.OrigPath); err != nil {
		return err
	}
	mode := os.FileMode(sc.OrigMode)
	if mode == 0 {
		mode = 0o644
	}
	_ = os.Chmod(sc.OrigPath, mode)
	_ = os.Remove(s.sidecarPath(sc.Name))
	return nil
}

// Purge overwrites the blob with zeros once (best-effort; not a guaranteed
// secure erase on CoW/journaled/SSD storage) and removes it and its sidecar.
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

// SidecarFor loads the sidecar for a stored blob (used to rebuild a Sidecar
// from just the store name).
func (s *Store) SidecarFor(name string) (Sidecar, error) {
	b, err := os.ReadFile(s.sidecarPath(name))
	if err != nil {
		return Sidecar{}, err
	}
	var sc Sidecar
	return sc, json.Unmarshal(b, &sc)
}

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

// moveFile renames src to dst, falling back to copy+remove across filesystems.
// It never reports success while leaving src in place.
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
		os.Remove(dst)
		return err
	}
	if err := os.Remove(src); err != nil {
		os.Remove(dst) // don't leave a duplicate; the original is authoritative
		return fmt.Errorf("quarantine: copied but could not remove original %s: %w", src, err)
	}
	return nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
