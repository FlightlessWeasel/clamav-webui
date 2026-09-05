package clamav

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ManagedUnits are the only systemd units this app will touch.
var ManagedUnits = []string{"clamav-daemon", "clamav-freshclam", "clamav-clamonacc"}

func isManaged(unit string) bool {
	for _, u := range ManagedUnits {
		if u == unit {
			return true
		}
	}
	return false
}

// ServiceState is a snapshot of one systemd unit.
type ServiceState struct {
	Unit      string `json:"unit"`
	Load      string `json:"load"`       // loaded, not-found, ...
	Active    string `json:"active"`     // active, inactive, failed, activating
	Sub       string `json:"sub"`        // running, dead, exited, ...
	Enabled   string `json:"enabled"`    // enabled, disabled, static, not-found, alias
	Installed bool   `json:"installed"`  // Load == "loaded"
	SinceUnix int64  `json:"since_unix"` // ActiveEnterTimestamp, 0 if unknown
}

var serviceActions = map[string]string{
	"start":   "start",
	"stop":    "stop",
	"restart": "restart",
	"enable":  "enable",
	"disable": "disable",
}

// Services returns the state of every managed unit.
func (m *Manager) Services(ctx context.Context) ([]ServiceState, error) {
	out := make([]ServiceState, 0, len(ManagedUnits))
	for _, u := range ManagedUnits {
		st, err := m.Service(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Unit < out[j].Unit })
	return out, nil
}

// Service returns the state of one managed unit.
func (m *Manager) Service(ctx context.Context, unit string) (ServiceState, error) {
	if !isManaged(unit) {
		return ServiceState{}, fmt.Errorf("%w: %s", ErrUnknownUnit, unit)
	}
	res, err := m.run.Run(ctx, Cmd{
		Name: "systemctl",
		Args: []string{"show", unit,
			"--property=LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestampMonotonic,ActiveEnterTimestamp"},
	})
	// `systemctl show` exits 0 even for missing units; a real error means
	// systemctl itself is unavailable.
	if err != nil && len(res.Stdout) == 0 {
		return ServiceState{}, fmt.Errorf("systemctl show %s: %w", unit, err)
	}

	kv := parseKV(string(res.Stdout))
	st := ServiceState{
		Unit:    unit,
		Load:    kv["LoadState"],
		Active:  kv["ActiveState"],
		Sub:     kv["SubState"],
		Enabled: kv["UnitFileState"],
	}
	st.Installed = st.Load == "loaded"
	if ts := kv["ActiveEnterTimestamp"]; ts != "" {
		st.SinceUnix = parseSystemdTimestamp(ts)
	}
	return st, nil
}

// ServiceAction runs start/stop/restart/enable/disable on a managed unit.
func (m *Manager) ServiceAction(ctx context.Context, unit, action string) error {
	if !isManaged(unit) {
		return fmt.Errorf("%w: %s", ErrUnknownUnit, unit)
	}
	verb, ok := serviceActions[action]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownAction, action)
	}
	res, err := m.run.Run(ctx, Cmd{Name: "systemctl", Args: []string{verb, unit}})
	if err != nil {
		return fmt.Errorf("systemctl %s %s: %w: %s", verb, unit, err, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// ServiceLogs returns the last n journal lines for a managed unit.
func (m *Manager) ServiceLogs(ctx context.Context, unit string, lines int) (string, error) {
	if !isManaged(unit) {
		return "", fmt.Errorf("%w: %s", ErrUnknownUnit, unit)
	}
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	res, err := m.run.Run(ctx, Cmd{
		Name: "journalctl",
		Args: []string{"-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso"},
	})
	if err != nil && len(res.Stdout) == 0 {
		return "", fmt.Errorf("journalctl -u %s: %w", unit, err)
	}
	return string(res.Stdout), nil
}

// parseKV parses "Key=Value" lines into a map.
func parseKV(s string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if i := strings.IndexByte(line, '='); i > 0 {
			out[line[:i]] = line[i+1:]
		}
	}
	return out
}

// parseSystemdTimestamp converts systemd's "Day YYYY-MM-DD HH:MM:SS TZ" into a
// unix seconds value, best-effort. Returns 0 on failure.
func parseSystemdTimestamp(s string) int64 {
	// systemd also exposes ActiveEnterTimestampMonotonic but that is relative to
	// boot; the human form is easier to consume and good enough for a UI badge.
	for _, layout := range []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.Unix()
		}
	}
	return 0
}
