-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 084: Add ratings, genres, and source tracking to library_titles.
--
-- Ratings come from *arr metadata providers (Radarr/Sonarr lookup endpoints).
-- genres is a JSON-encoded string array.
-- source indicates whether the title was synced from an *arr instance or
-- discovered by scanning a torrent client directly.
-- info_hash allows deduplicating torrent-client-sourced entries.

ALTER TABLE library_titles ADD COLUMN imdb_rating            REAL    NOT NULL DEFAULT 0;
ALTER TABLE library_titles ADD COLUMN tmdb_rating            REAL    NOT NULL DEFAULT 0;
ALTER TABLE library_titles ADD COLUMN metacritic_rating      REAL    NOT NULL DEFAULT 0;
ALTER TABLE library_titles ADD COLUMN rotten_tomatoes_rating REAL    NOT NULL DEFAULT 0;
ALTER TABLE library_titles ADD COLUMN audience_rating        REAL    NOT NULL DEFAULT 0;
ALTER TABLE library_titles ADD COLUMN genres                 TEXT    NOT NULL DEFAULT '';
ALTER TABLE library_titles ADD COLUMN source                 TEXT    NOT NULL DEFAULT 'arr'
    CHECK(source IN ('arr', 'torrent_client', 'manual'));
ALTER TABLE library_titles ADD COLUMN info_hash              TEXT    NOT NULL DEFAULT '';

-- Partial unique index so that torrent-client-sourced entries can be
-- de-duplicated by info_hash without affecting arr-sourced rows.
CREATE UNIQUE INDEX IF NOT EXISTS library_titles_hash_idx
    ON library_titles(owner_id, info_hash)
    WHERE info_hash != '';
