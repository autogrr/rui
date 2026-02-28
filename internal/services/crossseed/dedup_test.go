// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"context"
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/pkg/stringutils"
)

func TestService_deduplicateSourceTorrents_PreservesEpisodesAlongsideSeasonPacks(t *testing.T) {
	svc := &Service{
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	seasonPack := qbt.Torrent{
		Hash:    qbt.Ptr("hash-pack"),
		Name:    qbt.Ptr("Generic.Show.2025.S01.1080p.WEB-DL.DDP5.1.H.264-GEN"),
		AddedOn: qbt.Ptr(int64(2)),
	}
	episode := qbt.Torrent{
		Hash:    qbt.Ptr("hash-episode"),
		Name:    qbt.Ptr("Generic.Show.2025.S01E01.1080p.WEB-DL.DDP5.1.H.264-GEN"),
		AddedOn: qbt.Ptr(int64(1)),
	}

	deduped, duplicates := svc.deduplicateSourceTorrents(context.Background(), 1, []qbt.Torrent{seasonPack, episode})
	require.Len(t, deduped, 2, "season pack should not eliminate individual episodes during deduplication")
	require.Empty(t, duplicates)

	kept := make(map[string]struct{})
	for _, torrent := range deduped {
		kept[qbt.Deref(torrent.Hash)] = struct{}{}
	}

	require.Contains(t, kept, qbt.Deref(seasonPack.Hash))
	require.Contains(t, kept, qbt.Deref(episode.Hash))

	duplicateEpisodes := []qbt.Torrent{
		{
			Hash:    qbt.Ptr("hash-newer-episode"),
			Name: episode.Name,
			AddedOn: qbt.Ptr(int64(10)),
		},
		{
			Hash:    qbt.Ptr("hash-older-episode"),
			Name: episode.Name,
			AddedOn: qbt.Ptr(int64(5)),
		},
	}

	dedupedEpisodes, duplicateMap := svc.deduplicateSourceTorrents(context.Background(), 1, duplicateEpisodes)
	require.Len(t, dedupedEpisodes, 1, "exact episode duplicates should still collapse to the oldest torrent")
	require.Equal(t, "hash-older-episode", qbt.Deref(dedupedEpisodes[0].Hash))
	require.Contains(t, duplicateMap, "hash-older-episode")
	require.ElementsMatch(t, []string{"hash-newer-episode"}, duplicateMap["hash-older-episode"])
}

func TestService_deduplicateSourceTorrents_PrefersRootFolders(t *testing.T) {
	files := map[string][]qbt.TorrentFile{
		"hash-root": {
			{Name: qbt.Ptr("Show.S01/Show.S01E01.mkv"), Size: qbt.Ptr(int64(1 << 20))},
		},
		"hash-flat": {
			{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1 << 20))},
		},
	}

	svc := &Service{
		releaseCache:     NewReleaseCache(),
		syncManager:      &fakeSyncManager{files: files},
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	torrents := []qbt.Torrent{
		{Hash: qbt.Ptr("hash-flat"), Name: qbt.Ptr("Generic.Show.2025.S01E01.1080p.WEB-DL"), AddedOn: qbt.Ptr(int64(1))},
		{Hash: qbt.Ptr("hash-root"), Name: qbt.Ptr("Generic.Show.2025.S01E01.1080p.WEB-DL"), AddedOn: qbt.Ptr(int64(2))},
	}

	deduped, _ := svc.deduplicateSourceTorrents(context.Background(), 1, torrents)
	require.Len(t, deduped, 1)
	require.Equal(t, "hash-root", qbt.Deref(deduped[0].Hash), "prefer torrent with root folder")
}
