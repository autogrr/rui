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
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
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
}

// LibraryTitleStore manages library titles and seasons in SQLite.
type LibraryTitleStore struct {
	db dbinterface.Querier
}

// NewLibraryTitleStore creates a new LibraryTitleStore.
func NewLibraryTitleStore(db dbinterface.Querier) *LibraryTitleStore {
	return &LibraryTitleStore{db: db}
}

// Upsert inserts or updates a title identified by (owner_id, arr_instance_id, arr_item_id).
// When arr_instance_id or arr_item_id is nil, a plain INSERT is performed instead.
func (s *LibraryTitleStore) Upsert(ctx context.Context, p LibraryTitleUpsertParams) (*LibraryTitle, error) {
	const q = `
INSERT INTO library_titles
    (owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
     tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(owner_id, arr_instance_id, arr_item_id) DO UPDATE SET
    content_type   = excluded.content_type,
    title          = excluded.title,
    sort_title     = excluded.sort_title,
    year           = excluded.year,
    imdb_id        = excluded.imdb_id,
    tmdb_id        = excluded.tmdb_id,
    tvdb_id        = excluded.tvdb_id,
    tvmaze_id      = excluded.tvmaze_id,
    overview       = excluded.overview,
    status         = excluded.status,
    path           = excluded.path,
    has_file       = excluded.has_file
RETURNING id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id,
          tvdb_id, tvmaze_id, arr_instance_id, arr_item_id, overview, status, path,
          has_file, created_at, updated_at`

	return scanTitle(s.db.QueryRowContext(ctx, q,
		p.OwnerID, string(p.ContentType), p.Title, p.SortTitle, p.Year,
		p.IMDbID, p.TMDbID, p.TVDbID, p.TVMazeID,
		p.ArrInstanceID, p.ArrItemID, p.Overview, p.Status, p.Path, p.HasFile,
	))
}

// Get returns a library title by ID.
func (s *LibraryTitleStore) Get(ctx context.Context, id int) (*LibraryTitle, error) {
	const q = `
SELECT id, owner_id, content_type, title, sort_title, year, imdb_id, tmdb_id, tvdb_id,
       tvmaze_id, arr_instance_id, arr_item_id, overview, status, path, has_file,
       created_at, updated_at
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
       created_at, updated_at
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
       created_at, updated_at
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
       created_at, updated_at
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
		&t.HasFile, &t.CreatedAt, &t.UpdatedAt,
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
			&t.HasFile, &t.CreatedAt, &t.UpdatedAt,
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
