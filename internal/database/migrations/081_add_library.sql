-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 081: Native library management.
-- library_titles  – one row per tracked title (movie, TV show, etc.).
-- library_seasons – per-season stats for TV titles.
-- library_rules   – expr-based rules evaluated against incoming release matches.

-- ─── library_titles ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS library_titles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_type    TEXT    NOT NULL CHECK(content_type IN ('movie', 'tv', 'anime', 'music', 'book')),
    title           TEXT    NOT NULL,
    sort_title      TEXT    NOT NULL DEFAULT '',
    year            INTEGER,
    imdb_id         TEXT    NOT NULL DEFAULT '',
    tmdb_id         INTEGER NOT NULL DEFAULT 0,
    tvdb_id         INTEGER NOT NULL DEFAULT 0,
    tvmaze_id       INTEGER NOT NULL DEFAULT 0,
    -- arr source that populated this row; NULL = added manually
    arr_instance_id INTEGER REFERENCES arr_instances(id) ON DELETE SET NULL,
    arr_item_id     INTEGER, -- series/movie ID within the arr instance
    overview        TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT '',
    -- filesystem path as reported by the arr, empty when not known
    path            TEXT    NOT NULL DEFAULT '',
    has_file        BOOLEAN NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(owner_id, arr_instance_id, arr_item_id)
);

CREATE INDEX IF NOT EXISTS idx_library_titles_owner      ON library_titles(owner_id);
CREATE INDEX IF NOT EXISTS idx_library_titles_type       ON library_titles(owner_id, content_type);
CREATE INDEX IF NOT EXISTS idx_library_titles_imdb       ON library_titles(imdb_id) WHERE imdb_id != '';
CREATE INDEX IF NOT EXISTS idx_library_titles_tmdb       ON library_titles(tmdb_id) WHERE tmdb_id != 0;
CREATE INDEX IF NOT EXISTS idx_library_titles_tvdb       ON library_titles(tvdb_id) WHERE tvdb_id != 0;
CREATE INDEX IF NOT EXISTS idx_library_titles_arr_instance ON library_titles(arr_instance_id);

CREATE TRIGGER IF NOT EXISTS trg_library_titles_updated
AFTER UPDATE ON library_titles BEGIN
    UPDATE library_titles SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- ─── library_seasons ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS library_seasons (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    title_id       INTEGER NOT NULL REFERENCES library_titles(id) ON DELETE CASCADE,
    season_number  INTEGER NOT NULL,
    episode_count  INTEGER NOT NULL DEFAULT 0,
    has_files      BOOLEAN NOT NULL DEFAULT 0,
    UNIQUE(title_id, season_number)
);

CREATE INDEX IF NOT EXISTS idx_library_seasons_title ON library_seasons(title_id);

-- ─── library_rules ──────────────────────────────────────────────────────────
-- Rules are evaluated in sort_order (ascending) against each incoming release
-- that the library matcher identifies as a candidate for a tracked title.
-- expr_filter is a boolean expr-lang expression.  Env is LibraryMatchContext.
-- action determines what rui does when the rule fires (grab, skip, upgrade, remove).

CREATE TABLE IF NOT EXISTS library_rules (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT    NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT 1,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    -- comma-separated subset of: movie,tv,anime,music,book
    content_types TEXT    NOT NULL DEFAULT 'movie,tv',
    -- expr-lang boolean expression; empty = always match
    expr_filter   TEXT    NOT NULL DEFAULT '',
    -- action to take when the rule fires
    action        TEXT    NOT NULL DEFAULT 'grab' CHECK(action IN ('grab', 'skip', 'upgrade', 'remove')),
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_library_rules_owner  ON library_rules(owner_id, enabled, sort_order);

CREATE TRIGGER IF NOT EXISTS trg_library_rules_updated
AFTER UPDATE ON library_rules BEGIN
    UPDATE library_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
