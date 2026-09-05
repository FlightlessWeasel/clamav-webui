// Package worker runs long operations (apt install, freshclam, scans) off the
// request path. Each task is recorded in the jobs table and its output is both
// persisted and streamed over the SSE bus.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
)

// TaskFunc is the body of a job. Use jc.Logf to emit output; return a non-nil
// error to mark the job failed.
type TaskFunc func(ctx context.Context, jc *JobContext) error

// JobContext is handed to a running TaskFunc.
type JobContext struct {
	JobID int64
	m     *Manager
}

// Logf appends a line to the job log (persisted) and publishes a job-log event.
func (jc *JobContext) Logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if err := jc.m.db.AppendJobLog(jc.JobID, line+"\n"); err != nil {
		slog.Error("worker: append job log", "job", jc.JobID, "err", err)
	}
	jc.m.bus.Publish(sse.Event{Type: "job-log", Data: map[string]any{
		"job_id": jc.JobID,
		"line":   line,
	}})
}

type queued struct {
	id   int64
	kind string
	fn   TaskFunc
}

// Manager owns the worker goroutines and the job queue.
type Manager struct {
	db  *db.DB
	bus *sse.Bus

	ch     chan queued
	wg     sync.WaitGroup
	cancel context.CancelFunc
	ctx    context.Context
}

// New starts n worker goroutines (n >= 1).
func New(database *db.DB, bus *sse.Bus, n int) *Manager {
	if n < 1 {
		n = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		db:     database,
		bus:    bus,
		ch:     make(chan queued, 64),
		cancel: cancel,
		ctx:    ctx,
	}
	for i := 0; i < n; i++ {
		m.wg.Add(1)
		go m.loop()
	}
	return m
}

// Enqueue records a new job of the given kind and schedules fn. It returns the
// job id immediately. refID optionally links the job to another row (e.g. a
// scan).
func (m *Manager) Enqueue(kind string, refID *int64, fn TaskFunc) (int64, error) {
	id, err := m.db.CreateJob(kind, refID)
	if err != nil {
		return 0, err
	}
	m.bus.Publish(sse.Event{Type: "job", Data: map[string]any{"job_id": id, "kind": kind, "status": "queued"}})

	select {
	case m.ch <- queued{id: id, kind: kind, fn: fn}:
		return id, nil
	case <-m.ctx.Done():
		_ = m.db.FinishJob(id, "error", "server shutting down")
		return id, fmt.Errorf("worker stopped")
	}
}

// Shutdown cancels any running job and waits for the workers to exit.
func (m *Manager) Shutdown() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case q := <-m.ch:
			m.execute(q)
		}
	}
}

func (m *Manager) execute(q queued) {
	if err := m.db.SetJobRunning(q.id); err != nil {
		slog.Error("worker: set running", "job", q.id, "err", err)
	}
	m.bus.Publish(sse.Event{Type: "job", Data: map[string]any{"job_id": q.id, "kind": q.kind, "status": "running"}})

	jc := &JobContext{JobID: q.id, m: m}
	err := safeRun(m.ctx, q.fn, jc)

	status, errMsg := "done", ""
	if err != nil {
		status, errMsg = "error", err.Error()
		jc.Logf("error: %s", errMsg)
	}
	if e := m.db.FinishJob(q.id, status, errMsg); e != nil {
		slog.Error("worker: finish job", "job", q.id, "err", e)
	}
	m.bus.Publish(sse.Event{Type: "job", Data: map[string]any{
		"job_id": q.id, "kind": q.kind, "status": status, "error": errMsg,
	}})
}

func safeRun(ctx context.Context, fn TaskFunc, jc *JobContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx, jc)
}
