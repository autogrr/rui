// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package library provides rui's native library management: syncing titles from
// *arr information providers, matching incoming release names with rls, and
// evaluating expr-lang rules to decide what action to take.
package library

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/moistari/rls"
	"github.com/rs/zerolog/log"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/pkg/releases"
)

// ArrProvider is the subset of arr.Service used by the library service.
// Keeping this interface small (≤5 methods) satisfies the project linter rules.
type ArrProvider interface {
	GetAllSeries(ctx context.Context, instanceID int) ([]*sonarr.Series, error)
	GetAllMovies(ctx context.Context, instanceID int) ([]*radarr.Movie, error)
}

// SyncResult summarises a library sync run.
type SyncResult struct {
	InstanceID int    `json:"instance_id"`
	Upserted   int    `json:"upserted"`
	Deleted    int    `json:"deleted"` // titles removed from arr since last sync
	Errors     int    `json:"errors"`
	Duration   string `json:"duration"`
}

// RuleResult is the outcome of running library rules against a match.
type RuleResult struct {
	Action   models.LibraryAction
	RuleID   int
	RuleName string
}

// Service is the rui native library manager.
type Service struct {
	titleStore *models.LibraryTitleStore
	ruleStore  *models.LibraryRuleStore
	arrSvc     ArrProvider
	parser     *releases.Parser
	matcher    *Matcher

	// expr program cache: rule ID → compiled *vm.Program
	progCache sync.Map
}

// New creates a new library Service.
func New(
	titleStore *models.LibraryTitleStore,
	ruleStore *models.LibraryRuleStore,
	arrSvc ArrProvider,
) *Service {
	return &Service{
		titleStore: titleStore,
		ruleStore:  ruleStore,
		arrSvc:     arrSvc,
		parser:     releases.NewDefaultParser(),
		matcher:    NewMatcher(),
	}
}

// SyncSeries syncs all series from a Sonarr/Whisparr instance into the library.
func (s *Service) SyncSeries(ctx context.Context, ownerID, instanceID int) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{InstanceID: instanceID}

	series, err := s.arrSvc.GetAllSeries(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("fetch series from instance %d: %w", instanceID, err)
	}

	iid := instanceID
	for _, sv := range series {
		if sv == nil || sv.Title == "" {
			continue
		}
		year := sv.Year
		imdbID := sv.ImdbID
		aid := int(sv.ID)
		p := models.LibraryTitleUpsertParams{
			OwnerID:       ownerID,
			ContentType:   arrSeriesContentType(sv),
			Title:         sv.Title,
			SortTitle:     sv.SortTitle,
			Year:          &year,
			IMDbID:        imdbID,
			TVDbID:        int(sv.TvdbID),
			TVMazeID:      int(sv.TvMazeID),
			ArrInstanceID: &iid,
			ArrItemID:     &aid,
			Overview:      sv.Overview,
			Status:        sv.Status,
			Path:          sv.Path,
		}
		title, err := s.titleStore.Upsert(ctx, p)
		if err != nil {
			log.Warn().Err(err).Str("series", sv.Title).Msg("[LIBRARY] upsert series failed")
			result.Errors++
			continue
		}
		result.Upserted++

		if err := s.syncSeasons(ctx, title.ID, sv.Seasons); err != nil {
			log.Warn().Err(err).Int("titleID", title.ID).Msg("[LIBRARY] sync seasons failed")
		}
	}

	result.Duration = time.Since(start).Round(time.Millisecond).String()
	return result, nil
}

// SyncMovies syncs all movies from a Radarr instance into the library.
func (s *Service) SyncMovies(ctx context.Context, ownerID, instanceID int) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{InstanceID: instanceID}

	movies, err := s.arrSvc.GetAllMovies(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("fetch movies from instance %d: %w", instanceID, err)
	}

	iid := instanceID
	for _, mv := range movies {
		if mv == nil || mv.Title == "" {
			continue
		}
		year := mv.Year
		aid := int(mv.ID)
		p := models.LibraryTitleUpsertParams{
			OwnerID:       ownerID,
			ContentType:   models.LibraryContentTypeMovie,
			Title:         mv.Title,
			SortTitle:     mv.SortTitle,
			Year:          &year,
			IMDbID:        mv.ImdbID,
			TMDbID:        int(mv.TmdbID),
			ArrInstanceID: &iid,
			ArrItemID:     &aid,
			Overview:      mv.Overview,
			Status:        mv.Status,
			Path:          mv.Path,
			HasFile:       mv.HasFile,
		}
		if _, err := s.titleStore.Upsert(ctx, p); err != nil {
			log.Warn().Err(err).Str("movie", mv.Title).Msg("[LIBRARY] upsert movie failed")
			result.Errors++
			continue
		}
		result.Upserted++
	}

	result.Duration = time.Since(start).Round(time.Millisecond).String()
	return result, nil
}

// syncSeasons upserts season rows for a TV title.
func (s *Service) syncSeasons(ctx context.Context, titleID int, seasons []*sonarr.Season) error {
	for _, ssn := range seasons {
		if ssn == nil {
			continue
		}
		stats := ssn.Statistics
		hasFiles := false
		epCount := 0
		if stats != nil {
			epCount = stats.TotalEpisodeCount
			hasFiles = stats.EpisodeFileCount > 0
		}
		if err := s.titleStore.UpsertSeason(ctx, titleID, ssn.SeasonNumber, epCount, hasFiles); err != nil {
			return err
		}
	}
	return nil
}

// Match parses a release name and finds the best-matching library titles for
// the given owner.  Results are sorted by score descending.
func (s *Service) Match(ctx context.Context, ownerID int, releaseName string) ([]TitleMatch, error) {
	parsed := s.parser.Parse(releaseName)
	if parsed.Title == "" {
		return nil, nil
	}

	all, err := s.titleStore.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	matches := s.matcher.Match(parsed, all)
	// Enrich each match context with full release info.
	for i := range matches {
		matches[i].Context = buildMatchContext(parsed, matches[i].Title, matches[i].Score)
	}
	return matches, nil
}

// EvalRules evaluates all enabled library rules for the given owner against
// the provided MatchContext and returns the first matching rule's action.
// Returns nil when no rule matches.
func (s *Service) EvalRules(ctx context.Context, ownerID int, mc MatchContext) (*RuleResult, error) {
	rules, err := s.ruleStore.ListEnabled(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if !ruleAppliesToContentType(rule, mc.ContentType) {
			continue
		}
		if rule.ExprFilter == "" {
			return &RuleResult{Action: rule.Action, RuleID: rule.ID, RuleName: rule.Name}, nil
		}
		prog, err := s.compileRule(rule)
		if err != nil {
			log.Warn().Err(err).Int("ruleID", rule.ID).Str("rule", rule.Name).Msg("[LIBRARY] rule compile failed")
			continue
		}
		out, err := vm.Run(prog, mc)
		if err != nil {
			log.Warn().Err(err).Int("ruleID", rule.ID).Msg("[LIBRARY] rule eval failed")
			continue
		}
		if matched, ok := out.(bool); ok && matched {
			return &RuleResult{Action: rule.Action, RuleID: rule.ID, RuleName: rule.Name}, nil
		}
	}
	return nil, nil
}

// compileRule returns a cached compiled expr program for the given rule.
func (s *Service) compileRule(rule *models.LibraryRule) (*vm.Program, error) {
	type cached struct {
		prog   *vm.Program
		filter string
	}
	if v, ok := s.progCache.Load(rule.ID); ok {
		if c, ok := v.(cached); ok && c.filter == rule.ExprFilter {
			return c.prog, nil
		}
	}
	prog, err := expr.Compile(rule.ExprFilter,
		expr.AsBool(),
		expr.Env(MatchContext{}),
	)
	if err != nil {
		return nil, err
	}
	s.progCache.Store(rule.ID, cached{prog: prog, filter: rule.ExprFilter})
	return prog, nil
}

// InvalidateRuleCache removes a cached compiled program for the given rule ID.
func (s *Service) InvalidateRuleCache(ruleID int) {
	s.progCache.Delete(ruleID)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func buildMatchContext(parsed *rls.Release, title *models.LibraryTitle, score int) MatchContext {
	mc := MatchContext{
		Title:         parsed.Title,
		Year:          parsed.Year,
		Season:        parsed.Series,
		Episode:       parsed.Episode,
		Group:         parsed.Group,
		Source:        parsed.Source,
		Resolution:    parsed.Resolution,
		ContentType:   releaseContentType(parsed),
		LibraryTitle:  title.Title,
		LibraryIMDbID: title.IMDbID,
		LibraryTMDbID: title.TMDbID,
		LibraryTVDbID: title.TVDbID,
		HasFile:       title.HasFile,
		Score:         score,
	}
	if title.Year != nil {
		mc.LibraryYear = *title.Year
	}
	return mc
}

func releaseContentType(parsed *rls.Release) string {
	switch parsed.Type {
	case rls.Movie:
		return "movie"
	case rls.Episode, rls.Series:
		return "tv"
	default:
		if parsed.Series > 0 || parsed.Episode > 0 {
			return "tv"
		}
		return "unknown"
	}
}

func arrSeriesContentType(sv *sonarr.Series) models.LibraryContentType {
	if sv.SeriesType == "anime" {
		return models.LibraryContentTypeAnime
	}
	return models.LibraryContentTypeTV
}

func ruleAppliesToContentType(rule *models.LibraryRule, ct string) bool {
	if len(rule.ContentTypes) == 0 {
		return true
	}
	for _, t := range rule.ContentTypes {
		if strings.EqualFold(t, ct) {
			return true
		}
	}
	return false
}
