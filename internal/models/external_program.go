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

var ErrExternalProgramNotFound = errors.New("external program not found")

// PathMapping represents a path mapping from remote to local
type PathMapping struct {
	From string `json:"from"` // Remote path prefix
	To   string `json:"to"`   // Local path prefix
}

// ExternalProgram represents a configured external program that can be executed from the torrent context menu
type ExternalProgram struct {
	ID           int           `json:"id"`
	OwnerID      int           `json:"owner_id"`
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	ArgsTemplate string        `json:"args_template"`
	Enabled      bool          `json:"enabled"`
	UseTerminal  bool          `json:"use_terminal"`
	PathMappings []PathMapping `json:"path_mappings"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// ExternalProgramCreate represents the data needed to create a new external program
type ExternalProgramCreate struct {
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	ArgsTemplate string        `json:"args_template"`
	Enabled      bool          `json:"enabled"`
	UseTerminal  bool          `json:"use_terminal"`
	PathMappings []PathMapping `json:"path_mappings"`
}

// ExternalProgramUpdate represents the data needed to update an external program
type ExternalProgramUpdate struct {
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	ArgsTemplate string        `json:"args_template"`
	Enabled      bool          `json:"enabled"`
	UseTerminal  bool          `json:"use_terminal"`
	PathMappings []PathMapping `json:"path_mappings"`
}

// ExternalProgramExecute represents a request to execute an external program with torrent data
type ExternalProgramExecute struct {
	ProgramID  int      `json:"program_id"`
	InstanceID int      `json:"instance_id"`
	Hashes     []string `json:"hashes"`
}

type ExternalProgramStore struct {
	db dbinterface.Querier
}

func NewExternalProgramStore(db dbinterface.Querier) *ExternalProgramStore {
	return &ExternalProgramStore{db: db}
}

// scanExternalProgram scans a row from external_programs_view into an ExternalProgram
func scanExternalProgram(scanner interface{ Scan(...any) error }) (*ExternalProgram, error) {
	program := &ExternalProgram{}
	var enabled, useTerminal int
	var pathMappingsJSON string
	if err := scanner.Scan(
		&program.ID,
		&program.OwnerID,
		&program.Name,
		&program.Path,
		&program.ArgsTemplate,
		&enabled,
		&useTerminal,
		&pathMappingsJSON,
		&program.CreatedAt,
		&program.UpdatedAt,
	); err != nil {
		return nil, err
	}
	program.Enabled = enabled == 1
	program.UseTerminal = useTerminal == 1

	if pathMappingsJSON != "" && pathMappingsJSON != "[]" {
		if err := json.Unmarshal([]byte(pathMappingsJSON), &program.PathMappings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal path mappings: %w", err)
		}
	}
	return program, nil
}

const externalProgramViewCols = `id, owner_id, name, path, args_template, enabled, use_terminal, path_mappings, created_at, updated_at`

func (s *ExternalProgramStore) List(ctx context.Context) ([]*ExternalProgram, error) {
	query := `SELECT ` + externalProgramViewCols + ` FROM external_programs_view ORDER BY name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query external programs: %w", err)
	}
	defer rows.Close()

	var programs []*ExternalProgram
	for rows.Next() {
		program, err := scanExternalProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan external program: %w", err)
		}
		programs = append(programs, program)
	}

	return programs, rows.Err()
}

func (s *ExternalProgramStore) ListEnabled(ctx context.Context) ([]*ExternalProgram, error) {
	query := `SELECT ` + externalProgramViewCols + ` FROM external_programs_view WHERE enabled = 1 ORDER BY name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled external programs: %w", err)
	}
	defer rows.Close()

	var programs []*ExternalProgram
	for rows.Next() {
		program, err := scanExternalProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan external program: %w", err)
		}
		programs = append(programs, program)
	}

	return programs, rows.Err()
}

func (s *ExternalProgramStore) GetByID(ctx context.Context, id int) (*ExternalProgram, error) {
	query := `SELECT ` + externalProgramViewCols + ` FROM external_programs_view WHERE id = ?`

	program, err := scanExternalProgram(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExternalProgramNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get external program: %w", err)
	}

	return program, nil
}

func (s *ExternalProgramStore) Create(ctx context.Context, create *ExternalProgramCreate) (*ExternalProgram, error) {
	pathMappingsJSON, err := json.Marshal(create.PathMappings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal path mappings: %w", err)
	}
	pathMappingsStr := string(pathMappingsJSON)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern all string fields
	ids, err := dbinterface.InternStrings(ctx, tx, create.Name, create.Path, create.ArgsTemplate, pathMappingsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	// Get first user as owner
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get owner: %w", err)
	}

	var programID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO external_programs (owner_id, name_id, path_id, args_template_id, enabled, use_terminal, path_mappings_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, ownerID, ids[0], ids[1], ids[2], BoolToSQLite(create.Enabled), BoolToSQLite(create.UseTerminal), ids[3]).Scan(&programID)
	if err != nil {
		return nil, fmt.Errorf("failed to create external program: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return s.GetByID(ctx, int(programID))
}

func (s *ExternalProgramStore) Update(ctx context.Context, id int, update *ExternalProgramUpdate) (*ExternalProgram, error) {
	pathMappingsJSON, err := json.Marshal(update.PathMappings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal path mappings: %w", err)
	}
	pathMappingsStr := string(pathMappingsJSON)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern all string fields
	ids, err := dbinterface.InternStrings(ctx, tx, update.Name, update.Path, update.ArgsTemplate, pathMappingsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE external_programs
		SET name_id = ?, path_id = ?, args_template_id = ?, enabled = ?, use_terminal = ?, path_mappings_id = ?
		WHERE id = ?
	`, ids[0], ids[1], ids[2], BoolToSQLite(update.Enabled), BoolToSQLite(update.UseTerminal), ids[3], id)
	if err != nil {
		return nil, fmt.Errorf("failed to update external program: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, ErrExternalProgramNotFound
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return s.GetByID(ctx, id)
}

func (s *ExternalProgramStore) Delete(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM external_programs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete external program: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrExternalProgramNotFound
	}

	return nil
}
