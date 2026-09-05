package clamav

import (
	"sync"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

// Manager is the entry point for all ClamAV operations. Runner calls are
// stateless; confMu serialises the read-modify-write of the config files.
type Manager struct {
	run    Runner
	fsys   FS
	cfg    config.Config
	confMu sync.Mutex
}

// NewManager returns a Manager that runs real commands and reads the real
// filesystem.
func NewManager(cfg config.Config) *Manager {
	return &Manager{run: NewExecRunner(), fsys: osFS{}, cfg: cfg}
}

// NewManagerWithRunner injects a custom Runner (real filesystem), for tests and
// embedders.
func NewManagerWithRunner(cfg config.Config, r Runner) *Manager {
	return &Manager{run: r, fsys: osFS{}, cfg: cfg}
}

// NewManagerWithDeps injects both a Runner and an FS.
func NewManagerWithDeps(cfg config.Config, r Runner, fsys FS) *Manager {
	return &Manager{run: r, fsys: fsys, cfg: cfg}
}
