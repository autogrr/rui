-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 073: Intern tracker_customizations and dashboard_settings
-- Converts TEXT columns → string_pool references, adds owner_id.

-- ─── tracker_customizations ─────────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT display_name FROM tracker_customizations WHERE display_name IS NOT NULL
UNION SELECT DISTINCT domains FROM tracker_customizations WHERE domains IS NOT NULL
UNION SELECT DISTINCT included_in_stats FROM tracker_customizations WHERE included_in_stats IS NOT NULL AND included_in_stats != '';

CREATE TABLE tracker_customizations_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    domains_id INTEGER NOT NULL REFERENCES string_pool(id),
    included_in_stats_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tracker_customizations_new (
    id, owner_id, display_name_id, domains_id, included_in_stats_id, created_at, updated_at
)
SELECT
    t.id,
    (SELECT id FROM users LIMIT 1),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.display_name),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = t.domains),
    CASE WHEN t.included_in_stats IS NOT NULL AND t.included_in_stats != ''
         THEN (SELECT sp.id FROM string_pool sp WHERE sp.value = t.included_in_stats)
         ELSE NULL END,
    t.created_at, t.updated_at
FROM tracker_customizations t;

DROP TRIGGER IF EXISTS trg_tracker_customizations_updated;
DROP TABLE tracker_customizations;
ALTER TABLE tracker_customizations_new RENAME TO tracker_customizations;

CREATE INDEX IF NOT EXISTS idx_tracker_customizations_owner ON tracker_customizations(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_tracker_customizations_updated
AFTER UPDATE ON tracker_customizations BEGIN
    UPDATE tracker_customizations SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS tracker_customizations_view AS
SELECT tc.id, tc.owner_id, sp_dn.value AS display_name, sp_d.value AS domains,
       sp_is.value AS included_in_stats, tc.created_at, tc.updated_at
FROM tracker_customizations tc
JOIN string_pool sp_dn ON tc.display_name_id = sp_dn.id
JOIN string_pool sp_d  ON tc.domains_id = sp_d.id
LEFT JOIN string_pool sp_is ON tc.included_in_stats_id = sp_is.id;

-- ─── dashboard_settings ─────────────────────────────────────────────────────
-- user_id already references users table; just intern TEXT columns.

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT section_visibility FROM dashboard_settings WHERE section_visibility IS NOT NULL
UNION SELECT DISTINCT section_order FROM dashboard_settings WHERE section_order IS NOT NULL
UNION SELECT DISTINCT section_collapsed FROM dashboard_settings WHERE section_collapsed IS NOT NULL
UNION SELECT DISTINCT tracker_breakdown_sort_column FROM dashboard_settings WHERE tracker_breakdown_sort_column IS NOT NULL
UNION SELECT DISTINCT tracker_breakdown_sort_direction FROM dashboard_settings WHERE tracker_breakdown_sort_direction IS NOT NULL;

-- Ensure defaults exist
INSERT OR IGNORE INTO string_pool (value)
SELECT '{}' UNION SELECT '[]' UNION SELECT 'uploaded' UNION SELECT 'desc';

CREATE TABLE dashboard_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    section_visibility_id INTEGER NOT NULL REFERENCES string_pool(id),
    section_order_id INTEGER NOT NULL REFERENCES string_pool(id),
    section_collapsed_id INTEGER NOT NULL REFERENCES string_pool(id),
    tracker_breakdown_sort_column_id INTEGER REFERENCES string_pool(id),
    tracker_breakdown_sort_direction_id INTEGER REFERENCES string_pool(id),
    tracker_breakdown_items_per_page INTEGER DEFAULT 15,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO dashboard_settings_new (
    id, user_id, section_visibility_id, section_order_id, section_collapsed_id,
    tracker_breakdown_sort_column_id, tracker_breakdown_sort_direction_id,
    tracker_breakdown_items_per_page, created_at, updated_at
)
SELECT
    d.id, d.user_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.section_visibility),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.section_order),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.section_collapsed),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.tracker_breakdown_sort_column),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = d.tracker_breakdown_sort_direction),
    d.tracker_breakdown_items_per_page, d.created_at, d.updated_at
FROM dashboard_settings d;

DROP TRIGGER IF EXISTS trg_dashboard_settings_updated;
DROP TABLE dashboard_settings;
ALTER TABLE dashboard_settings_new RENAME TO dashboard_settings;

CREATE TRIGGER IF NOT EXISTS trg_dashboard_settings_updated
AFTER UPDATE ON dashboard_settings BEGIN
    UPDATE dashboard_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dashboard_settings_view AS
SELECT ds.id, ds.user_id,
       sp_sv.value AS section_visibility, sp_so.value AS section_order,
       sp_sc.value AS section_collapsed,
       sp_tbsc.value AS tracker_breakdown_sort_column,
       sp_tbsd.value AS tracker_breakdown_sort_direction,
       ds.tracker_breakdown_items_per_page, ds.created_at, ds.updated_at
FROM dashboard_settings ds
JOIN string_pool sp_sv ON ds.section_visibility_id = sp_sv.id
JOIN string_pool sp_so ON ds.section_order_id = sp_so.id
JOIN string_pool sp_sc ON ds.section_collapsed_id = sp_sc.id
LEFT JOIN string_pool sp_tbsc ON ds.tracker_breakdown_sort_column_id = sp_tbsc.id
LEFT JOIN string_pool sp_tbsd ON ds.tracker_breakdown_sort_direction_id = sp_tbsd.id;
