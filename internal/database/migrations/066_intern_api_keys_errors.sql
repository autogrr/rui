-- Migration 066: API keys + instance errors — add owner_id, finish interning
-- api_keys: intern key_hash → key_hash_id, add owner_id
-- client_api_keys: intern key_hash → key_hash_id, add owner_id
-- instance_errors: add owner_id (already interned)

-- 1. api_keys — intern key_hash, add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT key_hash FROM api_keys;

CREATE TABLE api_keys_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash_id  INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    name_id      INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

INSERT INTO api_keys_new (id, owner_id, key_hash_id, name_id, created_at, last_used_at)
SELECT a.id,
       (SELECT id FROM users LIMIT 1),
       (SELECT sp.id FROM string_pool sp WHERE sp.value = a.key_hash),
       a.name_id, a.created_at, a.last_used_at
FROM api_keys a;

-- Drop view BEFORE the table it depends on.
-- SQLite 3.25+ validates all views during ALTER TABLE RENAME.
DROP VIEW IF EXISTS api_keys_view;

DROP TABLE api_keys;
ALTER TABLE api_keys_new RENAME TO api_keys;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash  ON api_keys(key_hash_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner ON api_keys(owner_id);

DROP VIEW IF EXISTS api_keys_view;
CREATE VIEW api_keys_view AS
SELECT a.id, a.owner_id, sp_kh.value AS key_hash, sp_n.value AS name,
       a.created_at, a.last_used_at
FROM api_keys a
JOIN string_pool sp_kh ON a.key_hash_id = sp_kh.id
JOIN string_pool sp_n  ON a.name_id = sp_n.id;

-- 2. client_api_keys — intern key_hash, add owner_id
INSERT OR IGNORE INTO string_pool (value) SELECT DISTINCT key_hash FROM client_api_keys;

CREATE TABLE client_api_keys_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash_id     INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    client_name_id  INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    instance_id     INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at    TIMESTAMP
);

INSERT INTO client_api_keys_new (id, owner_id, key_hash_id, client_name_id, instance_id, created_at, last_used_at)
SELECT c.id,
       (SELECT id FROM users LIMIT 1),
       (SELECT sp.id FROM string_pool sp WHERE sp.value = c.key_hash),
       c.client_name_id, c.instance_id, c.created_at, c.last_used_at
FROM client_api_keys c;

-- Drop view BEFORE the table it depends on.
DROP VIEW IF EXISTS client_api_keys_view;

DROP TABLE client_api_keys;
ALTER TABLE client_api_keys_new RENAME TO client_api_keys;

CREATE UNIQUE INDEX IF NOT EXISTS idx_client_api_keys_hash     ON client_api_keys(key_hash_id);
CREATE INDEX IF NOT EXISTS idx_client_api_keys_owner    ON client_api_keys(owner_id);
CREATE INDEX IF NOT EXISTS idx_client_api_keys_instance ON client_api_keys(instance_id);

DROP VIEW IF EXISTS client_api_keys_view;
CREATE VIEW client_api_keys_view AS
SELECT c.id, c.owner_id, sp_kh.value AS key_hash, sp_cn.value AS client_name,
       c.instance_id, c.created_at, c.last_used_at
FROM client_api_keys c
JOIN string_pool sp_kh ON c.key_hash_id = sp_kh.id
JOIN string_pool sp_cn ON c.client_name_id = sp_cn.id;

-- 3. instance_errors — add owner_id (already has error_type_id, error_message_id)
CREATE TABLE instance_errors_new (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id      INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    error_type_id    INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    error_message_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    occurred_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO instance_errors_new (id, owner_id, instance_id, error_type_id, error_message_id, occurred_at)
SELECT ie.id,
       (SELECT id FROM users LIMIT 1),
       ie.instance_id, ie.error_type_id, ie.error_message_id, ie.occurred_at
FROM instance_errors ie;

DROP TRIGGER IF EXISTS cleanup_old_instance_errors;
-- Drop view BEFORE the table it depends on.
DROP VIEW IF EXISTS instance_errors_view;
DROP TABLE instance_errors;
ALTER TABLE instance_errors_new RENAME TO instance_errors;

CREATE INDEX IF NOT EXISTS idx_instance_errors_lookup ON instance_errors(instance_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_instance_errors_owner  ON instance_errors(owner_id);

CREATE TRIGGER IF NOT EXISTS cleanup_old_instance_errors
AFTER INSERT ON instance_errors BEGIN
    DELETE FROM instance_errors
    WHERE instance_id = NEW.instance_id
      AND id NOT IN (
          SELECT id FROM instance_errors
          WHERE instance_id = NEW.instance_id
          ORDER BY occurred_at DESC LIMIT 5);
END;

DROP VIEW IF EXISTS instance_errors_view;
CREATE VIEW instance_errors_view AS
SELECT ie.id, ie.owner_id, ie.instance_id,
       sp_et.value AS error_type, sp_em.value AS error_message, ie.occurred_at
FROM instance_errors ie
JOIN string_pool sp_et ON ie.error_type_id = sp_et.id
JOIN string_pool sp_em ON ie.error_message_id = sp_em.id;
