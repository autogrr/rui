// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/models"
	internalqb "github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/pkg/stringutils"
)

type rootlessSavePathSyncManager struct {
	files        map[string][]qbt.TorrentFile
	props        map[string]*qbt.TorrentProperties
	addedOptions map[string]string
}

func (m *rootlessSavePathSyncManager) GetTorrents(_ context.Context, _ int, filter qbt.TorrentFilterOptions) ([]qbt.Torrent, error) {
	if len(filter.Hashes) > 0 {
		torrents := make([]qbt.Torrent, 0, len(filter.Hashes))
		for _, hash := range filter.Hashes {
			torrents = append(torrents, qbt.Torrent{Hash: qbt.Ptr(hash)})
		}
		return torrents, nil
	}
	return []qbt.Torrent{{Hash: qbt.Ptr("dummy")}}, nil
}

func (m *rootlessSavePathSyncManager) GetTorrentFilesBatch(_ context.Context, _ int, hashes []string) (map[string][]qbt.TorrentFile, error) {
	result := make(map[string][]qbt.TorrentFile, len(hashes))
	for _, h := range hashes {
		if files, ok := m.files[strings.ToLower(h)]; ok {
			cp := make([]qbt.TorrentFile, len(files))
			copy(cp, files)
			result[normalizeHash(h)] = cp
		}
	}
	return result, nil
}

func (*rootlessSavePathSyncManager) ExportTorrent(context.Context, int, string) ([]byte, string, string, error) {
	return nil, "", "", errors.New("not implemented")
}

func (*rootlessSavePathSyncManager) HasTorrentByAnyHash(context.Context, int, []string) (*qbt.Torrent, bool, error) {
	return nil, false, nil
}

func (m *rootlessSavePathSyncManager) GetTorrentProperties(_ context.Context, _ int, hash string) (*qbt.TorrentProperties, error) {
	if props, ok := m.props[strings.ToLower(hash)]; ok {
		cp := *props
		return &cp, nil
	}
	return &qbt.TorrentProperties{SavePath: qbt.Ptr("/downloads")}, nil
}

func (*rootlessSavePathSyncManager) GetAppPreferences(context.Context, int) (qbt.AppPreferences, error) {
	return qbt.AppPreferences{TorrentContentLayout: "Original"}, nil
}

func (m *rootlessSavePathSyncManager) AddTorrent(_ context.Context, _ int, _ []byte, options map[string]string) error {
	m.addedOptions = make(map[string]string, len(options))
	maps.Copy(m.addedOptions, options)
	return nil
}

func (*rootlessSavePathSyncManager) BulkAction(context.Context, int, []string, string) error {
	return nil
}

func (*rootlessSavePathSyncManager) SetTags(context.Context, int, []string, string) error {
	return nil
}

func (*rootlessSavePathSyncManager) GetCachedInstanceTorrents(context.Context, int) ([]internalqb.CrossInstanceTorrentView, error) {
	return nil, nil
}

func (*rootlessSavePathSyncManager) ExtractDomainFromURL(string) string {
	return ""
}

func (*rootlessSavePathSyncManager) GetQBittorrentSyncManager(context.Context, int) (*internalqb.QBTSyncManager, error) {
	return nil, nil
}

func (*rootlessSavePathSyncManager) RenameTorrent(context.Context, int, string, string) error {
	return nil
}

func (*rootlessSavePathSyncManager) RenameTorrentFile(context.Context, int, string, string, string) error {
	return nil
}

func (*rootlessSavePathSyncManager) RenameTorrentFolder(context.Context, int, string, string, string) error {
	return nil
}

func (*rootlessSavePathSyncManager) GetCategories(context.Context, int) (map[string]qbt.Category, error) {
	return map[string]qbt.Category{}, nil
}

func (*rootlessSavePathSyncManager) CreateCategory(context.Context, int, string, string) error {
	return nil
}

// rootlessSavePathInstanceStore is a mock instance provider for tests
type rootlessSavePathInstanceStore struct {
	instances map[int]*models.Instance
}

func (m *rootlessSavePathInstanceStore) Get(_ context.Context, id int) (*models.Instance, error) {
	if inst, ok := m.instances[id]; ok {
		return inst, nil
	}
	// Return instance with hardlinks disabled by default
	return &models.Instance{
		ID:           id,
		UseHardlinks: false,
	}, nil
}

func (m *rootlessSavePathInstanceStore) List(_ context.Context) ([]*models.Instance, error) {
	result := make([]*models.Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst)
	}
	return result, nil
}

func TestProcessCrossSeedCandidate_RootlessContentDirOverridesSavePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := 1
	matchedHash := "matchedhash"
	newHash := "newhash"
	matchedName := "Show.S01E01.1080p.WEB-DL-GROUP"

	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
	}
	sourceFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
	}

	matchedTorrent := qbt.Torrent{
		Hash:        qbt.Ptr(matchedHash),
		Name:        qbt.Ptr(matchedName),
		Progress:    qbt.Ptr(float64(1.0)),
		Category:    qbt.Ptr("tv"),
		ContentPath: qbt.Ptr("/downloads/tv/Show.S01E01/Show.S01E01.mkv"),
	}

	sync := &rootlessSavePathSyncManager{
		files: map[string][]qbt.TorrentFile{
			matchedHash: candidateFiles,
			newHash:     sourceFiles,
		},
		props: map[string]*qbt.TorrentProperties{
			matchedHash: {SavePath: qbt.Ptr("/downloads/tv")},
		},
	}

	instanceStore := &rootlessSavePathInstanceStore{
		instances: map[int]*models.Instance{
			instanceID: {
				ID:           instanceID,
				UseHardlinks: false,
			},
		},
	}

	service := &Service{
		syncManager:      sync,
		instanceStore:    instanceStore,
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		automationSettingsLoader: func(context.Context) (*models.CrossSeedAutomationSettings, error) {
			return models.DefaultCrossSeedAutomationSettings(), nil
		},
	}

	startPaused := true
	req := &CrossSeedRequest{
		StartPaused: &startPaused,
	}

	candidate := CrossSeedCandidate{
		InstanceID:   instanceID,
		InstanceName: "Test",
		Torrents:     []qbt.Torrent{matchedTorrent},
	}

	result := service.processCrossSeedCandidate(ctx, candidate, []byte("torrent"), newHash, "", matchedName, req, service.releaseCache.Parse(matchedName), sourceFiles, nil)
	require.True(t, result.Success)
	require.Equal(t, "added", result.Status)

	require.NotNil(t, sync.addedOptions)
	require.Equal(t, "false", sync.addedOptions["autoTMM"])
	require.Equal(t, "/downloads/tv/Show.S01E01", sync.addedOptions["savepath"])
	require.Equal(t, "Original", sync.addedOptions["contentLayout"])
	require.Equal(t, "true", sync.addedOptions["skip_checking"])
}

func TestProcessCrossSeedCandidate_RootlessContentDirOverridesSavePath_MultiFileUsesContentPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := 1
	matchedHash := "matchedhash"
	newHash := "newhash"
	matchedName := "Show.S01E01.1080p.WEB-DL-GROUP"

	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
		{Name: qbt.Ptr("Show.S01E01.srt"), Size: qbt.Ptr(int64(128))},
	}
	sourceFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
		{Name: qbt.Ptr("Show.S01E01.srt"), Size: qbt.Ptr(int64(128))},
	}

	matchedTorrent := qbt.Torrent{
		Hash:        qbt.Ptr(matchedHash),
		Name:        qbt.Ptr(matchedName),
		Progress:    qbt.Ptr(float64(1.0)),
		Category:    qbt.Ptr("tv"),
		ContentPath: qbt.Ptr("/downloads/tv/Show.S01E01"),
	}

	sync := &rootlessSavePathSyncManager{
		files: map[string][]qbt.TorrentFile{
			matchedHash: candidateFiles,
			newHash:     sourceFiles,
		},
		props: map[string]*qbt.TorrentProperties{
			matchedHash: {SavePath: qbt.Ptr("/downloads/tv")},
		},
	}

	instanceStore := &rootlessSavePathInstanceStore{
		instances: map[int]*models.Instance{
			instanceID: {
				ID:           instanceID,
				UseHardlinks: false,
			},
		},
	}

	service := &Service{
		syncManager:      sync,
		instanceStore:    instanceStore,
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		automationSettingsLoader: func(context.Context) (*models.CrossSeedAutomationSettings, error) {
			return models.DefaultCrossSeedAutomationSettings(), nil
		},
	}

	startPaused := true
	req := &CrossSeedRequest{
		StartPaused: &startPaused,
	}

	candidate := CrossSeedCandidate{
		InstanceID:   instanceID,
		InstanceName: "Test",
		Torrents:     []qbt.Torrent{matchedTorrent},
	}

	result := service.processCrossSeedCandidate(ctx, candidate, []byte("torrent"), newHash, "", matchedName, req, service.releaseCache.Parse(matchedName), sourceFiles, nil)
	require.True(t, result.Success)
	require.Equal(t, "added", result.Status)

	require.NotNil(t, sync.addedOptions)
	require.Equal(t, "false", sync.addedOptions["autoTMM"])
	require.Equal(t, "/downloads/tv/Show.S01E01", sync.addedOptions["savepath"])
	require.Equal(t, "Original", sync.addedOptions["contentLayout"])
	require.Equal(t, "true", sync.addedOptions["skip_checking"])
}

func TestProcessCrossSeedCandidate_RootlessContentDirNoopWhenSavePathMatches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := 1
	matchedHash := "matchedhash"
	newHash := "newhash"
	matchedName := "Show.S01E01.1080p.WEB-DL-GROUP"

	candidateFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
	}
	sourceFiles := []qbt.TorrentFile{
		{Name: qbt.Ptr("Show.S01E01.mkv"), Size: qbt.Ptr(int64(1024))},
	}

	matchedTorrent := qbt.Torrent{
		Hash:        qbt.Ptr(matchedHash),
		Name:        qbt.Ptr(matchedName),
		Progress:    qbt.Ptr(float64(1.0)),
		Category:    qbt.Ptr("tv"),
		ContentPath: qbt.Ptr("/downloads/tv/Show.S01E01/Show.S01E01.mkv"),
	}

	sync := &rootlessSavePathSyncManager{
		files: map[string][]qbt.TorrentFile{
			matchedHash: candidateFiles,
			newHash:     sourceFiles,
		},
		props: map[string]*qbt.TorrentProperties{
			matchedHash: {SavePath: qbt.Ptr("/downloads/tv/Show.S01E01")},
		},
	}

	instanceStore := &rootlessSavePathInstanceStore{
		instances: map[int]*models.Instance{
			instanceID: {
				ID:           instanceID,
				UseHardlinks: false,
			},
		},
	}

	service := &Service{
		syncManager:      sync,
		instanceStore:    instanceStore,
		releaseCache:     NewReleaseCache(),
		stringNormalizer: stringutils.NewDefaultNormalizer(),
		automationSettingsLoader: func(context.Context) (*models.CrossSeedAutomationSettings, error) {
			return models.DefaultCrossSeedAutomationSettings(), nil
		},
	}

	startPaused := true
	req := &CrossSeedRequest{
		StartPaused: &startPaused,
	}

	candidate := CrossSeedCandidate{
		InstanceID:   instanceID,
		InstanceName: "Test",
		Torrents:     []qbt.Torrent{matchedTorrent},
	}

	result := service.processCrossSeedCandidate(ctx, candidate, []byte("torrent"), newHash, "", matchedName, req, service.releaseCache.Parse(matchedName), sourceFiles, nil)
	require.True(t, result.Success)
	require.Equal(t, "added", result.Status)

	require.NotNil(t, sync.addedOptions)
	require.Equal(t, "false", sync.addedOptions["autoTMM"])
	require.Equal(t, "/downloads/tv/Show.S01E01", sync.addedOptions["savepath"])
	require.Equal(t, "Original", sync.addedOptions["contentLayout"])
	require.Equal(t, "true", sync.addedOptions["skip_checking"])
}
