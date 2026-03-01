// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

var (
	ErrIntakePipelineNotFound = errors.New("intake pipeline not found")
	ErrIntakeRuleNotFound     = errors.New("intake rule not found")
)

// IntakeAction mirrors LibraryAction for the intake layer.
type IntakeAction = LibraryAction

// IntakeStatus is the processing state of an intake event.
type IntakeStatus string

const (
	IntakeStatusPending IntakeStatus = "pending"
	IntakeStatusMatched IntakeStatus = "matched"
	IntakeStatusGrabbed IntakeStatus = "grabbed"
	IntakeStatusSkipped IntakeStatus = "skipped"
	IntakeStatusRemoved IntakeStatus = "removed"
	IntakeStatusError   IntakeStatus = "error"
)

// IntakePipeline is a named, ordered set of rules applied to incoming releases.
type IntakePipeline struct {
	ID           int       `json:"id"`
	OwnerID      int       `json:"owner_id"`
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	SortOrder    int       `json:"sort_order"`
	ContentTypes []string  `json:"content_types"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Rules is populated on demand.
	Rules []IntakeRule `json:"rules,omitempty"`
}

// IntakeRule is an expr-lang rule within an intake pipeline.
type IntakeRule struct {
	ID           int           `json:"id"`
	PipelineID   int           `json:"pipeline_id"`
	Name         string        `json:"name"`
	Enabled      bool          `json:"enabled"`
	SortOrder    int           `json:"sort_order"`
	ExprFilter   string        `json:"expr_filter"`
	Action       LibraryAction `json:"action"`
	ActionParams string        `json:"action_params"` // raw JSON
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// IntakeEvent is an immutable record of one release processed through intake.
type IntakeEvent struct {
	ID               int          `json:"id"`
	OwnerID          int          `json:"owner_id"`
	PipelineID       *int         `json:"pipeline_id,omitempty"`
	ReleaseName      string       `json:"release_name"`
	ContentType      string       `json:"content_type"`
	ParsedTitle      string       `json:"parsed_title"`
	ParsedYear       int          `json:"parsed_year,omitempty"`
	MatchedLibraryID *int         `json:"matched_library_id,omitempty"`
	MatchedTitle     string       `json:"matched_title,omitempty"`
	ArrInstanceID    *int         `json:"arr_instance_id,omitempty"`
	IMDbID           string       `json:"imdb_id,omitempty"`
	TMDbID           int          `json:"tmdb_id,omitempty"`
	TVDbID           int          `json:"tvdb_id,omitempty"`
	MatchedRuleID    *int         `json:"matched_rule_id,omitempty"`
	MatchedRuleName  string       `json:"matched_rule_name,omitempty"`
	Action           string       `json:"action,omitempty"`
	Status           IntakeStatus `json:"status"`
	Error            string       `json:"error,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	ProcessedAt      *time.Time   `json:"processed_at,omitempty"`
}

// ─── IntakePipelineStore ─────────────────────────────────────────────────────

// IntakePipelineStore manages intake pipelines and rules in SQLite.
type IntakePipelineStore struct {
	db dbinterface.Querier
}

// NewIntakePipelineStore creates a new IntakePipelineStore.
func NewIntakePipelineStore(db dbinterface.Querier) *IntakePipelineStore {
	return &IntakePipelineStore{db: db}
}

// CreatePipeline inserts a new intake pipeline.
func (s *IntakePipelineStore) CreatePipeline(ctx context.Context, p *IntakePipeline) (*IntakePipeline, error) {
	const q = `
INSERT INTO intake_pipelines (owner_id, name, enabled, sort_order, content_types, description)
VALUES (?,?,?,?,?,?)
RETURNING id, owner_id, name, enabled, sort_order, content_types, description, created_at, updated_at`
	return scanPipeline(s.db.QueryRowContext(ctx, q,
		p.OwnerID, p.Name, p.Enabled, p.SortOrder,
		joinContentTypes(p.ContentTypes), p.Description,
	))
}

// GetPipeline returns a pipeline by ID.
func (s *IntakePipelineStore) GetPipeline(ctx context.Context, id int) (*IntakePipeline, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, description, created_at, updated_at
               FROM intake_pipelines WHERE id = ?`
	p, err := scanPipeline(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakePipelineNotFound
	}
	return p, err
}

// ListPipelines returns all pipelines for the owner.
func (s *IntakePipelineStore) ListPipelines(ctx context.Context, ownerID int) ([]*IntakePipeline, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, description, created_at, updated_at
               FROM intake_pipelines WHERE owner_id = ? ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPipelines(rows)
}

// ListEnabledPipelines returns enabled pipelines sorted by sort_order.
func (s *IntakePipelineStore) ListEnabledPipelines(ctx context.Context, ownerID int) ([]*IntakePipeline, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, description, created_at, updated_at
               FROM intake_pipelines WHERE owner_id = ? AND enabled = 1 ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPipelines(rows)
}

// UpdatePipeline replaces mutable pipeline fields.
func (s *IntakePipelineStore) UpdatePipeline(ctx context.Context, p *IntakePipeline) (*IntakePipeline, error) {
	const q = `
UPDATE intake_pipelines SET name=?, enabled=?, sort_order=?, content_types=?, description=?
WHERE id = ? AND owner_id = ?
RETURNING id, owner_id, name, enabled, sort_order, content_types, description, created_at, updated_at`
	updated, err := scanPipeline(s.db.QueryRowContext(ctx, q,
		p.Name, p.Enabled, p.SortOrder, joinContentTypes(p.ContentTypes),
		p.Description, p.ID, p.OwnerID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakePipelineNotFound
	}
	return updated, err
}

// DeletePipeline removes a pipeline by ID (and cascades to its rules).
func (s *IntakePipelineStore) DeletePipeline(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM intake_pipelines WHERE id = ?`, id)
	return err
}

// ─── intake rules ────────────────────────────────────────────────────────────

// CreateRule inserts a new intake rule.
func (s *IntakePipelineStore) CreateRule(ctx context.Context, r *IntakeRule) (*IntakeRule, error) {
	const q = `
INSERT INTO intake_rules (pipeline_id, name, enabled, sort_order, expr_filter, action, action_params)
VALUES (?,?,?,?,?,?,?)
RETURNING id, pipeline_id, name, enabled, sort_order, expr_filter, action, action_params, created_at, updated_at`
	return scanIntakeRule(s.db.QueryRowContext(ctx, q,
		r.PipelineID, r.Name, r.Enabled, r.SortOrder,
		r.ExprFilter, string(r.Action), r.ActionParams,
	))
}

// GetRule returns an intake rule by ID.
func (s *IntakePipelineStore) GetRule(ctx context.Context, id int) (*IntakeRule, error) {
	const q = `SELECT id, pipeline_id, name, enabled, sort_order, expr_filter, action, action_params, created_at, updated_at
               FROM intake_rules WHERE id = ?`
	rule, err := scanIntakeRule(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeRuleNotFound
	}
	return rule, err
}

// ListRules returns all rules for a pipeline sorted by sort_order.
func (s *IntakePipelineStore) ListRules(ctx context.Context, pipelineID int) ([]*IntakeRule, error) {
	const q = `SELECT id, pipeline_id, name, enabled, sort_order, expr_filter, action, action_params, created_at, updated_at
               FROM intake_rules WHERE pipeline_id = ? ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIntakeRules(rows)
}

// ListEnabledRules returns enabled rules for a pipeline sorted by sort_order.
func (s *IntakePipelineStore) ListEnabledRules(ctx context.Context, pipelineID int) ([]*IntakeRule, error) {
	const q = `SELECT id, pipeline_id, name, enabled, sort_order, expr_filter, action, action_params, created_at, updated_at
               FROM intake_rules WHERE pipeline_id = ? AND enabled = 1 ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIntakeRules(rows)
}

// UpdateRule replaces mutable rule fields.
func (s *IntakePipelineStore) UpdateRule(ctx context.Context, r *IntakeRule) (*IntakeRule, error) {
	const q = `
UPDATE intake_rules SET name=?, enabled=?, sort_order=?, expr_filter=?, action=?, action_params=?
WHERE id = ? AND pipeline_id = ?
RETURNING id, pipeline_id, name, enabled, sort_order, expr_filter, action, action_params, created_at, updated_at`
	updated, err := scanIntakeRule(s.db.QueryRowContext(ctx, q,
		r.Name, r.Enabled, r.SortOrder, r.ExprFilter,
		string(r.Action), r.ActionParams, r.ID, r.PipelineID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeRuleNotFound
	}
	return updated, err
}

// DeleteRule removes a rule by ID.
func (s *IntakePipelineStore) DeleteRule(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM intake_rules WHERE id = ?`, id)
	return err
}

// ─── intake events ───────────────────────────────────────────────────────────

// IntakeEventStore manages the append-only event log.
type IntakeEventStore struct {
	db dbinterface.Querier
}

// NewIntakeEventStore creates a new IntakeEventStore.
func NewIntakeEventStore(db dbinterface.Querier) *IntakeEventStore {
	return &IntakeEventStore{db: db}
}

// Insert appends a new intake event and returns it with its generated ID.
func (s *IntakeEventStore) Insert(ctx context.Context, e *IntakeEvent) (*IntakeEvent, error) {
	const q = `
INSERT INTO intake_events
    (owner_id, pipeline_id, release_name, content_type, parsed_title, parsed_year,
     matched_library_id, matched_title, arr_instance_id, imdb_id, tmdb_id, tvdb_id,
     matched_rule_id, matched_rule_name, action, status, error, processed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
RETURNING id, owner_id, pipeline_id, release_name, content_type, parsed_title,
          parsed_year, matched_library_id, matched_title, arr_instance_id,
          imdb_id, tmdb_id, tvdb_id, matched_rule_id, matched_rule_name,
          action, status, error, created_at, processed_at`
	return scanEvent(s.db.QueryRowContext(ctx, q,
		e.OwnerID, e.PipelineID, e.ReleaseName, e.ContentType,
		e.ParsedTitle, e.ParsedYear, e.MatchedLibraryID, e.MatchedTitle,
		e.ArrInstanceID, e.IMDbID, e.TMDbID, e.TVDbID,
		e.MatchedRuleID, e.MatchedRuleName, e.Action,
		string(e.Status), e.Error, e.ProcessedAt,
	))
}

// ListRecent returns the most recent events for the owner.
func (s *IntakeEventStore) ListRecent(ctx context.Context, ownerID, limit int) ([]*IntakeEvent, error) {
	const q = `
SELECT id, owner_id, pipeline_id, release_name, content_type, parsed_title,
       parsed_year, matched_library_id, matched_title, arr_instance_id,
       imdb_id, tmdb_id, tvdb_id, matched_rule_id, matched_rule_name,
       action, status, error, created_at, processed_at
FROM intake_events WHERE owner_id = ? ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ─── scan helpers ────────────────────────────────────────────────────────────

func scanPipeline(row *sql.Row) (*IntakePipeline, error) {
	var p IntakePipeline
	var contentTypes string
	err := row.Scan(
		&p.ID, &p.OwnerID, &p.Name, &p.Enabled, &p.SortOrder,
		&contentTypes, &p.Description, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.ContentTypes = splitContentTypes(contentTypes)
	return &p, nil
}

func scanPipelines(rows *sql.Rows) ([]*IntakePipeline, error) {
	var out []*IntakePipeline
	for rows.Next() {
		var p IntakePipeline
		var contentTypes string
		if err := rows.Scan(
			&p.ID, &p.OwnerID, &p.Name, &p.Enabled, &p.SortOrder,
			&contentTypes, &p.Description, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.ContentTypes = splitContentTypes(contentTypes)
		out = append(out, &p)
	}
	return out, rows.Err()
}

func scanIntakeRule(row *sql.Row) (*IntakeRule, error) {
	var r IntakeRule
	var action string
	err := row.Scan(
		&r.ID, &r.PipelineID, &r.Name, &r.Enabled, &r.SortOrder,
		&r.ExprFilter, &action, &r.ActionParams, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.Action = LibraryAction(action)
	return &r, nil
}

func scanIntakeRules(rows *sql.Rows) ([]*IntakeRule, error) {
	var out []*IntakeRule
	for rows.Next() {
		var r IntakeRule
		var action string
		if err := rows.Scan(
			&r.ID, &r.PipelineID, &r.Name, &r.Enabled, &r.SortOrder,
			&r.ExprFilter, &action, &r.ActionParams, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.Action = LibraryAction(action)
		out = append(out, &r)
	}
	return out, rows.Err()
}

func scanEvent(row *sql.Row) (*IntakeEvent, error) {
	var e IntakeEvent
	var status string
	err := row.Scan(
		&e.ID, &e.OwnerID, &e.PipelineID, &e.ReleaseName, &e.ContentType,
		&e.ParsedTitle, &e.ParsedYear, &e.MatchedLibraryID, &e.MatchedTitle,
		&e.ArrInstanceID, &e.IMDbID, &e.TMDbID, &e.TVDbID,
		&e.MatchedRuleID, &e.MatchedRuleName, &e.Action,
		&status, &e.Error, &e.CreatedAt, &e.ProcessedAt,
	)
	if err != nil {
		return nil, err
	}
	e.Status = IntakeStatus(status)
	return &e, nil
}

func scanEvents(rows *sql.Rows) ([]*IntakeEvent, error) {
	var out []*IntakeEvent
	for rows.Next() {
		var e IntakeEvent
		var status string
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.PipelineID, &e.ReleaseName, &e.ContentType,
			&e.ParsedTitle, &e.ParsedYear, &e.MatchedLibraryID, &e.MatchedTitle,
			&e.ArrInstanceID, &e.IMDbID, &e.TMDbID, &e.TVDbID,
			&e.MatchedRuleID, &e.MatchedRuleName, &e.Action,
			&status, &e.Error, &e.CreatedAt, &e.ProcessedAt,
		); err != nil {
			return nil, err
		}
		e.Status = IntakeStatus(status)
		out = append(out, &e)
	}
	return out, rows.Err()
}
