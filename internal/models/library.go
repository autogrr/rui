// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

var (
	ErrLibraryTitleNotFound = errors.New("library title not found")
	ErrLibraryRuleNotFound  = errors.New("library rule not found")
)

// MediaMetadata holds enriched metadata for a title fetched from an information
// provider (*arr). Used during library sync and intake pipeline processing.
type MediaMetadata struct {
	// External identifiers (may be empty if the provider didn't return them).
	IMDbID   string
	TMDbID   int
	TVDbID   int
	TVMazeID int
	// Ratings stored on their native scale:
	//   IMDb/TMDb/Audience: 0–10
	//   Metacritic: 0–100
	//   RottenTomatoes: 0–100 (percentage)
	IMDbRating           float64
	TMDbRating           float64
	MetacriticRating     float64
	RottenTomatoesRating float64
	AudienceRating       float64
	// Genres is a comma-separated list ("Action, Adventure, Sci-Fi").
	Genres    string
	Overview  string
	Year      int
	Title     string
	SortTitle string
	Status    string
}

// IsEmpty returns true when no useful metadata was found.
func (m *MediaMetadata) IsEmpty() bool {
	return m == nil || (m.Title == "" && m.IMDbID == "" && m.TMDbID == 0 && m.TVDbID == 0)
}

// LibraryContentType is the kind of content a library title represents.
type LibraryContentType string

const (
	LibraryContentTypeMovie LibraryContentType = "movie"
	LibraryContentTypeTV    LibraryContentType = "tv"
	LibraryContentTypeAnime LibraryContentType = "anime"
	LibraryContentTypeMusic LibraryContentType = "music"
	LibraryContentTypeBook  LibraryContentType = "book"
)

// LibraryAction is the action a matching rule can trigger.
type LibraryAction string

const (
	LibraryActionGrab    LibraryAction = "grab"
	LibraryActionSkip    LibraryAction = "skip"
	LibraryActionUpgrade LibraryAction = "upgrade"
	LibraryActionRemove  LibraryAction = "remove"
)

// LibraryTitle is a tracked movie or TV show in the rui native library.
type LibraryTitle struct {
	ID            int                `json:"id"`
	OwnerID       int                `json:"owner_id"`
	ContentType   LibraryContentType `json:"content_type"`
	Title         string             `json:"title"`
	SortTitle     string             `json:"sort_title"`
	Year          *int               `json:"year,omitempty"`
	IMDbID        string             `json:"imdb_id,omitempty"`
	TMDbID        int                `json:"tmdb_id,omitempty"`
	TVDbID        int                `json:"tvdb_id,omitempty"`
	TVMazeID      int                `json:"tvmaze_id,omitempty"`
	ArrInstanceID *int               `json:"arr_instance_id,omitempty"`
	ArrItemID     *int               `json:"arr_item_id,omitempty"`
	Overview      string             `json:"overview,omitempty"`
	Status        string             `json:"status,omitempty"`
	Path          string             `json:"path,omitempty"`
	HasFile       bool               `json:"has_file"`
	// Ratings from the information provider (*arr lookup).
	IMDbRating           float64 `json:"imdb_rating,omitempty"`
	TMDbRating           float64 `json:"tmdb_rating,omitempty"`
	MetacriticRating     float64 `json:"metacritic_rating,omitempty"`
	RottenTomatoesRating float64 `json:"rotten_tomatoes_rating,omitempty"`
	AudienceRating       float64 `json:"audience_rating,omitempty"`
	// Genres is a comma-separated list.
	Genres string `json:"genres,omitempty"`
	// Source indicates how this entry entered the library.
	// One of: "arr", "torrent_client", "manual".
	Source string `json:"source"`
	// InfoHash is the torrent infohash for torrent_client-sourced entries.
	InfoHash string `json:"info_hash,omitempty"`
	// TorrentCount tracks how many torrents back this title (torrent_client source).
	TorrentCount int `json:"torrent_count,omitempty"`
	// EpisodeCount is the number of episode-style torrents seen for TV titles.
	EpisodeCount int `json:"episode_count,omitempty"`
	// Qualities is a comma-separated sorted list of unique quality labels observed
	// for this title, e.g. "1080p WEBDL HEVC, 4K REMUX HEVC DV".
	Qualities string    `json:"qualities,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Seasons is populated on demand; empty for movies.
	Seasons []LibrarySeason `json:"seasons,omitempty"`
}

// LibrarySeason represents one season of a TV library title.
type LibrarySeason struct {
	ID           int  `json:"id"`
	TitleID      int  `json:"title_id"`
	SeasonNumber int  `json:"season_number"`
	EpisodeCount int  `json:"episode_count"`
	HasFiles     bool `json:"has_files"`
}

// LibraryRule is an expr-lang rule evaluated against incoming releases.
type LibraryRule struct {
	ID           int           `json:"id"`
	OwnerID      int           `json:"owner_id"`
	Name         string        `json:"name"`
	Enabled      bool          `json:"enabled"`
	SortOrder    int           `json:"sort_order"`
	ContentTypes []string      `json:"content_types"` // split from comma-separated storage
	ExprFilter   string        `json:"expr_filter"`
	Action       LibraryAction `json:"action"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// LibraryTitleUpsertParams carries all mutable fields for creating/updating a title.
type LibraryTitleUpsertParams struct {
	OwnerID       int
	ContentType   LibraryContentType
	Title         string
	SortTitle     string
	Year          *int
	IMDbID        string
	TMDbID        int
	TVDbID        int
	TVMazeID      int
	ArrInstanceID *int
	ArrItemID     *int
	Overview      string
	Status        string
	Path          string
	HasFile       bool
	// Ratings from information provider lookup.
	IMDbRating           float64
	TMDbRating           float64
	MetacriticRating     float64
	RottenTomatoesRating float64
	AudienceRating       float64
	// Genres is a comma-separated list of genre strings.
	Genres string
	// Source is "arr", "torrent_client", or "manual".
	Source string
	// InfoHash is set for torrent_client-sourced entries.
	InfoHash string
	// TorrentCount is the number of torrents backing this title.
	TorrentCount int
	// EpisodeCount is the number of episode-style torrents seen for TV titles.
	EpisodeCount int
	// Qualities is a comma-separated sorted list of unique quality labels.
	Qualities string
}

// LibraryTitleStore manages library titles and seasons in SQLite.
type LibraryTitleStore struct {
	db dbinterface.Querier
}

// NewLibraryTitleStore creates a new LibraryTitleStore.
func NewLibraryTitleStore(db dbinterface.Querier) *LibraryTitleStore {
	return &LibraryTitleStore{db: db}
}

// sourceDefault normalises a Source value, defaulting to "arr".
func sourceDefault(s string) string {
	switch s {
	case "torrent_client", "manual":
		return s
	default:
		return "arr"
	}
}

// Upsert inserts or updates a title identified by (owner_id, arr_instance_id, arr_item_id)
// for arr-sourced entries, or by (owner_id, info_hash) for torrent_client-sourced entries.
func (s *LibraryTitleStore) Upsert(ctx context.Context, p LibraryTitleUpsertParams) (*LibraryTitle, error) {
	src := sourceDefault(p.Source)
	const q = `
INSERT INTO library_titles
    (owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id, tvmaze_id,
     arr_instance_id, arr_item_id, overview, status, path, has_file,
     imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
     genres, source, info_hash, torrent_count, episode_count, qualities)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(owner_id, arr_instance_id, arr_item_id) DO UPDATE SET
    content_type          = excluded.content_type,
    title                 = excluded.title,
    sort_title            = excluded.sort_title,
    year                  = excluded.year,
    imdb_id               = excluded.imdb_id,
    tmdb_id               = excluded.tmdb_id,
    tvdb_id               = excluded.tvdb_id,
    tvmaze_id             = excluded.tvmaze_id,
    overview              = excluded.overview,
    status                = excluded.status,
    path                  = excluded.path,
    has_file              = excluded.has_file,
    imdb_rating           = excluded.imdb_rating,
    tmdb_rating           = excluded.tmdb_rating,
    metacritic_rating     = excluded.metacritic_rating,
    rotten_tomatoes_rating = excluded.rotten_tomatoes_rating,
    audience_rating       = excluded.audience_rating,
    genres                = excluded.genres,
    source                = excluded.source
RETURNING id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
          tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
          imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
          genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at`

	return scanTitle(s.db.QueryRowContext(ctx, q,
		p.OwnerID, string(p.ContentType), p.Title, p.SortTitle, p.Year,
		p.IMDbID, p.TMDbID, p.TVDbID, p.TVMazeID,
		p.ArrInstanceID, p.ArrItemID, p.Overview, p.Status, p.Path, p.HasFile,
		p.IMDbRating, p.TMDbRating, p.MetacriticRating, p.RottenTomatoesRating, p.AudienceRating,
		p.Genres, src, p.InfoHash, p.TorrentCount, 0, "",
	))
}

// UpsertByHash inserts or updates a torrent-client-sourced title identified by info_hash.
// The ON CONFLICT clause targets the partial unique index on (owner_id, info_hash)
// WHERE info_hash != ” from migration 084.
func (s *LibraryTitleStore) UpsertByHash(ctx context.Context, p LibraryTitleUpsertParams) (*LibraryTitle, error) {
	p.Source = "torrent_client"
	const q = `
INSERT INTO library_titles
    (owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id, tvmaze_id,
     arr_instance_id, arr_item_id, overview, status, path, has_file,
     imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
     genres, source, info_hash, torrent_count, episode_count, qualities)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(owner_id, info_hash) WHERE info_hash != '' DO UPDATE SET
    content_type          = excluded.content_type,
    title                 = excluded.title,
    sort_title            = excluded.sort_title,
    year                  = excluded.year,
    imdb_id               = excluded.imdb_id,
    tmdb_id               = excluded.tmdb_id,
    tvdb_id               = excluded.tvdb_id,
    tvmaze_id             = excluded.tvmaze_id,
    overview              = excluded.overview,
    status                = excluded.status,
    path                  = excluded.path,
    has_file              = excluded.has_file,
    imdb_rating           = excluded.imdb_rating,
    tmdb_rating           = excluded.tmdb_rating,
    metacritic_rating     = excluded.metacritic_rating,
    rotten_tomatoes_rating = excluded.rotten_tomatoes_rating,
    audience_rating       = excluded.audience_rating,
    genres                = excluded.genres,
    source                = excluded.source,
    torrent_count         = excluded.torrent_count
RETURNING id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
          tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
          imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
          genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at`

	return scanTitle(s.db.QueryRowContext(ctx, q,
		p.OwnerID, string(p.ContentType), p.Title, p.SortTitle, p.Year,
		p.IMDbID, p.TMDbID, p.TVDbID, p.TVMazeID,
		p.ArrInstanceID, p.ArrItemID, p.Overview, p.Status, p.Path, p.HasFile,
		p.IMDbRating, p.TMDbRating, p.MetacriticRating, p.RottenTomatoesRating, p.AudienceRating,
		p.Genres, "torrent_client", p.InfoHash, p.TorrentCount, 0, "",
	))
}

// UpsertByTitle inserts or updates a torrent-client-sourced title identified by
// (owner_id, sort_title, content_type).  This groups all torrent hashes for the
// same parsed title into a single library entry.  The ON CONFLICT clause targets
// the partial unique index from migration 085.
func (s *LibraryTitleStore) UpsertByTitle(ctx context.Context, p LibraryTitleUpsertParams) (*LibraryTitle, error) {
	p.Source = "torrent_client"
	const q = `
INSERT INTO library_titles
    (owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id, tvmaze_id,
     arr_instance_id, arr_item_id, overview, status, path, has_file,
     imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
     genres, source, info_hash, torrent_count, episode_count, qualities)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(owner_id, sort_title, content_type) WHERE source = 'torrent_client' DO UPDATE SET
    title                 = excluded.title,
    year                  = CASE WHEN excluded.year IS NOT NULL AND excluded.year > 0 THEN excluded.year ELSE library_titles.year END,
    imdb_id               = CASE WHEN excluded.imdb_id != '' THEN excluded.imdb_id ELSE library_titles.imdb_id END,
    tmdb_id               = CASE WHEN excluded.tmdb_id > 0 THEN excluded.tmdb_id ELSE library_titles.tmdb_id END,
    tvdb_id               = CASE WHEN excluded.tvdb_id > 0 THEN excluded.tvdb_id ELSE library_titles.tvdb_id END,
    tvmaze_id             = CASE WHEN excluded.tvmaze_id > 0 THEN excluded.tvmaze_id ELSE library_titles.tvmaze_id END,
    overview              = CASE WHEN excluded.overview != '' THEN excluded.overview ELSE library_titles.overview END,
    status                = CASE WHEN excluded.status != '' THEN excluded.status ELSE library_titles.status END,
    path                  = CASE WHEN excluded.path != '' THEN excluded.path ELSE library_titles.path END,
    has_file              = excluded.has_file OR library_titles.has_file,
    imdb_rating           = CASE WHEN excluded.imdb_rating > 0 THEN excluded.imdb_rating ELSE library_titles.imdb_rating END,
    tmdb_rating           = CASE WHEN excluded.tmdb_rating > 0 THEN excluded.tmdb_rating ELSE library_titles.tmdb_rating END,
    metacritic_rating     = CASE WHEN excluded.metacritic_rating > 0 THEN excluded.metacritic_rating ELSE library_titles.metacritic_rating END,
    rotten_tomatoes_rating = CASE WHEN excluded.rotten_tomatoes_rating > 0 THEN excluded.rotten_tomatoes_rating ELSE library_titles.rotten_tomatoes_rating END,
    audience_rating       = CASE WHEN excluded.audience_rating > 0 THEN excluded.audience_rating ELSE library_titles.audience_rating END,
    genres                = CASE WHEN excluded.genres != '' THEN excluded.genres ELSE library_titles.genres END,
    source                = excluded.source,
    torrent_count         = excluded.torrent_count,
    episode_count         = excluded.episode_count,
    qualities             = excluded.qualities
RETURNING id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
          tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
          imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
          genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at`

	return scanTitle(s.db.QueryRowContext(ctx, q,
		p.OwnerID, string(p.ContentType), p.Title, p.SortTitle, p.Year,
		p.IMDbID, p.TMDbID, p.TVDbID, p.TVMazeID,
		p.ArrInstanceID, p.ArrItemID, p.Overview, p.Status, p.Path, p.HasFile,
		p.IMDbRating, p.TMDbRating, p.MetacriticRating, p.RottenTomatoesRating, p.AudienceRating,
		p.Genres, "torrent_client", p.InfoHash, p.TorrentCount, p.EpisodeCount, p.Qualities,
	))
}

// Get returns a library title by ID.
func (s *LibraryTitleStore) Get(ctx context.Context, id int) (*LibraryTitle, error) {
	const q = `
SELECT id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
       tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
       imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
       genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at
FROM library_titles WHERE id = ?`
	t, err := scanTitle(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLibraryTitleNotFound
	}
	return t, err
}

// List returns all titles for the given owner, ordered by sort_title.
func (s *LibraryTitleStore) List(ctx context.Context, ownerID int) ([]*LibraryTitle, error) {
	const q = `
SELECT id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
       tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
       imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
       genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at
FROM library_titles WHERE owner_id = ? ORDER BY sort_title, title`
	rows, err := s.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTitles(rows)
}

// ListByContentType returns titles for the given owner filtered by content type.
func (s *LibraryTitleStore) ListByContentType(ctx context.Context, ownerID int, ct LibraryContentType) ([]*LibraryTitle, error) {
	const q = `
SELECT id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
       tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
       imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
       genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at
FROM library_titles WHERE owner_id = ? AND content_type = ? ORDER BY sort_title, title`
	rows, err := s.db.QueryContext(ctx, q, ownerID, string(ct))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTitles(rows)
}

// Search returns titles whose title or sort_title matches a LIKE pattern.
func (s *LibraryTitleStore) Search(ctx context.Context, ownerID int, pattern string) ([]*LibraryTitle, error) {
	const q = `
SELECT id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
       tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
       imdb_rating, tmdb_rating, metacritic_rating, rotten_tomatoes_rating, audience_rating,
       genres, source, info_hash, torrent_count, episode_count, qualities, created_at, updated_at
FROM library_titles
WHERE owner_id = ? AND (title LIKE ? ESCAPE '\' OR sort_title LIKE ? ESCAPE '\')
ORDER BY sort_title, title LIMIT 100`
	like := "%" + escapeLike(pattern) + "%"
	rows, err := s.db.QueryContext(ctx, q, ownerID, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTitles(rows)
}

// Delete removes a library title by ID.
func (s *LibraryTitleStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM library_titles WHERE id = ?`, id)
	return err
}

// DeleteByArrInstance removes all titles synced from the given arr instance.
func (s *LibraryTitleStore) DeleteByArrInstance(ctx context.Context, ownerID, arrInstanceID int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM library_titles WHERE owner_id = ? AND arr_instance_id = ?`,
		ownerID, arrInstanceID)
	return err
}

// UpsertSeason inserts or updates a season row for the given title.
func (s *LibraryTitleStore) UpsertSeason(ctx context.Context, titleID, seasonNumber, episodeCount int, hasFiles bool) error {
	const q = `
INSERT INTO library_seasons (title_id, season_number, episode_count, has_files)
VALUES (?,?,?,?)
ON CONFLICT(title_id, season_number) DO UPDATE SET
    episode_count = excluded.episode_count,
    has_files     = excluded.has_files`
	_, err := s.db.ExecContext(ctx, q, titleID, seasonNumber, episodeCount, hasFiles)
	return err
}

// ListSeasons returns all seasons for the given library title.
func (s *LibraryTitleStore) ListSeasons(ctx context.Context, titleID int) ([]LibrarySeason, error) {
	const q = `SELECT id, title_id, season_number, episode_count, has_files
               FROM library_seasons WHERE title_id = ? ORDER BY season_number`
	rows, err := s.db.QueryContext(ctx, q, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LibrarySeason
	for rows.Next() {
		var ssn LibrarySeason
		if err := rows.Scan(&ssn.ID, &ssn.TitleID, &ssn.SeasonNumber, &ssn.EpisodeCount, &ssn.HasFiles); err != nil {
			return nil, err
		}
		out = append(out, ssn)
	}
	return out, rows.Err()
}

// ─── LibraryRuleStore ────────────────────────────────────────────────────────

// LibraryRuleStore manages library rules in SQLite.
type LibraryRuleStore struct {
	db dbinterface.Querier
}

// NewLibraryRuleStore creates a new LibraryRuleStore.
func NewLibraryRuleStore(db dbinterface.Querier) *LibraryRuleStore {
	return &LibraryRuleStore{db: db}
}

// Create inserts a new library rule.
func (s *LibraryRuleStore) Create(ctx context.Context, r *LibraryRule) (*LibraryRule, error) {
	const q = `
INSERT INTO library_rules (owner_id, name, enabled, sort_order, content_types, expr_filter, action)
VALUES (?,?,?,?,?,?,?)
RETURNING id, owner_id, name, enabled, sort_order, content_types, expr_filter, action, created_at, updated_at`
	return scanRule(s.db.QueryRowContext(ctx, q,
		r.OwnerID, r.Name, r.Enabled, r.SortOrder,
		joinContentTypes(r.ContentTypes), r.ExprFilter, string(r.Action),
	))
}

// Get returns a rule by ID.
func (s *LibraryRuleStore) Get(ctx context.Context, id int) (*LibraryRule, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, expr_filter, action, created_at, updated_at
               FROM library_rules WHERE id = ?`
	rule, err := scanRule(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLibraryRuleNotFound
	}
	return rule, err
}

// ListEnabled returns all enabled rules for the given owner, sorted by sort_order.
func (s *LibraryRuleStore) ListEnabled(ctx context.Context, ownerID int) ([]*LibraryRule, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, expr_filter, action, created_at, updated_at
               FROM library_rules WHERE owner_id = ? AND enabled = 1 ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

// List returns all rules for the owner regardless of enabled state.
func (s *LibraryRuleStore) List(ctx context.Context, ownerID int) ([]*LibraryRule, error) {
	const q = `SELECT id, owner_id, name, enabled, sort_order, content_types, expr_filter, action, created_at, updated_at
               FROM library_rules WHERE owner_id = ? ORDER BY sort_order, id`
	rows, err := s.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

// Update replaces mutable fields of a rule.
func (s *LibraryRuleStore) Update(ctx context.Context, r *LibraryRule) (*LibraryRule, error) {
	const q = `
UPDATE library_rules SET name=?, enabled=?, sort_order=?, content_types=?, expr_filter=?, action=?
WHERE id = ? AND owner_id = ?
RETURNING id, owner_id, name, enabled, sort_order, content_types, expr_filter, action, created_at, updated_at`
	updated, err := scanRule(s.db.QueryRowContext(ctx, q,
		r.Name, r.Enabled, r.SortOrder,
		joinContentTypes(r.ContentTypes), r.ExprFilter, string(r.Action),
		r.ID, r.OwnerID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLibraryRuleNotFound
	}
	return updated, err
}

// Delete removes a rule by ID.
func (s *LibraryRuleStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM library_rules WHERE id = ?`, id)
	return err
}

// ─── scan helpers ────────────────────────────────────────────────────────────

func scanTitle(row *sql.Row) (*LibraryTitle, error) {
	var t LibraryTitle
	var ct string
	err := row.Scan(
		&t.ID, &t.OwnerID, &ct, &t.Title, &t.SortTitle, &t.Year,
		&t.IMDbID, &t.TMDbID, &t.TVDbID, &t.TVMazeID,
		&t.ArrInstanceID, &t.ArrItemID, &t.Overview, &t.Status, &t.Path,
		&t.HasFile,
		&t.IMDbRating, &t.TMDbRating, &t.MetacriticRating, &t.RottenTomatoesRating, &t.AudienceRating,
		&t.Genres, &t.Source, &t.InfoHash, &t.TorrentCount, &t.EpisodeCount, &t.Qualities,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.ContentType = LibraryContentType(ct)
	return &t, nil
}

func scanTitles(rows *sql.Rows) ([]*LibraryTitle, error) {
	var out []*LibraryTitle
	for rows.Next() {
		var t LibraryTitle
		var ct string
		err := rows.Scan(
			&t.ID, &t.OwnerID, &ct, &t.Title, &t.SortTitle, &t.Year,
			&t.IMDbID, &t.TMDbID, &t.TVDbID, &t.TVMazeID,
			&t.ArrInstanceID, &t.ArrItemID, &t.Overview, &t.Status, &t.Path,
			&t.HasFile,
			&t.IMDbRating, &t.TMDbRating, &t.MetacriticRating, &t.RottenTomatoesRating, &t.AudienceRating,
			&t.Genres, &t.Source, &t.InfoHash, &t.TorrentCount, &t.EpisodeCount, &t.Qualities,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		t.ContentType = LibraryContentType(ct)
		out = append(out, &t)
	}
	return out, rows.Err()
}

func scanRule(row *sql.Row) (*LibraryRule, error) {
	var r LibraryRule
	var action, contentTypes string
	err := row.Scan(
		&r.ID, &r.OwnerID, &r.Name, &r.Enabled, &r.SortOrder,
		&contentTypes, &r.ExprFilter, &action, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.Action = LibraryAction(action)
	r.ContentTypes = splitContentTypes(contentTypes)
	return &r, nil
}

func scanRules(rows *sql.Rows) ([]*LibraryRule, error) {
	var out []*LibraryRule
	for rows.Next() {
		var r LibraryRule
		var action, contentTypes string
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.Name, &r.Enabled, &r.SortOrder,
			&contentTypes, &r.ExprFilter, &action, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.Action = LibraryAction(action)
		r.ContentTypes = splitContentTypes(contentTypes)
		out = append(out, &r)
	}
	return out, rows.Err()
}

func splitContentTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func joinContentTypes(types []string) string {
	return strings.Join(types, ",")
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
