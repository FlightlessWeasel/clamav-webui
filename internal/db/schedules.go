package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Schedule is a row from the schedules table.
type Schedule struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	CronExpr  string          `json:"cron_expr"`
	Paths     []string        `json:"paths"`
	Options   json.RawMessage `json:"options"`
	Enabled   bool            `json:"enabled"`
	LastRunID int64           `json:"last_run_id,omitempty"`
	LastRunAt string          `json:"last_run_at,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// ScheduleInput is the mutable part of a schedule.
type ScheduleInput struct {
	Name     string
	CronExpr string
	Paths    []string
	Options  any
	Enabled  bool
}

// CreateSchedule inserts a schedule and returns its id.
func (d *DB) CreateSchedule(in ScheduleInput) (int64, error) {
	pj, _ := json.Marshal(in.Paths)
	oj, _ := json.Marshal(in.Options)
	res, err := d.Exec(`
		INSERT INTO schedules (name, cron_expr, paths_json, options_json, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		in.Name, in.CronExpr, string(pj), string(oj), boolToInt(in.Enabled))
	if err != nil {
		return 0, fmt.Errorf("create schedule: %w", err)
	}
	return res.LastInsertId()
}

// UpdateSchedule replaces the mutable fields of a schedule.
func (d *DB) UpdateSchedule(id int64, in ScheduleInput) error {
	pj, _ := json.Marshal(in.Paths)
	oj, _ := json.Marshal(in.Options)
	res, err := d.Exec(`
		UPDATE schedules
		SET name=?, cron_expr=?, paths_json=?, options_json=?, enabled=?, updated_at=datetime('now')
		WHERE id=?`,
		in.Name, in.CronExpr, string(pj), string(oj), boolToInt(in.Enabled), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSchedule removes a schedule.
func (d *DB) DeleteSchedule(id int64) error {
	res, err := d.Exec(`DELETE FROM schedules WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetScheduleLastRun records the most recent scan a schedule kicked off.
func (d *DB) SetScheduleLastRun(id, scanID int64) error {
	_, err := d.Exec(`UPDATE schedules SET last_run_id=?, last_run_at=datetime('now') WHERE id=?`, scanID, id)
	return err
}

// GetSchedule returns one schedule.
func (d *DB) GetSchedule(id int64) (Schedule, error) {
	return scanScheduleRow(d.QueryRow(scheduleSelect+` WHERE id=?`, id))
}

// ListSchedules returns all schedules, newest first.
func (d *DB) ListSchedules() ([]Schedule, error) {
	rows, err := d.Query(scheduleSelect + ` ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Schedule
	for rows.Next() {
		s, err := scanScheduleRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EnabledSchedules returns only the enabled schedules (for the scheduler).
func (d *DB) EnabledSchedules() ([]Schedule, error) {
	all, err := d.ListSchedules()
	if err != nil {
		return nil, err
	}
	var out []Schedule
	for _, s := range all {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

const scheduleSelect = `
	SELECT id, name, cron_expr, paths_json, options_json, enabled,
	       COALESCE(last_run_id,0), COALESCE(last_run_at,''), created_at, updated_at
	FROM schedules`

func scanScheduleRow(s scanner) (Schedule, error) {
	var sc Schedule
	var pathsJSON, optsJSON string
	var enabled int
	err := s.Scan(&sc.ID, &sc.Name, &sc.CronExpr, &pathsJSON, &optsJSON, &enabled,
		&sc.LastRunID, &sc.LastRunAt, &sc.CreatedAt, &sc.UpdatedAt)
	if err == sql.ErrNoRows {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}
	_ = json.Unmarshal([]byte(pathsJSON), &sc.Paths)
	sc.Options = json.RawMessage(optsJSON)
	sc.Enabled = enabled != 0
	return sc, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
