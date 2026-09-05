package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad job id")
		return
	}
	job, err := s.db.GetJob(id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, job)
}
