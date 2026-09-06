package clamav

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"
)

// MountConfig controls whether disk-image scan targets (.iso and friends) are
// expanded to their contents before scanning, instead of handed to clamd as one
// opaque blob. clamd/clamscan will not read a multi-GB image past its size
// limits, so a raw scan of a large image returns "clean" almost instantly
// without inspecting anything; the individual files inside are what matter.
//
// The image is loop-mounted read-only when possible, and otherwise extracted to
// a scratch directory (ExtractDir) — the fallback for environments where a loop
// mount is not permitted, e.g. an unprivileged container.
type MountConfig struct {
	Enabled    bool     `json:"enabled"`
	Extensions []string `json:"extensions"`
	// ExtractDir is the scratch root for the extract fallback. Empty means "let
	// the caller pick" (it uses a directory under the app state dir). Extraction
	// copies the whole image, so this should have room for the largest one.
	ExtractDir string `json:"extract_dir"`
}

// DefaultMountConfig is the off-by-default configuration. Mounting an untrusted
// filesystem image runs kernel filesystem drivers as root, so it is opt-in.
func DefaultMountConfig() MountConfig {
	return MountConfig{Enabled: false, Extensions: []string{".iso", ".udf", ".img"}}
}

// normalizedExts returns the configured extensions trimmed, lower-cased, and
// dot-prefixed, with blanks dropped.
func (c MountConfig) normalizedExts() []string {
	out := make([]string, 0, len(c.Extensions))
	for _, e := range c.Extensions {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out = append(out, e)
	}
	return out
}

// Sanitized returns cfg with its extension list normalized (see normalizedExts)
// and de-duplicated, and ExtractDir trimmed. The Extensions slice is non-nil.
func (c MountConfig) Sanitized() MountConfig {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range c.normalizedExts() {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return MountConfig{Enabled: c.Enabled, Extensions: out, ExtractDir: strings.TrimSpace(c.ExtractDir)}
}

// IsImage reports whether p ends in one of the configured image extensions.
func (c MountConfig) IsImage(p string) bool {
	lp := strings.ToLower(p)
	for _, e := range c.normalizedExts() {
		if strings.HasSuffix(lp, e) {
			return true
		}
	}
	return false
}

// PreparedImage records one image target that was expanded to a directory of
// its contents, and how — so cleanup knows whether to unmount or just delete.
type PreparedImage struct {
	Image     string // original image path, e.g. /games/x.iso
	Dir       string // directory holding its contents
	Extracted bool   // true = a copy to rm -rf; false = a loop mount to unmount
}

// imageMountOpts keeps an untrusted image from contributing devices, setuid
// bits, or executables to the host, and mounts it read-only.
const imageMountOpts = "loop,ro,nodev,nosuid,noexec"

// imageFSTypes is the list of `mount -t` values to try in order. The empty
// string lets mount(8) auto-detect; the rest cover images it cannot.
var imageFSTypes = []string{"", "udf", "iso9660"}

// PrepareImages expands every entry in targets that looks like a disk image
// (per cfg) into a directory of its contents: a read-only loop mount under
// mountBase, or — if mounting fails — an extraction under extractBase. It
// returns the rewritten target list (each image swapped for its directory), the
// images that were prepared, and a cleanup func.
//
// An image that can be neither mounted nor extracted is left in the list so the
// raw blob is still scanned; the reason goes to logf. cleanup unmounts every
// mount, deletes every extraction, and removes both base dirs. It is always
// safe to call and should be deferred by the caller.
func (m *Manager) PrepareImages(ctx context.Context, targets []string, cfg MountConfig, mountBase, extractBase string, logf func(string)) (paths []string, prepared []PreparedImage, cleanup func()) {
	cleanup = func() {}
	if !cfg.Enabled {
		return targets, nil, cleanup
	}
	if logf == nil {
		logf = func(string) {}
	}

	out := make([]string, 0, len(targets))
	for i, p := range targets {
		if !cfg.IsImage(p) {
			out = append(out, p)
			continue
		}

		mdir := path.Join(mountBase, strconv.Itoa(i))
		mErr := m.fsys.MkdirAll(mdir, 0o700)
		if mErr == nil {
			mErr = m.mountImage(ctx, p, mdir)
		}
		if mErr == nil {
			logf("mounted " + p + " read-only at " + mdir)
			prepared = append(prepared, PreparedImage{Image: p, Dir: mdir})
			out = append(out, mdir)
			continue
		}
		_ = m.fsys.Remove(mdir)
		logf("mount " + p + ": " + mErr.Error())

		edir := path.Join(extractBase, strconv.Itoa(i))
		if err := m.extractImage(ctx, p, edir, logf); err != nil {
			_ = m.fsys.RemoveAll(edir)
			logf("extract " + p + ": " + err.Error() + " (scanning the raw image instead)")
			out = append(out, p)
			continue
		}
		prepared = append(prepared, PreparedImage{Image: p, Dir: edir, Extracted: true})
		out = append(out, edir)
	}

	done := prepared
	cleanup = func() {
		for _, pi := range done {
			if pi.Extracted {
				_ = m.fsys.RemoveAll(pi.Dir)
			} else {
				m.unmount(context.WithoutCancel(ctx), pi.Dir)
			}
		}
		_ = m.fsys.RemoveAll(mountBase)
		if extractBase != mountBase {
			_ = m.fsys.RemoveAll(extractBase)
		}
	}
	return out, prepared, cleanup
}

// mountImage tries each filesystem type in turn, returning nil on the first
// success. If none work it returns an error carrying mount(8)'s own message(s)
// for each distinct failure — "exit status 32" alone tells the operator
// nothing.
func (m *Manager) mountImage(ctx context.Context, image, dir string) error {
	seen := map[string]bool{}
	var msgs []string
	for _, fstype := range imageFSTypes {
		args := make([]string, 0, 6)
		if fstype != "" {
			args = append(args, "-t", fstype)
		}
		args = append(args, "-o", imageMountOpts, image, dir)
		res, err := m.run.Run(ctx, Cmd{Name: "mount", Args: args})
		if err == nil {
			return nil
		}
		if msg := shortMountErr(err, res.Stderr); !seen[msg] {
			seen[msg] = true
			msgs = append(msgs, msg)
		}
	}
	return errors.New(strings.Join(msgs, "; "))
}

// extractImage unpacks a disk image into dir. 7-Zip (7zz, then 7z) handles UDF
// as well as ISO9660/Joliet; bsdtar (libarchive) is the ISO9660-only fallback.
// Output is streamed to logf so a long extraction is visible in the job log.
func (m *Manager) extractImage(ctx context.Context, image, dir string, logf func(string)) error {
	var cmd Cmd
	switch {
	case m.has("7zz"):
		cmd = Cmd{Name: "7zz", Args: []string{"x", "-bd", "-y", "-o" + dir, image}}
	case m.has("7z"):
		cmd = Cmd{Name: "7z", Args: []string{"x", "-bd", "-y", "-o" + dir, image}}
	case m.has("bsdtar"):
		cmd = Cmd{Name: "bsdtar", Args: []string{"-x", "-f", image, "-C", dir}}
	default:
		return errors.New("need 7zz/7z (7-Zip, reads UDF) or bsdtar to extract a disk image without mounting")
	}
	if err := m.fsys.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	logf("extracting " + image + " with " + cmd.Name + " — this copies the whole image and can take a while")
	err := m.run.Stream(ctx, cmd, logf)
	if err == nil {
		return nil
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return errors.New(cmd.Name + " exited " + strconv.Itoa(ee.Code))
	}
	return err
}

func (m *Manager) has(name string) bool {
	_, ok := m.run.LookPath(name)
	return ok
}

// shortMountErr reduces a failed mount to its first, most useful line: mount(8)
// writes the real reason ("unknown filesystem type", "failed to setup loop
// device: ...", "wrong fs type, bad option, bad superblock ...") to stderr and
// only signals it through a generic exit code.
func shortMountErr(err error, stderr []byte) string {
	s := strings.TrimSpace(string(stderr))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.TrimPrefix(s, "mount: ")
	if s == "" {
		return err.Error()
	}
	return s
}

// unmount detaches dir and removes the (now empty) mountpoint. A busy mount —
// a scanner still holding a handle, or a slow backing device — is detached
// lazily so cleanup never blocks the worker.
func (m *Manager) unmount(ctx context.Context, dir string) {
	if _, err := m.run.Run(ctx, Cmd{Name: "umount", Args: []string{dir}}); err != nil {
		_, _ = m.run.Run(ctx, Cmd{Name: "umount", Args: []string{"-l", dir}})
	}
	_ = m.fsys.Remove(dir)
}

// RelabelPath rewrites any reference to a prepared image's contents back to the
// "<image>!/<path within image>" shape ClamAV uses for archive members. It is
// used both on a bare finding path and on a whole scanner output line; a string
// with no prepared-dir prefix is returned unchanged.
func RelabelPath(s string, prepared []PreparedImage) string {
	for _, pi := range prepared {
		s = strings.ReplaceAll(s, pi.Dir+"/", pi.Image+"!/")
		s = strings.ReplaceAll(s, pi.Dir, pi.Image)
	}
	return s
}

// UnmountLeftovers unmounts anything still mounted under any of baseDirs from a
// previous process (a crash between prepare and cleanup) and removes each
// baseDir. Best effort; intended to be called once at startup.
func (m *Manager) UnmountLeftovers(ctx context.Context, baseDirs ...string) {
	if b, err := m.fsys.ReadFile("/proc/self/mounts"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			// Our mountpoints never contain spaces, so mount(8)'s \040 escaping
			// does not matter and a plain prefix check is enough.
			mp := fields[1]
			for _, base := range baseDirs {
				if strings.HasPrefix(mp, strings.TrimRight(base, "/")+"/") {
					m.unmount(ctx, mp)
				}
			}
		}
	}
	for _, base := range baseDirs {
		if base != "" {
			_ = m.fsys.RemoveAll(base)
		}
	}
}
