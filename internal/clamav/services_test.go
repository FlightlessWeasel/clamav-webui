package clamav

import (
	"context"
	"errors"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func TestServiceStateParsing(t *testing.T) {
	f := newFake().on(
		"systemctl show clamav-daemon --property=LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestampMonotonic,ActiveEnterTimestamp",
		fakeResp{stdout: "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nActiveEnterTimestamp=Fri 2024-09-20 08:15:11 UTC\n"},
	)
	m := NewManagerWithRunner(config.Defaults(), f)

	st, err := m.Service(context.Background(), "clamav-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if st.Active != "active" || st.Sub != "running" || st.Enabled != "enabled" {
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
		"systemctl show clamav-clamonacc --property=LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestampMonotonic,ActiveEnterTimestamp",
		fakeResp{stdout: "LoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\n"},
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

func TestServicesListsAllManaged(t *testing.T) {
	f := newFake()
	for _, u := range ManagedUnits {
		f.on("systemctl show "+u+" --property=LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestampMonotonic,ActiveEnterTimestamp",
			fakeResp{stdout: "LoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"})
	}
	m := NewManagerWithRunner(config.Defaults(), f)
	got, err := m.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(ManagedUnits) {
		t.Fatalf("got %d states, want %d", len(got), len(ManagedUnits))
	}
}
