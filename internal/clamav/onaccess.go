package clamav

import (
	"context"
	"fmt"
)

// OnAccessStatus is the current state of real-time (on-access) scanning.
type OnAccessStatus struct {
	Supported     bool         `json:"supported"` // clamonacc is installed
	DaemonActive  bool         `json:"daemon_active"`
	Clamonacc     ServiceState `json:"clamonacc"`
	Enabled       bool         `json:"enabled"` // clamonacc active
	WatchPaths    []string     `json:"watch_paths"`
	ExcludePaths  []string     `json:"exclude_paths"`
	ExcludeUnames []string     `json:"exclude_unames"`
	Prevention    bool         `json:"prevention"`
}

// OnAccessConfig is the desired on-access configuration.
type OnAccessConfig struct {
	Enabled       bool     `json:"enabled"`
	Paths         []string `json:"paths"`
	ExcludePaths  []string `json:"exclude_paths"`
	ExcludeUnames []string `json:"exclude_unames"`
	Prevention    bool     `json:"prevention"`
}

// OnAccessStatus probes clamonacc and reads the relevant clamd.conf keys.
func (m *Manager) OnAccessStatus(ctx context.Context) (OnAccessStatus, error) {
	var st OnAccessStatus
	_, st.Supported = m.run.LookPath("clamonacc")

	if svc, err := m.Service(ctx, "clamav-daemon"); err == nil {
		st.DaemonActive = svc.Active == "active"
	}
	if svc, err := m.Service(ctx, "clamav-clamonacc"); err == nil {
		st.Clamonacc = svc
		st.Enabled = svc.Active == "active"
	}

	conf, err := m.ReadConf("clamd")
	if err != nil {
		return st, err
	}
	for _, e := range conf.Entries {
		switch e.Name {
		case "OnAccessIncludePath":
			st.WatchPaths = e.Values
		case "OnAccessExcludePath":
			st.ExcludePaths = e.Values
		case "OnAccessExcludeUname":
			st.ExcludeUnames = e.Values
		case "OnAccessPrevention":
			st.Prevention = len(e.Values) > 0 && confValueBool(e.Values[0])
		}
	}
	return st, nil
}

// ApplyOnAccess enables or disables real-time scanning. Enabling: write the
// on-access keys into clamd.conf, (re)start + enable clamav-daemon, then enable
// + start clamav-clamonacc. Disabling only stops clamonacc; clamd.conf is left
// untouched.
func (m *Manager) ApplyOnAccess(ctx context.Context, cfg OnAccessConfig) error {
	if _, ok := m.run.LookPath("clamonacc"); !ok {
		return ErrNotInstalled
	}

	if !cfg.Enabled {
		_ = m.ServiceAction(ctx, "clamav-clamonacc", "stop")
		if err := m.ServiceAction(ctx, "clamav-clamonacc", "disable"); err != nil {
			return fmt.Errorf("disable clamav-clamonacc: %w", err)
		}
		return nil
	}

	daemon, err := m.Service(ctx, "clamav-daemon")
	if err != nil {
		return fmt.Errorf("check clamav-daemon: %w", err)
	}
	if !daemon.Installed {
		return fmt.Errorf("%w: clamav-daemon unit not found", ErrNotInstalled)
	}

	unames := cfg.ExcludeUnames
	if len(unames) == 0 {
		unames = []string{"clamav"} // never scan clamd's own reads -> avoids a loop
	}
	prevention := "no"
	if cfg.Prevention {
		prevention = "yes"
	}
	updates := map[string][]string{
		"OnAccessIncludePath":  cfg.Paths,
		"OnAccessExcludePath":  cfg.ExcludePaths,
		"OnAccessExcludeUname": unames,
		"OnAccessPrevention":   {prevention},
	}
	if err := m.WriteConf("clamd", updates); err != nil {
		return err
	}

	_ = m.ServiceAction(ctx, "clamav-daemon", "enable")
	if err := m.ServiceAction(ctx, "clamav-daemon", "restart"); err != nil {
		// clamd.conf is already changed; roll it back and try once more so we
		// don't leave the daemon down.
		_ = m.RestoreConfBackup("clamd")
		_ = m.ServiceAction(ctx, "clamav-daemon", "restart")
		return fmt.Errorf("restart clamav-daemon (rolled back clamd.conf): %w", err)
	}
	if err := m.ServiceAction(ctx, "clamav-clamonacc", "enable"); err != nil {
		return fmt.Errorf("enable clamav-clamonacc: %w", err)
	}
	if err := m.ServiceAction(ctx, "clamav-clamonacc", "restart"); err != nil {
		return fmt.Errorf("start clamav-clamonacc: %w", err)
	}
	return nil
}
