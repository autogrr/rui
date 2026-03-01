-- Migration 068: Instance settings — reannounce + crossseed completion
-- Intern JSON TEXT columns, add owner_id

-- 1. instance_reannounce_settings — intern categories_json, tags_json, trackers_json
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT categories_json FROM instance_reannounce_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT tags_json FROM instance_reannounce_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT trackers_json FROM instance_reannounce_settings;

CREATE TABLE instance_reannounce_settings_new (
    instance_id INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    initial_wait_seconds INTEGER NOT NULL DEFAULT 15,
    reannounce_interval_seconds INTEGER NOT NULL DEFAULT 7,
    max_age_seconds INTEGER NOT NULL DEFAULT 600,
    aggressive INTEGER NOT NULL DEFAULT 0,
    monitor_all INTEGER NOT NULL DEFAULT 1,
    exclude_categories INTEGER NOT NULL DEFAULT 0,
    categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_tags INTEGER NOT NULL DEFAULT 0,
    tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_trackers INTEGER NOT NULL DEFAULT 0,
    trackers_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    max_retries INTEGER NOT NULL DEFAULT 50,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO instance_reannounce_settings_new (
    instance_id, owner_id, enabled, initial_wait_seconds, reannounce_interval_seconds,
    max_age_seconds, aggressive, monitor_all,
    exclude_categories, categories_json_id,
    exclude_tags, tags_json_id,
    exclude_trackers, trackers_json_id,
    max_retries, updated_at
)
SELECT
    rs.instance_id,
    (SELECT id FROM users LIMIT 1),
    rs.enabled, rs.initial_wait_seconds, rs.reannounce_interval_seconds,
    rs.max_age_seconds, rs.aggressive, rs.monitor_all,
    rs.exclude_categories,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = rs.categories_json),
    rs.exclude_tags,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = rs.tags_json),
    rs.exclude_trackers,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = rs.trackers_json),
    rs.max_retries, rs.updated_at
FROM instance_reannounce_settings rs;

DROP TRIGGER IF EXISTS trg_instance_reannounce_settings_updated;
DROP TABLE instance_reannounce_settings;
ALTER TABLE instance_reannounce_settings_new RENAME TO instance_reannounce_settings;

CREATE INDEX IF NOT EXISTS idx_reannounce_settings_owner ON instance_reannounce_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_reannounce_settings_updated
AFTER UPDATE ON instance_reannounce_settings BEGIN
    UPDATE instance_reannounce_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

DROP VIEW IF EXISTS instance_reannounce_settings_view;
CREATE VIEW instance_reannounce_settings_view AS
SELECT rs.instance_id, rs.owner_id, rs.enabled, rs.initial_wait_seconds,
       rs.reannounce_interval_seconds, rs.max_age_seconds, rs.aggressive,
       rs.monitor_all, rs.exclude_categories, sp_cj.value AS categories_json,
       rs.exclude_tags, sp_tj.value AS tags_json,
       rs.exclude_trackers, sp_trj.value AS trackers_json,
       rs.max_retries, rs.updated_at
FROM instance_reannounce_settings rs
JOIN string_pool sp_cj  ON rs.categories_json_id = sp_cj.id
JOIN string_pool sp_tj  ON rs.tags_json_id = sp_tj.id
JOIN string_pool sp_trj ON rs.trackers_json_id = sp_trj.id;

-- 2. instance_crossseed_completion_settings — intern all JSON columns, add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT categories_json FROM instance_crossseed_completion_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT tags_json FROM instance_crossseed_completion_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT exclude_categories_json FROM instance_crossseed_completion_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT exclude_tags_json FROM instance_crossseed_completion_settings;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT indexer_ids_json FROM instance_crossseed_completion_settings;

CREATE TABLE instance_crossseed_completion_settings_new (
    instance_id INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    indexer_ids_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO instance_crossseed_completion_settings_new (
    instance_id, owner_id, enabled,
    categories_json_id, tags_json_id, exclude_categories_json_id,
    exclude_tags_json_id, indexer_ids_json_id, updated_at
)
SELECT
    cs.instance_id,
    (SELECT id FROM users LIMIT 1),
    cs.enabled,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = cs.categories_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = cs.tags_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = cs.exclude_categories_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = cs.exclude_tags_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = cs.indexer_ids_json),
    cs.updated_at
FROM instance_crossseed_completion_settings cs;

DROP TRIGGER IF EXISTS trg_instance_crossseed_completion_settings_updated;
DROP TABLE instance_crossseed_completion_settings;
ALTER TABLE instance_crossseed_completion_settings_new RENAME TO instance_crossseed_completion_settings;

CREATE INDEX IF NOT EXISTS idx_crossseed_completion_settings_owner ON instance_crossseed_completion_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_crossseed_completion_settings_updated
AFTER UPDATE ON instance_crossseed_completion_settings BEGIN
    UPDATE instance_crossseed_completion_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

DROP VIEW IF EXISTS instance_crossseed_completion_settings_view;
CREATE VIEW instance_crossseed_completion_settings_view AS
SELECT cs.instance_id, cs.owner_id, cs.enabled,
       sp_cj.value AS categories_json, sp_tj.value AS tags_json,
       sp_ecj.value AS exclude_categories_json, sp_etj.value AS exclude_tags_json,
       sp_ij.value AS indexer_ids_json, cs.updated_at
FROM instance_crossseed_completion_settings cs
JOIN string_pool sp_cj  ON cs.categories_json_id = sp_cj.id
JOIN string_pool sp_tj  ON cs.tags_json_id = sp_tj.id
JOIN string_pool sp_ecj ON cs.exclude_categories_json_id = sp_ecj.id
JOIN string_pool sp_etj ON cs.exclude_tags_json_id = sp_etj.id
JOIN string_pool sp_ij  ON cs.indexer_ids_json_id = sp_ij.id;
