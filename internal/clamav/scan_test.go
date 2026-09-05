package clamav

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseScanLinesInfected(t *testing.T) {
	var res ScanResult
	var found []ScanFinding
	for _, line := range strings.Split(readTestdata(t, "clamscan_infected.txt"), "\n") {
		parseScanLine(line, &res, func(f ScanFinding) { found = append(found, f) })
	}
	if res.Scanned != 4 || res.Infected != 2 {
		t.Errorf("scanned=%d infected=%d", res.Scanned, res.Infected)
	}
	if res.EngineVersion != "1.0.3" {
		t.Errorf("engine = %q", res.EngineVersion)
	}
	if len(found) != 2 {
		t.Fatalf("callbacks = %d, want 2", len(found))
	}
	if found[0].Path != "/tmp/scan/eicar.com" || found[0].Signature != "Win.Test.EICAR_HDB-1" {
		t.Errorf("finding[0] = %+v", found[0])
	}
	if found[1].Path != "/tmp/scan/sub/nested-eicar.txt" {
		t.Errorf("finding[1] = %+v", found[1])
	}
}

func TestParseScanLinesClean(t *testing.T) {
	var res ScanResult
	n := 0
	for _, line := range strings.Split(readTestdata(t, "clamscan_clean.txt"), "\n") {
		parseScanLine(line, &res, func(ScanFinding) { n++ })
	}
	if n != 0 || res.Infected != 0 || res.Scanned != 2 {
		t.Errorf("clean parse wrong: findings=%d infected=%d scanned=%d", n, res.Infected, res.Scanned)
	}
}

func TestParseScanLinesClamdSignatureWithHash(t *testing.T) {
	var res ScanResult
	var found []ScanFinding
	for _, line := range strings.Split(readTestdata(t, "clamdscan_infected.txt"), "\n") {
		parseScanLine(line, &res, func(f ScanFinding) { found = append(found, f) })
	}
	if len(found) != 2 {
		t.Fatalf("findings = %d", len(found))
	}
	if found[0].Signature != "Win.Test.EICAR_HDB-1(a1b2c3d4:5678)" {
		t.Errorf("sig = %q", found[0].Signature)
	}
}

func TestScanUsesClamdWhenDaemonActive(t *testing.T) {
	f := newFake().have("clamscan", "clamdscan").
		on(showKey("clamav-daemon"), fakeResp{stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"}).
		on("clamdscan --fdpass --multiscan --stdout /tmp/scan", fakeResp{
			stdout: readTestdata(t, "clamdscan_infected.txt"), exit: 1,
		})
	m := NewManagerWithRunner(config.Defaults(), f)

	res, err := m.Scan(context.Background(), []string{"/tmp/scan"}, ScanOptions{}, ScanCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UsedDaemon {
		t.Error("should have used clamdscan")
	}
	if res.Infected != 2 {
		t.Errorf("infected = %d", res.Infected)
	}
}

func TestScanFallsBackToClamscan(t *testing.T) {
	f := newFake().have("clamscan").
		on("clamscan --stdout --recursive /tmp/scan", fakeResp{
			stdout: readTestdata(t, "clamscan_infected.txt"), exit: 1,
		})
	m := NewManagerWithRunner(config.Defaults(), f)

	var live []ScanFinding
	var lastProgress [2]int
	res, err := m.Scan(context.Background(), []string{"/tmp/scan"}, ScanOptions{Recursive: true}, ScanCallbacks{
		Finding:  func(x ScanFinding) { live = append(live, x) },
		Progress: func(scanned, infected int) { lastProgress = [2]int{scanned, infected} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.UsedDaemon {
		t.Error("should not have used the daemon")
	}
	if res.Scanned != 4 || res.Infected != 2 || len(live) != 2 {
		t.Errorf("res=%+v live=%d", res, len(live))
	}
	if lastProgress != [2]int{4, 2} {
		t.Errorf("final progress = %v, want [4 2]", lastProgress)
	}
}

func TestScanDaemonScannedCountFallback(t *testing.T) {
	// clamdscan's summary has no "Scanned files:" line — the OK/FOUND line
	// count must fill it in.
	f := newFake().have("clamscan", "clamdscan").
		on(showKey("clamav-daemon"), fakeResp{stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"}).
		on("clamdscan --fdpass --multiscan --stdout /tmp/scan", fakeResp{stdout: strings.Join([]string{
			"/tmp/scan/a: OK",
			"/tmp/scan/b: OK",
			"/tmp/scan/c: Test.Sig FOUND",
			"----------- SCAN SUMMARY -----------",
			"Infected files: 1",
			"Time: 0.5 sec (0 m 0 s)",
		}, "\n"), exit: 1})
	m := NewManagerWithRunner(config.Defaults(), f)

	res, err := m.Scan(context.Background(), []string{"/tmp/scan"}, ScanOptions{}, ScanCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3 (fallback line count)", res.Scanned)
	}
}

func TestScanExitTwoIsError(t *testing.T) {
	f := newFake().have("clamscan").
		on("clamscan --stdout /nope", fakeResp{stderr: "cannot access /nope", exit: 2})
	m := NewManagerWithRunner(config.Defaults(), f)

	_, err := m.Scan(context.Background(), []string{"/nope"}, ScanOptions{}, ScanCallbacks{})
	if err == nil {
		t.Fatal("expected error for exit code 2")
	}
}

func TestScanNotInstalled(t *testing.T) {
	m := NewManagerWithRunner(config.Defaults(), newFake())
	_, err := m.Scan(context.Background(), []string{"/x"}, ScanOptions{}, ScanCallbacks{})
	if err != ErrNotInstalled {
		t.Fatalf("err = %v", err)
	}
}
