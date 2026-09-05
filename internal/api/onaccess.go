package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

func (s *Server) handleGetOnAccess(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	st, err := s.clam.OnAccessStatus(ctx)
	if err != nil {
		slog.Error("onaccess: status", "err", err)
		writeError(w, http.StatusBadGateway, "could not read on-access status")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutOnAccess(w http.ResponseWriter, r *http.Request) {
	var cfg clamav.OnAccessConfig
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	for _, p := range cfg.Paths {
		if err := s.checkTargetPath(p); err != nil {
			writeError(w, http.StatusBadRequest, "watch path rejected: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	err := s.clam.ApplyOnAccess(ctx, cfg)
	if errors.Is(err, clamav.ErrNotInstalled) {
		writeError(w, http.StatusBadRequest, "clamonacc is not installed")
		return
	}
	if err != nil {
		slog.Error("onaccess: apply", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.bus.Publish(sse.Event{Type: "onaccess", Data: map[string]any{"enabled": cfg.Enabled}})
	st, _ := s.clam.OnAccessStatus(context.WithoutCancel(ctx))
	writeJSON(w, http.StatusOK, st)
}
