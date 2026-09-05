package clamav

import (
	"context"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func TestDetectNotInstalled(t *testing.T) {
	f := newFake().on("apt-cache policy clamav", fakeResp{stdout: "clamav:\n  Installed: (none)\n  Candidate: 1.0.3+dfsg-1\n"})
	m := NewManagerWithRunner(config.Defaults(), f)

	in, err := m.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if in.Installed {
		t.Error("Installed should be false")
	}
	if in.AptInstalled != "" || in.AptCandidate != "1.0.3+dfsg-1" {
		t.Errorf("apt versions = %q / %q", in.AptInstalled, in.AptCandidate)
	}
	if in.UpgradeAvailable {
		t.Error("UpgradeAvailable should be false when nothing is installed")
	}
}

func TestDetectInstalledWithDB(t *testing.T) {
	f := newFake().
		have("clamscan", "clamdscan", "freshclam", "clamonacc").
		on("clamscan --version", fakeResp{stdout: "ClamAV 1.0.3/27000/Fri Sep 20 08:15:11 2024\n"}).
		on("apt-cache policy clamav", fakeResp{stdout: "clamav:\n  Installed: 1.0.3+dfsg-1\n  Candidate: 1.0.5+dfsg-1\n"})
	m := NewManagerWithRunner(config.Defaults(), f)

	in, err := m.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !in.Installed || !in.HasDaemon || !in.HasFreshclam || !in.HasClamonacc {
		t.Fatalf("presence flags wrong: %+v", in)
	}
	if in.EngineVersion != "1.0.3" {
		t.Errorf("EngineVersion = %q", in.EngineVersion)
	}
	if in.DBVersion != "27000" {
		t.Errorf("DBVersion = %q", in.DBVersion)
	}
	if in.DBDate != "Fri Sep 20 08:15:11 2024" {
		t.Errorf("DBDate = %q", in.DBDate)
	}
	if !in.UpgradeAvailable {
		t.Error("UpgradeAvailable should be true (1.0.3 != 1.0.5)")
	}
}

func TestDetectInstalledNoDB(t *testing.T) {
	f := newFake().
		have("clamscan").
		on("clamscan --version", fakeResp{stdout: "ClamAV 1.0.3\n"}).
		on("apt-cache policy clamav", fakeResp{stdout: "clamav:\n  Installed: 1.0.3+dfsg-1\n  Candidate: 1.0.3+dfsg-1\n"})
	m := NewManagerWithRunner(config.Defaults(), f)

	in, _ := m.Detect(context.Background())
	if in.EngineVersion != "1.0.3" || in.DBVersion != "" {
		t.Errorf("got engine=%q db=%q, want 1.0.3 / empty", in.EngineVersion, in.DBVersion)
	}
	if in.UpgradeAvailable {
		t.Error("equal versions must not report an upgrade")
	}
}
