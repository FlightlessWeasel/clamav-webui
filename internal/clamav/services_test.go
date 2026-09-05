package clamav

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func showKey(units ...string) string {
	return "systemctl show " + strings.Join(units, " ") + " " + showProperties
}

func TestServiceStateParsing(t *testing.T) {
	f := newFake().on(
		showKey("clamav-daemon"),
		fakeResp{stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nActiveEnterTimestamp=Fri 2024-09-20 08:15:11 UTC\n"},
	)
	m := NewManagerWithRunner(config.Defaults(), f)

	st, err := m.Service(context.Background(), "clamav-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if st.Unit != "clamav-daemon" || st.Active != "active" || st.Sub != "running" || st.Enabled != "enabled" {
		t.Errorf("state = %+v", st)
	}
	if !st.Installed {
		t.Error("Installed should be true for LoadState=loaded")
	}
	if st.SinceUnix == 0 {
		t.Error("SinceUnix should be parsed from ActiveEnterTimestamp")
	}
}

func TestServiceNotFound(t *testing.T) {
	f := newFake().on(
		showKey("clamav-clamonacc"),
		fakeResp{stdout: "Id=clamav-clamonacc.service\nLoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\n"},
	)
	m := NewManagerWithRunner(config.Defaults(), f)

	st, err := m.Service(context.Background(), "clamav-clamonacc")
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed {
		t.Error("Installed should be false for not-found")
	}
}

func TestServiceRejectsUnmanagedUnit(t *testing.T) {
	m := NewManagerWithRunner(config.Defaults(), newFake())
	_, err := m.Service(context.Background(), "sshd")
	if !errors.Is(err, ErrUnknownUnit) {
		t.Fatalf("err = %v, want ErrUnknownUnit", err)
	}
	if err := m.ServiceAction(context.Background(), "sshd", "stop"); !errors.Is(err, ErrUnknownUnit) {
		t.Fatalf("action err = %v, want ErrUnknownUnit", err)
	}
}

func TestServiceActionRejectsUnknownAction(t *testing.T) {
	m := NewManagerWithRunner(config.Defaults(), newFake())
	err := m.ServiceAction(context.Background(), "clamav-daemon", "sabotage")
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
}

func TestServiceActionRuns(t *testing.T) {
	f := newFake().on("systemctl restart clamav-daemon", fakeResp{})
	m := NewManagerWithRunner(config.Defaults(), f)
	if err := m.ServiceAction(context.Background(), "clamav-daemon", "restart"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0] != "systemctl restart clamav-daemon" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestServicesBatchesOneCall(t *testing.T) {
	var sb strings.Builder
	for i, u := range ManagedUnits {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Id=" + u + ".service\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n")
	}
	f := newFake().on(showKey(ManagedUnits...), fakeResp{stdout: sb.String()})
	m := NewManagerWithRunner(config.Defaults(), f)

	got, err := m.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(ManagedUnits) {
		t.Fatalf("got %d states, want %d", len(got), len(ManagedUnits))
	}
	if len(f.calls) != 1 {
		t.Errorf("expected exactly one systemctl call, got %d: %v", len(f.calls), f.calls)
	}
	for _, st := range got {
		if st.Unit == "" || !st.Installed {
			t.Errorf("bad state: %+v", st)
		}
	}
}

func TestServicesFillsMissingUnit(t *testing.T) {
	// Only one unit reported back; the other two must still appear as not-found.
	f := newFake().on(showKey(ManagedUnits...), fakeResp{
		stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n",
	})
	m := NewManagerWithRunner(config.Defaults(), f)

	got, err := m.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d", len(got))
	}
	for _, st := range got {
		if st.Unit == "clamav-daemon" && !st.Installed {
			t.Error("clamav-daemon should be installed")
		}
		if st.Unit != "clamav-daemon" && st.Installed {
			t.Errorf("%s should be not-found", st.Unit)
		}
	}
}
