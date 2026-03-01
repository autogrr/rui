// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

// CrossSeedBlocklistEntry represents a per-instance cross-seed blocklist item.
type CrossSeedBlocklistEntry struct {
	InstanceID int       `json:"instanceId"`
	InfoHash   string    `json:"infoHash"`
	OwnerID    int       `json:"ownerId"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type CrossSeedBlocklistStore struct {
	db dbinterface.Querier
}

func NewCrossSeedBlocklistStore(db dbinterface.Querier) *CrossSeedBlocklistStore {
	return &CrossSeedBlocklistStore{db: db}
}

func (s *CrossSeedBlocklistStore) List(ctx context.Context, instanceID int) ([]*CrossSeedBlocklistEntry, error) {
	query := `
		SELECT instance_id, infohash, owner_id, note, created_at
		FROM cross_seed_blocklist_view
	`
	args := []any{}
	if instanceID > 0 {
		query += " WHERE instance_id = ?"
		args = append(args, instanceID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*CrossSeedBlocklistEntry
	for rows.Next() {
		var entry CrossSeedBlocklistEntry
		if err := rows.Scan(&entry.InstanceID, &entry.InfoHash, &entry.OwnerID, &entry.Note, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (s *CrossSeedBlocklistStore) Get(ctx context.Context, instanceID int, infoHash string) (*CrossSeedBlocklistEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT instance_id, infohash, owner_id, note, created_at
		FROM cross_seed_blocklist_view
		WHERE instance_id = ? AND infohash = ?
	`, instanceID, normalizeInfoHash(infoHash))

	var entry CrossSeedBlocklistEntry
	if err := row.Scan(&entry.InstanceID, &entry.InfoHash, &entry.OwnerID, &entry.Note, &entry.CreatedAt); err != nil {
		return nil, err
	}

	return &entry, nil
}

func (s *CrossSeedBlocklistStore) Upsert(ctx context.Context, entry *CrossSeedBlocklistEntry) (*CrossSeedBlocklistEntry, error) {
	if entry == nil {
		return nil, errors.New("entry is nil")
	}
	if entry.InstanceID <= 0 {
		return nil, errors.New("instanceID must be positive")
	}

	normalized := normalizeInfoHash(entry.InfoHash)
	if normalized == "" {
		return nil, errors.New("infohash is required")
	}
	note := strings.TrimSpace(entry.Note)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up owner_id from the instance
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM instances WHERE id = ?`, entry.InstanceID).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get owner from instance: %w", err)
	}

	// Intern infohash and note
	stringsToIntern := []string{normalized}
	if note == "" {
		// Need to intern empty string for note
		noteID, err := dbinterface.InternEmptyString(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to intern empty note: %w", err)
		}

		ids, err := dbinterface.InternStrings(ctx, tx, normalized)
		if err != nil {
			return nil, fmt.Errorf("failed to intern strings: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cross_seed_blocklist (instance_id, infohash_id, owner_id, note_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(instance_id, infohash_id)
			DO UPDATE SET note_id = excluded.note_id
		`, entry.InstanceID, ids[0], ownerID, noteID)
		if err != nil {
			return nil, err
		}
	} else {
		stringsToIntern = append(stringsToIntern, note)

		ids, err := dbinterface.InternStrings(ctx, tx, stringsToIntern...)
		if err != nil {
			return nil, fmt.Errorf("failed to intern strings: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cross_seed_blocklist (instance_id, infohash_id, owner_id, note_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(instance_id, infohash_id)
			DO UPDATE SET note_id = excluded.note_id
		`, entry.InstanceID, ids[0], ownerID, ids[1])
		if err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.Get(ctx, entry.InstanceID, normalized)
}

func (s *CrossSeedBlocklistStore) Delete(ctx context.Context, instanceID int, infoHash string) error {
	normalized := normalizeInfoHash(infoHash)
	if normalized == "" {
		return errors.New("infohash is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up infohash_id from string_pool
	var infohashID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM string_pool WHERE value = ?`, normalized).Scan(&infohashID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return err
	}

	res, err := tx.ExecContext(ctx, `
		DELETE FROM cross_seed_blocklist
		WHERE instance_id = ? AND infohash_id = ?
	`, instanceID, infohashID)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FindBlocked returns the first blocked infohash from the provided list.
func (s *CrossSeedBlocklistStore) FindBlocked(ctx context.Context, instanceID int, hashes []string) (string, bool, error) {
	if instanceID <= 0 || len(hashes) == 0 {
		return "", false, nil
	}

	normalized := normalizeInfoHashList(hashes)
	if len(normalized) == 0 {
		return "", false, nil
	}

	placeholders := buildPlaceholders(len(normalized))
	query := fmt.Sprintf(`
		SELECT infohash
		FROM cross_seed_blocklist_view
		WHERE instance_id = ? AND infohash IN (%s)
		LIMIT 1
	`, placeholders)

	args := make([]any, 0, len(normalized)+1)
	args = append(args, instanceID)
	for _, h := range normalized {
		args = append(args, h)
	}

	var infohash string
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&infohash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}

	return infohash, true, nil
}

func normalizeInfoHash(value string) string {
	return normalizeLowerTrim(value)
}

func normalizeInfoHashList(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeInfoHash(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func buildPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	var sb strings.Builder
	for i := range count {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteByte('?')
	}
	return sb.String()
}
