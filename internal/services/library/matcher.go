// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package library

import (
	"strings"
	"unicode"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/moistari/rls"

	"github.com/autogrr/rui/internal/models"
)

// MatchContext is the environment passed to expr-lang library rules.
// All fields should be exported (expr accesses them by name).
type MatchContext struct {
	// ── Release fields from rls ──────────────────────────────────────────────
	Title       string // parsed title from the release name
	Year        int    // release year (0 if not detected)
	Season      int    // season number (0 if not a TV episode)
	Episode     int    // episode number (0 if not a single episode)
	Group       string // release group tag
	Source      string // e.g. "WEB-DL", "Blu-ray"
	Resolution  string // e.g. "1080p", "4K"
	ContentType string // "movie", "tv", "anime", "music", "book", "unknown"

	// ── Library title fields ─────────────────────────────────────────────────
	LibraryTitle  string // canonical title from the library entry
	LibraryYear   int    // year of the library entry (0 if unknown)
	LibraryIMDbID string // IMDb identifier
	LibraryTMDbID int    // TMDB identifier
	LibraryTVDbID int    // TVDB identifier
	HasFile       bool   // whether the arr reports a local file for this title

	// ── Match quality ────────────────────────────────────────────────────────
	Score int // 0–100 title similarity score; 100 = exact match
}

// TitleMatch pairs a library title with its relevance score.
type TitleMatch struct {
	Title *models.LibraryTitle
	Score int
	// Context is pre-populated for rule evaluation.
	Context MatchContext
}

// Matcher matches a parsed rls release against a slice of library titles.
type Matcher struct{}

// NewMatcher creates a Matcher.
func NewMatcher() *Matcher { return &Matcher{} }

// Match finds library titles similar to the parsed release.  Candidates are
// ranked by title similarity; exact matches score 100, fuzzy matches score
// 60–99.  Year mismatches (if both sides have a year) drop the score by 10.
// Only candidates with score ≥ 50 are returned.
func (m *Matcher) Match(parsed *rls.Release, candidates []*models.LibraryTitle) []TitleMatch {
	normRelease := normalizeTitle(parsed.Title)
	if normRelease == "" {
		return nil
	}

	var results []TitleMatch
	for _, t := range candidates {
		score := titleScore(normRelease, t.Title, t.SortTitle)
		if score < 50 {
			continue
		}
		// Year penalty: if both sides declare a year and they differ, reduce score.
		if parsed.Year > 0 && t.Year != nil && *t.Year > 0 && parsed.Year != *t.Year {
			score -= 10
		}
		if score < 50 {
			continue
		}
		results = append(results, TitleMatch{
			Title: t,
			Score: score,
		})
	}
	// Sort by score descending (insertion sort for small slices).
	sortByScore(results)
	return results
}

// titleScore computes a 0–100 similarity between a normalised release title and
// a library title's raw and sort-title variants.
func titleScore(normRelease, rawTitle, sortTitle string) int {
	normLib := normalizeTitle(rawTitle)
	normSort := normalizeTitle(sortTitle)

	if normRelease == normLib || normRelease == normSort {
		return 100
	}

	// Check substring containment (covers "The Dark Knight" vs "Dark Knight").
	if strings.Contains(normLib, normRelease) || strings.Contains(normRelease, normLib) {
		return 88
	}

	// Fuzzy match.
	if fuzzy.MatchFold(normLib, normRelease) {
		return 75
	}
	if fuzzy.MatchFold(normSort, normRelease) {
		return 70
	}

	// Levenshtein-based rank from fuzzysearch.
	rank := fuzzy.RankMatch(normRelease, normLib)
	if rank >= 0 {
		// rank 0 = perfect, higher = worse; cap at 40 to stay ≥ 50 threshold.
		capped := 65 - rank
		if capped > 65 {
			capped = 65
		}
		return capped
	}

	return 0
}

// normalizeTitle converts a title to lowercase, collapses whitespace, and
// removes leading "the "/"a "/"an " articles and common punctuation so that
// "The Dark Knight (2008)" and "dark knight" both normalise to "dark knight".
func normalizeTitle(s string) string {
	// Remove parenthetical year if present: "Title (2020)" → "Title"
	if idx := strings.LastIndex(s, "("); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	// Lowercase.
	s = strings.ToLower(s)
	// Remove punctuation except spaces.
	s = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return ' '
		}
		return r
	}, s)
	// Collapse whitespace.
	parts := strings.Fields(s)
	// Strip leading articles.
	if len(parts) > 0 {
		switch parts[0] {
		case "the", "a", "an":
			parts = parts[1:]
		}
	}
	return strings.Join(parts, " ")
}

func sortByScore(m []TitleMatch) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].Score > m[j-1].Score; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}
