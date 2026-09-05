// Package scheduler runs enabled scan schedules on their cron expressions. It
// owns no scan logic: on each tick it calls back into the caller, which creates
// and enqueues the scan.
package scheduler

import (
	"log/slog"
	"sync"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/robfig/cron/v3"
)

// RunFunc is invoked (from a cron goroutine) when schedule id is due.
type RunFunc func(scheduleID int64)

// Scheduler keeps the cron entry set in sync with the enabled schedules table.
type Scheduler struct {
	db  *db.DB
	run RunFunc

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[int64]cron.EntryID
}

// New returns a stopped Scheduler.
func New(database *db.DB, run RunFunc) *Scheduler {
	return &Scheduler{db: database, run: run, entries: map[int64]cron.EntryID{}}
}

// ValidateSpec reports whether spec is a legal 5-field cron expression.
func ValidateSpec(spec string) error {
	_, err := cron.ParseStandard(spec)
	return err
}

// Start builds the cron runner and loads the current enabled schedules.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		return nil
	}
	// SkipIfStillRunning guards against a slow callback; runScheduledScan also
	// skips a tick while the schedule's previous scan is unfinished.
	s.cron = cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	s.cron.Start()
	return s.syncLocked()
}

// Stop halts the cron runner. Start can be called again afterwards.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.cron = nil
	s.entries = map[int64]cron.EntryID{}
}

// Reload re-reads the schedules table and adds/removes cron entries to match.
// Call it after any schedule CRUD.
func (s *Scheduler) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return nil
	}
	return s.syncLocked()
}

func (s *Scheduler) syncLocked() error {
	want, err := s.db.EnabledSchedules()
	if err != nil {
		return err
	}

	// Rebuild from scratch: schedule count is tiny and this keeps spec changes,
	// enable/disable and deletes all handled by one code path.
	for id, entryID := range s.entries {
		s.cron.Remove(entryID)
		delete(s.entries, id)
	}
	for _, sc := range want {
		entryID, err := s.cron.AddFunc(sc.CronExpr, func() { s.run(sc.ID) })
		if err != nil {
			slog.Error("scheduler: bad cron expr, skipping", "schedule", sc.ID, "expr", sc.CronExpr, "err", err)
			continue
		}
		s.entries[sc.ID] = entryID
	}
	slog.Info("scheduler: synced", "active", len(s.entries))
	return nil
}
