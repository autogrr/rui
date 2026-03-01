// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

type LogExclusions struct {
	ID        int       `json:"id"`
	OwnerID   int       `json:"ownerId"`
	Patterns  []string  `json:"patterns"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LogExclusionsInput struct {
	Patterns []string `json:"patterns"`
}

type LogExclusionsStore struct {
	db dbinterface.Querier
}

func NewLogExclusionsStore(db dbinterface.Querier) *LogExclusionsStore {
	return &LogExclusionsStore{db: db}
}

// Get returns log exclusions, creating defaults if none exist
func (s *LogExclusionsStore) Get(ctx context.Context) (*LogExclusions, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, patterns, created_at, updated_at
		FROM log_exclusions_view
		LIMIT 1
	`)

	var le LogExclusions
	var patternsJSON string

	err := row.Scan(&le.ID, &le.OwnerID, &patternsJSON, &le.CreatedAt, &le.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return s.createDefault(ctx)
	}
	if err != nil {
		return nil, err
	}

	// Parse JSON field
	if patternsJSON != "" && patternsJSON != "[]" {
		if err := json.Unmarshal([]byte(patternsJSON), &le.Patterns); err != nil {
			le.Patterns = []string{}
		}
	} else {
		le.Patterns = []string{}
	}

	return &le, nil
}

// Update replaces patterns
func (s *LogExclusionsStore) Update(ctx context.Context, input *LogExclusionsInput) (*LogExclusions, error) {
	if input == nil {
		return nil, errors.New("input is nil")
	}

	// Ensure we have a record (creates if none)
	existing, err := s.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Handle nil patterns as empty array
	patterns := input.Patterns
	if patterns == nil {
		patterns = []string{}
	}

	// Serialize JSON
	patternsJSON, err := json.Marshal(patterns)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern the patterns JSON string
	ids, err := dbinterface.InternStrings(ctx, tx, string(patternsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to intern patterns: %w", err)
	}

	// Update in database
	_, err = tx.ExecContext(ctx, `
		UPDATE log_exclusions
		SET patterns_id = ?
		WHERE id = ?
	`, ids[0], existing.ID)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.Get(ctx)
}

// createDefault creates empty log exclusions
func (s *LogExclusionsStore) createDefault(ctx context.Context) (*LogExclusions, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up owner_id from first user
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get owner: %w", err)
	}

	// Intern the empty JSON array literal
	ids, err := dbinterface.InternStrings(ctx, tx, "[]")
	if err != nil {
		return nil, fmt.Errorf("failed to intern patterns: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO log_exclusions (owner_id, patterns_id)
		VALUES (?, ?)
	`, ownerID, ids[0])
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &LogExclusions{
		ID:        int(id),
		OwnerID:   ownerID,
		Patterns:  []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}
