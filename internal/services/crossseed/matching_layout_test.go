// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/moistari/rls"
	"github.com/stretchr/testify/require"

	internalqb "github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/pkg/releases"
	"github.com/autogrr/rui/pkg/stringutils"
)

func TestGetMatchType_EnforcesLayoutCompatibility(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}
	sourceRelease := rls.Release{Title: "Example", Year: 2024}
	candidateRelease := rls.Release{Title: "Example", Year: 2024}

	sourceFiles := []qbt.TorrentFile{{Name: qbt.Ptr("Example.2024.1080p.mkv"), Size: qbt.Ptr(int64(4 << 30))}}
	archiveFiles := []qbt.TorrentFile{{Name: qbt.Ptr("Example.part01.rar"), Size: qbt.Ptr(int64(2 << 30))}, {Name: qbt.Ptr("Example.part02.r00"), Size: qbt.Ptr(int64(2 << 30))}}

	match := svc.getMatchType(&sourceRelease, &candidateRelease, sourceFiles, archiveFiles)
	require.Empty(t, match, "mkv torrent should not match rar-only candidate")

	archiveMatch := svc.getMatchType(&sourceRelease, &candidateRelease, archiveFiles, archiveFiles)
	require.NotEmpty(t, archiveMatch, "identical archive layouts should match")

	fileMatch := svc.getMatchType(&sourceRelease, &candidateRelease, sourceFiles, sourceFiles)
	require.Equal(t, "exact", fileMatch, "identical file layouts should be exact matches")
}

func TestFindBestCandidateMatch_PrefersLayoutCompatibleTorrent(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		syncManager: &candidateSelectionSyncManager{
			files: map[string][]qbt.TorrentFile{
				"rar": {{Name: qbt.Ptr("Example.part01.rar"), Size: qbt.Ptr(int64(2 << 30))}, {Name: qbt.Ptr("Example.part02.r00"), Size: qbt.Ptr(int64(2 << 30))}},
				"mkv": {{Name: qbt.Ptr("Example.2024.1080p.mkv"), Size: qbt.Ptr(int64(4 << 30))}},
			},
		},
	}

	sourceRelease := rls.Release{Title: "Example", Year: 2024}
	sourceFiles := []qbt.TorrentFile{{Name: qbt.Ptr("Example.2024.1080p.mkv"), Size: qbt.Ptr(int64(4 << 30))}}

	candidate := CrossSeedCandidate{
		InstanceID: 1,
		Torrents: []qbt.Torrent{
			{Hash: qbt.Ptr("rar"), Name: qbt.Ptr("Example.RAR.Release"), Progress: qbt.Ptr(float64(1.0))},
			{Hash: qbt.Ptr("mkv"), Name: qbt.Ptr("Example.2024.1080p.GRP"), Progress: qbt.Ptr(float64(1.0))},
		},
	}

	filesByHash := svc.batchLoadCandidateFiles(context.Background(), candidate.InstanceID, candidate.Torrents)
	bestTorrent, files, matchType, _ := svc.findBestCandidateMatch(context.Background(), candidate, &sourceRelease, sourceFiles, filesByHash, 5.0)
	require.NotNil(t, bestTorrent)
	require.Equal(t, "mkv", qbt.Deref(bestTorrent.Hash))
	require.Equal(t, "exact", matchType)
	require.Len(t, files, 1)
}

func TestFindBestCandidateMatch_PrefersTopLevelFolderOnTie(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		syncManager: &candidateSelectionSyncManager{
			files: map[string][]qbt.TorrentFile{
				"single": {{Name: qbt.Ptr("payload.bin"), Size: qbt.Ptr(int64(4 << 30))}},
				"folder": {
					{Name: qbt.Ptr("folder/payload.bin"), Size: qbt.Ptr(int64(4 << 30))},
					{Name: qbt.Ptr("folder/extra.txt"), Size: qbt.Ptr(int64(1 << 20))},
				},
			},
		},
	}

	sourceRelease := rls.Release{}
	sourceFiles := []qbt.TorrentFile{{Name: qbt.Ptr("PAYLOAD.bin"), Size: qbt.Ptr(int64(4 << 30))}}

	candidate := CrossSeedCandidate{
		InstanceID: 1,
		Torrents: []qbt.Torrent{
			{Hash: qbt.Ptr("single"), Name: qbt.Ptr("Minimal.Payload"), Progress: qbt.Ptr(float64(1.0))},
			{Hash: qbt.Ptr("folder"), Name: qbt.Ptr("Minimal.Payload"), Progress: qbt.Ptr(float64(1.0))},
		},
	}

	singleRelease := svc.releaseCache.Parse("Minimal.Payload")
	singleMatch := svc.getMatchType(&sourceRelease, singleRelease, sourceFiles, svc.syncManager.(*candidateSelectionSyncManager).files["single"])
	folderMatch := svc.getMatchType(&sourceRelease, singleRelease, sourceFiles, svc.syncManager.(*candidateSelectionSyncManager).files["folder"])
	require.Equal(t, singleMatch, folderMatch, "test setup should create identical match priorities")
	require.Equal(t, "size", singleMatch)

	filesByHash := svc.batchLoadCandidateFiles(context.Background(), candidate.InstanceID, candidate.Torrents)
	bestTorrent, files, matchType, _ := svc.findBestCandidateMatch(context.Background(), candidate, &sourceRelease, sourceFiles, filesByHash, 5.0)
	require.NotNil(t, bestTorrent)
	require.Equal(t, "folder", qbt.Deref(bestTorrent.Hash), "top-level folder layout should win tie-breakers")
	require.Equal(t, "size", matchType)
	require.Len(t, files, 2, "should return folder-based file list")
}

func TestFindBestCandidateMatch_RejectsSeasonPackAgainstEpisodeCandidate(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		syncManager: &candidateSelectionSyncManager{
			files: map[string][]qbt.TorrentFile{
				"ep": {{Name: qbt.Ptr("The.Show.S01E01.2160p.WEB-DL-GRP.mkv"), Size: qbt.Ptr(int64(2 << 30))}},
			},
		},
	}

	seasonPackName := "The.Show.S01.2160p.WEB-DL-GRP"
	sourceRelease := svc.releaseCache.Parse(seasonPackName)
	sourceFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("The.Show.S01E01.2160p.WEB-DL-GRP.mkv"), Size: qbt.Ptr(int64(2 << 30))},
		{Name: qbt.Ptr("The.Show.S01E02.2160p.WEB-DL-GRP.mkv"), Size: qbt.Ptr(int64(2 << 30))},
	}

	candidate := CrossSeedCandidate{
		InstanceID: 1,
		Torrents: []qbt.Torrent{
			{Hash: qbt.Ptr("ep"), Name: qbt.Ptr("The.Show.S01E01.2160p.WEB-DL-GRP"), Progress: qbt.Ptr(float64(1.0))},
		},
	}

	filesByHash := svc.batchLoadCandidateFiles(context.Background(), candidate.InstanceID, candidate.Torrents)
	bestTorrent, files, matchType, rejectReason := svc.findBestCandidateMatch(context.Background(), candidate, sourceRelease, sourceFiles, filesByHash, 5.0)
	require.Nil(t, bestTorrent)
	require.Nil(t, files)
	require.Empty(t, matchType)
	require.Equal(t, rejectReasonSeasonPackFromEpisode, rejectReason)
}

type candidateSelectionSyncManager struct {
	files map[string][]qbt.TorrentFile
}

func (c *candidateSelectionSyncManager) GetTorrents(context.Context, int, qbt.TorrentFilterOptions) ([]qbt.Torrent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) GetTorrentFiles(_ context.Context, _ int, hash string) (*[]qbt.TorrentFile, error) {
	key := strings.ToLower(hash)
	files, ok := c.files[key]
	if !ok {
		return nil, fmt.Errorf("files not found")
	}
	copyFiles := make([]qbt.TorrentFile, len(files))
	copy(copyFiles, files)
	return &copyFiles, nil
}

func (c *candidateSelectionSyncManager) GetTorrentProperties(context.Context, int, string) (*qbt.TorrentProperties, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) GetAppPreferences(_ context.Context, _ int) (qbt.AppPreferences, error) {
	return qbt.AppPreferences{TorrentContentLayout: "Original"}, nil
}

func (c *candidateSelectionSyncManager) GetTorrentFilesBatch(ctx context.Context, instanceID int, hashes []string) (map[string][]qbt.TorrentFile, error) {
	result := make(map[string][]qbt.TorrentFile, len(hashes))
	for _, h := range hashes {
		if files, err := c.GetTorrentFiles(ctx, instanceID, h); err == nil && files != nil {
			result[normalizeHash(h)] = *files
		}
	}
	return result, nil
}

func (*candidateSelectionSyncManager) ExportTorrent(context.Context, int, string) ([]byte, string, string, error) {
	return nil, "", "", errors.New("not implemented")
}

func (*candidateSelectionSyncManager) HasTorrentByAnyHash(context.Context, int, []string) (*qbt.Torrent, bool, error) {
	return nil, false, nil
}

func (c *candidateSelectionSyncManager) AddTorrent(context.Context, int, []byte, map[string]string) error {
	return fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) BulkAction(context.Context, int, []string, string) error {
	return fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) GetCachedInstanceTorrents(context.Context, int) ([]internalqb.CrossInstanceTorrentView, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) ExtractDomainFromURL(string) string {
	return ""
}

func (c *candidateSelectionSyncManager) GetQBittorrentSyncManager(context.Context, int) (*internalqb.QBTSyncManager, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) RenameTorrent(context.Context, int, string, string) error {
	return fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) RenameTorrentFile(context.Context, int, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) RenameTorrentFolder(context.Context, int, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (c *candidateSelectionSyncManager) SetTags(context.Context, int, []string, string) error {
	return nil
}

func (c *candidateSelectionSyncManager) GetCategories(_ context.Context, _ int) (map[string]qbt.Category, error) {
	return map[string]qbt.Category{}, nil
}

func (c *candidateSelectionSyncManager) CreateCategory(_ context.Context, _ int, _, _ string) error {
	return nil
}

func TestGetMatchTypeFromTitle_FallbackWhenReleaseKeysMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}
	targetName := "[TestGroup] Example Show - 1150 (1080p) [ABCDEF01]"
	candidateName := "[TestGroup] Example Show - 1150 (1080p) [ABCDEF01]"
	targetRelease := rls.Release{Title: "Example Show"}
	candidateRelease := rls.Release{Title: "Example Show"}

	// Use a filename that won't produce any usable release keys when parsed.
	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("random_data_file.bin"), Size: qbt.Ptr(int64(1024))},
	}

	match := svc.getMatchTypeFromTitle(targetName, candidateName, &targetRelease, &candidateRelease, candidateFiles)
	require.Equal(t, "partial-in-pack", match, "fallback should treat matching titles as candidates when parsing fails")
}

func TestGetMatchTypeFromTitle_NonEpisodicRequiresMatchingReleaseKey(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	// Non-episodic content with different years should not match purely because
	// candidate files have some parsed metadata.
	targetName := "Movie.2020.1080p.BluRay.x264-GROUP"
	targetRelease := rls.Release{
		Title: "Movie 2020",
		Year:  2020,
	}

	candidateName := "Completely.Different.Movie.2012.1080p.BluRay.x264-OTHER"
	candidateRelease := rls.Release{
		Title: "Different Movie 2012",
		Year:  2012,
	}

	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Different.Movie.2012.1080p.BluRay.x264-OTHER.mkv"), Size: qbt.Ptr(int64(4 << 30))},
	}

	match := svc.getMatchTypeFromTitle(targetName, candidateName, &targetRelease, &candidateRelease, candidateFiles)
	require.Empty(t, match, "non-episodic candidates with mismatched release keys should not match")
}

func TestGetMatchTypeFromTitle_NonEpisodicWithMatchingReleaseKey(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	targetName := "Movie.2020.1080p.BluRay.x264-GROUP"
	targetRelease := rls.Release{
		Title: "Movie 2020",
		Year:  2020,
	}

	candidateName := "Another.Movie.2020.1080p.BluRay.x264-OTHER"
	candidateRelease := rls.Release{
		Title: "Another Movie 2020",
		Year:  2020,
	}

	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Another.Movie.2020.1080p.BluRay.x264-OTHER.mkv"), Size: qbt.Ptr(int64(4 << 30))},
	}

	match := svc.getMatchTypeFromTitle(targetName, candidateName, &targetRelease, &candidateRelease, candidateFiles)
	require.Equal(t, "partial-in-pack", match, "non-episodic candidates with matching release keys should match")
}

func TestGetMatchTypeFromTitle_GameSceneReleasesWithRARFiles(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}

	// Game scene releases have RAR files with group prefixes, no year, no episode info.
	// These should match based on title alone when the titles match.
	targetName := "Oddsparks.An.Automation.Adventure.Coaster.Rush-RUNE"
	targetRelease := rls.Release{
		Title: "Oddsparks An Automation Adventure Coaster Rush",
		Group: "RUNE",
		Type:  rls.Game,
	}

	candidateName := "Oddsparks.An.Automation.Adventure.Coaster.Rush-RUNE"
	candidateRelease := rls.Release{
		Title: "Oddsparks An Automation Adventure Coaster Rush",
		Group: "RUNE",
		Type:  rls.Game,
	}

	// RAR files don't produce usable release keys when parsed
	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("rune-oddsparks.rar"), Size: qbt.Ptr(int64(100 << 20))},
		{Name: qbt.Ptr("rune-oddsparks.r00"), Size: qbt.Ptr(int64(100 << 20))},
		{Name: qbt.Ptr("rune-oddsparks.r01"), Size: qbt.Ptr(int64(100 << 20))},
		{Name: qbt.Ptr("rune-oddsparks.sfv"), Size: qbt.Ptr(int64(1024))},
		{Name: qbt.Ptr("rune-oddsparks.nfo"), Size: qbt.Ptr(int64(4096))},
	}

	match := svc.getMatchTypeFromTitle(targetName, candidateName, &targetRelease, &candidateRelease, candidateFiles)
	require.Equal(t, "release-match", match, "game scene releases with RAR files should match when titles match")
}

func TestGetMatchType_FileNameFallback(t *testing.T) {
	t.Parallel()

	svc := &Service{
		releaseCache:     releases.NewDefaultParser(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
	}
	sourceRelease := rls.Release{Title: "Example Show"}
	candidateRelease := rls.Release{Title: "Example Show"}

	sourceFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("[TestGroup] Example Show - 1150 (1080p) [ABCDEF01].mkv"), Size: qbt.Ptr(int64(1 << 30))},
	}
	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("[TestGroup] Example Show - 1150 (1080p) [ABCDEF01]/[TestGroup] Example Show - 1150 (1080p) [ABCDEF01].mkv"), Size: qbt.Ptr(int64(1 << 30))},
	}

	match := svc.getMatchType(&sourceRelease, &candidateRelease, sourceFiles, candidateFiles)
	require.Equal(t, "size", match, "single-file torrents with matching base names should fallback to size match")
}
