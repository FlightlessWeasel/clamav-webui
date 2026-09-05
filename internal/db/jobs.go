package db

import (
	"database/sql"
	"fmt"
)

// Job is a row from the jobs table: one apt/freshclam/scan invocation and its
// streamed output.
type Job struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Status     string `json:"status"` // queued, running, done, error
	RefID      int64  `json:"ref_id,omitempty"`
	Log        string `json:"log"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// CreateJob inserts a queued job and returns its id. refID may be nil.
func (d *DB) CreateJob(kind string, refID *int64) (int64, error) {
	res, err := d.Exec(`INSERT INTO jobs (kind, status, ref_id) VALUES (?, 'queued', ?)`, kind, refID)
	if err != nil {
		return 0, fmt.Errorf("create job: %w", err)
	}
	return res.LastInsertId()
}

// SetJobRunning marks a job running and stamps started_at.
func (d *DB) SetJobRunning(id int64) error {
	_, err := d.Exec(`UPDATE jobs SET status='running', started_at=datetime('now') WHERE id=?`, id)
	return err
}

// AppendJobLog appends chunk (already newline-terminated by the caller as
// needed) to a job's log column.
func (d *DB) AppendJobLog(id int64, chunk string) error {
	_, err := d.Exec(`UPDATE jobs SET log = log || ? WHERE id=?`, chunk, id)
	return err
}

// FinishJob sets the terminal status ("done" or "error"), the error text, and
// finished_at.
func (d *DB) FinishJob(id int64, status, errMsg string) error {
	_, err := d.Exec(`UPDATE jobs SET status=?, error=?, finished_at=datetime('now') WHERE id=?`,
		status, errMsg, id)
	return err
}

// FailStaleJobs marks every still-queued or still-running job as errored. It is
// called at startup (a previous process died mid-job) and at shutdown.
func (d *DB) FailStaleJobs(reason string) (int64, error) {
	res, err := d.Exec(`
		UPDATE jobs SET status='error', error=?, finished_at=datetime('now')
		WHERE status IN ('queued','running')`, reason)
	if err != nil {
		return 0, fmt.Errorf("fail stale jobs: %w", err)
	}
	return res.RowsAffected()
}

// RunningJobForRef returns the id of a running job of the given kind linked to
// refID, if one exists.
func (d *DB) RunningJobForRef(kind string, refID int64) (int64, bool, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM jobs WHERE kind=? AND ref_id=? AND status='running' ORDER BY id DESC LIMIT 1`,
		kind, refID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// GetJob returns one job by id.
func (d *DB) GetJob(id int64) (Job, error) {
	return scanJob(d.QueryRow(`
		SELECT id, kind, status, COALESCE(ref_id,0), log, error,
		       COALESCE(started_at,''), COALESCE(finished_at,''), created_at
		FROM jobs WHERE id=?`, id))
}

// RecentJobs returns the newest jobs, capped at limit.
func (d *DB) RecentJobs(limit int) ([]Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`
		SELECT id, kind, status, COALESCE(ref_id,0), log, error,
		       COALESCE(started_at,''), COALESCE(finished_at,''), created_at
		FROM jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (Job, error) {
	var j Job
	err := s.Scan(&j.ID, &j.Kind, &j.Status, &j.RefID, &j.Log, &j.Error,
		&j.StartedAt, &j.FinishedAt, &j.CreatedAt)
	if err == sql.ErrNoRows {
		return Job{}, ErrNotFound
	}
	return j, err
}
