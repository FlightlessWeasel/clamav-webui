package api

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

// tailerCtl lets an /api/onaccess change interrupt the current journal follow
// so the tailer re-evaluates whether to keep following.
type tailerCtl struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (t *tailerCtl) arm(c context.CancelFunc) {
	t.mu.Lock()
	t.cancel = c
	t.mu.Unlock()
}

func (t *tailerCtl) kick() {
	t.mu.Lock()
	c := t.cancel
	t.mu.Unlock()
	if c != nil {
		c()
	}
}

// tailerKick asks the on-access tailer to re-check status now.
func (s *Server) tailerKick() {
	if s.tailer != nil {
		s.tailer.kick()
	}
}

// runOnAccessTailer follows clamd's journal while on-access scanning is enabled
// and turns each FOUND line into a quarantine item + an event. It runs for the
// life of the server and is not started under the dev simulator.
func (s *Server) runOnAccessTailer(ctx context.Context) {
	backoff := 5 * time.Second

	for ctx.Err() == nil {
		st, err := s.clam.OnAccessStatus(ctx)
		if err != nil || !st.Enabled {
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			continue
		}

		followCtx, cancel := context.WithCancel(ctx)
		s.tailer.arm(cancel)
		slog.Info("on-access tailer: following clamav-daemon journal")

		start := time.Now()
		serr := s.clam.StreamJournal(followCtx, "clamav-daemon", func(line string) {
			if path, sig, ok := clamav.ParseOnAccessFound(line); ok {
				s.onOnAccessDetection(path, sig)
			}
		})
		cancel()
		s.tailer.arm(nil)

		if followCtx.Err() == context.Canceled && ctx.Err() == nil {
			continue // a kick() — loop back and re-check status immediately
		}
		// journalctl exited on its own. If it barely ran, it's probably failing
		// (permissions?) — back off further and make some noise.
		if time.Since(start) < 2*time.Second {
			slog.Error("on-access tailer: journalctl exited immediately", "err", serr, "retry_in", backoff)
			if !sleepCtx(ctx, backoff) {
				return
			}
			if backoff < 5*time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Second
		if !sleepCtx(ctx, backoff) {
			return
		}
	}
}

// onOnAccessDetection quarantines the file a real-time hit named (best-effort)
// and records the detection.
func (s *Server) onOnAccessDetection(path, signature string) {
	quarantined := false
	if err := s.checkTargetPath(path); err == nil {
		if sc, herr := s.qstore.Hold(path, signature, 0); herr == nil {
			if id, derr := s.db.AddQuarantine(sc.Name, sc.OrigPath, sc.Signature, sc.SHA256, nil, sc.OrigMode); derr == nil {
				quarantined = true
				s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": id, "action": "held", "path": sc.OrigPath}})
			} else {
				slog.Error("on-access: record quarantine", "err", derr)
				_ = s.qstore.Restore(sc)
			}
		} else {
			slog.Warn("on-access: could not quarantine (already handled?)", "path", path, "err", herr)
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
