package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
)

func (s *Server) handleSignatures(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sig, err := s.clam.Signatures(ctx)
	if err != nil {
		slog.Error("signatures", "err", err)
		writeError(w, http.StatusBadGateway, "could not read signature state")
		return
	}
	writeJSON(w, http.StatusOK, sig)
}

func (s *Server) handleSignaturesUpdate(w http.ResponseWriter, r *http.Request) {
	s.enqueueJob(w, "freshclam", 20*time.Minute, func(ctx context.Context, jc *worker.JobContext) error {
		return s.clam.UpdateSignatures(ctx, func(line string) { jc.Logf("%s", line) })
	})
}
