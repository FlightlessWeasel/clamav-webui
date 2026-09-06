-- Disk-image auto-mount configuration for scans. JSON blob:
--   {"enabled": bool, "extensions": [".iso", ".udf", ".img"]}
-- Off by default; see internal/clamav/mount.go.
ALTER TABLE settings ADD COLUMN scan_mount_json TEXT NOT NULL DEFAULT '{}';
