package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

func newTestManager(t *testing.T) (*Manager, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	m := New(d, sse.NewBus(), 1)
	t.Cleanup(m.Shutdown)
	return m, d
}

func waitJob(t *testing.T, d *db.DB, id int64, want string) db.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, err := d.GetJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == want {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := d.GetJob(id)
	t.Fatalf("job %d status = %q, want %q", id, j.Status, want)
	return j
}

func TestJobRunsAndLogs(t *testing.T) {
	m, d := newTestManager(t)

	id, err := m.Enqueue("test", nil, func(_ context.Context, jc *JobContext) error {
		jc.Logf("hello %s", "world")
		jc.Logf("second line")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	j := waitJob(t, d, id, "done")
	if j.Log != "hello world\nsecond line\n" {
		t.Errorf("log = %q", j.Log)
	}
	if j.StartedAt == "" || j.FinishedAt == "" {
		t.Errorf("timestamps not set: %+v", j)
	}
}

func TestJobError(t *testing.T) {
	m, d := newTestManager(t)
	id, _ := m.Enqueue("test", nil, func(_ context.Context, _ *JobContext) error {
		return errors.New("boom")
	})
	j := waitJob(t, d, id, "error")
	if j.Error != "boom" {
		t.Errorf("error = %q", j.Error)
	}
}

func TestJobPanicIsCaught(t *testing.T) {
	m, d := newTestManager(t)
	id, _ := m.Enqueue("test", nil, func(_ context.Context, _ *JobContext) error {
		panic("kaboom")
	})
	j := waitJob(t, d, id, "error")
	if j.Error == "" {
		t.Error("panic should be recorded as job error")
	}
}

func TestStaleJobsFailedOnStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")

	d1, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a job left mid-flight by a killed process.
	id, _ := d1.CreateJob("scan", nil)
	if err := d1.SetJobRunning(id); err != nil {
		t.Fatal(err)
	}
	d1.Close()

	d2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	m := New(d2, sse.NewBus(), 1)
	defer m.Shutdown()

	j, err := d2.GetJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != "error" || j.Error == "" {
		t.Fatalf("stale job not reconciled: %+v", j)
	}
}

func TestCancelStopsRunningJob(t *testing.T) {
	m, d := newTestManager(t)

	started := make(chan struct{})
	id, err := m.Enqueue("test", nil, func(ctx context.Context, _ *JobContext) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	if !m.Cancel(id) {
		t.Fatal("Cancel returned false for a running job")
	}
	j := waitJob(t, d, id, "error")
	if j.Error == "" {
		t.Errorf("canceled job should record an error, got %+v", j)
	}
	if m.Cancel(id) {
		t.Error("Cancel should return false once the job has finished")
	}
}

func TestEventsPublished(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	bus := sse.NewBus()
	sub, cancel := bus.Subscribe()
	defer cancel()

	m := New(d, bus, 1)
	defer m.Shutdown()

	_, err = m.Enqueue("test", nil, func(_ context.Context, jc *JobContext) error {
		jc.Logf("x")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case ev := <-sub:
			seen[ev.Type] = true
		case <-timeout:
			t.Fatalf("did not see expected events, saw: %v", seen)
		}
	}
	if !seen["job"] || !seen["job-log"] {
		t.Errorf("missing event types: %v", seen)
	}
}
