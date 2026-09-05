package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/FlightlessWeasel/clamav-webui/internal/auth"
)

type passwordRequest struct {
	Password string `json:"password"`
}

// handleStatus reports what the SPA needs before authenticating: whether initial
// setup is done and whether this request carries a live session.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.GetSettings()
	if err != nil {
		slog.Error("status: get settings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	authed := false
	if c, err := r.Cookie(sessionCookie); err == nil {
		authed = s.sessions.Valid(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_complete": settings.SetupComplete,
		"authenticated":  authed,
		"version":        s.version,
		"tls":            s.cfg.TLSEnabled(),
	})
}

// handleSetup sets the admin password exactly once, on a fresh install.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if settings.SetupComplete {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}

	var req passwordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	if len([]rune(strings.TrimSpace(req.Password))) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "password too short")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.db.SetAdminPassword(hash); err != nil {
		slog.Error("setup: store password", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.startSession(w); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	slog.Info("initial setup completed")
	w.WriteHeader(http.StatusNoContent)
}

// handleLogin verifies the admin password and starts a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.logins.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, wait a minute")
		return
	}

	settings, err := s.db.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !settings.SetupComplete {
		writeError(w, http.StatusConflict, "setup not completed")
		return
	}

	var req passwordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}

	ok, err := auth.VerifyPassword(settings.AdminPasswordHash, req.Password)
	if err != nil || !ok {
		if err != nil {
			slog.Error("login: verify", "err", err)
		}
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.logins.Reset(ip)
	if err := s.startSession(w); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLogout ends the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.endSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func clientIP(r *http.Request) string {
	// chi's RealIP middleware has already normalised RemoteAddr from proxy
	// headers when present.
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 && !strings.Contains(host[i+1:], "]") {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}

func jsonErrMsg(err error) string {
	if errors.Is(err, io.EOF) {
		return "empty request body"
	}
	return "invalid request body"
}
