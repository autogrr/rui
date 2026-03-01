-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 080: Add expr_filter to instance_backup_settings for expr-lang torrent filtering.

ALTER TABLE instance_backup_settings ADD COLUMN expr_filter TEXT NOT NULL DEFAULT '';

-- Rebuild the view to expose the new column (the old view still uses SELECT *
-- on the underlying table via the join, but naming it explicitly avoids surprises).
DROP VIEW IF EXISTS instance_backup_settings_view;
CREATE VIEW instance_backup_settings_view AS
SELECT bs.instance_id, bs.owner_id, bs.enabled,
       bs.hourly_enabled, bs.daily_enabled, bs.weekly_enabled, bs.monthly_enabled,
       bs.keep_hourly, bs.keep_daily, bs.keep_weekly, bs.keep_monthly,
       bs.include_categories, bs.include_tags,
       COALESCE(sp_cp.value, '') AS custom_path,
       bs.expr_filter,
       bs.created_at, bs.updated_at
FROM instance_backup_settings bs
LEFT JOIN string_pool sp_cp ON bs.custom_path_id = sp_cp.id;
