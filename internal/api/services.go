package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	states, err := s.clam.Services(ctx)
	if err != nil {
		slog.Error("services: list", "err", err)
		writeError(w, http.StatusBadGateway, "could not query systemd")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": states})
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	unit := chi.URLParam(r, "unit")
	action := chi.URLParam(r, "action")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	err := s.clam.ServiceAction(ctx, unit, action)
	switch {
	case errors.Is(err, clamav.ErrUnknownUnit):
		writeError(w, http.StatusNotFound, "unknown unit")
		return
	case errors.Is(err, clamav.ErrUnknownAction):
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	case err != nil:
		slog.Error("services: action", "unit", unit, "action", action, "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	state, err := s.clam.Service(ctx, unit)
	if err != nil {
		s.bus.Publish(sse.Event{Type: "service", Data: map[string]string{"unit": unit, "action": action}})
		writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
		return
	}
	s.bus.Publish(sse.Event{Type: "service", Data: state})
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	unit := chi.URLParam(r, "unit")
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	out, err := s.clam.ServiceLogs(ctx, unit, lines)
	if errors.Is(err, clamav.ErrUnknownUnit) {
		writeError(w, http.StatusNotFound, "unknown unit")
		return
	}
	if err != nil {
		slog.Error("services: logs", "unit", unit, "err", err)
		writeError(w, http.StatusBadGateway, "could not read journal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"unit": unit, "logs": out})
}
