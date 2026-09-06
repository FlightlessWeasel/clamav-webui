package db

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Settings is the singleton application settings row.
type Settings struct {
	SetupComplete     bool
	AdminPasswordHash string
	SessionSecret     string
	ScanDefaultsJSON  string
	NotifyConfigJSON  string
	ScanMountJSON     string
}

// GetSettings reads the singleton settings row (id = 1).
func (d *DB) GetSettings() (Settings, error) {
	var s Settings
	err := d.QueryRow(`
		SELECT setup_complete, admin_password_hash, session_secret,
		       scan_defaults_json, notify_config_json, scan_mount_json
		FROM settings WHERE id = 1`).
		Scan(&s.SetupComplete, &s.AdminPasswordHash, &s.SessionSecret,
			&s.ScanDefaultsJSON, &s.NotifyConfigJSON, &s.ScanMountJSON)
	if err != nil {
		return Settings{}, fmt.Errorf("get settings: %w", err)
	}
	return s, nil
}

// SetAdminPassword stores hash and marks setup complete.
func (d *DB) SetAdminPassword(hash string) error {
	_, err := d.Exec(`
		UPDATE settings
		SET admin_password_hash = ?, setup_complete = 1, updated_at = datetime('now')
		WHERE id = 1`, hash)
	if err != nil {
		return fmt.Errorf("set admin password: %w", err)
	}
	return nil
}

// SetNotifyConfig stores the notification configuration JSON.
func (d *DB) SetNotifyConfig(jsonStr string) error {
	_, err := d.Exec(`UPDATE settings SET notify_config_json = ?, updated_at = datetime('now') WHERE id = 1`, jsonStr)
	return err
}

// SetScanMountConfig stores the disk-image auto-mount configuration JSON.
func (d *DB) SetScanMountConfig(jsonStr string) error {
	_, err := d.Exec(`UPDATE settings SET scan_mount_json = ?, updated_at = datetime('now') WHERE id = 1`, jsonStr)
	return err
}

// EnsureSessionSecret returns the persisted session secret, generating and
// storing one on first call.
func (d *DB) EnsureSessionSecret() (string, error) {
	s, err := d.GetSettings()
	if err != nil {
		return "", err
	}
	if s.SessionSecret != "" {
		return s.SessionSecret, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	secret := base64.RawStdEncoding.EncodeToString(b)
	if _, err := d.Exec(`UPDATE settings SET session_secret = ?, updated_at = datetime('now') WHERE id = 1`, secret); err != nil {
		return "", fmt.Errorf("store session secret: %w", err)
	}
	return secret, nil
}
