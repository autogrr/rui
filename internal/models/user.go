// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"

	"github.com/autogrr/rui/internal/dbinterface"
	"modernc.org/sqlite"
	lib "modernc.org/sqlite/lib"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUserAlreadyExists = errors.New("user already exists")

type User struct {
	ID           int     `json:"id"`
	Username     string  `json:"username"`
	PasswordHash string  `json:"-"`
	DisplayName  *string `json:"display_name,omitempty"`
	Email        *string `json:"email,omitempty"`
	IsActive     bool    `json:"is_active"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type UserStore struct {
	db dbinterface.Querier
}

func NewUserStore(db dbinterface.Querier) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, username, passwordHash string) (*User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids, err := dbinterface.InternStrings(ctx, tx, username, passwordHash)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO users (username_id, password_hash_id)
		VALUES (?, ?)
	`

	result, err := tx.ExecContext(ctx, query, ids[0], ids[1])
	if err != nil {
		var sqlErr *sqlite.Error
		if errors.As(err, &sqlErr) {
			if sqlErr.Code() == lib.SQLITE_CONSTRAINT_UNIQUE {
				return nil, ErrUserAlreadyExists
			}
		}
		return nil, err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByID(ctx, int(userID))
}

// Get returns the first user (legacy compatibility for single-user mode).
func (s *UserStore) Get(ctx context.Context) (*User, error) {
	query := `
		SELECT id, username, password_hash, display_name, email,
		       is_active, created_at, updated_at
		FROM users_view
		ORDER BY id LIMIT 1
	`

	user := &User{}
	err := s.db.QueryRowContext(ctx, query).Scan(
		&user.ID, &user.Username, &user.PasswordHash,
		&user.DisplayName, &user.Email,
		&user.IsActive, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserStore) GetByID(ctx context.Context, id int) (*User, error) {
	query := `
		SELECT id, username, password_hash, display_name, email,
		       is_active, created_at, updated_at
		FROM users_view
		WHERE id = ?
	`

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash,
		&user.DisplayName, &user.Email,
		&user.IsActive, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, password_hash, display_name, email,
		       is_active, created_at, updated_at
		FROM users_view
		WHERE username = ?
	`

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash,
		&user.DisplayName, &user.Email,
		&user.IsActive, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserStore) UpdatePassword(ctx context.Context, userID int, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ids, err := dbinterface.InternStrings(ctx, tx, passwordHash)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE users SET password_hash_id = ? WHERE id = ?`,
		ids[0], userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return tx.Commit()
}

func (s *UserStore) Exists(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
