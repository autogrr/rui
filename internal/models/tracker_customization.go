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

type TrackerCustomization struct {
	ID              int       `json:"id"`
	OwnerID         int       `json:"ownerId"`
	DisplayName     string    `json:"displayName"`
	Domains         []string  `json:"domains"`
	IncludedInStats []string  `json:"includedInStats,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type TrackerCustomizationStore struct {
	db dbinterface.Querier
}

func NewTrackerCustomizationStore(db dbinterface.Querier) *TrackerCustomizationStore {
	return &TrackerCustomizationStore{db: db}
}

func (s *TrackerCustomizationStore) List(ctx context.Context) ([]*TrackerCustomization, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, display_name, domains, included_in_stats, created_at, updated_at
		FROM tracker_customizations_view
		ORDER BY display_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customizations []*TrackerCustomization
	for rows.Next() {
		var c TrackerCustomization
		var domainsStr string
		var includedStr sql.NullString

		if err := rows.Scan(&c.ID, &c.OwnerID, &c.DisplayName, &domainsStr, &includedStr, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}

		c.Domains = splitDomains(domainsStr)
		if includedStr.Valid {
			c.IncludedInStats = splitDomains(includedStr.String)
		}
		customizations = append(customizations, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return customizations, nil
}

func (s *TrackerCustomizationStore) Get(ctx context.Context, id int) (*TrackerCustomization, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, display_name, domains, included_in_stats, created_at, updated_at
		FROM tracker_customizations_view
		WHERE id = ?
	`, id)

	var c TrackerCustomization
	var domainsStr string
	var includedStr sql.NullString

	if err := row.Scan(&c.ID, &c.OwnerID, &c.DisplayName, &domainsStr, &includedStr, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}

	c.Domains = splitDomains(domainsStr)
	if includedStr.Valid {
		c.IncludedInStats = splitDomains(includedStr.String)
	}
	return &c, nil
}

func (s *TrackerCustomizationStore) Create(ctx context.Context, c *TrackerCustomization) (*TrackerCustomization, error) {
	if c == nil {
		return nil, errors.New("customization is nil")
	}

	domainsStr := joinDomains(c.Domains)
	includedStr := joinDomains(c.IncludedInStats)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up owner_id - use the first user since tracker customizations are global
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get owner: %w", err)
	}

	// Intern required strings
	ids, err := dbinterface.InternStrings(ctx, tx, c.DisplayName, domainsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	// Intern optional included_in_stats
	var includedID sql.NullInt64
	if includedStr != "" {
		nullableIDs, err := dbinterface.InternStringNullable(ctx, tx, &includedStr)
		if err != nil {
			return nil, fmt.Errorf("failed to intern included_in_stats: %w", err)
		}
		includedID = nullableIDs[0]
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO tracker_customizations (owner_id, display_name_id, domains_id, included_in_stats_id)
		VALUES (?, ?, ?, ?)
	`, ownerID, ids[0], ids[1], includedID)
	if err != nil {
		return nil, err
	}

	insertedID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.Get(ctx, int(insertedID))
}

func (s *TrackerCustomizationStore) Update(ctx context.Context, c *TrackerCustomization) (*TrackerCustomization, error) {
	if c == nil {
		return nil, errors.New("customization is nil")
	}

	domainsStr := joinDomains(c.Domains)
	includedStr := joinDomains(c.IncludedInStats)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern required strings
	ids, err := dbinterface.InternStrings(ctx, tx, c.DisplayName, domainsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	// Intern optional included_in_stats
	var includedID sql.NullInt64
	if includedStr != "" {
		nullableIDs, err := dbinterface.InternStringNullable(ctx, tx, &includedStr)
		if err != nil {
			return nil, fmt.Errorf("failed to intern included_in_stats: %w", err)
		}
		includedID = nullableIDs[0]
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tracker_customizations
		SET display_name_id = ?, domains_id = ?, included_in_stats_id = ?
		WHERE id = ?
	`, ids[0], ids[1], includedID, c.ID)
	if err != nil {
		return nil, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, sql.ErrNoRows
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.Get(ctx, c.ID)
}

func (s *TrackerCustomizationStore) Delete(ctx context.Context, id int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tracker_customizations WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func splitDomains(domainsStr string) []string {
	if domainsStr == "" {
		return nil
	}

	parts := strings.Split(domainsStr, ",")
	var domains []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			domains = append(domains, trimmed)
		}
	}
	return domains
}

func joinDomains(domains []string) string {
	var cleaned []string
	seen := make(map[string]struct{})
	for _, d := range domains {
		trimmed := strings.TrimSpace(d)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, ",")
}

// ResolveTrackerDisplayName resolves a tracker domain to its display name using customizations.
// Fallback chain: customization display name → indexerName → domain.
//
// The domain should be extracted from the tracker announce URL (lowercase, no port).
// indexerName is typically from the cross-seed request and serves as a fallback.
func ResolveTrackerDisplayName(domain string, indexerName string, customizations []*TrackerCustomization) string {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Look for a matching customization
	for _, c := range customizations {
		for _, d := range c.Domains {
			if strings.ToLower(d) == domain {
				return c.DisplayName
			}
		}
	}

	// Fall back to indexer name if provided
	if indexerName != "" {
		return indexerName
	}

	// Fall back to domain itself
	if domain != "" && domain != "unknown" {
		return domain
	}

	return "Unknown"
}
