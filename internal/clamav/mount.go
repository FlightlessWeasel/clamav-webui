package clamav

import (
	"context"
	"path"
	"strconv"
	"strings"
)

// MountConfig controls whether disk-image scan targets (.iso and friends) are
// loop-mounted and scanned by their contents instead of as one opaque blob.
// clamd/clamscan will not read a multi-GB image past its size limits, so a raw
// scan of a large image returns "clean" almost instantly without inspecting
// anything; mounting exposes the individual files, which are what matters.
type MountConfig struct {
	Enabled    bool     `json:"enabled"`
	Extensions []string `json:"extensions"`
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
// and de-duplicated. The returned Extensions slice is non-nil.
func (c MountConfig) Sanitized() MountConfig {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range c.normalizedExts() {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return MountConfig{Enabled: c.Enabled, Extensions: out}
}

// IsImage reports whether path ends in one of the configured image extensions.
func (c MountConfig) IsImage(p string) bool {
	lp := strings.ToLower(p)
	for _, e := range c.normalizedExts() {
		if strings.HasSuffix(lp, e) {
			return true
		}
	}
	return false
}

// MountedImage records one loop-mounted image and the directory its contents
// were mounted at.
type MountedImage struct {
	Image string // original image path, e.g. /games/x.iso
	Dir   string // mountpoint holding its contents
}

// imageMountOpts keeps an untrusted image from contributing devices, setuid
// bits, or executables to the host, and mounts it read-only.
const imageMountOpts = "loop,ro,nodev,nosuid,noexec"

// imageFSTypes is the list of `mount -t` values to try in order. The empty
// string lets mount(8) auto-detect; the rest cover images it cannot.
var imageFSTypes = []string{"", "udf", "iso9660"}

// MountImages loop-mounts every entry in targets that looks like a disk image
// (per cfg) at a fresh directory under baseDir, and returns: the rewritten
// target list (each image swapped for its mountpoint), the mounts that were
// made, and a cleanup func. An image that fails to mount is left untouched in
// the list so the raw blob is still scanned, and the reason is sent to logf.
//
// cleanup unmounts every mount and removes baseDir. It is always safe to call
// (including when nothing was mounted) and should be deferred by the caller.
func (m *Manager) MountImages(ctx context.Context, targets []string, cfg MountConfig, baseDir string, logf func(string)) (paths []string, mounts []MountedImage, cleanup func()) {
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
		dir := path.Join(baseDir, strconv.Itoa(i))
		if err := m.fsys.MkdirAll(dir, 0o700); err != nil {
			logf("mount " + p + ": " + err.Error() + " (scanning the raw image instead)")
			out = append(out, p)
			continue
		}
		if err := m.mountImage(ctx, p, dir); err != nil {
			_ = m.fsys.Remove(dir)
			logf("mount " + p + ": " + err.Error() + " (scanning the raw image instead)")
			out = append(out, p)
			continue
		}
		logf("mounted " + p + " read-only at " + dir)
		mounts = append(mounts, MountedImage{Image: p, Dir: dir})
		out = append(out, dir)
	}

	made := mounts
	cleanup = func() {
		for _, mi := range made {
			m.unmount(context.WithoutCancel(ctx), mi.Dir)
		}
		if baseDir != "" {
			_ = m.fsys.RemoveAll(baseDir)
		}
	}
	return out, mounts, cleanup
}

// mountImage tries each filesystem type in turn, returning nil on the first
// success and the last error if none work.
func (m *Manager) mountImage(ctx context.Context, image, dir string) error {
	var lastErr error
	for _, fstype := range imageFSTypes {
		args := make([]string, 0, 6)
		if fstype != "" {
			args = append(args, "-t", fstype)
		}
		args = append(args, "-o", imageMountOpts, image, dir)
		_, err := m.run.Run(ctx, Cmd{Name: "mount", Args: args})
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
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

// RelabelPath rewrites any reference to a mounted image's contents back to the
// "<image>!/<path within image>" shape ClamAV uses for archive members. It is
// used both on a bare finding path and on a whole scanner output line; a string
// with no mount prefix is returned unchanged.
func RelabelPath(s string, mounts []MountedImage) string {
	for _, mi := range mounts {
		s = strings.ReplaceAll(s, mi.Dir+"/", mi.Image+"!/")
		s = strings.ReplaceAll(s, mi.Dir, mi.Image)
	}
	return s
}

// UnmountLeftovers unmounts anything still mounted under baseDir from a previous
// process (a crash between mount and cleanup) and removes baseDir. Best effort;
// intended to be called once at startup.
func (m *Manager) UnmountLeftovers(ctx context.Context, baseDir string) {
	if b, err := m.fsys.ReadFile("/proc/self/mounts"); err == nil {
		prefix := strings.TrimRight(baseDir, "/") + "/"
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			// Our mountpoints never contain spaces, so mount(8)'s \040 escaping
			// does not matter and a plain prefix check is enough.
			if mp := fields[1]; strings.HasPrefix(mp, prefix) {
				m.unmount(ctx, mp)
			}
		}
	}
	_ = m.fsys.RemoveAll(baseDir)
}
