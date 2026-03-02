-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 086: Library quality tracking per title.
--
-- Adds two new columns to library_titles so the UI can show quality variants
-- (resolution + source + codec + HDR tags such as "1080p WEBDL HEVC") and an
-- episode count for TV content derived from torrent-client scanning.
--
-- Both columns are populated by ScanTorrentClients; arr-sourced rows leave them
-- at their default values (0 and '') since arr already provides richer data.

ALTER TABLE library_titles ADD COLUMN episode_count INTEGER NOT NULL DEFAULT 0;

-- Comma-separated sorted list of unique quality labels observed for this title,
-- e.g. "1080p WEBDL HEVC, 4K REMUX HEVC DV HDR10, 720p WEBRIP AVC".
-- Empty for arr-sourced rows where quality is tracked per-file by the arr app.
ALTER TABLE library_titles ADD COLUMN qualities TEXT NOT NULL DEFAULT '';
