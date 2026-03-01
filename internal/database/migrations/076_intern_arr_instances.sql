-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 076: Intern arr_instances and arr_id_cache
-- Converts TEXT columns → string_pool references, adds owner_id.

-- ─── arr_instances ──────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT type FROM arr_instances WHERE type IS NOT NULL
UNION SELECT DISTINCT api_key_encrypted FROM arr_instances WHERE api_key_encrypted IS NOT NULL
UNION SELECT DISTINCT last_test_status FROM arr_instances WHERE last_test_status IS NOT NULL
UNION SELECT DISTINCT last_test_error FROM arr_instances WHERE last_test_error IS NOT NULL
UNION SELECT DISTINCT basic_password_encrypted FROM arr_instances WHERE basic_password_encrypted IS NOT NULL;

-- Ensure defaults exist
INSERT OR IGNORE INTO string_pool (value) SELECT 'unknown';

CREATE TABLE arr_instances_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type_id INTEGER NOT NULL REFERENCES string_pool(id),
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    base_url_id INTEGER NOT NULL REFERENCES string_pool(id),
    api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled BOOLEAN DEFAULT 1,
    priority INTEGER DEFAULT 0,
    timeout_seconds INTEGER DEFAULT 15,
    last_test_at TIMESTAMP,
    last_test_status_id INTEGER REFERENCES string_pool(id),
    last_test_error_id INTEGER REFERENCES string_pool(id),
    basic_username_id INTEGER REFERENCES string_pool(id),
    basic_password_encrypted_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO arr_instances_new (
    id, owner_id, type_id, name_id, base_url_id, api_key_encrypted_id,
    enabled, priority, timeout_seconds, last_test_at,
    last_test_status_id, last_test_error_id,
    basic_username_id, basic_password_encrypted_id,
    created_at, updated_at
)
SELECT
    ai.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ai.type),
    ai.name_id, ai.base_url_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ai.api_key_encrypted),
    ai.enabled, ai.priority, ai.timeout_seconds, ai.last_test_at,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ai.last_test_status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ai.last_test_error),
    ai.basic_username_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ai.basic_password_encrypted),
    ai.created_at, ai.updated_at
FROM arr_instances ai;

DROP TRIGGER IF EXISTS update_arr_instances_updated_at;
DROP VIEW IF EXISTS arr_instances_view;
DROP TABLE arr_instances;
ALTER TABLE arr_instances_new RENAME TO arr_instances;

CREATE INDEX IF NOT EXISTS idx_arr_instances_owner ON arr_instances(owner_id);
CREATE INDEX IF NOT EXISTS idx_arr_instances_enabled ON arr_instances(enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_arr_instances_type_base_url ON arr_instances(type_id, base_url_id);

CREATE TRIGGER IF NOT EXISTS trg_arr_instances_updated
AFTER UPDATE ON arr_instances BEGIN
    UPDATE arr_instances SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS arr_instances_view AS
SELECT ai.id, ai.owner_id, sp_t.value AS type, sp_n.value AS name,
       sp_bu.value AS base_url, sp_ak.value AS api_key_encrypted,
       ai.enabled, ai.priority, ai.timeout_seconds, ai.last_test_at,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error,
       sp_bun.value AS basic_username, sp_bpe.value AS basic_password_encrypted,
       ai.created_at, ai.updated_at
FROM arr_instances ai
JOIN string_pool sp_t  ON ai.type_id = sp_t.id
JOIN string_pool sp_n  ON ai.name_id = sp_n.id
JOIN string_pool sp_bu ON ai.base_url_id = sp_bu.id
JOIN string_pool sp_ak ON ai.api_key_encrypted_id = sp_ak.id
LEFT JOIN string_pool sp_lts ON ai.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ai.last_test_error_id = sp_lte.id
LEFT JOIN string_pool sp_bun ON ai.basic_username_id = sp_bun.id
LEFT JOIN string_pool sp_bpe ON ai.basic_password_encrypted_id = sp_bpe.id;

-- ─── arr_id_cache ───────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT title_hash FROM arr_id_cache WHERE title_hash IS NOT NULL
UNION SELECT DISTINCT content_type FROM arr_id_cache WHERE content_type IS NOT NULL
UNION SELECT DISTINCT imdb_id FROM arr_id_cache WHERE imdb_id IS NOT NULL;

CREATE TABLE arr_id_cache_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    content_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    arr_instance_id INTEGER REFERENCES arr_instances(id) ON DELETE SET NULL,
    imdb_id_sid INTEGER REFERENCES string_pool(id),
    tmdb_id INTEGER,
    tvdb_id INTEGER,
    tvmaze_id INTEGER,
    is_negative BOOLEAN DEFAULT 0,
    cached_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    UNIQUE(title_hash_id, content_type_id)
);

INSERT INTO arr_id_cache_new (
    id, title_hash_id, content_type_id, arr_instance_id,
    imdb_id_sid, tmdb_id, tvdb_id, tvmaze_id,
    is_negative, cached_at, expires_at
)
SELECT
    c.id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = c.title_hash),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = c.content_type),
    c.arr_instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = c.imdb_id),
    c.tmdb_id, c.tvdb_id, c.tvmaze_id,
    c.is_negative, c.cached_at, c.expires_at
FROM arr_id_cache c;

DROP TABLE arr_id_cache;
ALTER TABLE arr_id_cache_new RENAME TO arr_id_cache;

CREATE INDEX IF NOT EXISTS idx_arr_id_cache_lookup ON arr_id_cache(title_hash_id, content_type_id);
CREATE INDEX IF NOT EXISTS idx_arr_id_cache_expires ON arr_id_cache(expires_at);

CREATE VIEW IF NOT EXISTS arr_id_cache_view AS
SELECT aic.id, sp_th.value AS title_hash, sp_ct.value AS content_type,
       aic.arr_instance_id, sp_imdb.value AS imdb_id,
       aic.tmdb_id, aic.tvdb_id, aic.tvmaze_id, aic.is_negative,
       aic.cached_at, aic.expires_at
FROM arr_id_cache aic
JOIN string_pool sp_th ON aic.title_hash_id = sp_th.id
JOIN string_pool sp_ct ON aic.content_type_id = sp_ct.id
LEFT JOIN string_pool sp_imdb ON aic.imdb_id_sid = sp_imdb.id;
