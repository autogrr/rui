// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

// TorznabTorrentCacheEntry represents a cached torrent payload downloaded from an indexer.
type TorznabTorrentCacheEntry struct {
	IndexerID   int
	CacheKey    string
	GUID        string
	DownloadURL string
	InfoHash    string
	Title       string
	SizeBytes   int64
	TorrentData []byte
}

// TorznabTorrentCacheStore manages cached Torznab torrent files.
type TorznabTorrentCacheStore struct {
	db dbinterface.Querier
}

// NewTorznabTorrentCacheStore constructs a new cache store.
func NewTorznabTorrentCacheStore(db dbinterface.Querier) *TorznabTorrentCacheStore {
	return &TorznabTorrentCacheStore{db: db}
}

// Fetch returns cached torrent data when available and not expired.
// maxAge <= 0 disables expiration checks.
func (s *TorznabTorrentCacheStore) Fetch(ctx context.Context, indexerID int, cacheKey string, maxAge time.Duration) ([]byte, bool, error) {
	if indexerID <= 0 || cacheKey == "" {
		return nil, false, fmt.Errorf("invalid cache lookup parameters")
	}

	const query = `
		SELECT id, torrent_data, cached_at
		FROM torznab_torrent_cache_view
		WHERE indexer_id = ? AND cache_key = ?
	`

	var (
		id       int64
		data     []byte
		cachedAt time.Time
	)

	err := s.db.QueryRowContext(ctx, query, indexerID, cacheKey).Scan(&id, &data, &cachedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fetch torrent cache: %w", err)
	}

	if maxAge > 0 && time.Since(cachedAt) > maxAge {
		// Expired entry; remove asynchronously
		go s.deleteEntry(context.Background(), id)
		return nil, false, nil
	}

	// Update last_used timestamp, ignoring failures so we don't block serving cached data
	go s.touchEntry(context.Background(), id)

	return data, true, nil
}

// Store inserts or updates a cached torrent payload.
func (s *TorznabTorrentCacheStore) Store(ctx context.Context, entry *TorznabTorrentCacheEntry) error {
	if entry == nil {
		return fmt.Errorf("entry cannot be nil")
	}
	if entry.IndexerID <= 0 {
		return fmt.Errorf("indexer id must be positive")
	}
	if entry.CacheKey == "" {
		return fmt.Errorf("cache key required")
	}
	if len(entry.TorrentData) == 0 {
		return fmt.Errorf("torrent data required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store torrent cache: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Intern cache_key (required, non-empty)
	requiredIDs, err := dbinterface.InternStrings(ctx, tx, entry.CacheKey)
	if err != nil {
		return fmt.Errorf("store torrent cache: intern cache_key: %w", err)
	}
	cacheKeyID := requiredIDs[0]

	// Intern optional strings: guid, download_url, info_hash, title
	guid := &entry.GUID
	downloadURL := &entry.DownloadURL
	infoHash := &entry.InfoHash
	title := &entry.Title
	nullableIDs, err := dbinterface.InternStringNullable(ctx, tx, guid, downloadURL, infoHash, title)
	if err != nil {
		return fmt.Errorf("store torrent cache: intern optional strings: %w", err)
	}

	const query = `
		INSERT INTO torznab_torrent_cache (
			indexer_id, cache_key_id, guid_id, download_url_id, info_hash_id, title_id, size_bytes, torrent_data, cached_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(indexer_id, cache_key_id) DO UPDATE SET
			guid_id = excluded.guid_id,
			download_url_id = excluded.download_url_id,
			info_hash_id = COALESCE(excluded.info_hash_id, torznab_torrent_cache.info_hash_id),
			title_id = excluded.title_id,
			size_bytes = excluded.size_bytes,
			torrent_data = excluded.torrent_data,
			cached_at = CURRENT_TIMESTAMP,
			last_used_at = CURRENT_TIMESTAMP
	`

	if _, err := tx.ExecContext(ctx, query,
		entry.IndexerID,
		cacheKeyID,
		nullableIDs[0], // guid_id
		nullableIDs[1], // download_url_id
		nullableIDs[2], // info_hash_id
		nullableIDs[3], // title_id
		entry.SizeBytes,
		entry.TorrentData,
	); err != nil {
		return fmt.Errorf("store torrent cache entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store torrent cache: commit: %w", err)
	}

	return nil
}

// Cleanup removes entries older than the provided age, returning the number of deleted rows.
func (s *TorznabTorrentCacheStore) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, nil
	}

	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM torznab_torrent_cache WHERE last_used_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup torrent cache: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return rows, nil
}

func (s *TorznabTorrentCacheStore) touchEntry(ctx context.Context, id int64) {
	_, _ = s.db.ExecContext(ctx,
		"UPDATE torznab_torrent_cache SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?",
		id,
	)
}

func (s *TorznabTorrentCacheStore) deleteEntry(ctx context.Context, id int64) {
	_, _ = s.db.ExecContext(ctx, "DELETE FROM torznab_torrent_cache WHERE id = ?", id)
}
