-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 079: Add expr_filter_id to automations for expr-lang torrent pre-filter support.

-- Intern the empty string so all existing rows get a valid default FK.
INSERT OR IGNORE INTO string_pool (value) VALUES ('');

ALTER TABLE automations ADD COLUMN expr_filter_id INTEGER REFERENCES string_pool(id);

-- Populate existing rows with the empty string ID.
UPDATE automations SET expr_filter_id = (SELECT id FROM string_pool WHERE value = '')
WHERE expr_filter_id IS NULL;

-- Rebuild automations_view to expose expr_filter as a plain string column.
DROP VIEW IF EXISTS automations_view;
CREATE VIEW automations_view AS
SELECT a.id, a.owner_id, a.instance_id,
       sp_n.value  AS name,
       sp_tp.value AS tracker_pattern,
       sp_c.value  AS conditions,
       a.enabled, a.sort_order, a.interval_seconds,
       sp_fs.value AS free_space_source,
       COALESCE(sp_ef.value, '') AS expr_filter,
       a.dry_run, a.created_at, a.updated_at
FROM automations a
JOIN string_pool sp_n  ON a.name_id         = sp_n.id
JOIN string_pool sp_tp ON a.tracker_pattern_id = sp_tp.id
JOIN string_pool sp_c  ON a.conditions_id   = sp_c.id
LEFT JOIN string_pool sp_fs ON a.free_space_source_id = sp_fs.id
LEFT JOIN string_pool sp_ef ON a.expr_filter_id       = sp_ef.id;
