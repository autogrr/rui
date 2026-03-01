-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 078: Intern notification_targets, log_exclusions, and licenses
-- Converts TEXT columns → string_pool references, adds owner_id.

-- ─── notification_targets ───────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT name FROM notification_targets WHERE name IS NOT NULL
UNION SELECT DISTINCT url FROM notification_targets WHERE url IS NOT NULL
UNION SELECT DISTINCT event_types FROM notification_targets WHERE event_types IS NOT NULL;

INSERT OR IGNORE INTO string_pool (value) SELECT '[]';

CREATE TABLE notification_targets_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    url_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1,
    event_types_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO notification_targets_new (
    id, owner_id, name_id, url_id, enabled, event_types_id, created_at, updated_at
)
SELECT
    n.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = n.name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = n.url),
    n.enabled,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = n.event_types),
    n.created_at, n.updated_at
FROM notification_targets n;

DROP TRIGGER IF EXISTS trg_notification_targets_updated;
DROP TABLE notification_targets;
ALTER TABLE notification_targets_new RENAME TO notification_targets;

CREATE INDEX IF NOT EXISTS idx_notification_targets_owner ON notification_targets(owner_id);
CREATE INDEX IF NOT EXISTS idx_notification_targets_enabled ON notification_targets(enabled);

CREATE TRIGGER IF NOT EXISTS trg_notification_targets_updated
AFTER UPDATE ON notification_targets BEGIN
    UPDATE notification_targets SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS notification_targets_view AS
SELECT nt.id, nt.owner_id, sp_n.value AS name, sp_u.value AS url,
       nt.enabled, sp_et.value AS event_types, nt.created_at, nt.updated_at
FROM notification_targets nt
JOIN string_pool sp_n  ON nt.name_id = sp_n.id
JOIN string_pool sp_u  ON nt.url_id = sp_u.id
JOIN string_pool sp_et ON nt.event_types_id = sp_et.id;

-- ─── log_exclusions ─────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT patterns FROM log_exclusions WHERE patterns IS NOT NULL;

CREATE TABLE log_exclusions_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    patterns_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO log_exclusions_new (id, owner_id, patterns_id, created_at, updated_at)
SELECT
    le.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = le.patterns),
    le.created_at, le.updated_at
FROM log_exclusions le;

DROP TRIGGER IF EXISTS trg_log_exclusions_updated;
DROP TABLE log_exclusions;
ALTER TABLE log_exclusions_new RENAME TO log_exclusions;

CREATE INDEX IF NOT EXISTS idx_log_exclusions_owner ON log_exclusions(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_log_exclusions_updated
AFTER UPDATE ON log_exclusions BEGIN
    UPDATE log_exclusions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS log_exclusions_view AS
SELECT le.id, le.owner_id, sp_p.value AS patterns, le.created_at, le.updated_at
FROM log_exclusions le
JOIN string_pool sp_p ON le.patterns_id = sp_p.id;

-- ─── licenses ───────────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT license_key FROM licenses WHERE license_key IS NOT NULL
UNION SELECT DISTINCT product_name FROM licenses WHERE product_name IS NOT NULL
UNION SELECT DISTINCT status FROM licenses WHERE status IS NOT NULL
UNION SELECT DISTINCT polar_customer_id FROM licenses WHERE polar_customer_id IS NOT NULL
UNION SELECT DISTINCT polar_product_id FROM licenses WHERE polar_product_id IS NOT NULL
UNION SELECT DISTINCT polar_activation_id FROM licenses WHERE polar_activation_id IS NOT NULL
UNION SELECT DISTINCT username FROM licenses WHERE username IS NOT NULL
UNION SELECT DISTINCT provider FROM licenses WHERE provider IS NOT NULL
UNION SELECT DISTINCT dodo_instance_id FROM licenses WHERE dodo_instance_id IS NOT NULL;

CREATE TABLE licenses_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_key_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    product_name_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    status_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    activated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME,
    last_validated DATETIME DEFAULT CURRENT_TIMESTAMP,
    polar_customer_id_sid INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    polar_product_id_sid INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    polar_activation_id_sid INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    username_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    provider_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    dodo_instance_id_sid INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO licenses_new (
    id, owner_id, license_key_id, product_name_id, status_id,
    activated_at, expires_at, last_validated,
    polar_customer_id_sid, polar_product_id_sid, polar_activation_id_sid,
    username_id, provider_id, dodo_instance_id_sid,
    created_at, updated_at
)
SELECT
    l.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.license_key),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.product_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.status),
    l.activated_at, l.expires_at, l.last_validated,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.polar_customer_id),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.polar_product_id),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.polar_activation_id),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.username),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.provider),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = l.dodo_instance_id),
    l.created_at, l.updated_at
FROM licenses l;

DROP TABLE licenses;
ALTER TABLE licenses_new RENAME TO licenses;

CREATE INDEX IF NOT EXISTS idx_licenses_status ON licenses(status_id);
CREATE INDEX IF NOT EXISTS idx_licenses_product ON licenses(product_name_id);
CREATE INDEX IF NOT EXISTS idx_licenses_key ON licenses(license_key_id);
CREATE INDEX IF NOT EXISTS idx_licenses_owner ON licenses(owner_id);

CREATE VIEW IF NOT EXISTS licenses_view AS
SELECT l.id, l.owner_id,
       sp_lk.value AS license_key, sp_pn.value AS product_name, sp_st.value AS status,
       l.activated_at, l.expires_at, l.last_validated,
       sp_pci.value AS polar_customer_id, sp_ppi.value AS polar_product_id,
       sp_pai.value AS polar_activation_id, sp_un.value AS username,
       sp_pr.value AS provider, sp_di.value AS dodo_instance_id,
       l.created_at, l.updated_at
FROM licenses l
JOIN string_pool sp_lk ON l.license_key_id = sp_lk.id
JOIN string_pool sp_pn ON l.product_name_id = sp_pn.id
JOIN string_pool sp_st ON l.status_id = sp_st.id
LEFT JOIN string_pool sp_pci ON l.polar_customer_id_sid = sp_pci.id
LEFT JOIN string_pool sp_ppi ON l.polar_product_id_sid = sp_ppi.id
LEFT JOIN string_pool sp_pai ON l.polar_activation_id_sid = sp_pai.id
LEFT JOIN string_pool sp_un  ON l.username_id = sp_un.id
LEFT JOIN string_pool sp_pr  ON l.provider_id = sp_pr.id
LEFT JOIN string_pool sp_di  ON l.dodo_instance_id_sid = sp_di.id;

-- ─── Drop old sessions table ────────────────────────────────────────────────
-- SCS sessions are being replaced by JWT sessions in a separate database.
DROP TABLE IF EXISTS sessions;
