// Package config loads runtime configuration for clamav-webui from an optional
// JSON file plus CLAMWEB_* environment overrides, and derives the paths the rest
// of the app uses.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the resolved runtime configuration.
type Config struct {
	// Addr is the TCP listen address, e.g. ":8080".
	Addr string `json:"addr"`
	// ConfigDir holds the SQLite DB, the quarantine store and other state.
	ConfigDir string `json:"config_dir"`
	// LogLevel is one of debug, info, warn, error.
	LogLevel string `json:"log_level"`
	// TLSCert and TLSKey, when both set, switch the server to HTTPS.
	TLSCert string `json:"tls_cert"`
	TLSKey  string `json:"tls_key"`
	// ClamAVDBDir is where freshclam keeps signature files.
	ClamAVDBDir string `json:"clamav_db_dir"`
	// ClamdConf and FreshclamConf are the ClamAV config files to manage.
	ClamdConf     string `json:"clamd_conf"`
	FreshclamConf string `json:"freshclam_conf"`
	// BrowseRoot bounds the directory picker; requests outside it are refused.
	BrowseRoot string `json:"browse_root"`
}

// Defaults returns a Config populated with the built-in defaults.
func Defaults() Config {
	return Config{
		Addr:          ":8080",
		ConfigDir:     "/var/lib/clamav-webui",
		LogLevel:      "info",
		ClamAVDBDir:   "/var/lib/clamav",
		ClamdConf:     "/etc/clamav/clamd.conf",
		FreshclamConf: "/etc/clamav/freshclam.conf",
		BrowseRoot:    "/",
	}
}

// Load reads the optional JSON file at path (empty path = skip), then applies
// CLAMWEB_* environment overrides, then validates.
func Load(path string) (Config, error) {
	cfg := Defaults()

	if path == "" {
		path = os.Getenv("CLAMWEB_CONFIG_FILE")
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config file %s: %w", path, err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
		}
	}

	applyEnv(&cfg)

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	for _, e := range []struct {
		key string
		dst *string
	}{
		{"CLAMWEB_ADDR", &cfg.Addr},
		{"CLAMWEB_CONFIG_DIR", &cfg.ConfigDir},
		{"CLAMWEB_LOG_LEVEL", &cfg.LogLevel},
		{"CLAMWEB_TLS_CERT", &cfg.TLSCert},
		{"CLAMWEB_TLS_KEY", &cfg.TLSKey},
		{"CLAMWEB_CLAMAV_DB_DIR", &cfg.ClamAVDBDir},
		{"CLAMWEB_CLAMD_CONF", &cfg.ClamdConf},
		{"CLAMWEB_FRESHCLAM_CONF", &cfg.FreshclamConf},
		{"CLAMWEB_BROWSE_ROOT", &cfg.BrowseRoot},
	} {
		if v := os.Getenv(e.key); v != "" {
			*e.dst = v
		}
	}
}

func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("addr must not be empty")
	}
	if c.ConfigDir == "" {
		return fmt.Errorf("config_dir must not be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level %q: must be debug, info, warn or error", c.LogLevel)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("tls_cert and tls_key must be set together")
	}
	return nil
}

// TLSEnabled reports whether the server should serve HTTPS.
func (c Config) TLSEnabled() bool { return c.TLSCert != "" && c.TLSKey != "" }

// DBPath is the SQLite database file path.
func (c Config) DBPath() string { return filepath.Join(c.ConfigDir, "clamav-webui.db") }

// QuarantineDir is where infected files are moved.
func (c Config) QuarantineDir() string { return filepath.Join(c.ConfigDir, "quarantine") }

// EnsureDirs creates ConfigDir and QuarantineDir if missing.
func (c Config) EnsureDirs() error {
	for _, d := range []string{c.ConfigDir, c.QuarantineDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

// Redacted returns a copy safe to log (no secret material today, but a hook for
// when there is).
func (c Config) Redacted() Config { return c }

// String renders the config as a single line for startup logging.
func (c Config) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "addr=%s config_dir=%s log_level=%s tls=%t", c.Addr, c.ConfigDir, c.LogLevel, c.TLSEnabled())
	return b.String()
}
