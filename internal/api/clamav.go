package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
)

func (s *Server) handleClamAVInstall(w http.ResponseWriter, r *http.Request) {
	s.enqueueApt(w, "apt-install", func(ctx context.Context, jc *worker.JobContext) error {
		return s.clam.InstallClamAV(ctx, func(line string) { jc.Logf("%s", line) })
	})
}

func (s *Server) handleClamAVUpgrade(w http.ResponseWriter, r *http.Request) {
	s.enqueueApt(w, "apt-upgrade", func(ctx context.Context, jc *worker.JobContext) error {
		return s.clam.UpgradeClamAV(ctx, func(line string) { jc.Logf("%s", line) })
	})
}

// enqueueApt schedules an apt job with a hard time cap and returns its id.
func (s *Server) enqueueApt(w http.ResponseWriter, kind string, fn worker.TaskFunc) {
	wrapped := func(ctx context.Context, jc *worker.JobContext) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		return fn(ctx, jc)
	}
	id, err := s.jobs.Enqueue(kind, nil, wrapped)
	if err != nil {
		slog.Error("clamav: enqueue", "kind", kind, "err", err)
		writeError(w, http.StatusServiceUnavailable, "could not schedule job")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}
