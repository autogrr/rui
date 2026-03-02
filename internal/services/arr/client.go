// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package arr

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golift.io/starr"
	"golift.io/starr/lidarr"
	"golift.io/starr/radarr"
	"golift.io/starr/readarr"
	"golift.io/starr/sonarr"

	"github.com/autogrr/rui/internal/models"
)

const defaultTimeout = 15 * time.Second

// Client wraps golift/starr sub-clients for interacting with *arr applications.
// Sonarr and Whisparr use the Sonarr v3 API surface; Radarr, Lidarr, Readarr
// each use their own typed sub-client.
type Client struct {
	instanceType models.ArrInstanceType
	cfg          *starr.Config
	timeout      time.Duration
}

// NewClient creates a golift/starr-backed Client for the given instance type.
func NewClient(baseURL, apiKey string, basicUser, basicPass *string, instanceType models.ArrInstanceType, timeoutSeconds int) *Client {
	timeout := defaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	cfg := &starr.Config{
		APIKey:   apiKey,
		URL:      strings.TrimRight(baseURL, "/"),
		Client:   starr.Client(timeout, false),
		HTTPUser: strVal(basicUser),
		HTTPPass: strVal(basicPass),
	}

	return &Client{instanceType: instanceType, cfg: cfg, timeout: timeout}
}

// Ping tests connectivity by checking /ping and then /api/v3/system/status.
func (c *Client) Ping(ctx context.Context) error {
	switch {
	case c.instanceType.IsSonarrCompatible():
		return c.pingSonarr(ctx)
	case c.instanceType.IsRadarrCompatible():
		return c.pingRadarr(ctx)
	case c.instanceType == models.ArrInstanceTypeLidarr:
		return c.pingLidarr(ctx)
	case c.instanceType == models.ArrInstanceTypeReadarr:
		return c.pingReadarr(ctx)
	default:
		return fmt.Errorf("unsupported instance type: %s", c.instanceType)
	}
}

func (c *Client) pingSonarr(ctx context.Context) error {
	s := sonarr.New(c.cfg)
	if err := s.PingContext(ctx); err != nil {
		return fmt.Errorf("sonarr ping: %w", err)
	}
	status, err := s.GetSystemStatusContext(ctx)
	if err != nil {
		return fmt.Errorf("sonarr system status: %w", err)
	}
	if status.AppName == "" {
		return fmt.Errorf("sonarr returned empty AppName")
	}
	return nil
}

func (c *Client) pingRadarr(ctx context.Context) error {
	r := radarr.New(c.cfg)
	if err := r.PingContext(ctx); err != nil {
		return fmt.Errorf("radarr ping: %w", err)
	}
	status, err := r.GetSystemStatusContext(ctx)
	if err != nil {
		return fmt.Errorf("radarr system status: %w", err)
	}
	if status.AppName == "" {
		return fmt.Errorf("radarr returned empty AppName")
	}
	return nil
}

func (c *Client) pingLidarr(ctx context.Context) error {
	l := lidarr.New(c.cfg)
	if err := l.PingContext(ctx); err != nil {
		return fmt.Errorf("lidarr ping: %w", err)
	}
	return nil
}

func (c *Client) pingReadarr(ctx context.Context) error {
	r := readarr.New(c.cfg)
	if err := r.PingContext(ctx); err != nil {
		return fmt.Errorf("readarr ping: %w", err)
	}
	return nil
}

// ParseTitle calls the arr parse endpoint to resolve a release name into
// external IDs. Returns nil IDs when no match is found (not an error).
func (c *Client) ParseTitle(ctx context.Context, title string) (*models.ExternalIDs, error) {
	switch {
	case c.instanceType.IsSonarrCompatible():
		return c.parseSonarr(ctx, title)
	case c.instanceType.IsRadarrCompatible():
		return c.parseRadarr(ctx, title)
	default:
		return nil, nil // lidarr/readarr do not use title→IDs in the same way
	}
}

// parseSonarr uses golift/starr's Sonarr client with a custom response
// struct that captures the series field (not exposed by sonarr.ParseOutput).
func (c *Client) parseSonarr(ctx context.Context, title string) (*models.ExternalIDs, error) {
	type sonarrSeriesIDs struct {
		TVDbID   int    `json:"tvdbId"`
		TVMazeID int    `json:"tvMazeId"`
		TMDbID   int    `json:"tmdbId"`
		IMDbID   string `json:"imdbId"`
	}
	var out struct {
		Series *sonarrSeriesIDs `json:"series"`
	}
	req := starr.Request{
		URI:   "v3/parse",
		Query: url.Values{"title": {title}},
	}
	if err := sonarr.New(c.cfg).GetInto(ctx, req, &out); err != nil {
		return nil, fmt.Errorf("sonarr parse %q: %w", title, err)
	}
	if out.Series == nil {
		return nil, nil
	}
	ids := &models.ExternalIDs{}
	if out.Series.TVDbID > 0 {
		ids.TVDbID = out.Series.TVDbID
	}
	if out.Series.TVMazeID > 0 {
		ids.TVMazeID = out.Series.TVMazeID
	}
	if out.Series.TMDbID > 0 {
		ids.TMDbID = out.Series.TMDbID
	}
	if out.Series.IMDbID != "" && out.Series.IMDbID != "0" {
		ids.IMDbID = out.Series.IMDbID
	}
	if ids.IsEmpty() {
		return nil, nil
	}
	return ids, nil
}

// parseRadarr uses Radarr's Lookup endpoint as a parse fallback (Radarr has
// no dedicated parse endpoint). The first lookup result is used.
func (c *Client) parseRadarr(ctx context.Context, title string) (*models.ExternalIDs, error) {
	movies, err := radarr.New(c.cfg).LookupContext(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("radarr lookup %q: %w", title, err)
	}
	if len(movies) == 0 {
		return nil, nil
	}
	m := movies[0]
	ids := &models.ExternalIDs{}
	if m.TmdbID > 0 {
		ids.TMDbID = int(m.TmdbID)
	}
	if m.ImdbID != "" && m.ImdbID != "0" {
		ids.IMDbID = m.ImdbID
	}
	if ids.IsEmpty() {
		return nil, nil
	}
	return ids, nil
}

// GetAllSeries returns every series known to a Sonarr-compatible instance.
func (c *Client) GetAllSeries(ctx context.Context) ([]*sonarr.Series, error) {
	if !c.instanceType.IsSonarrCompatible() {
		return nil, fmt.Errorf("GetAllSeries: not supported for %s", c.instanceType)
	}
	series, err := sonarr.New(c.cfg).GetAllSeriesContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sonarr GetAllSeries: %w", err)
	}
	return series, nil
}

// GetAllMovies returns every movie known to a Radarr instance.
func (c *Client) GetAllMovies(ctx context.Context) ([]*radarr.Movie, error) {
	if !c.instanceType.IsRadarrCompatible() {
		return nil, fmt.Errorf("GetAllMovies: not supported for %s", c.instanceType)
	}
	movies, err := radarr.New(c.cfg).GetMovieContext(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("radarr GetAllMovies: %w", err)
	}
	return movies, nil
}

// SearchSeries performs a title-based series lookup on a Sonarr-compatible
// instance against the upstream metadata provider.
func (c *Client) SearchSeries(ctx context.Context, term string) ([]*sonarr.Series, error) {
	if !c.instanceType.IsSonarrCompatible() {
		return nil, fmt.Errorf("SearchSeries: not supported for %s", c.instanceType)
	}
	results, err := sonarr.New(c.cfg).GetSeriesLookupContext(ctx, term, 0)
	if err != nil {
		return nil, fmt.Errorf("sonarr series lookup %q: %w", term, err)
	}
	return results, nil
}

// SearchMovies performs a title-based movie lookup on a Radarr instance
// against the upstream metadata provider.
func (c *Client) SearchMovies(ctx context.Context, term string) ([]*radarr.Movie, error) {
	if !c.instanceType.IsRadarrCompatible() {
		return nil, fmt.Errorf("SearchMovies: not supported for %s", c.instanceType)
	}
	results, err := radarr.New(c.cfg).LookupContext(ctx, term)
	if err != nil {
		return nil, fmt.Errorf("radarr movie lookup %q: %w", term, err)
	}
	return results, nil
}

// InstanceType returns the configured instance type.
func (c *Client) InstanceType() models.ArrInstanceType { return c.instanceType }

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string { return c.cfg.URL }

// GetMovieMetadata fetches enriched metadata (ratings, genres, IDs) for a movie
// title from a Radarr-compatible instance using the lookup endpoint.
// Returns nil when no match is found.
func (c *Client) GetMovieMetadata(ctx context.Context, title string) (*MediaMetadata, error) {
	if !c.instanceType.IsRadarrCompatible() {
		return nil, fmt.Errorf("GetMovieMetadata: not supported for %s", c.instanceType)
	}
	movies, err := radarr.New(c.cfg).LookupContext(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("radarr metadata lookup %q: %w", title, err)
	}
	if len(movies) == 0 {
		return nil, nil
	}
	m := movies[0]
	meta := &MediaMetadata{
		Title:     m.Title,
		SortTitle: m.SortTitle,
		Year:      m.Year,
		Overview:  m.Overview,
		Status:    m.Status,
		Genres:    strings.Join(m.Genres, ", "),
	}
	if m.TmdbID > 0 {
		meta.TMDbID = int(m.TmdbID)
	}
	if m.ImdbID != "" && m.ImdbID != "0" {
		meta.IMDbID = m.ImdbID
	}
	meta.TMDbRating = openRatingVal(m.Ratings, "tmdb")
	meta.IMDbRating = openRatingVal(m.Ratings, "imdb")
	// Metacritic is on a 0–100 scale; keep raw (stored as-is in DB).
	if v := openRatingVal(m.Ratings, "metacritic"); v > 0 {
		meta.MetacriticRating = v
	}
	// Rotten Tomatoes is a percentage (0–100); keep raw.
	if v := openRatingVal(m.Ratings, "rottenTomatoes"); v > 0 {
		meta.RottenTomatoesRating = v
	}
	return meta, nil
}

// GetSeriesMetadata fetches enriched metadata for a TV series from a
// Sonarr-compatible instance using the lookup endpoint.
// Returns nil when no match is found.
func (c *Client) GetSeriesMetadata(ctx context.Context, title string) (*MediaMetadata, error) {
	if !c.instanceType.IsSonarrCompatible() {
		return nil, fmt.Errorf("GetSeriesMetadata: not supported for %s", c.instanceType)
	}
	results, err := sonarr.New(c.cfg).GetSeriesLookupContext(ctx, title, 0)
	if err != nil {
		return nil, fmt.Errorf("sonarr metadata lookup %q: %w", title, err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	sv := results[0]
	meta := &MediaMetadata{
		Title:     sv.Title,
		SortTitle: sv.SortTitle,
		Year:      sv.Year,
		Overview:  sv.Overview,
		Status:    sv.Status,
		Genres:    strings.Join(sv.Genres, ", "),
	}
	if sv.TvdbID > 0 {
		meta.TVDbID = int(sv.TvdbID)
	}
	if sv.TvMazeID > 0 {
		meta.TVMazeID = int(sv.TvMazeID)
	}
	if sv.ImdbID != "" && sv.ImdbID != "0" {
		meta.IMDbID = sv.ImdbID
	}
	// Sonarr exposes a single *starr.Ratings (not OpenRatings).
	if sv.Ratings != nil && sv.Ratings.Value > 0 {
		meta.AudienceRating = sv.Ratings.Value
	}
	return meta, nil
}

// openRatingVal extracts a rating value from an OpenRatings map by key.
// Returns 0 when the key is absent.
func openRatingVal(r starr.OpenRatings, key string) float64 {
	if r == nil {
		return 0
	}
	v, ok := r[key]
	if !ok {
		return 0
	}
	return v.Value
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
