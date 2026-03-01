-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 071: Intern cross_seed_settings + cross_seed_search_settings
-- Converts all TEXT columns → string_pool references, removes CHECK(id=1)
-- singleton constraints, adds owner_id for multi-user support.

-- ─── cross_seed_settings ────────────────────────────────────────────────────

-- Intern all existing TEXT values
INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT category FROM cross_seed_settings WHERE category IS NOT NULL
UNION SELECT DISTINCT target_instance_ids FROM cross_seed_settings WHERE target_instance_ids IS NOT NULL
UNION SELECT DISTINCT target_indexer_ids FROM cross_seed_settings WHERE target_indexer_ids IS NOT NULL
UNION SELECT DISTINCT rss_automation_tags FROM cross_seed_settings WHERE rss_automation_tags IS NOT NULL
UNION SELECT DISTINCT seeded_search_tags FROM cross_seed_settings WHERE seeded_search_tags IS NOT NULL
UNION SELECT DISTINCT completion_search_tags FROM cross_seed_settings WHERE completion_search_tags IS NOT NULL
UNION SELECT DISTINCT webhook_tags FROM cross_seed_settings WHERE webhook_tags IS NOT NULL
UNION SELECT DISTINCT rss_source_categories FROM cross_seed_settings WHERE rss_source_categories IS NOT NULL
UNION SELECT DISTINCT rss_source_tags FROM cross_seed_settings WHERE rss_source_tags IS NOT NULL
UNION SELECT DISTINCT rss_source_exclude_categories FROM cross_seed_settings WHERE rss_source_exclude_categories IS NOT NULL
UNION SELECT DISTINCT rss_source_exclude_tags FROM cross_seed_settings WHERE rss_source_exclude_tags IS NOT NULL
UNION SELECT DISTINCT webhook_source_categories FROM cross_seed_settings WHERE webhook_source_categories IS NOT NULL
UNION SELECT DISTINCT webhook_source_tags FROM cross_seed_settings WHERE webhook_source_tags IS NOT NULL
UNION SELECT DISTINCT webhook_source_exclude_categories FROM cross_seed_settings WHERE webhook_source_exclude_categories IS NOT NULL
UNION SELECT DISTINCT webhook_source_exclude_tags FROM cross_seed_settings WHERE webhook_source_exclude_tags IS NOT NULL
UNION SELECT DISTINCT custom_category FROM cross_seed_settings WHERE custom_category IS NOT NULL
UNION SELECT DISTINCT category_affix_mode FROM cross_seed_settings WHERE category_affix_mode IS NOT NULL
UNION SELECT DISTINCT category_affix FROM cross_seed_settings WHERE category_affix IS NOT NULL
UNION SELECT DISTINCT redacted_api_key_encrypted FROM cross_seed_settings WHERE redacted_api_key_encrypted IS NOT NULL
UNION SELECT DISTINCT orpheus_api_key_encrypted FROM cross_seed_settings WHERE orpheus_api_key_encrypted IS NOT NULL;

-- Ensure default values exist in string_pool for NOT NULL columns
INSERT OR IGNORE INTO string_pool (value)
SELECT '[]' UNION SELECT '["cross-seed"]' UNION SELECT 'suffix' UNION SELECT '.cross' UNION SELECT '';

CREATE TABLE cross_seed_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT 0,
    run_interval_minutes INTEGER NOT NULL DEFAULT 120,
    start_paused BOOLEAN NOT NULL DEFAULT 1,
    category_id INTEGER REFERENCES string_pool(id),
    target_instance_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    target_indexer_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    max_results_per_run INTEGER NOT NULL DEFAULT 50,
    find_individual_episodes BOOLEAN NOT NULL DEFAULT 0,
    size_mismatch_tolerance_percent REAL NOT NULL DEFAULT 5.0,
    use_category_from_indexer BOOLEAN NOT NULL DEFAULT 0,
    run_external_program_id INTEGER REFERENCES external_programs(id) ON DELETE SET NULL,
    rss_automation_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    seeded_search_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    completion_search_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    inherit_source_tags BOOLEAN NOT NULL DEFAULT 0,
    use_cross_category_suffix BOOLEAN NOT NULL DEFAULT 1,
    skip_auto_resume_rss BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_seeded_search BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_completion BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_webhook BOOLEAN NOT NULL DEFAULT 0,
    rss_source_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_exclude_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_exclude_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_exclude_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_exclude_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    skip_recheck BOOLEAN NOT NULL DEFAULT 0,
    skip_piece_boundary_safety_check BOOLEAN NOT NULL DEFAULT 1,
    use_custom_category BOOLEAN NOT NULL DEFAULT 0,
    custom_category_id INTEGER REFERENCES string_pool(id),
    use_cross_category_affix INTEGER NOT NULL DEFAULT 1,
    category_affix_mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    category_affix_id INTEGER NOT NULL REFERENCES string_pool(id),
    gazelle_enabled BOOLEAN NOT NULL DEFAULT 0,
    redacted_api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    orpheus_api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cross_seed_settings_new (
    id, owner_id, enabled, run_interval_minutes, start_paused,
    category_id, target_instance_ids_id, target_indexer_ids_id,
    max_results_per_run, find_individual_episodes, size_mismatch_tolerance_percent,
    use_category_from_indexer, run_external_program_id,
    rss_automation_tags_id, seeded_search_tags_id, completion_search_tags_id,
    webhook_tags_id, inherit_source_tags, use_cross_category_suffix,
    skip_auto_resume_rss, skip_auto_resume_seeded_search,
    skip_auto_resume_completion, skip_auto_resume_webhook,
    rss_source_categories_id, rss_source_tags_id,
    rss_source_exclude_categories_id, rss_source_exclude_tags_id,
    webhook_source_categories_id, webhook_source_tags_id,
    webhook_source_exclude_categories_id, webhook_source_exclude_tags_id,
    skip_recheck, skip_piece_boundary_safety_check,
    use_custom_category, custom_category_id,
    use_cross_category_affix, category_affix_mode_id, category_affix_id,
    gazelle_enabled, redacted_api_key_encrypted_id, orpheus_api_key_encrypted_id,
    created_at, updated_at
)
SELECT
    s.id,
    (SELECT id FROM users LIMIT 1),
    s.enabled, s.run_interval_minutes, s.start_paused,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.category),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.target_instance_ids),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.target_indexer_ids),
    s.max_results_per_run, s.find_individual_episodes, s.size_mismatch_tolerance_percent,
    s.use_category_from_indexer, s.run_external_program_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.rss_automation_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.seeded_search_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.completion_search_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.webhook_tags),
    s.inherit_source_tags, s.use_cross_category_suffix,
    s.skip_auto_resume_rss, s.skip_auto_resume_seeded_search,
    s.skip_auto_resume_completion, s.skip_auto_resume_webhook,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.rss_source_categories),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.rss_source_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.rss_source_exclude_categories),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.rss_source_exclude_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.webhook_source_categories),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.webhook_source_tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.webhook_source_exclude_categories),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.webhook_source_exclude_tags),
    s.skip_recheck, s.skip_piece_boundary_safety_check,
    s.use_custom_category,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.custom_category),
    s.use_cross_category_affix,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.category_affix_mode),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.category_affix),
    s.gazelle_enabled,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.redacted_api_key_encrypted),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.orpheus_api_key_encrypted),
    s.created_at, s.updated_at
FROM cross_seed_settings s;

DROP TRIGGER IF EXISTS trg_cross_seed_settings_updated;
DROP TABLE cross_seed_settings;
ALTER TABLE cross_seed_settings_new RENAME TO cross_seed_settings;

CREATE INDEX IF NOT EXISTS idx_cross_seed_settings_external_program
    ON cross_seed_settings(run_external_program_id);

CREATE TRIGGER IF NOT EXISTS trg_cross_seed_settings_updated
AFTER UPDATE ON cross_seed_settings BEGIN
    UPDATE cross_seed_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS cross_seed_settings_view AS
SELECT css.id, css.owner_id, css.enabled, css.run_interval_minutes, css.start_paused,
       sp_cat.value AS category, sp_tii.value AS target_instance_ids,
       sp_txi.value AS target_indexer_ids, css.max_results_per_run,
       css.find_individual_episodes, css.size_mismatch_tolerance_percent,
       css.use_category_from_indexer, css.run_external_program_id,
       sp_rat.value AS rss_automation_tags, sp_sst.value AS seeded_search_tags,
       sp_cst.value AS completion_search_tags, sp_wt.value AS webhook_tags,
       css.inherit_source_tags, css.use_cross_category_suffix,
       css.skip_auto_resume_rss, css.skip_auto_resume_seeded_search,
       css.skip_auto_resume_completion, css.skip_auto_resume_webhook,
       sp_rsc.value AS rss_source_categories, sp_rst.value AS rss_source_tags,
       sp_rsec.value AS rss_source_exclude_categories, sp_rset.value AS rss_source_exclude_tags,
       sp_wsc.value AS webhook_source_categories, sp_wst.value AS webhook_source_tags,
       sp_wsec.value AS webhook_source_exclude_categories, sp_wset.value AS webhook_source_exclude_tags,
       css.skip_recheck, css.skip_piece_boundary_safety_check, css.use_custom_category,
       sp_cc.value AS custom_category, css.use_cross_category_affix,
       sp_cam.value AS category_affix_mode, sp_ca.value AS category_affix,
       css.gazelle_enabled, sp_rak.value AS redacted_api_key_encrypted,
       sp_oak.value AS orpheus_api_key_encrypted, css.created_at, css.updated_at
FROM cross_seed_settings css
LEFT JOIN string_pool sp_cat ON css.category_id = sp_cat.id
JOIN string_pool sp_tii ON css.target_instance_ids_id = sp_tii.id
JOIN string_pool sp_txi ON css.target_indexer_ids_id = sp_txi.id
JOIN string_pool sp_rat ON css.rss_automation_tags_id = sp_rat.id
JOIN string_pool sp_sst ON css.seeded_search_tags_id = sp_sst.id
JOIN string_pool sp_cst ON css.completion_search_tags_id = sp_cst.id
JOIN string_pool sp_wt  ON css.webhook_tags_id = sp_wt.id
JOIN string_pool sp_rsc ON css.rss_source_categories_id = sp_rsc.id
JOIN string_pool sp_rst ON css.rss_source_tags_id = sp_rst.id
JOIN string_pool sp_rsec ON css.rss_source_exclude_categories_id = sp_rsec.id
JOIN string_pool sp_rset ON css.rss_source_exclude_tags_id = sp_rset.id
JOIN string_pool sp_wsc ON css.webhook_source_categories_id = sp_wsc.id
JOIN string_pool sp_wst ON css.webhook_source_tags_id = sp_wst.id
JOIN string_pool sp_wsec ON css.webhook_source_exclude_categories_id = sp_wsec.id
JOIN string_pool sp_wset ON css.webhook_source_exclude_tags_id = sp_wset.id
LEFT JOIN string_pool sp_cc ON css.custom_category_id = sp_cc.id
JOIN string_pool sp_cam ON css.category_affix_mode_id = sp_cam.id
JOIN string_pool sp_ca  ON css.category_affix_id = sp_ca.id
JOIN string_pool sp_rak ON css.redacted_api_key_encrypted_id = sp_rak.id
JOIN string_pool sp_oak ON css.orpheus_api_key_encrypted_id = sp_oak.id;

-- ─── cross_seed_search_settings ─────────────────────────────────────────────

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT categories FROM cross_seed_search_settings WHERE categories IS NOT NULL
UNION SELECT DISTINCT tags FROM cross_seed_search_settings WHERE tags IS NOT NULL
UNION SELECT DISTINCT indexer_ids FROM cross_seed_search_settings WHERE indexer_ids IS NOT NULL;

CREATE TABLE cross_seed_search_settings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL,
    categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    cooldown_minutes INTEGER NOT NULL DEFAULT 720,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cross_seed_search_settings_new (
    id, owner_id, instance_id, categories_id, tags_id, indexer_ids_id,
    interval_seconds, cooldown_minutes, created_at, updated_at
)
SELECT
    s.id,
    (SELECT id FROM users LIMIT 1),
    s.instance_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.categories),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.tags),
    (SELECT sp.id FROM string_pool sp WHERE sp.value = s.indexer_ids),
    s.interval_seconds, s.cooldown_minutes, s.created_at, s.updated_at
FROM cross_seed_search_settings s;

DROP TRIGGER IF EXISTS cross_seed_search_settings_updated_at;
DROP TABLE cross_seed_search_settings;
ALTER TABLE cross_seed_search_settings_new RENAME TO cross_seed_search_settings;

CREATE TRIGGER IF NOT EXISTS cross_seed_search_settings_updated_at
AFTER UPDATE ON cross_seed_search_settings
FOR EACH ROW
BEGIN
    UPDATE cross_seed_search_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS cross_seed_search_settings_view AS
SELECT csss.id, csss.owner_id, csss.instance_id,
       sp_c.value AS categories, sp_t.value AS tags, sp_ii.value AS indexer_ids,
       csss.interval_seconds, csss.cooldown_minutes, csss.created_at, csss.updated_at
FROM cross_seed_search_settings csss
JOIN string_pool sp_c  ON csss.categories_id = sp_c.id
JOIN string_pool sp_t  ON csss.tags_id = sp_t.id
JOIN string_pool sp_ii ON csss.indexer_ids_id = sp_ii.id;
