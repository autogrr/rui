package crossseed

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qbt "github.com/autogrr/go-qbittorrent"
	_ "modernc.org/sqlite"

	"github.com/autogrr/rui/internal/database"
	"github.com/autogrr/rui/internal/dbinterface"
	"github.com/autogrr/rui/internal/models"
	internalqb "github.com/autogrr/rui/internal/qbittorrent"
)

// testQuerier wraps sql.DB to implement dbinterface.Querier for store tests.
type testQuerier struct {
	*sql.DB
}

type testTx struct {
	*sql.Tx
}

func (t *testTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, query, args...)
}

func (t *testTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, query, args...)
}

func (t *testTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, query, args...)
}

func (t *testTx) Commit() error {
	return t.Tx.Commit()
}

func (t *testTx) Rollback() error {
	return t.Tx.Rollback()
}

func (q *testQuerier) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbinterface.TxQuerier, error) {
	tx, err := q.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &testTx{Tx: tx}, nil
}

type staticInstanceStore struct {
	inst *models.Instance
}

func (s *staticInstanceStore) Get(_ context.Context, id int) (*models.Instance, error) {
	if s.inst != nil && s.inst.ID == id {
		return s.inst, nil
	}
	return nil, models.ErrInstanceNotFound
}

func (s *staticInstanceStore) List(_ context.Context) ([]*models.Instance, error) {
	if s.inst == nil {
		return []*models.Instance{}, nil
	}
	return []*models.Instance{s.inst}, nil
}

type completionGazelleSyncMock struct {
	torrent         qbt.Torrent
	getTorrentsHits int
}

func (m *completionGazelleSyncMock) GetTorrents(_ context.Context, _ int, filter qbt.TorrentFilterOptions) ([]qbt.Torrent, error) {
	m.getTorrentsHits++
	if len(filter.Hashes) == 0 {
		return []qbt.Torrent{m.torrent}, nil
	}
	for _, h := range filter.Hashes {
		if normalizeHash(h) == normalizeHash(qbt.Deref(m.torrent.Hash)) {
			return []qbt.Torrent{m.torrent}, nil
		}
	}
	return []qbt.Torrent{}, nil
}

func (m *completionGazelleSyncMock) GetTorrentFilesBatch(_ context.Context, _ int, hashes []string) (map[string][]qbt.TorrentFile, error) {
	out := make(map[string][]qbt.TorrentFile, len(hashes))
	for _, h := range hashes {
		out[normalizeHash(h)] = []qbt.TorrentFile{{Name: qbt.Ptr("01 - track.flac"), Size: qbt.Ptr(int64(123))}}
	}
	return out, nil
}

func (m *completionGazelleSyncMock) ExportTorrent(context.Context, int, string) ([]byte, string, string, error) {
	return nil, "", "", nil
}

func (m *completionGazelleSyncMock) HasTorrentByAnyHash(context.Context, int, []string) (*qbt.Torrent, bool, error) {
	return nil, false, nil
}

func (m *completionGazelleSyncMock) GetTorrentProperties(context.Context, int, string) (*qbt.TorrentProperties, error) {
	return &qbt.TorrentProperties{}, nil
}

func (m *completionGazelleSyncMock) GetAppPreferences(context.Context, int) (qbt.AppPreferences, error) {
	return qbt.AppPreferences{}, nil
}

func (m *completionGazelleSyncMock) AddTorrent(context.Context, int, []byte, map[string]string) error {
	return nil
}

func (m *completionGazelleSyncMock) BulkAction(context.Context, int, []string, string) error {
	return nil
}

func (m *completionGazelleSyncMock) GetCachedInstanceTorrents(context.Context, int) ([]internalqb.CrossInstanceTorrentView, error) {
	return nil, nil
}

func (m *completionGazelleSyncMock) ExtractDomainFromURL(urlStr string) string {
	u, err := url.Parse(strings.TrimSpace(urlStr))
	if err == nil && u != nil && u.Host != "" {
		return u.Hostname()
	}
	// best-effort: raw host fallback
	urlStr = strings.TrimSpace(urlStr)
	urlStr = strings.TrimPrefix(urlStr, "http://")
	urlStr = strings.TrimPrefix(urlStr, "https://")
	if i := strings.IndexByte(urlStr, '/'); i >= 0 {
		urlStr = urlStr[:i]
	}
	return strings.TrimSpace(urlStr)
}

func (m *completionGazelleSyncMock) GetQBittorrentSyncManager(context.Context, int) (*internalqb.QBTSyncManager, error) {
	return nil, nil
}

func (m *completionGazelleSyncMock) RenameTorrent(context.Context, int, string, string) error {
	return nil
}

func (m *completionGazelleSyncMock) RenameTorrentFile(context.Context, int, string, string, string) error {
	return nil
}

func (m *completionGazelleSyncMock) RenameTorrentFolder(context.Context, int, string, string, string) error {
	return nil
}

func (m *completionGazelleSyncMock) SetTags(context.Context, int, []string, string) error {
	return nil
}

func (m *completionGazelleSyncMock) GetCategories(context.Context, int) (map[string]qbt.Category, error) {
	return map[string]qbt.Category{}, nil
}

func (m *completionGazelleSyncMock) CreateCategory(context.Context, int, string, string) error {
	return nil
}

func TestHandleTorrentCompletion_AllowsGazelleWhenJackettMissing(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", "file:completion_gazelle_guard?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := &testQuerier{DB: db}

	_, err = q.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS string_pool (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL UNIQUE
		);
		CREATE TABLE instance_crossseed_completion_settings (
			instance_id INTEGER NOT NULL,
			owner_id INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL,
			categories_json_id INTEGER NOT NULL REFERENCES string_pool(id),
			tags_json_id INTEGER NOT NULL REFERENCES string_pool(id),
			exclude_categories_json_id INTEGER NOT NULL REFERENCES string_pool(id),
			exclude_tags_json_id INTEGER NOT NULL REFERENCES string_pool(id),
			indexer_ids_json_id INTEGER NOT NULL REFERENCES string_pool(id),
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (instance_id, owner_id)
		);
		CREATE VIEW instance_crossseed_completion_settings_view AS
		SELECT cs.instance_id, cs.owner_id, cs.enabled,
		       sp_cj.value AS categories_json, sp_tj.value AS tags_json,
		       sp_ecj.value AS exclude_categories_json, sp_etj.value AS exclude_tags_json,
		       sp_ij.value AS indexer_ids_json, cs.updated_at
		FROM instance_crossseed_completion_settings cs
		JOIN string_pool sp_cj  ON cs.categories_json_id = sp_cj.id
		JOIN string_pool sp_tj  ON cs.tags_json_id = sp_tj.id
		JOIN string_pool sp_ecj ON cs.exclude_categories_json_id = sp_ecj.id
		JOIN string_pool sp_etj ON cs.exclude_tags_json_id = sp_etj.id
		JOIN string_pool sp_ij  ON cs.indexer_ids_json_id = sp_ij.id;
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Insert string_pool values for the JSON fields
	_, err = q.ExecContext(context.Background(), `INSERT INTO string_pool (value) VALUES ('[]')`)
	if err != nil {
		t.Fatalf("insert string pool: %v", err)
	}
	var emptyArrayID int64
	err = q.QueryRowContext(context.Background(), `SELECT id FROM string_pool WHERE value = '[]'`).Scan(&emptyArrayID)
	if err != nil {
		t.Fatalf("get string pool id: %v", err)
	}

	_, err = q.ExecContext(context.Background(), `
		INSERT INTO instance_crossseed_completion_settings (
			instance_id, owner_id, enabled, categories_json_id, tags_json_id,
			exclude_categories_json_id, exclude_tags_json_id, indexer_ids_json_id, updated_at
		) VALUES (1, 1, 1, ?, ?, ?, ?, ?, ?);
	`, emptyArrayID, emptyArrayID, emptyArrayID, emptyArrayID, emptyArrayID, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert completion settings: %v", err)
	}

	completionStore := models.NewInstanceCrossSeedCompletionStore(q)

	src := qbt.Torrent{
		Hash:         qbt.Ptr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Name:         qbt.Ptr("test (2026) [FLAC]"),
		Tracker:      qbt.Ptr("https://flacsfor.me/announce"),
		Progress:     qbt.Ptr(float64(1.0)),
		CompletionOn: qbt.Ptr(int64(123)),
	}

	syncMock := &completionGazelleSyncMock{torrent: src}

	svc := &Service{
		instanceStore: &staticInstanceStore{inst: &models.Instance{ID: 1, Name: "music"}},
		syncManager:   syncMock,
		// jackettService intentionally nil
		completionStore: completionStore,
		automationSettingsLoader: func(context.Context) (*models.CrossSeedAutomationSettings, error) {
			return &models.CrossSeedAutomationSettings{
				GazelleEnabled: true,
				OrpheusAPIKey:  "ops-key",
			}, nil
		},

		releaseCache: NewReleaseCache(),
	}

	svc.HandleTorrentCompletion(context.Background(), 1, src)

	if syncMock.getTorrentsHits == 0 {
		t.Fatalf("expected completion flow to call syncManager.GetTorrents for Gazelle even when jackett is nil")
	}
}

func TestExecuteCompletionSearch_GazelleSourceSkipsTorznab(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "completion-gazelle-search.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO string_pool (value) VALUES ('test-user'), ('test-hash')")
	if err != nil {
		t.Fatalf("insert string pool: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO users (username_id, password_hash_id) VALUES (
		(SELECT id FROM string_pool WHERE value = 'test-user'),
		(SELECT id FROM string_pool WHERE value = 'test-hash'))`)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	store, err := models.NewCrossSeedStore(db, key)
	if err != nil {
		t.Fatalf("new cross-seed store: %v", err)
	}
	_, err = store.UpsertSettings(context.Background(), 1, &models.CrossSeedAutomationSettings{
		GazelleEnabled: true,
		OrpheusAPIKey:  "ops-key",
	})
	if err != nil {
		t.Fatalf("persist cross-seed settings: %v", err)
	}

	src := qbt.Torrent{
		Hash:     qbt.Ptr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Name:     qbt.Ptr("test (2026) [FLAC]"),
		Tracker:  qbt.Ptr("https://flacsfor.me/announce"),
		Progress: qbt.Ptr(float64(1.0)),
	}
	syncMock := &completionGazelleSyncMock{torrent: src}

	svc := &Service{
		instanceStore:   &staticInstanceStore{inst: &models.Instance{ID: 1, Name: "music"}},
		syncManager:     syncMock,
		jackettService:  newFailingJackettService(errors.New("jackett indexer probe should be skipped")),
		automationStore: store,
		releaseCache:    NewReleaseCache(),
	}

	err = svc.executeCompletionSearch(context.Background(), 1, &src, &models.CrossSeedAutomationSettings{
		GazelleEnabled:         true,
		OrpheusAPIKey:          "ops-key",
		FindIndividualEpisodes: true,
	}, &models.InstanceCrossSeedCompletionSettings{
		InstanceID: 1,
		Enabled:    true,
		IndexerIDs: []int{999},
	})
	if err != nil {
		t.Fatalf("expected gazelle completion path to skip torznab probe, got error: %v", err)
	}
}

func TestExecuteCompletionSearch_GazelleSourceFallsBackToTorznabWhenTargetKeyMissing(t *testing.T) {
	t.Parallel()

	src := qbt.Torrent{
		Hash:     qbt.Ptr("cccccccccccccccccccccccccccccccccccccccc"),
		Name:     qbt.Ptr("test (2026) [FLAC]"),
		Tracker:  qbt.Ptr("https://flacsfor.me/announce"),
		Progress: qbt.Ptr(float64(1.0)),
	}
	syncMock := &completionGazelleSyncMock{torrent: src}

	const fallbackErrMsg = "expected torznab fallback path"
	svc := &Service{
		instanceStore:  &staticInstanceStore{inst: &models.Instance{ID: 1, Name: "music"}},
		syncManager:    syncMock,
		jackettService: newFailingJackettService(errors.New(fallbackErrMsg)),
		releaseCache:   NewReleaseCache(),
	}

	err := svc.executeCompletionSearch(context.Background(), 1, &src, &models.CrossSeedAutomationSettings{
		GazelleEnabled:         true,
		FindIndividualEpisodes: true,
	}, &models.InstanceCrossSeedCompletionSettings{
		InstanceID: 1,
		Enabled:    true,
		IndexerIDs: []int{999},
	})
	if err == nil {
		t.Fatalf("expected completion path to fall back to torznab when opposite-site Gazelle key is missing")
	}
	if !strings.Contains(err.Error(), fallbackErrMsg) {
		t.Fatalf("expected torznab fallback error %q, got: %v", fallbackErrMsg, err)
	}
}

func TestExecuteCompletionSearch_GazelleSourceFallsBackToTorznabWhenTargetKeyUndecryptable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "completion-gazelle-undecryptable.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO string_pool (value) VALUES ('test-user'), ('test-hash')")
	if err != nil {
		t.Fatalf("insert string pool: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO users (username_id, password_hash_id) VALUES (
		(SELECT id FROM string_pool WHERE value = 'test-user'),
		(SELECT id FROM string_pool WHERE value = 'test-hash'))`)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	goodStore, err := models.NewCrossSeedStore(db, key)
	if err != nil {
		t.Fatalf("new cross-seed store: %v", err)
	}
	_, err = goodStore.UpsertSettings(ctx, 1, &models.CrossSeedAutomationSettings{
		GazelleEnabled: true,
		OrpheusAPIKey:  "ops-key",
	})
	if err != nil {
		t.Fatalf("persist cross-seed settings: %v", err)
	}

	badKey := make([]byte, 32)
	for i := range badKey {
		badKey[i] = byte(i + 1)
	}
	badStore, err := models.NewCrossSeedStore(db, badKey)
	if err != nil {
		t.Fatalf("new cross-seed store (bad key): %v", err)
	}
	settings, err := badStore.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("load settings with bad key: %v", err)
	}

	src := qbt.Torrent{
		Hash:     qbt.Ptr("dddddddddddddddddddddddddddddddddddddddd"),
		Name:     qbt.Ptr("test (2026) [FLAC]"),
		Tracker:  qbt.Ptr("https://flacsfor.me/announce"),
		Progress: qbt.Ptr(float64(1.0)),
	}
	syncMock := &completionGazelleSyncMock{torrent: src}

	const fallbackErrMsg = "expected torznab fallback path"
	svc := &Service{
		instanceStore:   &staticInstanceStore{inst: &models.Instance{ID: 1, Name: "music"}},
		syncManager:     syncMock,
		jackettService:  newFailingJackettService(errors.New(fallbackErrMsg)),
		automationStore: badStore,
		releaseCache:    NewReleaseCache(),
	}

	err = svc.executeCompletionSearch(ctx, 1, &src, settings, &models.InstanceCrossSeedCompletionSettings{
		InstanceID: 1,
		Enabled:    true,
		IndexerIDs: []int{999},
	})
	if err == nil {
		t.Fatalf("expected completion path to fall back to torznab when opposite-site Gazelle key is undecryptable")
	}
	if !strings.Contains(err.Error(), fallbackErrMsg) {
		t.Fatalf("expected torznab fallback error %q, got: %v", fallbackErrMsg, err)
	}
}
