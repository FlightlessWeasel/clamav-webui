package db

import (
	"database/sql"
	"fmt"
)

// QuarantineItem is a row from the quarantine table.
type QuarantineItem struct {
	ID        int64  `json:"id"`
	StoreName string `json:"store_name"`
	OrigPath  string `json:"orig_path"`
	Signature string `json:"signature"`
	SHA256    string `json:"sha256"`
	ScanID    int64  `json:"scan_id,omitempty"`
	Status    string `json:"status"` // held, restored, deleted
	OrigMode  uint32 `json:"orig_mode"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AddQuarantine records a newly held file. scanID may be nil.
func (d *DB) AddQuarantine(storeName, origPath, signature, sha256 string, scanID *int64, origMode uint32) (int64, error) {
	res, err := d.Exec(`
		INSERT INTO quarantine (store_name, orig_path, signature, sha256, scan_id, status, orig_mode)
		VALUES (?, ?, ?, ?, ?, 'held', ?)`,
		storeName, origPath, signature, sha256, scanID, origMode)
	if err != nil {
		return 0, fmt.Errorf("add quarantine: %w", err)
	}
	return res.LastInsertId()
}

// SetQuarantineStatus moves an item to "restored" or "deleted".
func (d *DB) SetQuarantineStatus(id int64, status string) error {
	res, err := d.Exec(`UPDATE quarantine SET status=?, updated_at=datetime('now') WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetQuarantine returns one item by id.
func (d *DB) GetQuarantine(id int64) (QuarantineItem, error) {
	return scanQuarantineRow(d.QueryRow(quarantineSelect+` WHERE id=?`, id))
}

// ListQuarantine returns every item (held, restored and deleted), newest first.
func (d *DB) ListQuarantine() ([]QuarantineItem, error) {
	rows, err := d.Query(quarantineSelect + ` ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QuarantineItem
	for rows.Next() {
		it, err := scanQuarantineRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CountQuarantineHeld returns how many items are currently in the store.
func (d *DB) CountQuarantineHeld() (int, error) {
	var n int
	err := d.QueryRow(`SELECT count(*) FROM quarantine WHERE status='held'`).Scan(&n)
	return n, err
}

const quarantineSelect = `
	SELECT id, store_name, orig_path, signature, sha256, COALESCE(scan_id,0),
	       status, orig_mode, created_at, updated_at
	FROM quarantine`

func scanQuarantineRow(s scanner) (QuarantineItem, error) {
	var it QuarantineItem
	err := s.Scan(&it.ID, &it.StoreName, &it.OrigPath, &it.Signature, &it.SHA256,
		&it.ScanID, &it.Status, &it.OrigMode, &it.CreatedAt, &it.UpdatedAt)
	if err == sql.ErrNoRows {
		return QuarantineItem{}, ErrNotFound
	}
	return it, err
}
