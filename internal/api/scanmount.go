package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
)

// scanMountRoot is the directory under which per-scan image mountpoints live.
// Slash-joined because ClamAV reports the paths it finds with forward slashes.
func scanMountRoot(configDir string) string {
	return path.Join(filepath.ToSlash(configDir), "mnt")
}

// scanMountConfig loads the disk-image auto-mount configuration, falling back to
// the default (disabled) when it is unset or unparseable.
func (s *Server) scanMountConfig() clamav.MountConfig {
	cfg := clamav.DefaultMountConfig()
	st, err := s.db.GetSettings()
	if err != nil {
		slog.Error("scan-mount: load settings", "err", err)
		return cfg
	}
	if raw := st.ScanMountJSON; raw != "" && raw != "{}" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = clamav.DefaultMountConfig().Extensions
	}
	return cfg
}

func (s *Server) handleGetScanMount(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.scanMountConfig())
}

func (s *Server) handlePutScanMount(w http.ResponseWriter, r *http.Request) {
	var cfg clamav.MountConfig
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	cfg = cfg.Sanitized() // trim/lower/dedupe extensions; Extensions is non-nil
	stored, _ := json.Marshal(cfg)
	if err := s.db.SetScanMountConfig(string(stored)); err != nil {
		slog.Error("scan-mount: persist", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.scanMountConfig())
}
