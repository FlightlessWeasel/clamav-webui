package clamav

import (
	"context"
	"strings"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func TestInstallClamAVStreamsBothCommands(t *testing.T) {
	f := newFake().
		on("apt-get update", fakeResp{lines: []string{"Reading package lists..."}}).
		on("apt-get install -y clamav clamav-daemon clamav-freshclam", fakeResp{lines: []string{
			"Setting up clamav-daemon ...", "Processing triggers ...",
		}})
	m := NewManagerWithRunner(config.Defaults(), f)

	var log []string
	if err := m.InstallClamAV(context.Background(), func(s string) { log = append(log, s) }); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(log, "\n")
	for _, want := range []string{"apt-get update", "Reading package lists", "apt-get install -y clamav", "Setting up clamav-daemon"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log missing %q\n---\n%s", want, joined)
		}
	}
}

func TestUpgradeUsesOnlyUpgradeFlag(t *testing.T) {
	f := newFake().
		on("apt-get update", fakeResp{}).
		on("apt-get install -y --only-upgrade clamav clamav-daemon clamav-freshclam", fakeResp{lines: []string{"done"}})
	m := NewManagerWithRunner(config.Defaults(), f)

	if err := m.UpgradeClamAV(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range f.calls {
		if c == "apt-get install -y --only-upgrade clamav clamav-daemon clamav-freshclam" {
			found = true
		}
	}
	if !found {
		t.Errorf("--only-upgrade install not called; calls = %v", f.calls)
	}
}
