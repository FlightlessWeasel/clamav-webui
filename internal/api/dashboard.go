package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type dashboardResponse struct {
	Install    any `json:"install"`
	Services   any `json:"services"`
	Quarantine int `json:"quarantine_held"`
	LastScan   any `json:"last_scan"`
}

// handleDashboard returns the aggregate status the dashboard renders.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	install, err := s.clam.Detect(ctx)
	if err != nil {
		slog.Error("dashboard: detect", "err", err)
		writeError(w, http.StatusBadGateway, "could not probe ClamAV")
		return
	}
	services, err := s.clam.Services(ctx)
	if err != nil {
		slog.Error("dashboard: services", "err", err)
		writeError(w, http.StatusBadGateway, "could not query systemd")
		return
	}

	writeJSON(w, http.StatusOK, dashboardResponse{
		Install:    install,
		Services:   services,
		Quarantine: 0,   // populated once the quarantine step lands
		LastScan:   nil, // populated once scanning lands
	})
}
