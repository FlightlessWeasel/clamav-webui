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
	in := MountConfig{Enabled: true, Extensions: []string{"ISO", " .iso ", "", ".IMG", "img"}, ExtractDir: " /scratch "}
	got := in.Sanitized()
	if !got.Enabled {
		t.Error("Enabled dropped")
	}
	if want := []string{".iso", ".img"}; !reflect.DeepEqual(got.Extensions, want) {
		t.Errorf("Extensions = %v, want %v", got.Extensions, want)
	}
	if got.ExtractDir != "/scratch" {
		t.Errorf("ExtractDir = %q, want trimmed", got.ExtractDir)
	}
	empty := MountConfig{}.Sanitized()
	if empty.Extensions == nil {
		t.Error("Extensions should be non-nil even when empty")
	}
}

func TestPrepareImagesDisabledIsNoOp(t *testing.T) {
	m := NewManagerWithRunner(config.Defaults(), newFake())
	in := []string{"/games/x.iso", "/srv/data"}
	out, prepared, cleanup := m.PrepareImages(context.Background(), in, MountConfig{Enabled: false}, t.TempDir(), t.TempDir(), nil)
	defer cleanup()
	if !reflect.DeepEqual(out, in) || prepared != nil {
		t.Fatalf("disabled: out=%v prepared=%v", out, prepared)
	}
}

func TestPrepareImagesMountsScanAndRelabels(t *testing.T) {
	mbase, ebase := t.TempDir(), t.TempDir()
	mp := path.Join(mbase, "0")
	f := newFake().on("mount -o loop,ro,nodev,nosuid,noexec /games/x.iso "+mp, fakeResp{})
	f.on("umount "+mp, fakeResp{})
	m := NewManagerWithRunner(config.Defaults(), f)

	cfg := MountConfig{Enabled: true, Extensions: []string{".iso"}}
	out, prepared, cleanup := m.PrepareImages(context.Background(), []string{"/games/x.iso", "/srv/data"}, cfg, mbase, ebase, nil)

	if want := []string{mp, "/srv/data"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v, want %v", out, want)
	}
	if len(prepared) != 1 || prepared[0].Image != "/games/x.iso" || prepared[0].Dir != mp || prepared[0].Extracted {
		t.Fatalf("prepared = %+v", prepared)
	}
	if got := RelabelPath(path.Join(mp, "evil/setup.exe"), prepared); got != "/games/x.iso!/evil/setup.exe" {
		t.Errorf("relabel nested = %q", got)
	}
	if got := RelabelPath(mp+"/evil.exe: Win.Test.EICAR_HDB-1 FOUND", prepared); got != "/games/x.iso!/evil.exe: Win.Test.EICAR_HDB-1 FOUND" {
		t.Errorf("relabel line = %q", got)
	}
	if got := RelabelPath("/srv/data/f", prepared); got != "/srv/data/f" {
		t.Errorf("relabel outside = %q", got)
	}

	cleanup()
	if !contains(f.calls, "umount "+mp) {
		t.Errorf("cleanup did not umount: calls=%v", f.calls)
	}
}

func TestPrepareImagesExtractsWhenMountFails(t *testing.T) {
	mbase, ebase := t.TempDir(), t.TempDir()
	mp, ep := path.Join(mbase, "0"), path.Join(ebase, "0")
	f := newFake().have("7z")
	for _, k := range []string{
		"mount -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
		"mount -t udf -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
		"mount -t iso9660 -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
	} {
		f.on(k, fakeResp{err: errors.New("exit status 32"), stderr: "mount: /mnt: must be superuser to use mount.\n"})
	}
	f.on("7z x -bd -y -o"+ep+" /games/x.iso", fakeResp{stdout: "Everything is Ok\n"})
	m := NewManagerWithRunner(config.Defaults(), f)

	var logs []string
	out, prepared, cleanup := m.PrepareImages(context.Background(), []string{"/games/x.iso"},
		MountConfig{Enabled: true, Extensions: []string{".iso"}}, mbase, ebase, func(l string) { logs = append(logs, l) })
	defer cleanup()

	if want := []string{ep}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v, want the extraction dir %q", out, ep)
	}
	if len(prepared) != 1 || !prepared[0].Extracted || prepared[0].Dir != ep {
		t.Fatalf("prepared = %+v, want one extracted entry", prepared)
	}
	if !contains(f.calls, "7z x -bd -y -o"+ep+" /games/x.iso") {
		t.Errorf("7z not invoked: %v", f.calls)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "must be superuser") || !strings.Contains(joined, "extracting /games/x.iso") {
		t.Errorf("log missing mount reason / extract notice: %q", joined)
	}
}

func TestPrepareImagesNoExtractorFallsBackToRawImage(t *testing.T) {
	mbase, ebase := t.TempDir(), t.TempDir()
	mp := path.Join(mbase, "0")
	f := newFake() // no mount responses, no 7z/bsdtar on PATH
	for _, k := range []string{
		"mount -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
		"mount -t udf -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
		"mount -t iso9660 -o loop,ro,nodev,nosuid,noexec /games/x.iso " + mp,
	} {
		f.on(k, fakeResp{err: errors.New("exit status 32")})
	}
	m := NewManagerWithRunner(config.Defaults(), f)

	out, prepared, cleanup := m.PrepareImages(context.Background(), []string{"/games/x.iso"},
		MountConfig{Enabled: true, Extensions: []string{".iso"}}, mbase, ebase, nil)
	defer cleanup()

	if want := []string{"/games/x.iso"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v, want raw image kept", out)
	}
	if len(prepared) != 0 {
		t.Fatalf("prepared = %+v, want none", prepared)
	}
}

func TestUnmountLeftovers(t *testing.T) {
	base := "/var/lib/clamav-webui/mnt"
	ffs := newFakeFS().put("mounts", "/dev/loop0 "+base+"/7/0 iso9660 ro 0 0\n"+
		"/dev/loop1 "+base+"/7/1 udf ro 0 0\n"+
		"/dev/sda1 / ext4 rw 0 0\n", time.Now())
	f := newFake().on("umount "+base+"/7/0", fakeResp{}).on("umount "+base+"/7/1", fakeResp{})
	m := NewManagerWithDeps(config.Defaults(), f, ffs)

	m.UnmountLeftovers(context.Background(), base, "/var/lib/clamav-webui/extract")

	for _, want := range []string{"umount " + base + "/7/0", "umount " + base + "/7/1"} {
		if !contains(f.calls, want) {
			t.Errorf("missing %q in calls %v", want, f.calls)
		}
	}
	if contains(f.calls, "umount /") {
		t.Error("unmounted a path outside the base dirs")
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
