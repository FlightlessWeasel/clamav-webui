package clamav

import "errors"

var (
	// ErrUnknownUnit is returned for a systemd unit outside ManagedUnits.
	ErrUnknownUnit = errors.New("clamav: unknown or unmanaged unit")
	// ErrUnknownAction is returned for a service action that is not allowed.
	ErrUnknownAction = errors.New("clamav: unknown service action")
	// ErrNotInstalled is returned when an operation needs ClamAV but clamscan
	// is not on PATH.
	ErrNotInstalled = errors.New("clamav: ClamAV is not installed")
	// ErrUnknownConf is returned for a config target other than clamd/freshclam.
	ErrUnknownConf = errors.New("clamav: unknown config file")
	// ErrConfKeyNotAllowed is returned when a write touches a non-whitelisted key.
	ErrConfKeyNotAllowed = errors.New("clamav: config key is not editable")
)
