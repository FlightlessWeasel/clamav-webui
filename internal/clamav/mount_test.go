package clamav

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func TestMountConfigIsImage(t *testing.T) {
	cfg := MountConfig{Extensions: []string{".iso", "img", " .UDF "}}
	cases := map[string]bool{
		"/games/x.iso":         true,
		"/games/X.ISO":         true,
		"/dump/disk.img":       true,
		"/media/movie.udf":     true,
		"/games/setup.exe":     false,
		"/games/iso-notes.txt": false,
		"/srv/data":            false,
	}
	for p, want := range cases {
		if got := cfg.IsImage(p); got != want {
			t.Errorf("IsImage(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestMountConfigSanitized(t *testing.T) {
	in := MountConfig{Enabled: true, Extensions: []string{"ISO", " .iso ", "", ".IMG", "img"}}
	got := in.Sanitized()
	if !got.Enabled {
		t.Error("Enabled dropped")
	}
	if want := []string{".iso", ".img"}; !reflect.DeepEqual(got.Extensions, want) {
		t.Errorf("Extensions = %v, want %v", got.Extensions, want)
	}
	empty := MountConfig{}.Sanitized()
	if empty.Extensions == nil {
		t.Error("Extensions should be non-nil even when empty")
	}
}

func TestMountImagesDisabledIsNoOp(t *testing.T) {
	m := NewManagerWithRunner(config.Defaults(), newFake())
	in := []string{"/games/x.iso", "/srv/data"}
	out, mounts, cleanup := m.MountImages(context.Background(), in, MountConfig{Enabled: false}, t.TempDir(), nil)
	defer cleanup()
	if !reflect.DeepEqual(out, in) || mounts != nil {
		t.Fatalf("disabled: out=%v mounts=%v", out, mounts)
	}
}

func TestMountImagesMountsScanAndRelabels(t *testing.T) {
	base := t.TempDir()
	mp := path.Join(base, "0")
	f := newFake().on("mount -o loop,ro,nodev,nosuid,noexec /games/x.iso "+mp, fakeResp{})
	f.on("umount "+mp, fakeResp{})
	m := NewManagerWithRunner(config.Defaults(), f)

	cfg := MountConfig{Enabled: true, Extensions: []string{".iso"}}
	out, mounts, cleanup := m.MountImages(context.Background(), []string{"/games/x.iso", "/srv/data"}, cfg, base, nil)

	if want := []string{mp, "/srv/data"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v, want %v", out, want)
	}
	if len(mounts) != 1 || mounts[0].Image != "/games/x.iso" || mounts[0].Dir != mp {
		t.Fatalf("mounts = %+v", mounts)
	}
	if got := RelabelPath(path.Join(mp, "evil/setup.exe"), mounts); got != "/games/x.iso!/evil/setup.exe" {
		t.Errorf("relabel nested = %q", got)
	}
	if got := RelabelPath(mp, mounts); got != "/games/x.iso" {
		t.Errorf("relabel root = %q", got)
	}
	if got := RelabelPath("/srv/data/f", mounts); got != "/srv/data/f" {
		t.Errorf("relabel outside = %q", got)
	}
	// A whole scanner output line, not just a bare path.
	line := mp + "/evil.exe: Win.Test.EICAR_HDB-1 FOUND"
	if got := RelabelPath(line, mounts); got != "/games/x.iso!/evil.exe: Win.Test.EICAR_HDB-1 FOUND" {
		t.Errorf("relabel line = %q", got)
	}

	cleanup()
	if !contains(f.calls, "umount "+mp) {
		t.Errorf("cleanup did not umount: calls=%v", f.calls)
	}
}

func TestMountImagesFallsBackToRawImageOnFailure(t *testing.T) {
	base := t.TempDir()
	mp := path.Join(base, "0")
	// Every filesystem type fails; mount(8) writes the real reason to stderr
	// and only signals failure through a generic exit code.
	f := newFake().
		on("mount -o loop,ro,nodev,nosuid,noexec /games/x.iso "+mp,
			fakeResp{err: errors.New("exit status 32"), stderr: "mount: /games/x.iso: wrong fs type, bad option, bad superblock\n"}).
		on("mount -t udf -o loop,ro,nodev,nosuid,noexec /games/x.iso "+mp,
			fakeResp{err: errors.New("exit status 32"), stderr: "mount: unknown filesystem type 'udf'.\n"}).
		on("mount -t iso9660 -o loop,ro,nodev,nosuid,noexec /games/x.iso "+mp,
			fakeResp{err: errors.New("exit status 32"), stderr: "mount: /games/x.iso: wrong fs type, bad option, bad superblock\n"})
	m := NewManagerWithRunner(config.Defaults(), f)

	var logs []string
	out, mounts, cleanup := m.MountImages(context.Background(), []string{"/games/x.iso"},
		MountConfig{Enabled: true, Extensions: []string{".iso"}}, base, func(l string) { logs = append(logs, l) })
	defer cleanup()

	if want := []string{"/games/x.iso"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v, want raw image kept", out)
	}
	if len(mounts) != 0 {
		t.Fatalf("mounts = %+v, want none", mounts)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "wrong fs type") || !strings.Contains(joined, "unknown filesystem type 'udf'") {
		t.Errorf("mount stderr not surfaced in log: %q", joined)
	}
	if strings.Contains(joined, "exit status 32") {
		t.Errorf("bare exit code leaked instead of the real message: %q", joined)
	}
}

func TestUnmountLeftovers(t *testing.T) {
	base := "/var/lib/clamav-webui/mnt"
	ffs := newFakeFS().put("mounts", "/dev/loop0 "+base+"/7/0 iso9660 ro 0 0\n"+
		"/dev/loop1 "+base+"/7/1 udf ro 0 0\n"+
		"/dev/sda1 / ext4 rw 0 0\n", time.Now())
	f := newFake().on("umount "+base+"/7/0", fakeResp{}).on("umount "+base+"/7/1", fakeResp{})
	m := NewManagerWithDeps(config.Defaults(), f, ffs)

	m.UnmountLeftovers(context.Background(), base)

	for _, want := range []string{"umount " + base + "/7/0", "umount " + base + "/7/1"} {
		if !contains(f.calls, want) {
			t.Errorf("missing %q in calls %v", want, f.calls)
		}
	}
	if contains(f.calls, "umount /") {
		t.Error("unmounted a path outside the base dir")
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
