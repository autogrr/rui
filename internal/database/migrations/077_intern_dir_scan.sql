-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 077: Intern dir_scan_settings, directories, runs, files, run_injections
-- Converts TEXT → string_pool, removes CHECK(id=1), adds owner_id.

-- ─── dir_scan_settings ──────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT match_mode FROM dir_scan_settings WHERE match_mode IS NOT NULL
UNION SELECT DISTINCT category FROM dir_scan_settings WHERE category IS NOT NULL
UNION SELECT DISTINCT tags FROM dir_scan_settings WHERE tags IS NOT NULL;

INSERT OR IGNORE INTO string_pool (value) SELECT 'strict';

CREATE TABLE dir_scan_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    match_mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    size_tolerance_percent REAL NOT NULL DEFAULT 5.0,
    min_piece_ratio REAL NOT NULL DEFAULT 0.98,
    allow_partial INTEGER NOT NULL DEFAULT 0,
    skip_piece_boundary_safety_check INTEGER NOT NULL DEFAULT 1,
    start_paused INTEGER NOT NULL DEFAULT 1,
    category_id INTEGER REFERENCES string_pool(id),
    tags_id INTEGER REFERENCES string_pool(id),
    max_searchees_per_run INTEGER NOT NULL DEFAULT 0,
    max_searchee_age_days INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO dir_scan_settings_new (
    id, owner_id, enabled, match_mode_id,
    size_tolerance_percent, min_piece_ratio, allow_partial,
    skip_piece_boundary_safety_check, start_paused,
    category_id, tags_id, max_searchees_per_run, max_searchee_age_days,
    created_at, updated_at
)
SELECT
    s.id,
    (SELECT id FROM users LIMIT 1),
    s.enabled,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.match_mode),
    s.size_tolerance_percent, s.min_piece_ratio, s.allow_partial,
    s.skip_piece_boundary_safety_check, s.start_paused,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.category),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.tags),
    s.max_searchees_per_run, s.max_searchee_age_days,
    s.created_at, s.updated_at
FROM dir_scan_settings s
WHERE EXISTS (SELECT 1 FROM users);

DROP TRIGGER IF EXISTS trg_dir_scan_settings_updated;
DROP TABLE dir_scan_settings;
ALTER TABLE dir_scan_settings_new RENAME TO dir_scan_settings;

CREATE TRIGGER IF NOT EXISTS trg_dir_scan_settings_updated
AFTER UPDATE ON dir_scan_settings BEGIN
    UPDATE dir_scan_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dir_scan_settings_view AS
SELECT dss.id, dss.owner_id, dss.enabled, sp_mm.value AS match_mode,
       dss.size_tolerance_percent, dss.min_piece_ratio, dss.allow_partial,
       dss.skip_piece_boundary_safety_check, dss.start_paused,
       sp_c.value AS category, sp_t.value AS tags,
       dss.max_searchees_per_run, dss.max_searchee_age_days, dss.created_at, dss.updated_at
FROM dir_scan_settings dss
JOIN string_pool sp_mm ON dss.match_mode_id = sp_mm.id
LEFT JOIN string_pool sp_c ON dss.category_id = sp_c.id
LEFT JOIN string_pool sp_t ON dss.tags_id = sp_t.id;

-- ─── dir_scan_directories ───────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT path FROM dir_scan_directories WHERE path IS NOT NULL
UNION SELECT DISTINCT qbit_path_prefix FROM dir_scan_directories WHERE qbit_path_prefix IS NOT NULL
UNION SELECT DISTINCT category FROM dir_scan_directories WHERE category IS NOT NULL
UNION SELECT DISTINCT tags FROM dir_scan_directories WHERE tags IS NOT NULL;

CREATE TABLE dir_scan_directories_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path_id INTEGER NOT NULL REFERENCES string_pool(id),
    qbit_path_prefix_id INTEGER REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1,
    arr_instance_id INTEGER REFERENCES arr_instances(id) ON DELETE SET NULL,
    target_instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    scan_interval_minutes INTEGER NOT NULL DEFAULT 1440,
    last_scan_at DATETIME,
    category_id INTEGER REFERENCES string_pool(id),
    tags_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO dir_scan_directories_new (
    id, owner_id, path_id, qbit_path_prefix_id, enabled,
    arr_instance_id, target_instance_id, scan_interval_minutes, last_scan_at,
    category_id, tags_id, created_at, updated_at
)
SELECT
    d.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.path),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.qbit_path_prefix),
    d.enabled, d.arr_instance_id, d.target_instance_id,
    d.scan_interval_minutes, d.last_scan_at,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.category),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.tags),
    d.created_at, d.updated_at
FROM dir_scan_directories d;

DROP TRIGGER IF EXISTS trg_dir_scan_directories_updated;
DROP TABLE dir_scan_directories;
ALTER TABLE dir_scan_directories_new RENAME TO dir_scan_directories;

CREATE INDEX IF NOT EXISTS idx_dir_scan_directories_owner ON dir_scan_directories(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_dir_scan_directories_updated
AFTER UPDATE ON dir_scan_directories BEGIN
    UPDATE dir_scan_directories SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dir_scan_directories_view AS
SELECT dsd.id, dsd.owner_id, sp_p.value AS path, sp_qp.value AS qbit_path_prefix,
       dsd.enabled, dsd.arr_instance_id, dsd.target_instance_id,
       dsd.scan_interval_minutes, dsd.last_scan_at,
       sp_c.value AS category, sp_t.value AS tags, dsd.created_at, dsd.updated_at
FROM dir_scan_directories dsd
JOIN string_pool sp_p ON dsd.path_id = sp_p.id
LEFT JOIN string_pool sp_qp ON dsd.qbit_path_prefix_id = sp_qp.id
LEFT JOIN string_pool sp_c ON dsd.category_id = sp_c.id
LEFT JOIN string_pool sp_t ON dsd.tags_id = sp_t.id;

-- ─── dir_scan_runs ──────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT status FROM dir_scan_runs WHERE status IS NOT NULL
UNION SELECT DISTINCT triggered_by FROM dir_scan_runs WHERE triggered_by IS NOT NULL
UNION SELECT DISTINCT error_message FROM dir_scan_runs WHERE error_message IS NOT NULL;

CREATE TABLE dir_scan_runs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    files_found INTEGER NOT NULL DEFAULT 0,
    files_skipped INTEGER NOT NULL DEFAULT 0,
    matches_found INTEGER NOT NULL DEFAULT 0,
    torrents_added INTEGER NOT NULL DEFAULT 0,
    error_message_id INTEGER REFERENCES string_pool(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

INSERT INTO dir_scan_runs_new (
    id, owner_id, directory_id, status_id, triggered_by_id,
    files_found, files_skipped, matches_found, torrents_added,
    error_message_id, started_at, completed_at
)
SELECT
    r.id,
    (SELECT id FROM users LIMIT 1),
    r.directory_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.triggered_by),
    r.files_found, r.files_skipped, r.matches_found, r.torrents_added,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.error_message),
    r.started_at, r.completed_at
FROM dir_scan_runs r;

DROP TABLE dir_scan_runs;
ALTER TABLE dir_scan_runs_new RENAME TO dir_scan_runs;

CREATE INDEX IF NOT EXISTS idx_dir_scan_runs_directory ON dir_scan_runs(directory_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dir_scan_runs_owner ON dir_scan_runs(owner_id);

CREATE VIEW IF NOT EXISTS dir_scan_runs_view AS
SELECT dsr.id, dsr.owner_id, dsr.directory_id,
       sp_s.value AS status, sp_tb.value AS triggered_by,
       dsr.files_found, dsr.files_skipped, dsr.matches_found, dsr.torrents_added,
       sp_em.value AS error_message, dsr.started_at, dsr.completed_at
FROM dir_scan_runs dsr
JOIN string_pool sp_s  ON dsr.status_id = sp_s.id
JOIN string_pool sp_tb ON dsr.triggered_by_id = sp_tb.id
LEFT JOIN string_pool sp_em ON dsr.error_message_id = sp_em.id;

-- ─── dir_scan_files ─────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT file_path FROM dir_scan_files WHERE file_path IS NOT NULL
UNION SELECT DISTINCT status FROM dir_scan_files WHERE status IS NOT NULL
UNION SELECT DISTINCT matched_torrent_hash FROM dir_scan_files WHERE matched_torrent_hash IS NOT NULL;

CREATE TABLE dir_scan_files_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    file_path_id INTEGER NOT NULL REFERENCES string_pool(id),
    file_size INTEGER NOT NULL,
    file_mod_time DATETIME NOT NULL,
    file_id BLOB,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    matched_torrent_hash_id INTEGER REFERENCES string_pool(id),
    matched_indexer_id INTEGER,
    last_processed_at DATETIME,
    UNIQUE(directory_id, file_path_id)
);

INSERT INTO dir_scan_files_new (
    id, directory_id, file_path_id, file_size, file_mod_time, file_id,
    status_id, matched_torrent_hash_id, matched_indexer_id, last_processed_at
)
SELECT
    f.id, f.directory_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.file_path),
    f.file_size, f.file_mod_time, f.file_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.matched_torrent_hash),
    f.matched_indexer_id, f.last_processed_at
FROM dir_scan_files f;

DROP TABLE dir_scan_files;
ALTER TABLE dir_scan_files_new RENAME TO dir_scan_files;

CREATE INDEX IF NOT EXISTS idx_dir_scan_files_fileid ON dir_scan_files(directory_id, file_id) WHERE file_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dir_scan_files_directory ON dir_scan_files(directory_id);

CREATE VIEW IF NOT EXISTS dir_scan_files_view AS
SELECT dsf.id, dsf.directory_id, sp_fp.value AS file_path, dsf.file_size,
       dsf.file_mod_time, dsf.file_id, sp_s.value AS status,
       sp_mth.value AS matched_torrent_hash, dsf.matched_indexer_id, dsf.last_processed_at
FROM dir_scan_files dsf
JOIN string_pool sp_fp ON dsf.file_path_id = sp_fp.id
JOIN string_pool sp_s  ON dsf.status_id = sp_s.id
LEFT JOIN string_pool sp_mth ON dsf.matched_torrent_hash_id = sp_mth.id;

-- ─── dir_scan_run_injections ────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT status FROM dir_scan_run_injections WHERE status IS NOT NULL
UNION SELECT DISTINCT searchee_name FROM dir_scan_run_injections WHERE searchee_name IS NOT NULL
UNION SELECT DISTINCT torrent_name FROM dir_scan_run_injections WHERE torrent_name IS NOT NULL
UNION SELECT DISTINCT info_hash FROM dir_scan_run_injections WHERE info_hash IS NOT NULL
UNION SELECT DISTINCT content_type FROM dir_scan_run_injections WHERE content_type IS NOT NULL
UNION SELECT DISTINCT indexer_name FROM dir_scan_run_injections WHERE indexer_name IS NOT NULL
UNION SELECT DISTINCT tracker_domain FROM dir_scan_run_injections WHERE tracker_domain IS NOT NULL
UNION SELECT DISTINCT tracker_display_name FROM dir_scan_run_injections WHERE tracker_display_name IS NOT NULL
UNION SELECT DISTINCT link_mode FROM dir_scan_run_injections WHERE link_mode IS NOT NULL
UNION SELECT DISTINCT save_path FROM dir_scan_run_injections WHERE save_path IS NOT NULL
UNION SELECT DISTINCT category FROM dir_scan_run_injections WHERE category IS NOT NULL
UNION SELECT DISTINCT tags FROM dir_scan_run_injections WHERE tags IS NOT NULL
UNION SELECT DISTINCT error_message FROM dir_scan_run_injections WHERE error_message IS NOT NULL;

CREATE TABLE dir_scan_run_injections_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES dir_scan_runs(id) ON DELETE CASCADE,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    searchee_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    torrent_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    info_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    content_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_name_id INTEGER REFERENCES string_pool(id),
    tracker_domain_id INTEGER REFERENCES string_pool(id),
    tracker_display_name_id INTEGER REFERENCES string_pool(id),
    link_mode_id INTEGER REFERENCES string_pool(id),
    save_path_id INTEGER REFERENCES string_pool(id),
    category_id INTEGER REFERENCES string_pool(id),
    tags_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO dir_scan_run_injections_new (
    id, run_id, directory_id, status_id, searchee_name_id, torrent_name_id,
    info_hash_id, content_type_id, indexer_name_id, tracker_domain_id,
    tracker_display_name_id, link_mode_id, save_path_id,
    category_id, tags_id, error_message_id, created_at
)
SELECT
    i.id, i.run_id, i.directory_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.searchee_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.torrent_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.info_hash),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.content_type),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.indexer_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.tracker_domain),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.tracker_display_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.link_mode),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.save_path),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.category),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.error_message),
    i.created_at
FROM dir_scan_run_injections i;

DROP TABLE dir_scan_run_injections;
ALTER TABLE dir_scan_run_injections_new RENAME TO dir_scan_run_injections;

CREATE INDEX IF NOT EXISTS idx_dir_scan_injections_run ON dir_scan_run_injections(run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dir_scan_injections_directory ON dir_scan_run_injections(directory_id, created_at DESC);

CREATE VIEW IF NOT EXISTS dir_scan_run_injections_view AS
SELECT dsi.id, dsi.run_id, dsi.directory_id,
       sp_s.value AS status, sp_sn.value AS searchee_name, sp_tn.value AS torrent_name,
       sp_ih.value AS info_hash, sp_ct.value AS content_type, sp_in.value AS indexer_name,
       sp_td.value AS tracker_domain, sp_tdn.value AS tracker_display_name,
       sp_lm.value AS link_mode, sp_sp.value AS save_path,
       sp_c.value AS category, sp_t.value AS tags, sp_em.value AS error_message, dsi.created_at
FROM dir_scan_run_injections dsi
JOIN string_pool sp_s  ON dsi.status_id = sp_s.id
JOIN string_pool sp_sn ON dsi.searchee_name_id = sp_sn.id
JOIN string_pool sp_tn ON dsi.torrent_name_id = sp_tn.id
JOIN string_pool sp_ih ON dsi.info_hash_id = sp_ih.id
JOIN string_pool sp_ct ON dsi.content_type_id = sp_ct.id
LEFT JOIN string_pool sp_in  ON dsi.indexer_name_id = sp_in.id
LEFT JOIN string_pool sp_td  ON dsi.tracker_domain_id = sp_td.id
LEFT JOIN string_pool sp_tdn ON dsi.tracker_display_name_id = sp_tdn.id
LEFT JOIN string_pool sp_lm  ON dsi.link_mode_id = sp_lm.id
LEFT JOIN string_pool sp_sp  ON dsi.save_path_id = sp_sp.id
LEFT JOIN string_pool sp_c   ON dsi.category_id = sp_c.id
LEFT JOIN string_pool sp_t   ON dsi.tags_id = sp_t.id
LEFT JOIN string_pool sp_em  ON dsi.error_message_id = sp_em.id;
