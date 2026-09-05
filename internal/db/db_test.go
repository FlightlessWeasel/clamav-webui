package db

import (
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrations(t *testing.T) {
	d := openTemp(t)

	// Every table from 0001 should exist.
	for _, tbl := range []string{"settings", "scans", "scan_findings", "schedules", "quarantine", "jobs", "events"} {
		var name string
		err := d.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}

	// Singleton settings row is seeded.
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM settings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("settings rows = %d, want 1", n)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")

	d1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	d1.Close()

	d2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer d2.Close()

	var applied int
	if err := d2.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied == 0 {
		t.Error("no migrations recorded")
	}
}

func openTemp(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
