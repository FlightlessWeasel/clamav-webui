package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

// runOnAccessTailer follows clamd's journal while on-access scanning is enabled
// and turns each FOUND line into a quarantine item + an event. It runs for the
// life of the server; it is not started under the dev simulator.
func (s *Server) runOnAccessTailer(ctx context.Context) {
	for ctx.Err() == nil {
		st, err := s.clam.OnAccessStatus(ctx)
		if err != nil || !st.Enabled {
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			continue
		}

		slog.Info("on-access tailer: following clamav-daemon journal")
		_ = s.clam.StreamJournal(ctx, "clamav-daemon", func(line string) {
			path, sig, ok := clamav.ParseOnAccessFound(line)
			if ok {
				s.onOnAccessDetection(path, sig)
			}
		})

		if !sleepCtx(ctx, 5*time.Second) {
			return
		}
	}
}

// onOnAccessDetection quarantines the file a real-time hit named (best-effort)
// and records the detection.
func (s *Server) onOnAccessDetection(path, signature string) {
	quarantined := false
	if err := s.checkTargetPath(path); err == nil {
		if sc, err := s.qstore.Hold(path, signature, 0); err == nil {
			if id, derr := s.db.AddQuarantine(sc.Name, sc.OrigPath, sc.Signature, sc.SHA256, nil, sc.OrigMode); derr == nil {
				quarantined = true
				s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": id, "action": "held", "path": sc.OrigPath}})
			} else {
				slog.Error("on-access: record quarantine", "err", derr)
				_ = s.qstore.Restore(sc)
			}
		} else {
			slog.Warn("on-access: could not quarantine (already handled?)", "path", path, "err", err)
		}
	}

	msg := "On-access detection: " + signature + " at " + path
	if quarantined {
		msg += " (quarantined)"
	}
	if _, err := s.db.AddEvent("onaccess-detection", "critical", msg,
		map[string]any{"path": path, "signature": signature, "quarantined": quarantined}); err != nil {
		slog.Error("on-access: add event", "err", err)
	}
	s.bus.Publish(sse.Event{Type: "event", Data: map[string]any{
		"kind": "onaccess-detection", "severity": "critical", "message": msg,
	}})
	s.notify.Dispatch("onaccess-detection", msg)
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
