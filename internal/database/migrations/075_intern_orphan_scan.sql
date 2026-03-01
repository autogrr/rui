-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 075: Intern orphan_scan_settings, orphan_scan_runs, orphan_scan_files
-- Converts TEXT columns → string_pool references, adds owner_id.

-- ─── orphan_scan_settings ───────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT ignore_paths FROM orphan_scan_settings WHERE ignore_paths IS NOT NULL
UNION SELECT DISTINCT preview_sort FROM orphan_scan_settings WHERE preview_sort IS NOT NULL;

INSERT OR IGNORE INTO string_pool (value) SELECT 'size_desc';

CREATE TABLE orphan_scan_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    grace_period_minutes INTEGER NOT NULL DEFAULT 10,
    ignore_paths_id INTEGER REFERENCES string_pool(id),
    scan_interval_hours INTEGER NOT NULL DEFAULT 24,
    max_files_per_run INTEGER NOT NULL DEFAULT 10000,
    auto_cleanup_enabled INTEGER NOT NULL DEFAULT 0,
    auto_cleanup_max_files INTEGER NOT NULL DEFAULT 100,
    preview_sort_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id)
);

INSERT INTO orphan_scan_settings_new (
    id, owner_id, instance_id, enabled, grace_period_minutes,
    ignore_paths_id, scan_interval_hours, max_files_per_run,
    auto_cleanup_enabled, auto_cleanup_max_files, preview_sort_id,
    created_at, updated_at
)
SELECT
    s.id,
    (SELECT id FROM users LIMIT 1),
    s.instance_id, s.enabled, s.grace_period_minutes,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.ignore_paths),
    s.scan_interval_hours, s.max_files_per_run,
    s.auto_cleanup_enabled, s.auto_cleanup_max_files,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.preview_sort),
    s.created_at, s.updated_at
FROM orphan_scan_settings s;

DROP TRIGGER IF EXISTS trg_orphan_scan_settings_updated;
DROP TABLE orphan_scan_settings;
ALTER TABLE orphan_scan_settings_new RENAME TO orphan_scan_settings;

CREATE INDEX IF NOT EXISTS idx_orphan_scan_settings_owner ON orphan_scan_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_orphan_scan_settings_updated
AFTER UPDATE ON orphan_scan_settings BEGIN
    UPDATE orphan_scan_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS orphan_scan_settings_view AS
SELECT oss.id, oss.owner_id, oss.instance_id, oss.enabled, oss.grace_period_minutes,
       sp_ip.value AS ignore_paths, oss.scan_interval_hours, oss.max_files_per_run,
       oss.auto_cleanup_enabled, oss.auto_cleanup_max_files, sp_ps.value AS preview_sort,
       oss.created_at, oss.updated_at
FROM orphan_scan_settings oss
LEFT JOIN string_pool sp_ip ON oss.ignore_paths_id = sp_ip.id
JOIN string_pool sp_ps ON oss.preview_sort_id = sp_ps.id;

-- ─── orphan_scan_runs ───────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT status FROM orphan_scan_runs WHERE status IS NOT NULL
UNION SELECT DISTINCT triggered_by FROM orphan_scan_runs WHERE triggered_by IS NOT NULL
UNION SELECT DISTINCT scan_paths FROM orphan_scan_runs WHERE scan_paths IS NOT NULL
UNION SELECT DISTINCT error_message FROM orphan_scan_runs WHERE error_message IS NOT NULL;

CREATE TABLE orphan_scan_runs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    scan_paths_id INTEGER REFERENCES string_pool(id),
    files_found INTEGER DEFAULT 0,
    files_deleted INTEGER DEFAULT 0,
    folders_deleted INTEGER DEFAULT 0,
    bytes_reclaimed INTEGER DEFAULT 0,
    truncated INTEGER NOT NULL DEFAULT 0,
    error_message_id INTEGER REFERENCES string_pool(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

INSERT INTO orphan_scan_runs_new (
    id, owner_id, instance_id, status_id, triggered_by_id, scan_paths_id,
    files_found, files_deleted, folders_deleted, bytes_reclaimed, truncated,
    error_message_id, started_at, completed_at
)
SELECT
    r.id,
    (SELECT id FROM users LIMIT 1),
    r.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.triggered_by),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.scan_paths),
    r.files_found, r.files_deleted, r.folders_deleted, r.bytes_reclaimed, r.truncated,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.error_message),
    r.started_at, r.completed_at
FROM orphan_scan_runs r;

DROP TABLE orphan_scan_runs;
ALTER TABLE orphan_scan_runs_new RENAME TO orphan_scan_runs;

CREATE INDEX IF NOT EXISTS idx_orphan_scan_runs_instance ON orphan_scan_runs(instance_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_orphan_scan_runs_owner ON orphan_scan_runs(owner_id);

CREATE VIEW IF NOT EXISTS orphan_scan_runs_view AS
SELECT osr.id, osr.owner_id, osr.instance_id,
       sp_s.value AS status, sp_tb.value AS triggered_by, sp_sp.value AS scan_paths,
       osr.files_found, osr.files_deleted, osr.folders_deleted, osr.bytes_reclaimed,
       osr.truncated, sp_em.value AS error_message, osr.started_at, osr.completed_at
FROM orphan_scan_runs osr
JOIN string_pool sp_s  ON osr.status_id = sp_s.id
JOIN string_pool sp_tb ON osr.triggered_by_id = sp_tb.id
LEFT JOIN string_pool sp_sp ON osr.scan_paths_id = sp_sp.id
LEFT JOIN string_pool sp_em ON osr.error_message_id = sp_em.id;

-- ─── orphan_scan_files ──────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT file_path FROM orphan_scan_files WHERE file_path IS NOT NULL
UNION SELECT DISTINCT status FROM orphan_scan_files WHERE status IS NOT NULL
UNION SELECT DISTINCT error_message FROM orphan_scan_files WHERE error_message IS NOT NULL;

INSERT OR IGNORE INTO string_pool (value) SELECT 'pending';

CREATE TABLE orphan_scan_files_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES orphan_scan_runs(id) ON DELETE CASCADE,
    file_path_id INTEGER NOT NULL REFERENCES string_pool(id),
    file_size INTEGER NOT NULL,
    modified_at DATETIME,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id)
);

INSERT INTO orphan_scan_files_new (
    id, run_id, file_path_id, file_size, modified_at, status_id, error_message_id
)
SELECT
    f.id, f.run_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.file_path),
    f.file_size, f.modified_at,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.error_message)
FROM orphan_scan_files f;

DROP TABLE orphan_scan_files;
ALTER TABLE orphan_scan_files_new RENAME TO orphan_scan_files;

CREATE INDEX IF NOT EXISTS idx_orphan_scan_files_run ON orphan_scan_files(run_id);

CREATE VIEW IF NOT EXISTS orphan_scan_files_view AS
SELECT osf.id, osf.run_id, sp_fp.value AS file_path, osf.file_size, osf.modified_at,
       sp_s.value AS status, sp_em.value AS error_message
FROM orphan_scan_files osf
JOIN string_pool sp_fp ON osf.file_path_id = sp_fp.id
JOIN string_pool sp_s  ON osf.status_id = sp_s.id
LEFT JOIN string_pool sp_em ON osf.error_message_id = sp_em.id;
