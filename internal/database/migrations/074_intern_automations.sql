-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 074: Intern automations and automation_activity
-- Converts TEXT columns → string_pool references, adds owner_id.

-- ─── automations ────────────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT name FROM automations WHERE name IS NOT NULL
UNION SELECT DISTINCT tracker_pattern FROM automations WHERE tracker_pattern IS NOT NULL
UNION SELECT DISTINCT conditions FROM automations WHERE conditions IS NOT NULL
UNION SELECT DISTINCT free_space_source FROM automations WHERE free_space_source IS NOT NULL;

CREATE TABLE automations_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    tracker_pattern_id INTEGER NOT NULL REFERENCES string_pool(id),
    conditions_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER,
    free_space_source_id INTEGER REFERENCES string_pool(id),
    dry_run INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO automations_new (
    id, owner_id, instance_id, name_id, tracker_pattern_id, conditions_id,
    enabled, sort_order, interval_seconds, free_space_source_id, dry_run,
    created_at, updated_at
)
SELECT
    a.id,
    (SELECT id FROM users LIMIT 1),
    a.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = a.name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = a.tracker_pattern),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = a.conditions),
    a.enabled, a.sort_order, a.interval_seconds,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = a.free_space_source),
    a.dry_run,
    a.created_at, a.updated_at
FROM automations a;

DROP TRIGGER IF EXISTS trg_automations_updated;
DROP TABLE automations;
ALTER TABLE automations_new RENAME TO automations;

CREATE INDEX IF NOT EXISTS idx_automations_owner ON automations(owner_id);
CREATE INDEX IF NOT EXISTS idx_automations_instance ON automations(instance_id, sort_order, id);

CREATE TRIGGER IF NOT EXISTS trg_automations_updated
AFTER UPDATE ON automations BEGIN
    UPDATE automations SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS automations_view AS
SELECT a.id, a.owner_id, a.instance_id,
       sp_n.value AS name, sp_tp.value AS tracker_pattern, sp_c.value AS conditions,
       a.enabled, a.sort_order, a.interval_seconds, sp_fs.value AS free_space_source,
       a.dry_run, a.created_at, a.updated_at
FROM automations a
JOIN string_pool sp_n  ON a.name_id = sp_n.id
JOIN string_pool sp_tp ON a.tracker_pattern_id = sp_tp.id
JOIN string_pool sp_c  ON a.conditions_id = sp_c.id
LEFT JOIN string_pool sp_fs ON a.free_space_source_id = sp_fs.id;

-- ─── automation_activity ────────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT hash FROM automation_activity WHERE hash IS NOT NULL
UNION SELECT DISTINCT torrent_name FROM automation_activity WHERE torrent_name IS NOT NULL
UNION SELECT DISTINCT tracker_domain FROM automation_activity WHERE tracker_domain IS NOT NULL
UNION SELECT DISTINCT action FROM automation_activity WHERE action IS NOT NULL
UNION SELECT DISTINCT rule_name FROM automation_activity WHERE rule_name IS NOT NULL
UNION SELECT DISTINCT outcome FROM automation_activity WHERE outcome IS NOT NULL
UNION SELECT DISTINCT reason FROM automation_activity WHERE reason IS NOT NULL
UNION SELECT DISTINCT details FROM automation_activity WHERE details IS NOT NULL;

CREATE TABLE automation_activity_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    torrent_name_id INTEGER REFERENCES string_pool(id),
    tracker_domain_id INTEGER REFERENCES string_pool(id),
    action_id INTEGER NOT NULL REFERENCES string_pool(id),
    rule_id INTEGER,
    rule_name_id INTEGER REFERENCES string_pool(id),
    outcome_id INTEGER NOT NULL REFERENCES string_pool(id),
    reason_id INTEGER REFERENCES string_pool(id),
    details_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO automation_activity_new (
    id, owner_id, instance_id, hash_id, torrent_name_id, tracker_domain_id,
    action_id, rule_id, rule_name_id, outcome_id, reason_id, details_id, created_at
)
SELECT
    aa.id,
    (SELECT id FROM users LIMIT 1),
    aa.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.hash),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.torrent_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.tracker_domain),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.action),
    aa.rule_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.rule_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.outcome),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.reason),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = aa.details),
    aa.created_at
FROM automation_activity aa;

DROP TABLE automation_activity;
ALTER TABLE automation_activity_new RENAME TO automation_activity;

CREATE INDEX IF NOT EXISTS idx_automation_activity_instance ON automation_activity(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_automation_activity_owner ON automation_activity(owner_id);

CREATE VIEW IF NOT EXISTS automation_activity_view AS
SELECT aa.id, aa.owner_id, aa.instance_id,
       sp_h.value AS hash, sp_tn.value AS torrent_name, sp_td.value AS tracker_domain,
       sp_a.value AS action, aa.rule_id, sp_rn.value AS rule_name,
       sp_o.value AS outcome, sp_r.value AS reason, sp_d.value AS details, aa.created_at
FROM automation_activity aa
JOIN string_pool sp_h ON aa.hash_id = sp_h.id
JOIN string_pool sp_a ON aa.action_id = sp_a.id
JOIN string_pool sp_o ON aa.outcome_id = sp_o.id
LEFT JOIN string_pool sp_tn ON aa.torrent_name_id = sp_tn.id
LEFT JOIN string_pool sp_td ON aa.tracker_domain_id = sp_td.id
LEFT JOIN string_pool sp_rn ON aa.rule_name_id = sp_rn.id
LEFT JOIN string_pool sp_r  ON aa.reason_id = sp_r.id
LEFT JOIN string_pool sp_d  ON aa.details_id = sp_d.id;
