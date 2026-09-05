package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	view, err := s.clam.ReadConf(chi.URLParam(r, "which"))
	if errors.Is(err, clamav.ErrUnknownConf) {
		writeError(w, http.StatusNotFound, "unknown config file")
		return
	}
	if err != nil {
		slog.Error("config: read", "err", err)
		writeError(w, http.StatusBadGateway, "could not read config")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type configUpdateRequest struct {
	Updates map[string][]string `json:"updates"`
	Restart bool                `json:"restart"`
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	which := chi.URLParam(r, "which")

	var req configUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}

	err := s.clam.WriteConf(which, req.Updates)
	switch {
	case errors.Is(err, clamav.ErrUnknownConf):
		writeError(w, http.StatusNotFound, "unknown config file")
		return
	case errors.Is(err, clamav.ErrConfKeyNotAllowed):
		writeError(w, http.StatusBadRequest, "one or more keys are not editable")
		return
	case err != nil:
		slog.Error("config: write", "which", which, "err", err)
		writeError(w, http.StatusBadGateway, "could not write config: "+err.Error())
		return
	}

	view, _ := s.clam.ReadConf(which)
	restarted := false
	if req.Restart && view.Unit != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		if e := s.clam.ServiceAction(ctx, view.Unit, "restart"); e != nil {
			slog.Error("config: restart after write", "unit", view.Unit, "err", e)
		} else {
			restarted = true
			s.bus.Publish(sse.Event{Type: "service", Data: map[string]string{"unit": view.Unit, "action": "restart"}})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": view, "restarted": restarted})
}
