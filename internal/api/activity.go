package api

import (
	"net/http"
	"strconv"
)

// handleActivity returns the recent worker jobs and event-feed entries for the
// Activity page.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	jobs, err := s.db.RecentJobs(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	events, err := s.db.RecentEvents(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "events": events})
}
