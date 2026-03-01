// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package filesmanager

import (
	"context"
	"path/filepath"
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/database"
)

func setupFilesManagerDB(t *testing.T) (*database.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Create a test user (required by instances.owner_id FK)
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO string_pool (value) VALUES ('test-user'), ('test-hash')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (username_id, password_hash_id) VALUES (
		(SELECT id FROM string_pool WHERE value = 'test-user'),
		(SELECT id FROM string_pool WHERE value = 'test-hash'))`)
	require.NoError(t, err)

	// Seed instance row to satisfy foreign key constraints for cache writes.
	// The instances table uses fully interned columns (all TEXT via string_pool).
	_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO string_pool (value)
		VALUES ('instance-name'), ('instance-host'), ('instance-username'), ('enc')`)
	require.NoError(t, err)

	// The empty string ('') is already seeded by migrations; use it for hardlink defaults.
	_, err = db.ExecContext(ctx, `INSERT INTO instances (id, owner_id, name_id, host_id, username_id,
		password_encrypted_id, hardlink_base_dir_id, hardlink_dir_preset_id)
		VALUES (1,
			(SELECT id FROM users LIMIT 1),
			(SELECT id FROM string_pool WHERE value = 'instance-name'),
			(SELECT id FROM string_pool WHERE value = 'instance-host'),
			(SELECT id FROM string_pool WHERE value = 'instance-username'),
			(SELECT id FROM string_pool WHERE value = 'enc'),
			(SELECT id FROM string_pool WHERE value = ''),
			(SELECT id FROM string_pool WHERE value = ''))`)
	require.NoError(t, err)

	return db, ctx
}

func TestCacheFilesAndGetCachedFiles(t *testing.T) {
	t.Parallel()

	db, ctx := setupFilesManagerDB(t)
	svc := NewService(db)

	files := []qbt.TorrentFile{
		{
			Index: qbt.Ptr(0),
			Name: qbt.Ptr("example.mkv"),
			Size: qbt.Ptr(int64(1 << 20)),
			Progress: qbt.Ptr(float64(0.5)),
			Priority: qbt.Ptr(qbt.FilePriority(1)),
			PieceRange: []int{0, 1},
		},
	}

	require.NoError(t, svc.CacheFiles(ctx, 1, "hash", files))

	cached, err := svc.GetCachedFiles(ctx, 1, "hash")
	require.NoError(t, err)
	require.NotNil(t, cached, "cache should be available")
	require.Len(t, cached, 1)
	require.Equal(t, "example.mkv", qbt.Deref(cached[0].Name))
}

func TestCacheFilesBatch_MaintainsHashAlignment(t *testing.T) {
	t.Parallel()

	db, ctx := setupFilesManagerDB(t)
	svc := NewService(db)

	hashes := []string{"hash-a", "hash-b", "hash-c"}
	names := map[string]string{
		"hash-a": "alpha.mkv",
		"hash-b": "bravo.mkv",
		"hash-c": "charlie.mkv",
	}

	for attempt := 0; attempt < 3; attempt++ {
		// Reset cache tables to isolate each attempt.
		_, err := db.ExecContext(ctx, "DELETE FROM torrent_files_cache; DELETE FROM torrent_files_sync;")
		require.NoError(t, err)

		files := make(map[string][]qbt.TorrentFile, len(hashes))
		for _, hash := range hashes {
			files[hash] = []qbt.TorrentFile{
				{
					Index: qbt.Ptr(0),
					Name: qbt.Ptr(names[hash]),
					Size: qbt.Ptr(int64(attempt + 1)),
				},
			}
		}

		require.NoError(t, svc.CacheFilesBatch(ctx, 1, files))

		for _, hash := range hashes {
			cached, err := svc.GetCachedFiles(ctx, 1, hash)
			require.NoError(t, err)
			require.Len(t, cached, 1, "attempt %d hash %s", attempt, hash)
			require.Equalf(t, names[hash], qbt.Deref(cached[0].Name), "attempt %d hash %s", attempt, hash)
		}
	}
}
