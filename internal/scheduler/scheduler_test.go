package scheduler

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
)

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestValidateSpec(t *testing.T) {
	if err := ValidateSpec("*/5 * * * *"); err != nil {
		t.Errorf("valid 5-field spec rejected: %v", err)
	}
	if err := ValidateSpec("@every 30m"); err != nil {
		t.Errorf("@every rejected: %v", err)
	}
	if err := ValidateSpec("not a cron"); err == nil {
		t.Error("garbage spec accepted")
	}
}

func TestSchedulerFires(t *testing.T) {
	d := openDB(t)
	var fired int32
	s := New(d, func(int64) { atomic.AddInt32(&fired, 1) })

	if _, err := d.CreateSchedule(db.ScheduleInput{
		Name: "fast", CronExpr: "@every 1s", Paths: []string{"/tmp"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fired) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("schedule never fired")
}

func TestReloadTracksEnabledSet(t *testing.T) {
	d := openDB(t)
	s := New(d, func(int64) {})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	id, _ := d.CreateSchedule(db.ScheduleInput{Name: "a", CronExpr: "0 3 * * *", Paths: []string{"/srv"}, Enabled: true})
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := s.entryCount(); got != 1 {
		t.Fatalf("entries = %d, want 1", got)
	}

	_ = d.UpdateSchedule(id, db.ScheduleInput{Name: "a", CronExpr: "0 3 * * *", Paths: []string{"/srv"}, Enabled: false})
	s.Reload()
	if got := s.entryCount(); got != 0 {
		t.Fatalf("entries after disable = %d, want 0", got)
	}

	_ = d.UpdateSchedule(id, db.ScheduleInput{Name: "a", CronExpr: "0 3 * * *", Paths: []string{"/srv"}, Enabled: true})
	s.Reload()
	_ = d.DeleteSchedule(id)
	s.Reload()
	if got := s.entryCount(); got != 0 {
		t.Fatalf("entries after delete = %d, want 0", got)
	}
}

// entryCount is a test-only peek at the tracked cron entries.
func (s *Scheduler) entryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}
