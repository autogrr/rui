// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/autogrr/rui/internal/dbinterface"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupInstanceErrorTestDB(t *testing.T) (*mockQuerier, *InstanceErrorStore) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	_, err = sqlDB.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE string_pool (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL UNIQUE
		);
		CREATE INDEX idx_string_pool_value ON string_pool(value);
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username_id INTEGER REFERENCES string_pool(id)
		);
		INSERT INTO users (id) VALUES (1);
		CREATE TABLE instances (
			id INTEGER PRIMARY KEY,
			owner_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id) ON DELETE CASCADE,
			name_id INTEGER NOT NULL,
			host_id INTEGER NOT NULL,
			username_id INTEGER NOT NULL,
			password_encrypted_id INTEGER,
			basic_username_id INTEGER,
			basic_password_encrypted_id INTEGER,
			tls_skip_verify BOOLEAN NOT NULL DEFAULT 0,
			FOREIGN KEY (name_id) REFERENCES string_pool(id),
			FOREIGN KEY (host_id) REFERENCES string_pool(id),
			FOREIGN KEY (username_id) REFERENCES string_pool(id),
			FOREIGN KEY (password_encrypted_id) REFERENCES string_pool(id),
			FOREIGN KEY (basic_username_id) REFERENCES string_pool(id),
			FOREIGN KEY (basic_password_encrypted_id) REFERENCES string_pool(id)
		);
		CREATE TABLE instance_errors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id) ON DELETE CASCADE,
			instance_id INTEGER NOT NULL,
			error_type_id INTEGER NOT NULL,
			error_message_id INTEGER NOT NULL,
			occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(instance_id) REFERENCES instances(id) ON DELETE CASCADE,
			FOREIGN KEY(error_type_id) REFERENCES string_pool(id),
			FOREIGN KEY(error_message_id) REFERENCES string_pool(id)
		);
		CREATE TRIGGER IF NOT EXISTS cleanup_old_instance_errors
		AFTER INSERT ON instance_errors BEGIN
		    DELETE FROM instance_errors
		    WHERE instance_id = NEW.instance_id
		      AND id NOT IN (
		          SELECT id FROM instance_errors
		          WHERE instance_id = NEW.instance_id
		          ORDER BY occurred_at DESC LIMIT 5);
		END;
		CREATE VIEW instance_errors_view AS
		SELECT 
		    ie.id,
		    ie.owner_id,
		    ie.instance_id,
		    sp_type.value AS error_type,
		    sp_msg.value AS error_message,
		    ie.occurred_at
		FROM instance_errors ie
		JOIN string_pool sp_type ON ie.error_type_id = sp_type.id
		JOIN string_pool sp_msg ON ie.error_message_id = sp_msg.id;
	`)
	require.NoError(t, err)

	// Wrap with mock that implements Querier
	db := newMockQuerier(sqlDB)

	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	return db, NewInstanceErrorStore(db)
}

func countInstanceErrors(t *testing.T, db dbinterface.Querier, instanceID int) int {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM instance_errors_view WHERE instance_id = ?", instanceID).Scan(&count))
	return count
}

func TestInstanceErrorStore_RecordError_SkipsMissingInstance(t *testing.T) {
	ctx := context.Background()
	db, store := setupInstanceErrorTestDB(t)

	// Ensure no rows exist before recording
	require.Equal(t, 0, countInstanceErrors(t, db, 99))

	err := store.RecordError(ctx, 99, errors.New("connection refused"))
	require.NoError(t, err)

	require.Equal(t, 0, countInstanceErrors(t, db, 99), "errors should not be recorded for missing instances")
}

func TestInstanceErrorStore_RecordError_DeduplicatesWithinOneMinute(t *testing.T) {
	ctx := context.Background()
	db, store := setupInstanceErrorTestDB(t)

	// Insert instance - intern strings first
	var nameID, hostID, usernameID int64
	err := db.QueryRowContext(ctx, "INSERT INTO string_pool (value) VALUES (?) ON CONFLICT (value) DO UPDATE SET value = value RETURNING id", "test").Scan(&nameID)
	require.NoError(t, err)
	err = db.QueryRowContext(ctx, "INSERT INTO string_pool (value) VALUES (?) ON CONFLICT (value) DO UPDATE SET value = value RETURNING id", "http://localhost").Scan(&hostID)
	require.NoError(t, err)
	err = db.QueryRowContext(ctx, "INSERT INTO string_pool (value) VALUES (?) ON CONFLICT (value) DO UPDATE SET value = value RETURNING id", "user").Scan(&usernameID)
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO instances (id, owner_id, name_id, host_id, username_id, password_encrypted_id) VALUES (?, 1, ?, ?, ?, ?)", 1, nameID, hostID, usernameID, nameID)
	require.NoError(t, err)

	firstErr := errors.New("connection refused")
	require.NoError(t, store.RecordError(ctx, 1, firstErr))
	require.Equal(t, 1, countInstanceErrors(t, db, 1))

	// Duplicate error within a minute should be ignored
	require.NoError(t, store.RecordError(ctx, 1, firstErr))
	require.Equal(t, 1, countInstanceErrors(t, db, 1))

	// Different error should be recorded
	require.NoError(t, store.RecordError(ctx, 1, errors.New("authentication failed")))
	require.Equal(t, 2, countInstanceErrors(t, db, 1))
}
