package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/FlightlessWeasel/clamav-webui/internal/notify"
)

func (s *Server) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.notify.Config().Redacted())
}

func (s *Server) handlePutNotifications(w http.ResponseWriter, r *http.Request) {
	var cfg notify.Config
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	s.notify.SetConfig(cfg)

	stored, _ := json.Marshal(s.notify.Config()) // Config() has secrets merged back in
	if err := s.db.SetNotifyConfig(string(stored)); err != nil {
		slog.Error("notifications: persist", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.notify.Config().Redacted())
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if err := s.notify.Test(); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "sent"})
}
