// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"context"
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/pkg/stringutils"
)

func TestAnalyzeTorrentForSearchAsync_RejectsUnrelatedLargestFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instance := &models.Instance{ID: 1, Name: "Test"}

	movieTorrent := qbt.Torrent{
		Hash:     qbt.Ptr("deadbeef"),
		Name:     qbt.Ptr("Example.Movie.2001.1080p.BluRay.x264-GROUP"),
		Progress: qbt.Ptr(float64(1.0)),
		Size:     qbt.Ptr(int64(10 << 30)),
	}

	files := map[string][]qbt.TorrentFile{
		qbt.Deref(movieTorrent.Hash): {
			{
				Name: qbt.Ptr("Different.Series.S03.1080p.WEB-DL.DDP5.1.H.264-GROUP/Different.Series.S03E02.1080p.WEB-DL.DDP5.1.H.264-GROUP.mkv"),
				Size: qbt.Ptr(int64(8 << 30)),
			},
		},
	}

	service := &Service{
		instanceStore:    &fakeInstanceStore{instances: map[int]*models.Instance{instance.ID: instance}},
		syncManager:      newFakeSyncManager(instance, []qbt.Torrent{movieTorrent}, files),
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	result, err := service.AnalyzeTorrentForSearchAsync(ctx, instance.ID, qbt.Deref(movieTorrent.Hash), false)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "movie", result.TorrentInfo.ContentType, "should fall back to torrent name when largest file is unrelated")
	require.Equal(t, "movie", result.TorrentInfo.SearchType)
	require.Equal(t, []int{2000}, result.TorrentInfo.SearchCategories)
}

func TestAnalyzeTorrentForSearchAsync_UsesLargestFileWhenTitlesAlign(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instance := &models.Instance{ID: 1, Name: "Test"}

	tvTorrent := qbt.Torrent{
		Hash:     qbt.Ptr("abcd1234"),
		Name:     qbt.Ptr("MadeUp.Show"),
		Progress: qbt.Ptr(float64(1.0)),
		Size:     qbt.Ptr(int64(5 << 30)),
	}

	files := map[string][]qbt.TorrentFile{
		qbt.Deref(tvTorrent.Hash): {
			{
				Name: qbt.Ptr("MadeUp.Show.S01E02.1080p.WEB-DL.DDP5.1.H.264-GROUP.mkv"),
				Size: qbt.Ptr(int64(3 << 30)),
			},
		},
	}

	service := &Service{
		instanceStore:    &fakeInstanceStore{instances: map[int]*models.Instance{instance.ID: instance}},
		syncManager:      newFakeSyncManager(instance, []qbt.Torrent{tvTorrent}, files),
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	result, err := service.AnalyzeTorrentForSearchAsync(ctx, instance.ID, qbt.Deref(tvTorrent.Hash), false)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "tv", result.TorrentInfo.ContentType, "aligned largest file should refine content detection")
	require.Equal(t, "tvsearch", result.TorrentInfo.SearchType)
	require.Equal(t, []int{5000}, result.TorrentInfo.SearchCategories)
}

func TestAnalyzeTorrentForSearchAsync_TrustFileEpisodeMarkers_Miniseries(t *testing.T) {
	// Torka.aldrig.tarar.utan.handskar is a Swedish miniseries
	// Torrent name has year but no episode markers → parsed as movie
	// File name has E01 → parsed as TV episode
	// Should trust the file since titles match and file has explicit episode marker
	t.Parallel()

	ctx := context.Background()
	instance := &models.Instance{ID: 1, Name: "Test"}

	torrent := qbt.Torrent{
		Hash:     qbt.Ptr("torka123"),
		Name:     qbt.Ptr("Torka.aldrig.tarar.utan.handskar.2012.720p.BluRay.x264-HANDJOB"),
		Progress: qbt.Ptr(float64(1.0)),
		Size:     qbt.Ptr(int64(8 << 30)),
	}

	files := map[string][]qbt.TorrentFile{
		qbt.Deref(torrent.Hash): {
			{
				Name: qbt.Ptr("Torka.aldrig.tarar.utan.handskar.2012.720p.BluRay.x264-HANDJOB/Torka.aldrig.tarar.utan.handskar.E01.2012.720p.BluRay.x264-HANDJOB.mkv"),
				Size: qbt.Ptr(int64(4 << 30)),
			},
			{
				Name: qbt.Ptr("Torka.aldrig.tarar.utan.handskar.2012.720p.BluRay.x264-HANDJOB/Torka.aldrig.tarar.utan.handskar.E02.2012.720p.BluRay.x264-HANDJOB.mkv"),
				Size: qbt.Ptr(int64(4 << 30)),
			},
		},
	}

	service := &Service{
		instanceStore:    &fakeInstanceStore{instances: map[int]*models.Instance{instance.ID: instance}},
		syncManager:      newFakeSyncManager(instance, []qbt.Torrent{torrent}, files),
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	result, err := service.AnalyzeTorrentForSearchAsync(ctx, instance.ID, qbt.Deref(torrent.Hash), false)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "tv", result.TorrentInfo.ContentType, "should detect as TV when file has episode markers")
	require.Equal(t, "tvsearch", result.TorrentInfo.SearchType)
	require.Equal(t, []int{5000}, result.TorrentInfo.SearchCategories)
}

func TestAnalyzeTorrentForSearchAsync_TrustFileEpisodeMarkers_Anime(t *testing.T) {
	// Anime often uses " - 01 " style episode numbering without S/E prefixes
	// Torrent name has no episode markers → parsed as movie
	// File name has " - 01 " → parsed as TV episode
	// Should trust the file since titles match and file has explicit episode marker
	t.Parallel()

	ctx := context.Background()
	instance := &models.Instance{ID: 1, Name: "Test"}

	torrent := qbt.Torrent{
		Hash:     qbt.Ptr("takopii123"),
		Name:     qbt.Ptr("[SubsPlease] Takopii no Genzai (1080p)"),
		Progress: qbt.Ptr(float64(1.0)),
		Size:     qbt.Ptr(int64(9 << 30)),
	}

	files := map[string][]qbt.TorrentFile{
		qbt.Deref(torrent.Hash): {
			{
				Name: qbt.Ptr("[SubsPlease] Takopii no Genzai (1080p)/[SubsPlease] Takopii no Genzai - 01 (1080p) [2480DBD9].mkv"),
				Size: qbt.Ptr(int64(2 << 30)),
			},
			{
				Name: qbt.Ptr("[SubsPlease] Takopii no Genzai (1080p)/[SubsPlease] Takopii no Genzai - 02 (1080p) [C84AB672].mkv"),
				Size: qbt.Ptr(int64(1500 << 20)),
			},
			{
				Name: qbt.Ptr("[SubsPlease] Takopii no Genzai (1080p)/[SubsPlease] Takopii no Genzai - 03 (1080p) [A2386109].mkv"),
				Size: qbt.Ptr(int64(1500 << 20)),
			},
		},
	}

	service := &Service{
		instanceStore:    &fakeInstanceStore{instances: map[int]*models.Instance{instance.ID: instance}},
		syncManager:      newFakeSyncManager(instance, []qbt.Torrent{torrent}, files),
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	result, err := service.AnalyzeTorrentForSearchAsync(ctx, instance.ID, qbt.Deref(torrent.Hash), false)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "tv", result.TorrentInfo.ContentType, "should detect anime as TV when file has episode markers")
	require.Equal(t, "tvsearch", result.TorrentInfo.SearchType)
	require.Equal(t, []int{5000}, result.TorrentInfo.SearchCategories)
}
