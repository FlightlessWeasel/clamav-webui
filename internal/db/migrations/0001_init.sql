-- Singleton application settings. Exactly one row, id = 1.
CREATE TABLE settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    setup_complete      INTEGER NOT NULL DEFAULT 0,
    admin_password_hash TEXT    NOT NULL DEFAULT '',
    session_secret      TEXT    NOT NULL DEFAULT '',
    scan_defaults_json  TEXT    NOT NULL DEFAULT '{}',
    notify_config_json  TEXT    NOT NULL DEFAULT '{}',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO settings (id) VALUES (1);

-- One row per scan run (manual, scheduled or on-access triggered).
CREATE TABLE scans (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT    NOT NULL DEFAULT 'manual',
    status      TEXT    NOT NULL DEFAULT 'queued',
    paths_json  TEXT    NOT NULL DEFAULT '[]',
    options_json TEXT   NOT NULL DEFAULT '{}',
    engine      TEXT    NOT NULL DEFAULT '',
    db_version  TEXT    NOT NULL DEFAULT '',
    scanned     INTEGER NOT NULL DEFAULT 0,
    infected    INTEGER NOT NULL DEFAULT 0,
    error       TEXT    NOT NULL DEFAULT '',
    started_at  TEXT,
    finished_at TEXT,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_scans_created_at ON scans (created_at DESC);

-- Individual infected files found within a scan.
CREATE TABLE scan_findings (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id   INTEGER NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    path      TEXT    NOT NULL,
    signature TEXT    NOT NULL DEFAULT '',
    action    TEXT    NOT NULL DEFAULT 'none',
    created_at TEXT   NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_scan_findings_scan_id ON scan_findings (scan_id);

-- Recurring scan schedules evaluated by the in-app cron scheduler.
CREATE TABLE schedules (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,
    cron_expr    TEXT    NOT NULL,
    paths_json   TEXT    NOT NULL DEFAULT '[]',
    options_json TEXT    NOT NULL DEFAULT '{}',
    enabled      INTEGER NOT NULL DEFAULT 1,
    last_run_id  INTEGER REFERENCES scans (id) ON DELETE SET NULL,
    last_run_at  TEXT,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Quarantined files. The blob lives at <ConfigDir>/quarantine/<store_name>.
CREATE TABLE quarantine (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    store_name TEXT    NOT NULL UNIQUE,
    orig_path  TEXT    NOT NULL,
    signature  TEXT    NOT NULL DEFAULT '',
    sha256     TEXT    NOT NULL DEFAULT '',
    scan_id    INTEGER REFERENCES scans (id) ON DELETE SET NULL,
    status     TEXT    NOT NULL DEFAULT 'held',
    orig_mode  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_quarantine_status ON quarantine (status);

-- Worker job log: apt / freshclam / scan invocations and their streamed output.
CREATE TABLE jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT    NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'queued',
    ref_id     INTEGER,
    log        TEXT    NOT NULL DEFAULT '',
    error      TEXT    NOT NULL DEFAULT '',
    started_at TEXT,
    finished_at TEXT,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_jobs_created_at ON jobs (created_at DESC);

-- Audit + notification feed.
CREATE TABLE events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT    NOT NULL,
    severity TEXT    NOT NULL DEFAULT 'info',
    message  TEXT    NOT NULL DEFAULT '',
    meta_json TEXT   NOT NULL DEFAULT '{}',
    ts       TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_events_ts ON events (ts DESC);
