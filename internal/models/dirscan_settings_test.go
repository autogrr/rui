// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/models"
)

func TestDirScanStore_SettingsRoundTrip_MaxSearcheeAgeDays(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := setupDirScanTestDB(t)
	store := models.NewDirScanStore(db)
	ownerID := createTestUser(t, db)

	updated, err := store.UpdateSettings(ctx, ownerID, &models.DirScanSettings{
		Enabled:                      true,
		MatchMode:                    models.MatchModeStrict,
		SizeTolerancePercent:         5,
		MinPieceRatio:                98,
		MaxSearcheesPerRun:           25,
		MaxSearcheeAgeDays:           7,
		AllowPartial:                 false,
		SkipPieceBoundarySafetyCheck: true,
		StartPaused:                  true,
		Category:                     "",
		Tags:                         []string{"dirscan"},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 7, updated.MaxSearcheeAgeDays)

	reloaded, err := store.GetSettings(ctx, ownerID)
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	require.Equal(t, 7, reloaded.MaxSearcheeAgeDays)
	require.Equal(t, []string{"dirscan"}, reloaded.Tags)
}

func TestDirScanStore_UpdateSettings_RejectsNegativeMaxSearcheeAgeDays(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := setupDirScanTestDB(t)
	store := models.NewDirScanStore(db)
	ownerID := createTestUser(t, db)

	_, err := store.UpdateSettings(ctx, ownerID, &models.DirScanSettings{
		MaxSearcheeAgeDays: -1,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "maxSearcheeAgeDays must be >= 0")
}
