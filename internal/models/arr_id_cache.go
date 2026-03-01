// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

// ExternalIDs represents the external IDs resolved from ARR instances
type ExternalIDs struct {
	IMDbID   string `json:"imdb_id,omitempty"`
	TMDbID   int    `json:"tmdb_id,omitempty"`
	TVDbID   int    `json:"tvdb_id,omitempty"`
	TVMazeID int    `json:"tvmaze_id,omitempty"`
}

// IsEmpty returns true if no IDs are set
func (e *ExternalIDs) IsEmpty() bool {
	return e.IMDbID == "" && e.TMDbID == 0 && e.TVDbID == 0 && e.TVMazeID == 0
}

// ArrIDCacheEntry represents a cached ID lookup result
type ArrIDCacheEntry struct {
	ID            int64       `json:"id"`
	TitleHash     string      `json:"title_hash"`
	ContentType   string      `json:"content_type"`
	ArrInstanceID *int        `json:"arr_instance_id,omitempty"`
	ExternalIDs   ExternalIDs `json:"external_ids"`
	IsNegative    bool        `json:"is_negative"`
	CachedAt      time.Time   `json:"cached_at"`
	ExpiresAt     time.Time   `json:"expires_at"`
}

// ArrIDCacheStore manages the ARR ID cache in the database
type ArrIDCacheStore struct {
	db dbinterface.Querier
}

// NewArrIDCacheStore creates a new ArrIDCacheStore
func NewArrIDCacheStore(db dbinterface.Querier) *ArrIDCacheStore {
	return &ArrIDCacheStore{db: db}
}

// ComputeTitleHash computes a SHA256 hash of the normalized title for cache lookup
func ComputeTitleHash(title string) string {
	normalized := normalizeLowerTrim(title)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached ID entry if it exists and hasn't expired
func (s *ArrIDCacheStore) Get(ctx context.Context, titleHash, contentType string) (*ArrIDCacheEntry, error) {
	const query = `
		SELECT id, title_hash, content_type, arr_instance_id, imdb_id, tmdb_id, tvdb_id, tvmaze_id, is_negative, cached_at, expires_at
		FROM arr_id_cache_view
		WHERE title_hash = ? AND content_type = ? AND expires_at > CURRENT_TIMESTAMP
	`

	var entry ArrIDCacheEntry
	var imdbID *string
	var tmdbID, tvdbID, tvmazeID *int

	err := s.db.QueryRowContext(ctx, query, titleHash, contentType).Scan(
		&entry.ID,
		&entry.TitleHash,
		&entry.ContentType,
		&entry.ArrInstanceID,
		&imdbID,
		&tmdbID,
		&tvdbID,
		&tvmazeID,
		&entry.IsNegative,
		&entry.CachedAt,
		&entry.ExpiresAt,
	)
	if err != nil {
		return nil, err // Returns sql.ErrNoRows if not found
	}

	// Map nullable fields to ExternalIDs
	if imdbID != nil {
		entry.ExternalIDs.IMDbID = *imdbID
	}
	if tmdbID != nil {
		entry.ExternalIDs.TMDbID = *tmdbID
	}
	if tvdbID != nil {
		entry.ExternalIDs.TVDbID = *tvdbID
	}
	if tvmazeID != nil {
		entry.ExternalIDs.TVMazeID = *tvmazeID
	}

	return &entry, nil
}

// Set creates or updates a cache entry (upsert)
func (s *ArrIDCacheStore) Set(ctx context.Context, titleHash, contentType string, arrInstanceID *int, ids *ExternalIDs, isNegative bool, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)

	// Prepare nullable values for non-interned columns
	var tmdbID, tvdbID, tvmazeID *int

	if ids != nil {
		if ids.TMDbID > 0 {
			tmdbID = &ids.TMDbID
		}
		if ids.TVDbID > 0 {
			tvdbID = &ids.TVDbID
		}
		if ids.TVMazeID > 0 {
			tvmazeID = &ids.TVMazeID
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern required strings: title_hash and content_type
	requiredIDs, err := dbinterface.InternStrings(ctx, tx, titleHash, contentType)
	if err != nil {
		return fmt.Errorf("failed to intern strings: %w", err)
	}
	titleHashID, contentTypeID := requiredIDs[0], requiredIDs[1]

	// Intern optional imdb_id
	var imdbIDSid sql.NullInt64
	if ids != nil && ids.IMDbID != "" {
		nullableIDs, err := dbinterface.InternStringNullable(ctx, tx, &ids.IMDbID)
		if err != nil {
			return fmt.Errorf("failed to intern imdb_id: %w", err)
		}
		imdbIDSid = nullableIDs[0]
	}

	const query = `
		INSERT INTO arr_id_cache (title_hash_id, content_type_id, arr_instance_id, imdb_id_sid, tmdb_id, tvdb_id, tvmaze_id, is_negative, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(title_hash_id, content_type_id) DO UPDATE SET
			arr_instance_id = excluded.arr_instance_id,
			imdb_id_sid = excluded.imdb_id_sid,
			tmdb_id = excluded.tmdb_id,
			tvdb_id = excluded.tvdb_id,
			tvmaze_id = excluded.tvmaze_id,
			is_negative = excluded.is_negative,
			cached_at = CURRENT_TIMESTAMP,
			expires_at = excluded.expires_at
	`

	_, err = tx.ExecContext(ctx, query, titleHashID, contentTypeID, arrInstanceID, imdbIDSid, tmdbID, tvdbID, tvmazeID, isNegative, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set arr id cache entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete removes a specific cache entry
func (s *ArrIDCacheStore) Delete(ctx context.Context, titleHash, contentType string) error {
	const query = `
		DELETE FROM arr_id_cache WHERE id IN (
			SELECT id FROM arr_id_cache_view WHERE title_hash = ? AND content_type = ?
		)
	`

	_, err := s.db.ExecContext(ctx, query, titleHash, contentType)
	if err != nil {
		return fmt.Errorf("failed to delete arr id cache entry: %w", err)
	}

	return nil
}

// DeleteByArrInstance removes all cache entries for a specific ARR instance
func (s *ArrIDCacheStore) DeleteByArrInstance(ctx context.Context, arrInstanceID int) error {
	const query = `DELETE FROM arr_id_cache WHERE arr_instance_id = ?`

	_, err := s.db.ExecContext(ctx, query, arrInstanceID)
	if err != nil {
		return fmt.Errorf("failed to delete arr id cache entries for instance: %w", err)
	}

	return nil
}

// CleanupExpired removes all expired cache entries
func (s *ArrIDCacheStore) CleanupExpired(ctx context.Context) (int64, error) {
	const query = `DELETE FROM arr_id_cache WHERE expires_at <= CURRENT_TIMESTAMP`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired arr id cache entries: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// Count returns the total number of cache entries
func (s *ArrIDCacheStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM arr_id_cache_view").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count arr id cache entries: %w", err)
	}
	return count, nil
}

// CountValid returns the number of non-expired cache entries
func (s *ArrIDCacheStore) CountValid(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM arr_id_cache_view WHERE expires_at > CURRENT_TIMESTAMP").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count valid arr id cache entries: %w", err)
	}
	return count, nil
}
