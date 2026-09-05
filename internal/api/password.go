package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/FlightlessWeasel/clamav-webui/internal/auth"
)

type changePasswordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// handleChangePassword verifies the current admin password, stores a new hash,
// and invalidates every session (including this one) so all clients re-login.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}

	settings, err := s.db.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ok, err := auth.VerifyPassword(settings.AdminPasswordHash, req.Current)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if len([]rune(strings.TrimSpace(req.New))) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "new password too short")
		return
	}

	hash, err := auth.HashPassword(req.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.db.SetAdminPassword(hash); err != nil {
		slog.Error("password: store", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.sessions.DestroyAll()
	s.endSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}
