package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
)

func (s *Server) handleClamAVInstall(w http.ResponseWriter, r *http.Request) {
	s.enqueueJob(w, "apt-install", 30*time.Minute, func(ctx context.Context, jc *worker.JobContext) error {
		return s.clam.InstallClamAV(ctx, func(line string) { jc.Logf("%s", line) })
	})
}

func (s *Server) handleClamAVUpgrade(w http.ResponseWriter, r *http.Request) {
	s.enqueueJob(w, "apt-upgrade", 30*time.Minute, func(ctx context.Context, jc *worker.JobContext) error {
		return s.clam.UpgradeClamAV(ctx, func(line string) { jc.Logf("%s", line) })
	})
}

// enqueueJob schedules fn as a worker job with a hard time cap and writes
// {"job_id": N} with 202.
func (s *Server) enqueueJob(w http.ResponseWriter, kind string, timeout time.Duration, fn worker.TaskFunc) {
	wrapped := func(ctx context.Context, jc *worker.JobContext) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return fn(ctx, jc)
	}
	id, err := s.jobs.Enqueue(kind, nil, wrapped)
	if err != nil {
		slog.Error("enqueue job", "kind", kind, "err", err)
		writeError(w, http.StatusServiceUnavailable, "could not schedule job")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}
