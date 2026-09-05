package clamav

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func TestParseSigtoolInfo(t *testing.T) {
	out := `File: daily.cld
Build time: 20 Sep 2024 08-15 +0000
Version: 27000
Signatures: 2045623
Functionality level: 90
Builder: raynman
MD5: abc123
`
	v, s, bt := parseSigtoolInfo(out)
	if v != 27000 || s != 2045623 || bt != "20 Sep 2024 08-15 +0000" {
		t.Fatalf("got v=%d s=%d bt=%q", v, s, bt)
	}
}

func TestConfValue(t *testing.T) {
	conf := "# comment\nDatabaseMirror  database.clamav.net\nChecks 24\n\n#Checks 99\n"
	if got := confValue(conf, "Checks"); got != "24" {
		t.Errorf("Checks = %q", got)
	}
	if got := confValue(conf, "DatabaseMirror"); got != "database.clamav.net" {
		t.Errorf("DatabaseMirror = %q", got)
	}
	if got := confValue(conf, "Missing"); got != "" {
		t.Errorf("Missing = %q", got)
	}
}

func sigCfg() config.Config {
	c := config.Defaults()
	c.ClamAVDBDir = "/var/lib/clamav"
	c.FreshclamConf = "/etc/clamav/freshclam.conf"
	return c
}

func TestSignaturesAggregates(t *testing.T) {
	now := time.Now()
	fs := newFakeFS().
		put("daily.cld", "", now.Add(-2*time.Hour)).
		put("main.cvd", "", now.Add(-72*time.Hour)).
		put("freshclam.conf", "Checks 12\n", now)
	// bytecode.cld absent on purpose.

	f := newFake().have("freshclam", "sigtool").
		on("sigtool --info /var/lib/clamav/daily.cld", fakeResp{stdout: "Version: 27000\nSignatures: 2000000\n"}).
		on("sigtool --info /var/lib/clamav/main.cvd", fakeResp{stdout: "Version: 62\nSignatures: 6500000\n"}).
		on(showKey("clamav-freshclam"), fakeResp{stdout: "Id=clamav-freshclam.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"})

	m := NewManagerWithDeps(sigCfg(), f, fs)
	sig, err := m.Signatures(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sig.Databases) != 3 {
		t.Fatalf("want 3 db entries, got %d", len(sig.Databases))
	}
	if sig.TotalSigs != 8500000 {
		t.Errorf("TotalSigs = %d", sig.TotalSigs)
	}
	if sig.Checks != 12 {
		t.Errorf("Checks = %d", sig.Checks)
	}
	if sig.AgeSeconds < 3600 || sig.AgeSeconds > 3*3600 {
		t.Errorf("AgeSeconds = %d, expected ~2h", sig.AgeSeconds)
	}
	if sig.FreshclamService.Active != "active" {
		t.Errorf("freshclam service = %+v", sig.FreshclamService)
	}
	var bytecode SignatureDB
	for _, d := range sig.Databases {
		if d.Name == "bytecode" {
			bytecode = d
		}
	}
	if bytecode.Present {
		t.Errorf("bytecode should be absent: %+v", bytecode)
	}
}

func TestSignaturesNoDatabases(t *testing.T) {
	f := newFake().
		on(showKey("clamav-freshclam"), fakeResp{stdout: "Id=clamav-freshclam.service\nLoadState=not-found\nActiveState=inactive\nSubState=dead\n"})
	m := NewManagerWithDeps(sigCfg(), f, newFakeFS())

	sig, err := m.Signatures(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sig.AgeSeconds != -1 {
		t.Errorf("AgeSeconds = %d, want -1 when no DB present", sig.AgeSeconds)
	}
}

func TestUpdateSignaturesStopsAndRestartsService(t *testing.T) {
	f := newFake().have("freshclam").
		on(showKey("clamav-freshclam"), fakeResp{stdout: "Id=clamav-freshclam.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"}).
		on("systemctl stop clamav-freshclam", fakeResp{}).
		on("systemctl start clamav-freshclam", fakeResp{}).
		on("freshclam --stdout", fakeResp{lines: []string{"daily.cld updated (version: 27001)"}})

	m := NewManagerWithDeps(sigCfg(), f, newFakeFS())
	var log []string
	if err := m.UpdateSignatures(context.Background(), func(s string) { log = append(log, s) }); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(f.calls, " | ")
	if !strings.Contains(joined, "systemctl stop clamav-freshclam") ||
		!strings.Contains(joined, "freshclam --stdout") ||
		!strings.Contains(joined, "systemctl start clamav-freshclam") {
		t.Fatalf("call sequence wrong: %s", joined)
	}
	// stop must precede freshclam, which must precede start.
	stop, run, start := indexOf(f.calls, "systemctl stop clamav-freshclam"), indexOf(f.calls, "freshclam --stdout"), indexOf(f.calls, "systemctl start clamav-freshclam")
	if !(stop < run && run < start) {
		t.Errorf("ordering: stop=%d run=%d start=%d", stop, run, start)
	}
}

func TestUpdateSignaturesRequiresFreshclam(t *testing.T) {
	m := NewManagerWithDeps(sigCfg(), newFake(), newFakeFS())
	if err := m.UpdateSignatures(context.Background(), nil); err != ErrNotInstalled {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}
