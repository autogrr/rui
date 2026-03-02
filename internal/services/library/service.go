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

	qbt "github.com/autogrr/go-qbittorrent"
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

// MetadataProvider resolves enriched metadata (ratings, genres) for a title.
// arr.Service implements this interface via LookupMetadata.
type MetadataProvider interface {
	LookupMetadata(ctx context.Context, title string, contentType string) (*models.MediaMetadata, error)
}

// TorrentLister provides torrent listings from a qBittorrent instance.
// *qbittorrent.SyncManager satisfies this interface.
type TorrentLister interface {
	GetAllTorrents(ctx context.Context, instanceID int) ([]qbt.Torrent, error)
}

// SyncResult summarises a library sync run.
type SyncResult struct {
	InstanceID    int    `json:"instance_id"`
	Upserted      int    `json:"upserted"`
	TotalTorrents int    `json:"total_torrents"` // total torrents scanned from instance
	Deleted       int    `json:"deleted"`        // titles removed from arr since last sync
	Errors        int    `json:"errors"`
	Duration      string `json:"duration"`
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
	metaSvc    MetadataProvider
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

// WithMetadataProvider attaches an optional metadata provider (e.g. arr.Service)
// that is used during ARR sync and torrent-client scans to enrich ratings and genres.
func (s *Service) WithMetadataProvider(mp MetadataProvider) *Service {
	s.metaSvc = mp
	return s
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
			Genres:        strings.Join(sv.Genres, ", "),
			Source:        "arr",
		}
		// Single rating from Sonarr (value is typically a 0-10 scale).
		if sv.Ratings != nil && sv.Ratings.Value > 0 {
			p.AudienceRating = sv.Ratings.Value
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
			Genres:        strings.Join(mv.Genres, ", "),
			Source:        "arr",
		}
		// Radarr returns OpenRatings (map[string]Ratings).
		if mv.Ratings != nil {
			if v, ok := mv.Ratings["imdb"]; ok {
				p.IMDbRating = v.Value
			}
			if v, ok := mv.Ratings["tmdb"]; ok {
				p.TMDbRating = v.Value
			}
			if v, ok := mv.Ratings["metacritic"]; ok && v.Value > 0 {
				// Metacritic is on 0–100 scale; keep as-is (stored raw for UI display).
				p.MetacriticRating = v.Value
			}
			if v, ok := mv.Ratings["rottenTomatoes"]; ok && v.Value > 0 {
				p.RottenTomatoesRating = v.Value
			}
			// audience_rating: tmdb tends to be audience-weighted.
			p.AudienceRating = p.TMDbRating
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

// titleGroup accumulates torrents that belong to the same parsed title so they
// can be upserted as a single library entry with a torrent count.
type titleGroup struct {
	title       string // display title (first-seen casing)
	sortTitle   string // lower-cased for dedup key
	contentType models.LibraryContentType
	year        int         // best guess (non-zero wins)
	count       int         // number of torrent hashes in this group
	seasons     map[int]int // season → max episode count seen
}

// ScanTorrentClients reads all torrents from a qBittorrent instance, parses
// each torrent name with rls, groups results by normalised title + content type,
// and upserts one library entry per unique title with source="torrent_client".
func (s *Service) ScanTorrentClients(ctx context.Context, ownerID, instanceID int, lister TorrentLister) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{InstanceID: instanceID}

	torrents, err := lister.GetAllTorrents(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list torrents from instance %d: %w", instanceID, err)
	}

	log.Info().
		Int("instanceID", instanceID).
		Int("torrentCount", len(torrents)).
		Msg("[LIBRARY] scan: fetched torrents from instance")

	if len(torrents) == 0 {
		log.Warn().Int("instanceID", instanceID).Msg("[LIBRARY] scan: GetAllTorrents returned 0 — is SyncManager ready?")
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result, nil
	}

	result.TotalTorrents = len(torrents)

	groups := s.groupTorrentsByTitle(instanceID, torrents)
	s.upsertTitleGroups(ctx, ownerID, groups, result)

	result.Duration = time.Since(start).Round(time.Millisecond).String()
	log.Info().
		Int("instanceID", instanceID).
		Int("upserted", result.Upserted).
		Int("errors", result.Errors).
		Str("duration", result.Duration).
		Msg("[LIBRARY] scan: instance scan complete")

	return result, nil
}

// groupTorrentsByTitle parses every torrent name with rls and groups them by
// normalised (sort_title, content_type).
func (s *Service) groupTorrentsByTitle(instanceID int, torrents []qbt.Torrent) map[string]*titleGroup {
	groups := make(map[string]*titleGroup)
	var skippedEmpty, skippedParse int

	for i := range torrents {
		t := &torrents[i]
		if t.Name == nil || *t.Name == "" {
			skippedEmpty++
			continue
		}

		parsed := s.parser.Parse(*t.Name)
		if parsed.Title == "" {
			skippedParse++
			log.Debug().Str("torrent", *t.Name).Msg("[LIBRARY] scan: rls returned empty title, skipping")
			continue
		}

		ct := classifyContentType(parsed)
		sortTitle := strings.ToLower(parsed.Title)
		key := sortTitle + "\x00" + string(ct)

		g, exists := groups[key]
		if !exists {
			g = &titleGroup{
				title:       parsed.Title,
				sortTitle:   sortTitle,
				contentType: ct,
				seasons:     make(map[int]int),
			}
			groups[key] = g
		}
		g.count++

		if parsed.Year > 0 && g.year == 0 {
			g.year = parsed.Year
		}
		if parsed.Series > 0 && parsed.Episode > g.seasons[parsed.Series] {
			g.seasons[parsed.Series] = parsed.Episode
		}
	}

	log.Info().
		Int("instanceID", instanceID).
		Int("uniqueTitles", len(groups)).
		Int("skippedEmpty", skippedEmpty).
		Int("skippedParseFailed", skippedParse).
		Msg("[LIBRARY] scan: grouping complete")

	return groups
}

// upsertTitleGroups writes one library entry per title group and upserts
// season info for TV content.
func (s *Service) upsertTitleGroups(ctx context.Context, ownerID int, groups map[string]*titleGroup, result *SyncResult) {
	for _, g := range groups {
		p := models.LibraryTitleUpsertParams{
			OwnerID:      ownerID,
			ContentType:  g.contentType,
			Title:        g.title,
			SortTitle:    g.sortTitle,
			Source:       "torrent_client",
			TorrentCount: g.count,
		}
		if g.year > 0 {
			y := g.year
			p.Year = &y
		}

		title, err := s.titleStore.UpsertByTitle(ctx, p)
		if err != nil {
			log.Warn().Err(err).
				Str("title", g.title).
				Str("contentType", string(g.contentType)).
				Int("torrentCount", g.count).
				Msg("[LIBRARY] scan: upsert title failed")
			result.Errors++
			continue
		}
		result.Upserted++

		if title != nil {
			for seasonNum, epCount := range g.seasons {
				if err := s.titleStore.UpsertSeason(ctx, title.ID, seasonNum, epCount, true); err != nil {
					log.Debug().Err(err).Int("titleID", title.ID).Int("season", seasonNum).
						Msg("[LIBRARY] scan: upsert season failed")
				}
			}
		}
	}
}

// classifyContentType determines the library content type from a parsed release.
func classifyContentType(parsed *rls.Release) models.LibraryContentType {
	ctInfo := releases.DetermineContentType(parsed)
	ct := models.LibraryContentType(ctInfo.ContentType)
	if ct == "" || ct == "unknown" {
		return models.LibraryContentTypeMovie
	}
	return ct
}

// StartPeriodicScan launches a background goroutine that re-scans all qBittorrent
// instances every interval, keeping the "torrent_client" library entries fresh.
// The goroutine stops when ctx is cancelled.  Pass a non-positive interval to use
// the default of 30 minutes.
func (s *Service) StartPeriodicScan(
	ctx context.Context,
	ownerID int,
	interval time.Duration,
	getInstanceIDs func(context.Context) ([]int, error),
	lister TorrentLister,
) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	// scanAll runs one full pass over all active instances.
	scanAll := func() {
		ids, err := getInstanceIDs(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("[LIBRARY] periodic scan: list instances failed")
			return
		}
		for _, id := range ids {
			if _, err := s.ScanTorrentClients(ctx, ownerID, id, lister); err != nil {
				log.Warn().Err(err).Int("instanceID", id).Msg("[LIBRARY] periodic scan failed")
			}
		}
	}

	go func() {
		// Fire immediately on startup so the library is populated without
		// waiting for the first ticker tick.
		scanAll()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Info().Dur("interval", interval).Msg("[LIBRARY] periodic client scan started")
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("[LIBRARY] periodic client scan stopped")
				return
			case <-ticker.C:
				scanAll()
			}
		}
	}()
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
