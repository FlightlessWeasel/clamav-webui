// Package api wires the HTTP surface: JSON endpoints under /api and the embedded
// web UI for everything else.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/auth"
	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/config"
	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/FlightlessWeasel/clamav-webui/internal/webui"
	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server holds the dependencies shared by all handlers.
type Server struct {
	cfg      config.Config
	db       *db.DB
	version  string
	sessions *auth.SessionManager
	logins   *auth.LoginLimiter
	clam     *clamav.Manager
	bus      *sse.Bus
	jobs     *worker.Manager
	// sessionSecret is persisted at first start. Sessions are currently
	// in-memory only; the secret is reserved for signing persistent tokens.
	sessionSecret string
	mux           http.Handler
}

// New builds a Server, its background worker, and the route table. Call Close
// when done.
func New(cfg config.Config, database *db.DB, version string) (*Server, error) {
	bus := sse.NewBus()
	clam := clamav.NewManager(cfg)
	if os.Getenv("CLAMWEB_DEV_SIM") == "1" {
		slog.Warn("CLAMWEB_DEV_SIM=1: using the in-memory ClamAV simulator, not the real toolchain")
		clam = clamav.NewManagerWithRunner(cfg, clamav.NewSimRunner())
	}
	secret, err := database.EnsureSessionSecret()
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:           cfg,
		db:            database,
		version:       version,
		sessions:      auth.NewSessionManager(),
		logins:        auth.NewLoginLimiter(5, time.Minute),
		clam:          clam,
		bus:           bus,
		jobs:          worker.New(database, bus, 2),
		sessionSecret: secret,
	}
	s.mux = s.routes()
	return s, nil
}

// Handler is the root http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

// Close stops the background worker.
func (s *Server) Close() { s.jobs.Shutdown() }

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		// Public.
		r.Get("/health", s.handleHealth)
		r.Get("/version", s.handleVersion)
		r.Get("/status", s.handleStatus)
		r.Post("/setup", s.handleSetup)
		r.Post("/login", s.handleLogin)

		// Authenticated.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.csrfGuard)

			r.Post("/logout", s.handleLogout)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/events", s.handleEvents)

			r.Get("/services", s.handleServices)
			r.Post("/services/{unit}/{action}", s.handleServiceAction)
			r.Get("/services/{unit}/logs", s.handleServiceLogs)

			r.Post("/clamav/install", s.handleClamAVInstall)
			r.Post("/clamav/upgrade", s.handleClamAVUpgrade)

			r.Get("/jobs/{id}", s.handleGetJob)
		})
	})

	r.Handle("/*", webui.Handler())
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
}

// writeJSON serializes v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode", "err", err)
	}
}

// writeError sends a JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON reads the request body into v, capping the size at 1 MiB. Passing
// w lets MaxBytesReader mark the connection unusable if the limit is hit.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Debug("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"reqid", middleware.GetReqID(r.Context()),
		)
	})
}
