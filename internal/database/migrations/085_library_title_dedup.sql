-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 085: Library title-based dedup for torrent-client scanning.
--
-- The original approach of one library_title per info_hash is wrong for a
-- library: if you have 500 episodes of "Breaking Bad", you want ONE library
-- entry, not 500.  This migration adds:
--
-- 1. A torrent_count column to track how many torrents back a title.
-- 2. A partial unique index on (owner_id, sort_title, content_type) for
--    torrent_client-sourced entries so we can upsert by title.

ALTER TABLE library_titles ADD COLUMN torrent_count INTEGER NOT NULL DEFAULT 0;

-- Partial unique index: only covers torrent_client-sourced rows.
-- Arr-sourced rows use the existing UNIQUE(owner_id, arr_instance_id, arr_item_id).
CREATE UNIQUE INDEX IF NOT EXISTS library_titles_title_idx
    ON library_titles(owner_id, sort_title, content_type)
    WHERE source = 'torrent_client';
