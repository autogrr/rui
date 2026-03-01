// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
	"github.com/rs/zerolog/log"
)

const (
	defaultInitialWaitSeconds     = 15
	defaultReannounceIntervalSecs = 7
	defaultMaxAgeSeconds          = 600
	defaultMaxRetries             = 50
	minMaxRetries                 = 1
	maxMaxRetries                 = 50
)

// InstanceReannounceSettings stores per-instance tracker reannounce configuration.
type InstanceReannounceSettings struct {
	InstanceID                int       `json:"instanceId"`
	OwnerID                   int       `json:"ownerId"`
	Enabled                   bool      `json:"enabled"`
	InitialWaitSeconds        int       `json:"initialWaitSeconds"`
	ReannounceIntervalSeconds int       `json:"reannounceIntervalSeconds"`
	MaxAgeSeconds             int       `json:"maxAgeSeconds"`
	MaxRetries                int       `json:"maxRetries"`
	Aggressive                bool      `json:"aggressive"`
	MonitorAll                bool      `json:"monitorAll"`
	ExcludeCategories         bool      `json:"excludeCategories"`
	Categories                []string  `json:"categories"`
	ExcludeTags               bool      `json:"excludeTags"`
	Tags                      []string  `json:"tags"`
	ExcludeTrackers           bool      `json:"excludeTrackers"`
	Trackers                  []string  `json:"trackers"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

// InstanceReannounceStore manages persistence for InstanceReannounceSettings.
type InstanceReannounceStore struct {
	db dbinterface.Querier
}

// NewInstanceReannounceStore creates a new store.
func NewInstanceReannounceStore(db dbinterface.Querier) *InstanceReannounceStore {
	return &InstanceReannounceStore{db: db}
}

// DefaultInstanceReannounceSettings returns default values for a new instance.
func DefaultInstanceReannounceSettings(instanceID int) *InstanceReannounceSettings {
	return &InstanceReannounceSettings{
		InstanceID:                instanceID,
		Enabled:                   false,
		InitialWaitSeconds:        defaultInitialWaitSeconds,
		ReannounceIntervalSeconds: defaultReannounceIntervalSecs,
		MaxAgeSeconds:             defaultMaxAgeSeconds,
		MaxRetries:                defaultMaxRetries,
		Aggressive:                false,
		MonitorAll:                false,
		ExcludeCategories:         false,
		Categories:                []string{},
		ExcludeTags:               false,
		Tags:                      []string{},
		ExcludeTrackers:           false,
		Trackers:                  []string{},
	}
}

// Get returns settings for an instance, falling back to defaults if missing.
func (s *InstanceReannounceStore) Get(ctx context.Context, instanceID int) (*InstanceReannounceSettings, error) {
	const query = `SELECT instance_id, owner_id, enabled, initial_wait_seconds, reannounce_interval_seconds,
		max_age_seconds, max_retries, aggressive, monitor_all, categories_json, tags_json, trackers_json, updated_at,
		exclude_categories, exclude_tags, exclude_trackers
		FROM instance_reannounce_settings_view WHERE instance_id = ?`

	row := s.db.QueryRowContext(ctx, query, instanceID)
	settings, err := scanInstanceReannounceSettings(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultInstanceReannounceSettings(instanceID), nil
		}
		return nil, err
	}
	return settings, nil
}

// List returns settings for all instances that have overrides. Instances without overrides are omitted.
func (s *InstanceReannounceStore) List(ctx context.Context) ([]*InstanceReannounceSettings, error) {
	const query = `SELECT instance_id, owner_id, enabled, initial_wait_seconds, reannounce_interval_seconds,
		max_age_seconds, max_retries, aggressive, monitor_all, categories_json, tags_json, trackers_json, updated_at,
		exclude_categories, exclude_tags, exclude_trackers
		FROM instance_reannounce_settings_view`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*InstanceReannounceSettings
	for rows.Next() {
		settings, err := scanInstanceReannounceSettings(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, settings)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Upsert saves settings for an instance, creating or updating as needed.
func (s *InstanceReannounceStore) Upsert(ctx context.Context, settings *InstanceReannounceSettings) (*InstanceReannounceSettings, error) {
	if settings == nil {
		return nil, fmt.Errorf("settings cannot be nil")
	}

	coerced := sanitizeInstanceReannounceSettings(settings)
	catJSON, err := EncodeStringSliceJSON(coerced.Categories)
	if err != nil {
		return nil, err
	}
	tagJSON, err := EncodeStringSliceJSON(coerced.Tags)
	if err != nil {
		return nil, err
	}
	trackerJSON, err := EncodeStringSliceJSON(coerced.Trackers)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up owner_id from the instance
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM instances WHERE id = ?`, coerced.InstanceID).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get instance owner: %w", err)
	}

	// Intern JSON strings
	ids, err := dbinterface.InternStrings(ctx, tx, catJSON, tagJSON, trackerJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	const stmt = `INSERT INTO instance_reannounce_settings (
		instance_id, owner_id, enabled, initial_wait_seconds, reannounce_interval_seconds,
		max_age_seconds, max_retries, aggressive, monitor_all, categories_json_id, tags_json_id, trackers_json_id,
		exclude_categories, exclude_tags, exclude_trackers)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(instance_id) DO UPDATE SET
		owner_id = excluded.owner_id,
		enabled = excluded.enabled,
		initial_wait_seconds = excluded.initial_wait_seconds,
		reannounce_interval_seconds = excluded.reannounce_interval_seconds,
		max_age_seconds = excluded.max_age_seconds,
		max_retries = excluded.max_retries,
		aggressive = excluded.aggressive,
		monitor_all = excluded.monitor_all,
		categories_json_id = excluded.categories_json_id,
		tags_json_id = excluded.tags_json_id,
		trackers_json_id = excluded.trackers_json_id,
		exclude_categories = excluded.exclude_categories,
		exclude_tags = excluded.exclude_tags,
		exclude_trackers = excluded.exclude_trackers`

	_, err = tx.ExecContext(ctx, stmt,
		coerced.InstanceID,
		ownerID,
		BoolToSQLite(coerced.Enabled),
		coerced.InitialWaitSeconds,
		coerced.ReannounceIntervalSeconds,
		coerced.MaxAgeSeconds,
		coerced.MaxRetries,
		BoolToSQLite(coerced.Aggressive),
		BoolToSQLite(coerced.MonitorAll),
		ids[0],
		ids[1],
		ids[2],
		BoolToSQLite(coerced.ExcludeCategories),
		BoolToSQLite(coerced.ExcludeTags),
		BoolToSQLite(coerced.ExcludeTrackers),
	)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.Get(ctx, coerced.InstanceID)
}

func sanitizeInstanceReannounceSettings(s *InstanceReannounceSettings) *InstanceReannounceSettings {
	clone := *s
	if clone.InitialWaitSeconds <= 0 {
		clone.InitialWaitSeconds = defaultInitialWaitSeconds
	}
	if clone.ReannounceIntervalSeconds <= 0 {
		clone.ReannounceIntervalSeconds = defaultReannounceIntervalSecs
	}
	if clone.MaxAgeSeconds <= 0 {
		clone.MaxAgeSeconds = defaultMaxAgeSeconds
	}
	if clone.MaxRetries < minMaxRetries {
		log.Debug().
			Int("original", s.MaxRetries).
			Int("sanitized", minMaxRetries).
			Msg("reannounce: MaxRetries below minimum, clamping")
		clone.MaxRetries = minMaxRetries
	} else if clone.MaxRetries > maxMaxRetries {
		log.Debug().
			Int("original", s.MaxRetries).
			Int("sanitized", maxMaxRetries).
			Msg("reannounce: MaxRetries exceeded maximum, clamping")
		clone.MaxRetries = maxMaxRetries
	}
	clone.Categories = SanitizeStringSlice(clone.Categories)
	clone.Tags = SanitizeStringSlice(clone.Tags)
	clone.Trackers = SanitizeCommaSeparatedStringSlice(clone.Trackers)
	return &clone
}

func scanInstanceReannounceSettings(scanner interface {
	Scan(dest ...any) error
}) (*InstanceReannounceSettings, error) {
	var (
		instanceID           int
		ownerID              int
		enabledInt           int
		initialWait          int
		reannounceInterval   int
		maxAge               int
		maxRetries           int
		aggressiveInt        int
		monitorAllInt        int
		catJSON              sql.NullString
		tagJSON              sql.NullString
		trackerJSON          sql.NullString
		updatedAt            sql.NullTime
		excludeCategoriesInt int
		excludeTagsInt       int
		excludeTrackersInt   int
	)

	if err := scanner.Scan(
		&instanceID,
		&ownerID,
		&enabledInt,
		&initialWait,
		&reannounceInterval,
		&maxAge,
		&maxRetries,
		&aggressiveInt,
		&monitorAllInt,
		&catJSON,
		&tagJSON,
		&trackerJSON,
		&updatedAt,
		&excludeCategoriesInt,
		&excludeTagsInt,
		&excludeTrackersInt,
	); err != nil {
		return nil, err
	}

	categories, err := DecodeStringSliceJSON(catJSON)
	if err != nil {
		return nil, fmt.Errorf("decode categories: %w", err)
	}
	tags, err := DecodeStringSliceJSON(tagJSON)
	if err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	trackers, err := DecodeStringSliceJSON(trackerJSON)
	if err != nil {
		return nil, fmt.Errorf("decode trackers: %w", err)
	}

	settings := &InstanceReannounceSettings{
		InstanceID:                instanceID,
		OwnerID:                   ownerID,
		Enabled:                   enabledInt == 1,
		InitialWaitSeconds:        initialWait,
		ReannounceIntervalSeconds: reannounceInterval,
		MaxAgeSeconds:             maxAge,
		MaxRetries:                maxRetries,
		Aggressive:                aggressiveInt == 1,
		MonitorAll:                monitorAllInt == 1,
		ExcludeCategories:         excludeCategoriesInt == 1,
		Categories:                categories,
		ExcludeTags:               excludeTagsInt == 1,
		Tags:                      tags,
		ExcludeTrackers:           excludeTrackersInt == 1,
		Trackers:                  trackers,
	}

	if updatedAt.Valid {
		settings.UpdatedAt = updatedAt.Time
	}

	return sanitizeInstanceReannounceSettings(settings), nil
}
