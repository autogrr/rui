-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 072: Intern cross-seed runs, feed items, search runs/history, blocklist
-- Converts TEXT columns → string_pool references, adds owner_id for multi-user.

-- ─── cross_seed_runs ────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT triggered_by FROM cross_seed_runs WHERE triggered_by IS NOT NULL
UNION SELECT DISTINCT mode FROM cross_seed_runs WHERE mode IS NOT NULL
UNION SELECT DISTINCT status FROM cross_seed_runs WHERE status IS NOT NULL
UNION SELECT DISTINCT message FROM cross_seed_runs WHERE message IS NOT NULL
UNION SELECT DISTINCT error_message FROM cross_seed_runs WHERE error_message IS NOT NULL
UNION SELECT DISTINCT results_json FROM cross_seed_runs WHERE results_json IS NOT NULL;

CREATE TABLE cross_seed_runs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    total_feed_items INTEGER NOT NULL DEFAULT 0,
    candidates_found INTEGER NOT NULL DEFAULT 0,
    torrents_added INTEGER NOT NULL DEFAULT 0,
    torrents_failed INTEGER NOT NULL DEFAULT 0,
    torrents_skipped INTEGER NOT NULL DEFAULT 0,
    message_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    results_json_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cross_seed_runs_new (
    id, owner_id, triggered_by_id, mode_id, status_id,
    started_at, completed_at, total_feed_items, candidates_found,
    torrents_added, torrents_failed, torrents_skipped,
    message_id, error_message_id, results_json_id, created_at
)
SELECT
    r.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.triggered_by),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.mode),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.status),
    r.started_at, r.completed_at, r.total_feed_items, r.candidates_found,
    r.torrents_added, r.torrents_failed, r.torrents_skipped,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.message),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.error_message),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.results_json),
    r.created_at
FROM cross_seed_runs r;

DROP TABLE cross_seed_runs;
ALTER TABLE cross_seed_runs_new RENAME TO cross_seed_runs;

CREATE INDEX IF NOT EXISTS idx_cross_seed_runs_started ON cross_seed_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_cross_seed_runs_owner ON cross_seed_runs(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_runs_view AS
SELECT csr.id, csr.owner_id, sp_tb.value AS triggered_by, sp_m.value AS mode,
       sp_s.value AS status, csr.started_at, csr.completed_at,
       csr.total_feed_items, csr.candidates_found, csr.torrents_added,
       csr.torrents_failed, csr.torrents_skipped,
       sp_msg.value AS message, sp_em.value AS error_message, sp_rj.value AS results_json,
       csr.created_at
FROM cross_seed_runs csr
JOIN string_pool sp_tb ON csr.triggered_by_id = sp_tb.id
JOIN string_pool sp_m  ON csr.mode_id = sp_m.id
JOIN string_pool sp_s  ON csr.status_id = sp_s.id
LEFT JOIN string_pool sp_msg ON csr.message_id = sp_msg.id
LEFT JOIN string_pool sp_em  ON csr.error_message_id = sp_em.id
LEFT JOIN string_pool sp_rj  ON csr.results_json_id = sp_rj.id;

-- ─── cross_seed_feed_items ──────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT guid FROM cross_seed_feed_items WHERE guid IS NOT NULL
UNION SELECT DISTINCT title FROM cross_seed_feed_items WHERE title IS NOT NULL
UNION SELECT DISTINCT last_status FROM cross_seed_feed_items WHERE last_status IS NOT NULL
UNION SELECT DISTINCT info_hash FROM cross_seed_feed_items WHERE info_hash IS NOT NULL;

-- Ensure default value exists
INSERT OR IGNORE INTO string_pool (value) SELECT 'pending';

CREATE TABLE cross_seed_feed_items_new (
    guid_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title_id INTEGER REFERENCES string_pool(id),
    first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_status_id INTEGER NOT NULL REFERENCES string_pool(id),
    last_run_id INTEGER REFERENCES cross_seed_runs(id) ON DELETE SET NULL,
    info_hash_id INTEGER REFERENCES string_pool(id),
    PRIMARY KEY (guid_id, indexer_id)
);

INSERT INTO cross_seed_feed_items_new (
    guid_id, indexer_id, owner_id, title_id,
    first_seen_at, last_seen_at, last_status_id, last_run_id, info_hash_id
)
SELECT
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.guid),
    f.indexer_id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.title),
    f.first_seen_at, f.last_seen_at,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.last_status),
    f.last_run_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = f.info_hash)
FROM cross_seed_feed_items f;

DROP TRIGGER IF EXISTS cross_seed_feed_items_touch;
DROP TABLE cross_seed_feed_items;
ALTER TABLE cross_seed_feed_items_new RENAME TO cross_seed_feed_items;

CREATE INDEX IF NOT EXISTS idx_cross_seed_feed_items_indexer ON cross_seed_feed_items(indexer_id);
CREATE INDEX IF NOT EXISTS idx_cross_seed_feed_items_owner ON cross_seed_feed_items(owner_id);

CREATE TRIGGER IF NOT EXISTS cross_seed_feed_items_touch
AFTER UPDATE ON cross_seed_feed_items
FOR EACH ROW
BEGIN
    UPDATE cross_seed_feed_items
    SET last_seen_at = CURRENT_TIMESTAMP
    WHERE guid_id = NEW.guid_id AND indexer_id = NEW.indexer_id;
END;

CREATE VIEW IF NOT EXISTS cross_seed_feed_items_view AS
SELECT sp_g.value AS guid, csfi.indexer_id, csfi.owner_id, sp_t.value AS title,
       csfi.first_seen_at, csfi.last_seen_at, sp_ls.value AS last_status,
       csfi.last_run_id, sp_ih.value AS info_hash
FROM cross_seed_feed_items csfi
JOIN string_pool sp_g ON csfi.guid_id = sp_g.id
LEFT JOIN string_pool sp_t ON csfi.title_id = sp_t.id
JOIN string_pool sp_ls ON csfi.last_status_id = sp_ls.id
LEFT JOIN string_pool sp_ih ON csfi.info_hash_id = sp_ih.id;

-- ─── cross_seed_search_runs ────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT status FROM cross_seed_search_runs WHERE status IS NOT NULL
UNION SELECT DISTINCT message FROM cross_seed_search_runs WHERE message IS NOT NULL
UNION SELECT DISTINCT error_message FROM cross_seed_search_runs WHERE error_message IS NOT NULL
UNION SELECT DISTINCT filters_json FROM cross_seed_search_runs WHERE filters_json IS NOT NULL
UNION SELECT DISTINCT indexer_ids_json FROM cross_seed_search_runs WHERE indexer_ids_json IS NOT NULL
UNION SELECT DISTINCT results_json FROM cross_seed_search_runs WHERE results_json IS NOT NULL;

CREATE TABLE cross_seed_search_runs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    total_torrents INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    torrents_added INTEGER NOT NULL DEFAULT 0,
    torrents_failed INTEGER NOT NULL DEFAULT 0,
    torrents_skipped INTEGER NOT NULL DEFAULT 0,
    message_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    filters_json_id INTEGER REFERENCES string_pool(id),
    indexer_ids_json_id INTEGER REFERENCES string_pool(id),
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    cooldown_minutes INTEGER NOT NULL DEFAULT 360,
    results_json_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cross_seed_search_runs_new (
    id, owner_id, instance_id, status_id,
    started_at, completed_at, total_torrents, processed,
    torrents_added, torrents_failed, torrents_skipped,
    message_id, error_message_id, filters_json_id, indexer_ids_json_id,
    interval_seconds, cooldown_minutes, results_json_id, created_at
)
SELECT
    r.id,
    (SELECT id FROM users LIMIT 1),
    r.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.status),
    r.started_at, r.completed_at, r.total_torrents, r.processed,
    r.torrents_added, r.torrents_failed, r.torrents_skipped,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.message),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.error_message),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.filters_json),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.indexer_ids_json),
    r.interval_seconds, r.cooldown_minutes,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = r.results_json),
    r.created_at
FROM cross_seed_search_runs r;

DROP TABLE cross_seed_search_runs;
ALTER TABLE cross_seed_search_runs_new RENAME TO cross_seed_search_runs;

CREATE INDEX IF NOT EXISTS idx_cross_seed_search_runs_instance ON cross_seed_search_runs(instance_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_cross_seed_search_runs_owner ON cross_seed_search_runs(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_search_runs_view AS
SELECT cssr.id, cssr.owner_id, cssr.instance_id, sp_s.value AS status,
       cssr.started_at, cssr.completed_at, cssr.total_torrents, cssr.processed,
       cssr.torrents_added, cssr.torrents_failed, cssr.torrents_skipped,
       sp_msg.value AS message, sp_em.value AS error_message, sp_fj.value AS filters_json,
       sp_ij.value AS indexer_ids_json, cssr.interval_seconds, cssr.cooldown_minutes,
       sp_rj.value AS results_json, cssr.created_at
FROM cross_seed_search_runs cssr
JOIN string_pool sp_s ON cssr.status_id = sp_s.id
LEFT JOIN string_pool sp_msg ON cssr.message_id = sp_msg.id
LEFT JOIN string_pool sp_em ON cssr.error_message_id = sp_em.id
LEFT JOIN string_pool sp_fj ON cssr.filters_json_id = sp_fj.id
LEFT JOIN string_pool sp_ij ON cssr.indexer_ids_json_id = sp_ij.id
LEFT JOIN string_pool sp_rj ON cssr.results_json_id = sp_rj.id;

-- ─── cross_seed_search_history ──────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT torrent_hash FROM cross_seed_search_history WHERE torrent_hash IS NOT NULL;

CREATE TABLE cross_seed_search_history_new (
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    torrent_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_searched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, torrent_hash_id)
);

INSERT INTO cross_seed_search_history_new (
    instance_id, torrent_hash_id, owner_id, last_searched_at
)
SELECT
    h.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = h.torrent_hash),
    (SELECT id FROM users LIMIT 1),
    h.last_searched_at
FROM cross_seed_search_history h;

DROP TABLE cross_seed_search_history;
ALTER TABLE cross_seed_search_history_new RENAME TO cross_seed_search_history;

CREATE INDEX IF NOT EXISTS idx_cross_seed_search_history_last ON cross_seed_search_history(last_searched_at);
CREATE INDEX IF NOT EXISTS idx_cross_seed_search_history_owner ON cross_seed_search_history(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_search_history_view AS
SELECT cssh.instance_id, sp_th.value AS torrent_hash, cssh.owner_id, cssh.last_searched_at
FROM cross_seed_search_history cssh
JOIN string_pool sp_th ON cssh.torrent_hash_id = sp_th.id;

-- ─── cross_seed_blocklist ───────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT infohash FROM cross_seed_blocklist WHERE infohash IS NOT NULL
UNION SELECT DISTINCT note FROM cross_seed_blocklist WHERE note IS NOT NULL;

INSERT OR IGNORE INTO string_pool (value) SELECT '';

CREATE TABLE cross_seed_blocklist_new (
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    infohash_id INTEGER NOT NULL REFERENCES string_pool(id),
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, infohash_id)
);

INSERT INTO cross_seed_blocklist_new (
    instance_id, infohash_id, owner_id, note_id, created_at
)
SELECT
    b.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = b.infohash),
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = b.note),
    b.created_at
FROM cross_seed_blocklist b;

DROP TABLE cross_seed_blocklist;
ALTER TABLE cross_seed_blocklist_new RENAME TO cross_seed_blocklist;

CREATE INDEX IF NOT EXISTS idx_cross_seed_blocklist_instance ON cross_seed_blocklist(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cross_seed_blocklist_owner ON cross_seed_blocklist(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_blocklist_view AS
SELECT csb.instance_id, sp_ih.value AS infohash, csb.owner_id, sp_n.value AS note, csb.created_at
FROM cross_seed_blocklist csb
JOIN string_pool sp_ih ON csb.infohash_id = sp_ih.id
JOIN string_pool sp_n  ON csb.note_id = sp_n.id;
