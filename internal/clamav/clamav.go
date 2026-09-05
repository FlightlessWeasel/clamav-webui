package clamav

import "github.com/FlightlessWeasel/clamav-webui/internal/config"

// Manager is the entry point for all ClamAV operations. It is safe for
// concurrent use; the underlying Runner calls are stateless.
type Manager struct {
	run Runner
	cfg config.Config
}

// NewManager returns a Manager that runs real commands.
func NewManager(cfg config.Config) *Manager {
	return &Manager{run: NewExecRunner(), cfg: cfg}
}

// NewManagerWithRunner injects a custom Runner, for tests and embedders.
func NewManagerWithRunner(cfg config.Config, r Runner) *Manager {
	return &Manager{run: r, cfg: cfg}
}
