package clamav

import (
	"context"
	"fmt"
	"strings"
)

// aptPackages are installed together so the daemon, updater and on-access
// scanner are all available.
var aptPackages = []string{"clamav", "clamav-daemon", "clamav-freshclam"}

const aptNoninteractive = "DEBIAN_FRONTEND=noninteractive"

// InstallClamAV runs `apt-get update` then installs the ClamAV packages,
// streaming every output line to onLine.
func (m *Manager) InstallClamAV(ctx context.Context, onLine func(string)) error {
	return m.aptInstall(ctx, onLine, false)
}

// UpgradeClamAV runs `apt-get update` then upgrades the already-installed ClamAV
// packages, streaming output to onLine.
func (m *Manager) UpgradeClamAV(ctx context.Context, onLine func(string)) error {
	return m.aptInstall(ctx, onLine, true)
}

func (m *Manager) aptInstall(ctx context.Context, onLine func(string), onlyUpgrade bool) error {
	if onLine == nil {
		onLine = func(string) {}
	}

	onLine("$ apt-get update")
	if err := m.run.Stream(ctx, Cmd{
		Name: "apt-get",
		Args: []string{"update"},
		Env:  []string{aptNoninteractive},
	}, onLine); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}

	args := []string{"install", "-y"}
	if onlyUpgrade {
		args = append(args, "--only-upgrade")
	}
	args = append(args, aptPackages...)

	onLine("")
	onLine("$ apt-get " + strings.Join(args, " "))
	if err := m.run.Stream(ctx, Cmd{
		Name: "apt-get",
		Args: args,
		Env:  []string{aptNoninteractive},
	}, onLine); err != nil {
		return fmt.Errorf("apt-get install: %w", err)
	}
	return nil
}
