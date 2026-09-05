package api

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

const (
	staleAfter    = 7 * 24 * time.Hour
	freshCheckGap = 6 * time.Hour
	realertGap    = 24 * time.Hour
)

// runFreshnessMonitor periodically checks signature age and raises one alert
// per realertGap while the databases stay stale.
func (s *Server) runFreshnessMonitor(ctx context.Context) {
	// A short initial delay so this doesn't fire during startup churn.
	if !sleepCtx(ctx, time.Minute) {
		return
	}
	var lastAlert time.Time

	for {
		sig, err := s.clam.Signatures(ctx)
		if err == nil && sig.AgeSeconds >= 0 && time.Duration(sig.AgeSeconds)*time.Second > staleAfter {
			if time.Since(lastAlert) > realertGap {
				lastAlert = time.Now()
				days := sig.AgeSeconds / 86400
				msg := "ClamAV signatures are stale: last updated ~" + strconv.FormatInt(days, 10) + " days ago."
				if _, e := s.db.AddEvent("signatures-stale", "warning", msg, map[string]any{"age_seconds": sig.AgeSeconds}); e != nil {
					slog.Error("freshness monitor: add event", "err", e)
				}
				s.bus.Publish(sse.Event{Type: "event", Data: map[string]any{"kind": "signatures-stale", "severity": "warning", "message": msg}})
				s.notify.Dispatch("signatures-stale", msg)
			}
		} else {
			lastAlert = time.Time{}
		}

		if !sleepCtx(ctx, freshCheckGap) {
			return
		}
	}
}
