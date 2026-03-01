// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package dirscan

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/database"
	"github.com/autogrr/rui/internal/models"
)

func setupDirScanServiceTestDB(t *testing.T) *database.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "dirscan-service.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO string_pool (value) VALUES ('test-user'), ('test-hash')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (username_id, password_hash_id) VALUES (
		(SELECT id FROM string_pool WHERE value = 'test-user'),
		(SELECT id FROM string_pool WHERE value = 'test-hash'))`)
	require.NoError(t, err)

	return db
}

func TestService_CancelScan_QueuedRunBumpsLastScanAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := setupDirScanServiceTestDB(t)

	instanceStore, err := models.NewInstanceStore(db, []byte("01234567890123456789012345678901"))
	require.NoError(t, err)

	localFS := true
	instance, err := instanceStore.Create(ctx, 1, "Test", "http://localhost:8080", "user", "pass", nil, nil, false, &localFS)
	require.NoError(t, err)

	store := models.NewDirScanStore(db)
	dir, err := store.CreateDirectory(ctx, &models.DirScanDirectory{
		Path:                "/data/media",
		Enabled:             true,
		TargetInstanceID:    instance.ID,
		ScanIntervalMinutes: 60,
	})
	require.NoError(t, err)
	require.Nil(t, dir.LastScanAt)

	runID, err := store.CreateRunIfNoActive(ctx, dir.ID, "scheduled")
	require.NoError(t, err)
	require.Positive(t, runID)

	svc := &Service{store: store}
	require.NoError(t, svc.CancelScan(ctx, dir.ID))

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, models.DirScanRunStatusCanceled, run.Status)

	updatedDir, err := store.GetDirectory(ctx, dir.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedDir.LastScanAt, "queued cancel should bump last_scan_at to avoid immediate re-queue")
	require.WithinDuration(t, time.Now(), *updatedDir.LastScanAt, 5*time.Second)
	require.False(t, svc.isDueForScan(updatedDir), "directory should not be immediately due after queued cancel")

	active, err := store.HasActiveRun(ctx, dir.ID)
	require.NoError(t, err)
	require.False(t, active)
}
