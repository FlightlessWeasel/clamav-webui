package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Scan is a row from the scans table.
type Scan struct {
	ID         int64           `json:"id"`
	Source     string          `json:"source"`
	Status     string          `json:"status"` // queued, running, done, error, canceled
	Paths      []string        `json:"paths"`
	Options    json.RawMessage `json:"options"`
	Engine     string          `json:"engine"`
	DBVersion  string          `json:"db_version"`
	Scanned    int             `json:"scanned"`
	Infected   int             `json:"infected"`
	Error      string          `json:"error,omitempty"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	CreatedAt  string          `json:"created_at"`
}

// ScanFinding is a row from scan_findings.
type ScanFinding struct {
	ID        int64  `json:"id"`
	ScanID    int64  `json:"scan_id"`
	Path      string `json:"path"`
	Signature string `json:"signature"`
	Action    string `json:"action"` // none, quarantined, failed
	CreatedAt string `json:"created_at"`
}

// CreateScan inserts a queued scan and returns its id.
func (d *DB) CreateScan(source string, paths []string, options any) (int64, error) {
	pj, err := json.Marshal(paths)
	if err != nil {
		return 0, err
	}
	oj, err := json.Marshal(options)
	if err != nil {
		return 0, err
	}
	res, err := d.Exec(`INSERT INTO scans (source, status, paths_json, options_json) VALUES (?, 'queued', ?, ?)`,
		source, string(pj), string(oj))
	if err != nil {
		return 0, fmt.Errorf("create scan: %w", err)
	}
	return res.LastInsertId()
}

// StartScan marks a scan running and records the engine/DB versions in use.
func (d *DB) StartScan(id int64, engine, dbVersion string) error {
	_, err := d.Exec(`UPDATE scans SET status='running', engine=?, db_version=?, started_at=datetime('now') WHERE id=?`,
		engine, dbVersion, id)
	return err
}

// UpdateScanProgress records interim counts while a scan runs.
func (d *DB) UpdateScanProgress(id int64, scanned, infected int) error {
	_, err := d.Exec(`UPDATE scans SET scanned=?, infected=? WHERE id=?`, scanned, infected, id)
	return err
}

// FinishScan writes the terminal status and final counts.
func (d *DB) FinishScan(id int64, status string, scanned, infected int, errMsg string) error {
	_, err := d.Exec(`
		UPDATE scans SET status=?, scanned=?, infected=?, error=?, finished_at=datetime('now')
		WHERE id=?`, status, scanned, infected, errMsg, id)
	return err
}

// AddFinding records one infected file for a scan.
func (d *DB) AddFinding(scanID int64, path, signature string) (int64, error) {
	res, err := d.Exec(`INSERT INTO scan_findings (scan_id, path, signature) VALUES (?, ?, ?)`,
		scanID, path, signature)
	if err != nil {
		return 0, fmt.Errorf("add finding: %w", err)
	}
	return res.LastInsertId()
}

// SetFindingAction updates a finding's action column (e.g. "quarantined").
func (d *DB) SetFindingAction(findingID int64, action string) error {
	_, err := d.Exec(`UPDATE scan_findings SET action=? WHERE id=?`, action, findingID)
	return err
}

// GetScan returns one scan by id.
func (d *DB) GetScan(id int64) (Scan, error) {
	return scanScanRow(d.QueryRow(scanSelect+` WHERE id=?`, id))
}

// ListScans returns the newest scans, capped at limit.
func (d *DB) ListScans(limit int) ([]Scan, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(scanSelect+` ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Scan
	for rows.Next() {
		s, err := scanScanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ScanFindings returns all findings for a scan, oldest first.
func (d *DB) ScanFindings(scanID int64) ([]ScanFinding, error) {
	rows, err := d.Query(`
		SELECT id, scan_id, path, signature, action, created_at
		FROM scan_findings WHERE scan_id=? ORDER BY id`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScanFinding
	for rows.Next() {
		var f ScanFinding
		if err := rows.Scan(&f.ID, &f.ScanID, &f.Path, &f.Signature, &f.Action, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

const scanSelect = `
	SELECT id, source, status, paths_json, options_json, engine, db_version,
	       scanned, infected, error,
	       COALESCE(started_at,''), COALESCE(finished_at,''), created_at
	FROM scans`

func scanScanRow(s scanner) (Scan, error) {
	var sc Scan
	var pathsJSON, optsJSON string
	err := s.Scan(&sc.ID, &sc.Source, &sc.Status, &pathsJSON, &optsJSON, &sc.Engine, &sc.DBVersion,
		&sc.Scanned, &sc.Infected, &sc.Error, &sc.StartedAt, &sc.FinishedAt, &sc.CreatedAt)
	if err == sql.ErrNoRows {
		return Scan{}, ErrNotFound
	}
	if err != nil {
		return Scan{}, err
	}
	_ = json.Unmarshal([]byte(pathsJSON), &sc.Paths)
	sc.Options = json.RawMessage(optsJSON)
	return sc, nil
}
