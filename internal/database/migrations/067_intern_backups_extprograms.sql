-- Migration 067: Backups + external programs — add owner_id, intern remaining TEXT
-- instance_backup_settings: intern custom_path, add owner_id
-- instance_backup_runs: already interned, add owner_id
-- external_programs: intern all TEXT columns, add owner_id

-- 1. instance_backup_settings — intern custom_path, add owner_id
INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT custom_path FROM instance_backup_settings WHERE custom_path IS NOT NULL AND custom_path != '';

CREATE TABLE instance_backup_settings_new (
    instance_id     INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL DEFAULT 0,
    hourly_enabled  BOOLEAN NOT NULL DEFAULT 0,
    daily_enabled   BOOLEAN NOT NULL DEFAULT 0,
    weekly_enabled  BOOLEAN NOT NULL DEFAULT 0,
    monthly_enabled BOOLEAN NOT NULL DEFAULT 0,
    keep_hourly     INTEGER NOT NULL DEFAULT 0,
    keep_daily      INTEGER NOT NULL DEFAULT 7,
    keep_weekly     INTEGER NOT NULL DEFAULT 4,
    keep_monthly    INTEGER NOT NULL DEFAULT 12,
    include_categories BOOLEAN NOT NULL DEFAULT 1,
    include_tags    BOOLEAN NOT NULL DEFAULT 1,
    custom_path_id  INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO instance_backup_settings_new (
    instance_id, owner_id, enabled, hourly_enabled, daily_enabled, weekly_enabled, monthly_enabled,
    keep_hourly, keep_daily, keep_weekly, keep_monthly,
    include_categories, include_tags, custom_path_id, created_at, updated_at
)
SELECT
    bs.instance_id,
    (SELECT id FROM users LIMIT 1),
    bs.enabled, bs.hourly_enabled, bs.daily_enabled, bs.weekly_enabled, bs.monthly_enabled,
    bs.keep_hourly, bs.keep_daily, bs.keep_weekly, bs.keep_monthly,
    bs.include_categories, bs.include_tags,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = bs.custom_path),
    bs.created_at, bs.updated_at
FROM instance_backup_settings bs;

DROP TRIGGER IF EXISTS update_instance_backup_settings_updated_at;
DROP TABLE instance_backup_settings;
ALTER TABLE instance_backup_settings_new RENAME TO instance_backup_settings;

CREATE INDEX IF NOT EXISTS idx_backup_settings_owner ON instance_backup_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_backup_settings_updated
AFTER UPDATE ON instance_backup_settings BEGIN
    UPDATE instance_backup_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

DROP VIEW IF EXISTS instance_backup_settings_view;
CREATE VIEW instance_backup_settings_view AS
SELECT bs.instance_id, bs.owner_id, bs.enabled, bs.hourly_enabled, bs.daily_enabled,
       bs.weekly_enabled, bs.monthly_enabled, bs.keep_hourly, bs.keep_daily,
       bs.keep_weekly, bs.keep_monthly, bs.include_categories, bs.include_tags,
       sp_cp.value AS custom_path, bs.created_at, bs.updated_at
FROM instance_backup_settings bs
LEFT JOIN string_pool sp_cp ON bs.custom_path_id = sp_cp.id;

-- 2. instance_backup_runs — intern category_counts_json, categories_json, tags_json; add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT category_counts_json FROM instance_backup_runs WHERE category_counts_json IS NOT NULL AND category_counts_json != '';
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT categories_json FROM instance_backup_runs WHERE categories_json IS NOT NULL AND categories_json != '';
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT tags_json FROM instance_backup_runs WHERE tags_json IS NOT NULL AND tags_json != '';

CREATE TABLE instance_backup_runs_new (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id                INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id             INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind_id                 INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    status_id               INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    requested_by_id         INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    requested_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at              TIMESTAMP,
    completed_at            TIMESTAMP,
    archive_path_id         INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    manifest_path_id        INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    total_bytes             INTEGER NOT NULL DEFAULT 0,
    torrent_count           INTEGER NOT NULL DEFAULT 0,
    category_counts_json_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    categories_json_id      INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    tags_json_id            INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    error_message_id        INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT
);

INSERT INTO instance_backup_runs_new (
    id, owner_id, instance_id, kind_id, status_id, requested_by_id,
    requested_at, started_at, completed_at,
    archive_path_id, manifest_path_id, total_bytes, torrent_count,
    category_counts_json_id, categories_json_id, tags_json_id, error_message_id
)
SELECT
    br.id, (SELECT id FROM users LIMIT 1),
    br.instance_id, br.kind_id, br.status_id, br.requested_by_id,
    br.requested_at, br.started_at, br.completed_at,
    br.archive_path_id, br.manifest_path_id, br.total_bytes, br.torrent_count,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = br.category_counts_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = br.categories_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = br.tags_json),
    br.error_message_id
FROM instance_backup_runs br;

-- Drop view BEFORE the table it depends on.
DROP VIEW IF EXISTS instance_backup_runs_view;
DROP TABLE instance_backup_runs;
ALTER TABLE instance_backup_runs_new RENAME TO instance_backup_runs;

CREATE INDEX IF NOT EXISTS idx_backup_runs_instance ON instance_backup_runs(instance_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_runs_owner    ON instance_backup_runs(owner_id);

DROP VIEW IF EXISTS instance_backup_runs_view;
CREATE VIEW instance_backup_runs_view AS
SELECT br.id, br.owner_id, br.instance_id,
       sp_k.value AS kind, sp_s.value AS status, sp_rb.value AS requested_by,
       br.requested_at, br.started_at, br.completed_at,
       sp_ap.value AS archive_path, sp_mp.value AS manifest_path,
       br.total_bytes, br.torrent_count,
       sp_ccj.value AS category_counts_json, sp_cj.value AS categories_json,
       sp_tj.value AS tags_json, sp_em.value AS error_message
FROM instance_backup_runs br
JOIN string_pool sp_k  ON br.kind_id = sp_k.id
JOIN string_pool sp_s  ON br.status_id = sp_s.id
JOIN string_pool sp_rb ON br.requested_by_id = sp_rb.id
LEFT JOIN string_pool sp_ap  ON br.archive_path_id = sp_ap.id
LEFT JOIN string_pool sp_mp  ON br.manifest_path_id = sp_mp.id
LEFT JOIN string_pool sp_ccj ON br.category_counts_json_id = sp_ccj.id
LEFT JOIN string_pool sp_cj  ON br.categories_json_id = sp_cj.id
LEFT JOIN string_pool sp_tj  ON br.tags_json_id = sp_tj.id
LEFT JOIN string_pool sp_em  ON br.error_message_id = sp_em.id;

-- 3. external_programs — intern all TEXT columns, add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT name FROM external_programs;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT path FROM external_programs;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT args_template FROM external_programs;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT path_mappings FROM external_programs;

CREATE TABLE external_programs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    path_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    args_template_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    enabled INTEGER NOT NULL DEFAULT 1,
    use_terminal INTEGER NOT NULL DEFAULT 1,
    path_mappings_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO external_programs_new (
    id, owner_id, name_id, path_id, args_template_id, enabled, use_terminal,
    path_mappings_id, created_at, updated_at
)
SELECT
    ep.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ep.name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ep.path),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ep.args_template),
    ep.enabled, ep.use_terminal,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ep.path_mappings),
    ep.created_at, ep.updated_at
FROM external_programs ep;

DROP TABLE external_programs;
ALTER TABLE external_programs_new RENAME TO external_programs;

CREATE INDEX IF NOT EXISTS idx_external_programs_owner ON external_programs(owner_id);
CREATE INDEX IF NOT EXISTS idx_external_programs_enabled ON external_programs(enabled);

DROP VIEW IF EXISTS external_programs_view;
CREATE VIEW external_programs_view AS
SELECT ep.id, ep.owner_id, sp_n.value AS name, sp_p.value AS path,
       sp_at.value AS args_template, ep.enabled, ep.use_terminal,
       sp_pm.value AS path_mappings, ep.created_at, ep.updated_at
FROM external_programs ep
JOIN string_pool sp_n  ON ep.name_id = sp_n.id
JOIN string_pool sp_p  ON ep.path_id = sp_p.id
JOIN string_pool sp_at ON ep.args_template_id = sp_at.id
JOIN string_pool sp_pm ON ep.path_mappings_id = sp_pm.id;
