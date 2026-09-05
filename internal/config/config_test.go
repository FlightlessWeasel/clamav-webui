package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.DBPath() != filepath.Join("/var/lib/clamav-webui", "clamav-webui.db") {
		t.Errorf("DBPath = %q", cfg.DBPath())
	}
	if cfg.TLSEnabled() {
		t.Error("TLSEnabled = true, want false")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("CLAMWEB_ADDR", ":9999")
	t.Setenv("CLAMWEB_CONFIG_DIR", "/tmp/cw")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", cfg.Addr)
	}
	if cfg.ConfigDir != "/tmp/cw" {
		t.Errorf("ConfigDir = %q", cfg.ConfigDir)
	}
}

func TestLoadFileThenEnv(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "c.json")
	if err := os.WriteFile(f, []byte(`{"addr":":7000","log_level":"debug"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAMWEB_ADDR", ":7001")
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":7001" {
		t.Errorf("env should win: Addr = %q", cfg.Addr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("file value lost: LogLevel = %q", cfg.LogLevel)
	}
}

func TestValidateRejectsBadLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("CLAMWEB_LOG_LEVEL", "chatty")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for bad log level")
	}
}

func TestValidateRejectsHalfTLS(t *testing.T) {
	clearEnv(t)
	t.Setenv("CLAMWEB_TLS_CERT", "/x/cert.pem")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error when only tls_cert is set")
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"CLAMWEB_CONFIG_FILE", "CLAMWEB_ADDR", "CLAMWEB_CONFIG_DIR", "CLAMWEB_LOG_LEVEL",
		"CLAMWEB_TLS_CERT", "CLAMWEB_TLS_KEY", "CLAMWEB_CLAMAV_DB_DIR",
		"CLAMWEB_CLAMD_CONF", "CLAMWEB_FRESHCLAM_CONF",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}
