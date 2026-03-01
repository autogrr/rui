-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 082: Intake pipeline.
-- intake_pipelines – named, ordered processing pipelines.
-- intake_rules     – ordered expr-lang rules within each pipeline.
-- intake_events    – immutable log of every release processed through intake.

-- ─── intake_pipelines ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS intake_pipelines (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT 1,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    -- comma-separated subset of: movie,tv,anime,music,book,unknown
    content_types TEXT   NOT NULL DEFAULT 'movie,tv',
    description  TEXT    NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_intake_pipelines_owner ON intake_pipelines(owner_id, enabled, sort_order);

CREATE TRIGGER IF NOT EXISTS trg_intake_pipelines_updated
AFTER UPDATE ON intake_pipelines BEGIN
    UPDATE intake_pipelines SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- ─── intake_rules ───────────────────────────────────────────────────────────
-- Rules within a pipeline are evaluated in sort_order (ascending) against each
-- incoming release.  The first matching rule wins (first-match semantics).
-- expr_filter is a boolean expr-lang expression.  Env is IntakeMatchContext.

CREATE TABLE IF NOT EXISTS intake_rules (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id  INTEGER NOT NULL REFERENCES intake_pipelines(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT 1,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    -- boolean expr-lang expression; empty = always match
    expr_filter  TEXT    NOT NULL DEFAULT '',
    -- action executed when the rule fires
    action       TEXT    NOT NULL DEFAULT 'grab' CHECK(action IN ('grab', 'skip', 'upgrade', 'remove')),
    -- JSON-encoded action parameters (e.g. category, save_path, tags…)
    action_params TEXT   NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_intake_rules_pipeline ON intake_rules(pipeline_id, enabled, sort_order);

CREATE TRIGGER IF NOT EXISTS trg_intake_rules_updated
AFTER UPDATE ON intake_rules BEGIN
    UPDATE intake_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- ─── intake_events ──────────────────────────────────────────────────────────
-- Append-only log; rows are never updated (processed_at / action are set on
-- insert once the pipeline run is complete).

CREATE TABLE IF NOT EXISTS intake_events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id            INTEGER NOT NULL  REFERENCES users(id) ON DELETE CASCADE,
    pipeline_id         INTEGER           REFERENCES intake_pipelines(id) ON DELETE SET NULL,
    -- raw release name submitted to the intake pipeline
    release_name        TEXT    NOT NULL,
    -- content type determined by rls
    content_type        TEXT    NOT NULL DEFAULT '',
    -- canonical title extracted from the release name via rls
    parsed_title        TEXT    NOT NULL DEFAULT '',
    parsed_year         INTEGER NOT NULL DEFAULT 0,
    -- library title matched by the matcher (NULL = no match)
    matched_library_id  INTEGER           REFERENCES library_titles(id) ON DELETE SET NULL,
    matched_title       TEXT    NOT NULL DEFAULT '',
    -- arr instance that provided metadata for this event
    arr_instance_id     INTEGER           REFERENCES arr_instances(id) ON DELETE SET NULL,
    -- external IDs resolved via arr lookups
    imdb_id             TEXT    NOT NULL DEFAULT '',
    tmdb_id             INTEGER NOT NULL DEFAULT 0,
    tvdb_id             INTEGER NOT NULL DEFAULT 0,
    -- rule that fired; NULL = no rule matched (default action taken)
    matched_rule_id     INTEGER           REFERENCES intake_rules(id) ON DELETE SET NULL,
    matched_rule_name   TEXT    NOT NULL DEFAULT '',
    -- final decision
    action              TEXT    NOT NULL DEFAULT '' CHECK(action IN ('', 'grab', 'skip', 'upgrade', 'remove')),
    status              TEXT    NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'matched', 'grabbed', 'skipped', 'removed', 'error')),
    error               TEXT    NOT NULL DEFAULT '',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at        TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_intake_events_owner    ON intake_events(owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_intake_events_pipeline ON intake_events(pipeline_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_intake_events_library  ON intake_events(matched_library_id);
