package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

const (
	sessionCookie = "cw_session"
	csrfCookie    = "cw_csrf"
	csrfHeader    = "X-CSRF-Token"
)

// requireAuth rejects requests without a live session cookie.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.sessions.Valid(c.Value) {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfGuard enforces double-submit CSRF on state-changing methods. GET/HEAD/
// OPTIONS pass through.
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(csrfCookie)
		header := r.Header.Get(csrfHeader)
		if err != nil || cookie.Value == "" || header == "" ||
			subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			writeError(w, http.StatusForbidden, "bad or missing CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// startSession creates a session and writes the session + CSRF cookies.
func (s *Server) startSession(w http.ResponseWriter) error {
	id, err := s.sessions.Create()
	if err != nil {
		return err
	}
	token, err := randToken()
	if err != nil {
		return err
	}
	secure := s.cfg.TLSEnabled()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // the SPA must read this to echo it back in a header
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// endSession destroys the current session and clears its cookies.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.Destroy(c.Value)
	}
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name == sessionCookie,
			Secure:   s.cfg.TLSEnabled(),
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func randToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
