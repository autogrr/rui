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
)

var ErrClientAPIKeyNotFound = errors.New("client api key not found")

type ClientAPIKey struct {
	ID         int        `json:"id"`
	OwnerID    int        `json:"ownerId"`
	KeyHash    string     `json:"-"`
	ClientName string     `json:"clientName"`
	InstanceID int        `json:"instanceId"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type ClientAPIKeyStore struct {
	db dbinterface.Querier
}

func NewClientAPIKeyStore(db dbinterface.Querier) *ClientAPIKeyStore {
	return &ClientAPIKeyStore{db: db}
}

func (s *ClientAPIKeyStore) Create(ctx context.Context, clientName string, instanceID int) (string, *ClientAPIKey, error) {
	// Generate new API key
	rawKey, err := GenerateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the key for storage
	keyHash := HashAPIKey(rawKey)

	// Use a transaction to atomically intern the string and insert the API key
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Intern both key_hash and client_name
	ids, err := dbinterface.InternStrings(ctx, tx, keyHash, clientName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to intern strings: %w", err)
	}

	// Get owner from instance
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM instances WHERE id = ?`, instanceID).Scan(&ownerID); err != nil {
		return "", nil, fmt.Errorf("failed to get instance owner: %w", err)
	}

	// Insert the client API key
	clientAPIKey := &ClientAPIKey{}
	var createdAt, lastUsedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		INSERT INTO client_api_keys (owner_id, key_hash_id, client_name_id, instance_id) 
		VALUES (?, ?, ?, ?)
		RETURNING id, created_at, last_used_at
	`, ownerID, ids[0], ids[1], instanceID).Scan(
		&clientAPIKey.ID,
		&createdAt,
		&lastUsedAt,
	)

	if err != nil {
		return "", nil, err
	}

	if err = tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	clientAPIKey.OwnerID = ownerID
	clientAPIKey.KeyHash = keyHash
	clientAPIKey.ClientName = clientName
	clientAPIKey.InstanceID = instanceID
	clientAPIKey.CreatedAt = createdAt.Time
	if lastUsedAt.Valid {
		clientAPIKey.LastUsedAt = &lastUsedAt.Time
	}

	// Return both the raw key (to show user once) and the model
	return rawKey, clientAPIKey, nil
}

func (s *ClientAPIKeyStore) GetAll(ctx context.Context) ([]*ClientAPIKey, error) {
	query := `
		SELECT id, owner_id, key_hash, client_name, instance_id, created_at, last_used_at 
		FROM client_api_keys_view 
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*ClientAPIKey
	for rows.Next() {
		key := &ClientAPIKey{}
		err := rows.Scan(
			&key.ID,
			&key.OwnerID,
			&key.KeyHash,
			&key.ClientName,
			&key.InstanceID,
			&key.CreatedAt,
			&key.LastUsedAt,
		)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return keys, nil
}

func (s *ClientAPIKeyStore) GetByKeyHash(ctx context.Context, keyHash string) (*ClientAPIKey, error) {
	query := `
		SELECT id, owner_id, key_hash, client_name, instance_id, created_at, last_used_at 
		FROM client_api_keys_view 
		WHERE key_hash = ?
	`

	key := &ClientAPIKey{}
	err := s.db.QueryRowContext(ctx, query, keyHash).Scan(
		&key.ID,
		&key.OwnerID,
		&key.KeyHash,
		&key.ClientName,
		&key.InstanceID,
		&key.CreatedAt,
		&key.LastUsedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClientAPIKeyNotFound
	}

	if err != nil {
		return nil, err
	}

	return key, nil
}

func (s *ClientAPIKeyStore) ValidateKey(ctx context.Context, rawKey string) (*ClientAPIKey, error) {
	keyHash := HashAPIKey(rawKey)
	return s.GetByKeyHash(ctx, keyHash)
}

func (s *ClientAPIKeyStore) UpdateLastUsed(ctx context.Context, keyID int) error {
	query := `UPDATE client_api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, keyID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrClientAPIKeyNotFound
	}

	return nil
}

func (s *ClientAPIKeyStore) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM client_api_keys WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrClientAPIKeyNotFound
	}

	return nil
}

func (s *ClientAPIKeyStore) DeleteByInstanceID(ctx context.Context, instanceID int) error {
	query := `DELETE FROM client_api_keys WHERE instance_id = ?`
	_, err := s.db.ExecContext(ctx, query, instanceID)
	return err
}
