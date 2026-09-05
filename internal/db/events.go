package db

import (
	"encoding/json"
	"fmt"
)

// Event is a row from the events feed (audit + notification history).
type Event struct {
	ID       int64           `json:"id"`
	Kind     string          `json:"kind"`
	Severity string          `json:"severity"` // info, warning, critical
	Message  string          `json:"message"`
	Meta     json.RawMessage `json:"meta"`
	TS       string          `json:"ts"`
}

// AddEvent appends to the events feed. meta may be nil.
func (d *DB) AddEvent(kind, severity, message string, meta any) (int64, error) {
	mj := []byte("{}")
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			mj = b
		}
	}
	res, err := d.Exec(`INSERT INTO events (kind, severity, message, meta_json) VALUES (?, ?, ?, ?)`,
		kind, severity, message, string(mj))
	if err != nil {
		return 0, fmt.Errorf("add event: %w", err)
	}
	return res.LastInsertId()
}

// RecentEvents returns the newest events, capped at limit.
func (d *DB) RecentEvents(limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`
		SELECT id, kind, severity, message, meta_json, ts
		FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var meta string
		if err := rows.Scan(&e.ID, &e.Kind, &e.Severity, &e.Message, &meta, &e.TS); err != nil {
			return nil, err
		}
		e.Meta = json.RawMessage(meta)
		out = append(out, e)
	}
	return out, rows.Err()
}
