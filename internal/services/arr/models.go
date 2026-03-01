// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package arr provides types and helpers for the *arr integration layer.
// Raw API response structs for Sonarr and Radarr are provided by the
// golift/starr sub-packages; this file holds rui-specific composite types.
package arr

import "github.com/autogrr/rui/internal/models"

// SonarrSeries contains the series-level external IDs returned by Sonarr's
// parse endpoint.
type SonarrSeries struct {
	TVDbID   int    `json:"tvdbId"`
	TVMazeID int    `json:"tvMazeId"`
	TMDbID   int    `json:"tmdbId"`
	IMDbID   string `json:"imdbId"`
}

// SonarrParseResponse wraps the Sonarr /api/v3/parse response, exposing
// the series field that the golift/starr ParseOutput type does not export.
type SonarrParseResponse struct {
	Series *SonarrSeries `json:"series"`
}

// ExtractExternalIDs converts the Sonarr parse response into a unified
// ExternalIDs model. Returns nil when the Series is absent or all IDs are
// zero/empty.
func (r *SonarrParseResponse) ExtractExternalIDs() *models.ExternalIDs {
	if r.Series == nil {
		return nil
	}
	ids := &models.ExternalIDs{}
	if r.Series.TVDbID > 0 {
		ids.TVDbID = r.Series.TVDbID
	}
	if r.Series.TVMazeID > 0 {
		ids.TVMazeID = r.Series.TVMazeID
	}
	if r.Series.TMDbID > 0 {
		ids.TMDbID = r.Series.TMDbID
	}
	if r.Series.IMDbID != "" && r.Series.IMDbID != "0" {
		ids.IMDbID = r.Series.IMDbID
	}
	if ids.IsEmpty() {
		return nil
	}
	return ids
}

// RadarrMovie holds the movie-level external IDs from a Radarr response.
type RadarrMovie struct {
	TMDbID int    `json:"tmdbId"`
	IMDbID string `json:"imdbId"`
}

// RadarrParsedMovieInfo holds the parsed movie info IDs from a Radarr response.
type RadarrParsedMovieInfo struct {
	TMDbID int    `json:"tmdbId"`
	IMDbID string `json:"imdbId"`
}

// RadarrParseResponse wraps the Radarr lookup response, containing both
// parsed movie info and the matched movie record.
type RadarrParseResponse struct {
	ParsedMovieInfo *RadarrParsedMovieInfo `json:"parsedMovieInfo"`
	Movie           *RadarrMovie           `json:"movie"`
}

// ExtractExternalIDs converts the Radarr response into a unified ExternalIDs
// model. Movie fields take precedence over ParsedMovieInfo, with fallback for
// empty values. Returns nil when both are absent or all IDs are zero/empty.
func (r *RadarrParseResponse) ExtractExternalIDs() *models.ExternalIDs {
	if r.Movie == nil && r.ParsedMovieInfo == nil {
		return nil
	}

	ids := &models.ExternalIDs{}

	// Start with ParsedMovieInfo as fallback
	if r.ParsedMovieInfo != nil {
		if r.ParsedMovieInfo.TMDbID > 0 {
			ids.TMDbID = r.ParsedMovieInfo.TMDbID
		}
		if r.ParsedMovieInfo.IMDbID != "" && r.ParsedMovieInfo.IMDbID != "0" {
			ids.IMDbID = r.ParsedMovieInfo.IMDbID
		}
	}

	// Movie takes precedence
	if r.Movie != nil {
		if r.Movie.TMDbID > 0 {
			ids.TMDbID = r.Movie.TMDbID
		}
		if r.Movie.IMDbID != "" && r.Movie.IMDbID != "0" {
			ids.IMDbID = r.Movie.IMDbID
		}
	}

	if ids.IsEmpty() {
		return nil
	}
	return ids
}
