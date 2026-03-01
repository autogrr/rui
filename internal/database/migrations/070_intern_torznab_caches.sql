-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 070: Intern torznab cache tables
-- Converts TEXT columns → string_pool references in torznab_torrent_cache,
-- torznab_search_cache, and torznab_search_cache_settings.

-- ─── torznab_torrent_cache ──────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT cache_key FROM torznab_torrent_cache WHERE cache_key IS NOT NULL
UNION SELECT DISTINCT guid FROM torznab_torrent_cache WHERE guid IS NOT NULL
UNION SELECT DISTINCT download_url FROM torznab_torrent_cache WHERE download_url IS NOT NULL
UNION SELECT DISTINCT info_hash FROM torznab_torrent_cache WHERE info_hash IS NOT NULL
UNION SELECT DISTINCT title FROM torznab_torrent_cache WHERE title IS NOT NULL;

CREATE TABLE torznab_torrent_cache_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    cache_key_id INTEGER NOT NULL REFERENCES string_pool(id),
    guid_id INTEGER REFERENCES string_pool(id),
    download_url_id INTEGER REFERENCES string_pool(id),
    info_hash_id INTEGER REFERENCES string_pool(id),
    title_id INTEGER REFERENCES string_pool(id),
    size_bytes INTEGER,
    torrent_data BLOB NOT NULL,
    cached_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, cache_key_id)
);

INSERT INTO torznab_torrent_cache_new (
    id, indexer_id, cache_key_id, guid_id, download_url_id, info_hash_id,
    title_id, size_bytes, torrent_data, cached_at, last_used_at
)
SELECT
    t.id, t.indexer_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.cache_key),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.guid),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.download_url),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.info_hash),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.title),
    t.size_bytes, t.torrent_data, t.cached_at, t.last_used_at
FROM torznab_torrent_cache t;

DROP TABLE torznab_torrent_cache;
ALTER TABLE torznab_torrent_cache_new RENAME TO torznab_torrent_cache;

CREATE INDEX IF NOT EXISTS idx_torznab_torrent_cache_last_used ON torznab_torrent_cache(last_used_at);

CREATE VIEW IF NOT EXISTS torznab_torrent_cache_view AS
SELECT tc.id, tc.indexer_id,
       sp_ck.value AS cache_key, sp_g.value AS guid,
       sp_du.value AS download_url, sp_ih.value AS info_hash,
       sp_t.value AS title,
       tc.size_bytes, tc.torrent_data, tc.cached_at, tc.last_used_at
FROM torznab_torrent_cache tc
JOIN string_pool sp_ck ON tc.cache_key_id = sp_ck.id
LEFT JOIN string_pool sp_g  ON tc.guid_id = sp_g.id
LEFT JOIN string_pool sp_du ON tc.download_url_id = sp_du.id
LEFT JOIN string_pool sp_ih ON tc.info_hash_id = sp_ih.id
LEFT JOIN string_pool sp_t  ON tc.title_id = sp_t.id;

-- ─── torznab_search_cache ──────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT cache_key FROM torznab_search_cache WHERE cache_key IS NOT NULL
UNION SELECT DISTINCT scope FROM torznab_search_cache WHERE scope IS NOT NULL
UNION SELECT DISTINCT query FROM torznab_search_cache WHERE query IS NOT NULL
UNION SELECT DISTINCT categories_json FROM torznab_search_cache WHERE categories_json IS NOT NULL
UNION SELECT DISTINCT indexer_ids_json FROM torznab_search_cache WHERE indexer_ids_json IS NOT NULL
UNION SELECT DISTINCT indexer_matcher FROM torznab_search_cache WHERE indexer_matcher IS NOT NULL
UNION SELECT DISTINCT request_fingerprint FROM torznab_search_cache WHERE request_fingerprint IS NOT NULL;

CREATE TABLE torznab_search_cache_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cache_key_id INTEGER NOT NULL REFERENCES string_pool(id),
    scope_id INTEGER NOT NULL REFERENCES string_pool(id),
    query_id INTEGER REFERENCES string_pool(id),
    categories_json_id INTEGER REFERENCES string_pool(id),
    indexer_ids_json_id INTEGER REFERENCES string_pool(id),
    indexer_matcher_id INTEGER NOT NULL REFERENCES string_pool(id),
    request_fingerprint_id INTEGER NOT NULL REFERENCES string_pool(id),
    response_data BLOB NOT NULL,
    total_results INTEGER NOT NULL DEFAULT 0,
    cached_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    hit_count INTEGER NOT NULL DEFAULT 0
);

INSERT INTO torznab_search_cache_new (
    id, cache_key_id, scope_id, query_id, categories_json_id,
    indexer_ids_json_id, indexer_matcher_id, request_fingerprint_id,
    response_data, total_results, cached_at, last_used_at, expires_at, hit_count
)
SELECT
    s.id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.cache_key),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.scope),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.query),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.categories_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.indexer_ids_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.indexer_matcher),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.request_fingerprint),
    s.response_data, s.total_results, s.cached_at, s.last_used_at, s.expires_at, s.hit_count
FROM torznab_search_cache s;

DROP TABLE torznab_search_cache;
ALTER TABLE torznab_search_cache_new RENAME TO torznab_search_cache;

CREATE UNIQUE INDEX IF NOT EXISTS idx_torznab_search_cache_key ON torznab_search_cache(cache_key_id);
CREATE INDEX IF NOT EXISTS idx_torznab_search_cache_expires ON torznab_search_cache(expires_at);

CREATE VIEW IF NOT EXISTS torznab_search_cache_view AS
SELECT sc.id, sp_ck.value AS cache_key, sp_s.value AS scope, sp_q.value AS query,
       sp_cj.value AS categories_json, sp_ij.value AS indexer_ids_json,
       sp_im.value AS indexer_matcher, sp_rf.value AS request_fingerprint,
       sc.response_data, sc.total_results, sc.cached_at, sc.last_used_at,
       sc.expires_at, sc.hit_count
FROM torznab_search_cache sc
JOIN string_pool sp_ck ON sc.cache_key_id = sp_ck.id
JOIN string_pool sp_s  ON sc.scope_id = sp_s.id
JOIN string_pool sp_im ON sc.indexer_matcher_id = sp_im.id
JOIN string_pool sp_rf ON sc.request_fingerprint_id = sp_rf.id
LEFT JOIN string_pool sp_q  ON sc.query_id = sp_q.id
LEFT JOIN string_pool sp_cj ON sc.categories_json_id = sp_cj.id
LEFT JOIN string_pool sp_ij ON sc.indexer_ids_json_id = sp_ij.id;

-- ─── torznab_search_cache_settings ─────────────────────────────────────────
-- Remove CHECK(id=1) singleton, add owner_id for multi-user support.

CREATE TABLE torznab_search_cache_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    ttl_minutes INTEGER NOT NULL DEFAULT 1440,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO torznab_search_cache_settings_new (id, owner_id, ttl_minutes, updated_at)
SELECT s.id, (SELECT id FROM users LIMIT 1), s.ttl_minutes, s.updated_at
FROM torznab_search_cache_settings s
WHERE EXISTS (SELECT 1 FROM users);

DROP TRIGGER IF EXISTS torznab_search_cache_settings_updated_at;
DROP TABLE torznab_search_cache_settings;
ALTER TABLE torznab_search_cache_settings_new RENAME TO torznab_search_cache_settings;

CREATE TRIGGER IF NOT EXISTS torznab_search_cache_settings_updated_at
AFTER UPDATE ON torznab_search_cache_settings
FOR EACH ROW
BEGIN
    UPDATE torznab_search_cache_settings
    SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;
