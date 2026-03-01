-- Migration 069: Torznab indexers + sub-tables — intern remaining TEXT, add owner_id

-- 1. torznab_indexers — intern api_key_encrypted, backend, capabilities,
--    last_test_status, last_test_error, basic_password_encrypted; add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT api_key_encrypted FROM torznab_indexers WHERE api_key_encrypted IS NOT NULL;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT backend FROM torznab_indexers WHERE backend IS NOT NULL;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT capabilities FROM torznab_indexers WHERE capabilities IS NOT NULL;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT last_test_status FROM torznab_indexers WHERE last_test_status IS NOT NULL;
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT last_test_error FROM torznab_indexers WHERE last_test_error IS NOT NULL AND last_test_error != '';
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT basic_password_encrypted FROM torznab_indexers WHERE basic_password_encrypted IS NOT NULL AND basic_password_encrypted != '';

CREATE TABLE torznab_indexers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    base_url_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_id_string_id INTEGER REFERENCES string_pool(id),
    api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    backend_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled BOOLEAN DEFAULT 1,
    priority INTEGER DEFAULT 0,
    timeout_seconds INTEGER DEFAULT 30,
    capabilities_id INTEGER REFERENCES string_pool(id),
    last_test_at TIMESTAMP,
    last_test_status_id INTEGER REFERENCES string_pool(id),
    last_test_error_id INTEGER REFERENCES string_pool(id),
    limit_default INTEGER NOT NULL DEFAULT 100,
    limit_max INTEGER NOT NULL DEFAULT 100,
    basic_username_id INTEGER REFERENCES string_pool(id),
    basic_password_encrypted_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO torznab_indexers_new (
    id, owner_id, name_id, base_url_id, indexer_id_string_id,
    api_key_encrypted_id, backend_id, enabled, priority, timeout_seconds,
    capabilities_id, last_test_at, last_test_status_id, last_test_error_id,
    limit_default, limit_max, basic_username_id, basic_password_encrypted_id,
    created_at, updated_at
)
SELECT
    ti.id,
    (SELECT id FROM users LIMIT 1),
    ti.name_id, ti.base_url_id, ti.indexer_id_string_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.api_key_encrypted),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.backend),
    ti.enabled, ti.priority, ti.timeout_seconds,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.capabilities),
    ti.last_test_at,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.last_test_status),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.last_test_error),
    ti.limit_default, ti.limit_max,
    ti.basic_username_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = ti.basic_password_encrypted),
    ti.created_at, ti.updated_at
FROM torznab_indexers ti;

-- Must drop child tables that reference torznab_indexers first, then recreate
-- Save child data into temp tables
CREATE TEMP TABLE temp_torznab_capabilities AS SELECT * FROM torznab_indexer_capabilities;
CREATE TEMP TABLE temp_torznab_categories AS SELECT * FROM torznab_indexer_categories;
CREATE TEMP TABLE temp_torznab_errors AS SELECT * FROM torznab_indexer_errors;
CREATE TEMP TABLE temp_torznab_cooldowns AS SELECT * FROM torznab_indexer_cooldowns;
CREATE TEMP TABLE temp_torznab_latency AS SELECT * FROM torznab_indexer_latency;

DROP TABLE torznab_indexer_capabilities;
DROP TABLE torznab_indexer_categories;
DROP TRIGGER IF EXISTS trg_torznab_cooldowns_updated_at;
DROP TABLE torznab_indexer_cooldowns;
DROP TABLE torznab_indexer_latency;
DROP TABLE torznab_indexer_errors;

DROP TRIGGER IF EXISTS update_torznab_indexers_updated_at;
DROP VIEW IF EXISTS torznab_indexers_view;
DROP VIEW IF EXISTS torznab_indexer_latency_stats;
DROP VIEW IF EXISTS torznab_indexer_health;
DROP VIEW IF EXISTS torznab_indexer_capabilities_view;
DROP VIEW IF EXISTS torznab_indexer_categories_view;
DROP VIEW IF EXISTS torznab_indexer_errors_view;
DROP VIEW IF EXISTS torznab_indexer_cooldowns_view;
DROP VIEW IF EXISTS torznab_indexer_latency_view;
DROP TABLE torznab_indexers;
ALTER TABLE torznab_indexers_new RENAME TO torznab_indexers;

CREATE INDEX IF NOT EXISTS idx_torznab_indexers_owner ON torznab_indexers(owner_id);
CREATE INDEX IF NOT EXISTS idx_torznab_indexers_enabled ON torznab_indexers(enabled);
CREATE INDEX IF NOT EXISTS idx_torznab_indexers_priority ON torznab_indexers(priority DESC);

CREATE TRIGGER IF NOT EXISTS trg_torznab_indexers_updated
AFTER UPDATE ON torznab_indexers BEGIN
    UPDATE torznab_indexers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW torznab_indexers_view AS
SELECT ti.id, ti.owner_id, sp_n.value AS name, sp_bu.value AS base_url,
       sp_is.value AS indexer_id_string, sp_ak.value AS api_key_encrypted,
       sp_be.value AS backend, ti.enabled, ti.priority, ti.timeout_seconds,
       sp_cap.value AS capabilities, ti.last_test_at,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error,
       ti.limit_default, ti.limit_max,
       sp_bun.value AS basic_username, sp_bpe.value AS basic_password_encrypted,
       ti.created_at, ti.updated_at
FROM torznab_indexers ti
JOIN string_pool sp_n  ON ti.name_id = sp_n.id
JOIN string_pool sp_bu ON ti.base_url_id = sp_bu.id
JOIN string_pool sp_ak ON ti.api_key_encrypted_id = sp_ak.id
JOIN string_pool sp_be ON ti.backend_id = sp_be.id
LEFT JOIN string_pool sp_is  ON ti.indexer_id_string_id = sp_is.id
LEFT JOIN string_pool sp_cap ON ti.capabilities_id = sp_cap.id
LEFT JOIN string_pool sp_lts ON ti.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ti.last_test_error_id = sp_lte.id
LEFT JOIN string_pool sp_bun ON ti.basic_username_id = sp_bun.id
LEFT JOIN string_pool sp_bpe ON ti.basic_password_encrypted_id = sp_bpe.id;

-- 2. Recreate torznab_indexer_capabilities (already interned, just recreate FK)
CREATE TABLE torznab_indexer_capabilities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    capability_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, capability_type_id)
);
CREATE INDEX IF NOT EXISTS idx_torznab_capabilities_indexer ON torznab_indexer_capabilities(indexer_id);

INSERT INTO torznab_indexer_capabilities (id, indexer_id, capability_type_id, created_at)
SELECT id, indexer_id, capability_type_id, created_at FROM temp_torznab_capabilities;

CREATE VIEW torznab_indexer_capabilities_view AS
SELECT tc.id, tc.indexer_id, sp_ct.value AS capability_type, tc.created_at
FROM torznab_indexer_capabilities tc
JOIN string_pool sp_ct ON tc.capability_type_id = sp_ct.id;

-- 3. Recreate torznab_indexer_categories (already interned)
CREATE TABLE torznab_indexer_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL,
    category_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    parent_category_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_torznab_categories_indexer ON torznab_indexer_categories(indexer_id);

INSERT INTO torznab_indexer_categories (id, indexer_id, category_id, category_name_id, parent_category_id, created_at)
SELECT id, indexer_id, category_id, category_name_id, parent_category_id, created_at FROM temp_torznab_categories;

CREATE VIEW torznab_indexer_categories_view AS
SELECT tc.id, tc.indexer_id, tc.category_id, sp_cn.value AS category_name,
       tc.parent_category_id, tc.created_at
FROM torznab_indexer_categories tc
JOIN string_pool sp_cn ON tc.category_name_id = sp_cn.id;

-- 4. Recreate torznab_indexer_errors — intern error_code
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT error_code FROM temp_torznab_errors WHERE error_code IS NOT NULL AND error_code != '';

CREATE TABLE torznab_indexer_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    error_message_id INTEGER NOT NULL REFERENCES string_pool(id),
    error_code_id INTEGER REFERENCES string_pool(id),
    occurred_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP,
    error_count INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_indexer ON torznab_indexer_errors(indexer_id);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_occurred ON torznab_indexer_errors(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_unresolved ON torznab_indexer_errors(indexer_id, resolved_at) WHERE resolved_at IS NULL;

INSERT INTO torznab_indexer_errors (id, indexer_id, error_message_id, error_code_id, occurred_at, resolved_at, error_count)
SELECT te.id, te.indexer_id, te.error_message_id,
       (SELECT sp.id FROM string_pool sp WHERE sp.value = te.error_code),
       te.occurred_at, te.resolved_at, te.error_count
FROM temp_torznab_errors te;

CREATE VIEW torznab_indexer_errors_view AS
SELECT te.id, te.indexer_id, sp_em.value AS error_message, sp_ec.value AS error_code,
       te.occurred_at, te.resolved_at, te.error_count
FROM torznab_indexer_errors te
JOIN string_pool sp_em ON te.error_message_id = sp_em.id
LEFT JOIN string_pool sp_ec ON te.error_code_id = sp_ec.id;

-- 5. Recreate torznab_indexer_cooldowns — intern reason
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT reason FROM temp_torznab_cooldowns WHERE reason IS NOT NULL AND reason != '';

CREATE TABLE torznab_indexer_cooldowns (
    indexer_id INTEGER PRIMARY KEY REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    resume_at TIMESTAMP NOT NULL,
    cooldown_seconds INTEGER NOT NULL,
    reason_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_torznab_cooldowns_resume ON torznab_indexer_cooldowns(resume_at);

CREATE TRIGGER IF NOT EXISTS trg_torznab_cooldowns_updated
AFTER UPDATE ON torznab_indexer_cooldowns FOR EACH ROW BEGIN
    UPDATE torznab_indexer_cooldowns SET updated_at = CURRENT_TIMESTAMP WHERE indexer_id = NEW.indexer_id;
END;

INSERT INTO torznab_indexer_cooldowns (indexer_id, resume_at, cooldown_seconds, reason_id, created_at, updated_at)
SELECT cd.indexer_id, cd.resume_at, cd.cooldown_seconds,
       (SELECT sp.id FROM string_pool sp WHERE sp.value = cd.reason),
       cd.created_at, cd.updated_at
FROM temp_torznab_cooldowns cd;

CREATE VIEW torznab_indexer_cooldowns_view AS
SELECT cd.indexer_id, cd.resume_at, cd.cooldown_seconds,
       sp_r.value AS reason, cd.created_at, cd.updated_at
FROM torznab_indexer_cooldowns cd
LEFT JOIN string_pool sp_r ON cd.reason_id = sp_r.id;

-- 6. Recreate torznab_indexer_latency — intern operation_type
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT operation_type FROM temp_torznab_latency WHERE operation_type IS NOT NULL;

CREATE TABLE torznab_indexer_latency (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    operation_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    latency_ms INTEGER NOT NULL,
    success BOOLEAN NOT NULL,
    measured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_torznab_latency_indexer ON torznab_indexer_latency(indexer_id);
CREATE INDEX IF NOT EXISTS idx_torznab_latency_measured ON torznab_indexer_latency(measured_at DESC);

INSERT INTO torznab_indexer_latency (id, indexer_id, operation_type_id, latency_ms, success, measured_at)
SELECT tl.id, tl.indexer_id,
       (SELECT sp.id FROM string_pool sp WHERE sp.value = tl.operation_type),
       tl.latency_ms, tl.success, tl.measured_at
FROM temp_torznab_latency tl;

CREATE VIEW torznab_indexer_latency_view AS
SELECT tl.id, tl.indexer_id, sp_ot.value AS operation_type,
       tl.latency_ms, tl.success, tl.measured_at
FROM torznab_indexer_latency tl
JOIN string_pool sp_ot ON tl.operation_type_id = sp_ot.id;

CREATE VIEW torznab_indexer_latency_stats AS
SELECT tl.indexer_id, sp_ot.value AS operation_type,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN tl.success THEN 1 ELSE 0 END) AS successful_requests,
       AVG(tl.latency_ms) AS avg_latency_ms,
       MIN(tl.latency_ms) AS min_latency_ms,
       MAX(tl.latency_ms) AS max_latency_ms
FROM torznab_indexer_latency tl
JOIN string_pool sp_ot ON tl.operation_type_id = sp_ot.id
GROUP BY tl.indexer_id, sp_ot.value;

CREATE VIEW torznab_indexer_health AS
SELECT ti.id AS indexer_id, sp_n.value AS name,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error, ti.last_test_at,
       (SELECT COUNT(*) FROM torznab_indexer_errors te WHERE te.indexer_id = ti.id AND te.resolved_at IS NULL) AS unresolved_errors,
       (SELECT sp_em.value FROM torznab_indexer_errors te JOIN string_pool sp_em ON te.error_message_id = sp_em.id WHERE te.indexer_id = ti.id ORDER BY te.occurred_at DESC LIMIT 1) AS latest_error
FROM torznab_indexers ti
JOIN string_pool sp_n ON ti.name_id = sp_n.id
LEFT JOIN string_pool sp_lts ON ti.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ti.last_test_error_id = sp_lte.id;

-- Cleanup temp tables
DROP TABLE temp_torznab_capabilities;
DROP TABLE temp_torznab_categories;
DROP TABLE temp_torznab_errors;
DROP TABLE temp_torznab_cooldowns;
DROP TABLE temp_torznab_latency;
