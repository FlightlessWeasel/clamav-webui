package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// handleEvents streams SSE events to the browser until the client disconnects.
// Every write carries a short deadline so a stalled client cannot pin the
// handler goroutine (and its bus subscription) forever.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(s string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !write(": connected\n\n") {
		return
	}

	sub, cancel := s.bus.Subscribe()
	defer cancel()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if !write(": ping\n\n") {
				return
			}
		case ev, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(ev.Data)
			if err != nil {
				slog.Error("sse: marshal event", "type", ev.Type, "err", err)
				continue
			}
			if !write(fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data)) {
				return
			}
		}
	}
}
