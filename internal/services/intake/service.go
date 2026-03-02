// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package intake implements the rui intake pipeline.  An incoming release name
// is parsed with rls, external IDs are resolved through *arr instances (arrs
// are info providers only — monitored status is ignored), the result is matched
// against the native library, pipeline rules are evaluated in order, and the
// outcome is recorded as an immutable intake event.
package intake

import (
	"context"
	"fmt"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/arr"
	"github.com/autogrr/rui/internal/services/library"
	"github.com/autogrr/rui/pkg/releases"
)

// LibraryMatcher is the subset of library.Service used by the intake pipeline.
type LibraryMatcher interface {
	Match(ctx context.Context, ownerID int, releaseName string) ([]library.TitleMatch, error)
	EvalRules(ctx context.Context, ownerID int, mc library.MatchContext) (*library.RuleResult, error)
}

// ExternalIDLookup is the subset of arr.Service used for metadata resolution.
type ExternalIDLookup interface {
	LookupExternalIDs(ctx context.Context, title string, ct arr.ContentType) (*arr.ExternalIDsResult, error)
}

// MetadataEnricher fetches full metadata (ratings, genres) for a title from an
// information provider. arr.Service.LookupMetadata satisfies this interface.
type MetadataEnricher interface {
	LookupMetadata(ctx context.Context, title string, contentType string) (*models.MediaMetadata, error)
}

// Request is submitted to Process for a release to be evaluated.
type Request struct {
	OwnerID     int
	PipelineID  *int // nil = evaluate all enabled pipelines
	ReleaseName string
}

// Result summarises intake processing for one release.
type Result struct {
	Event   *models.IntakeEvent
	Action  models.LibraryAction
	Matched bool
}

// Service runs the intake pipeline.
type Service struct {
	pipelineStore *models.IntakePipelineStore
	eventStore    *models.IntakeEventStore
	libMatcher    LibraryMatcher
	arrLookup     ExternalIDLookup
	metaEnricher  MetadataEnricher // optional; enriches ratings when not in library
	parser        *releases.Parser

	// progCache: rule ID -> compiled program + the filter string it was compiled for.
	progCache map[int]*cachedProg
}

type cachedProg struct {
	prog   *vm.Program
	filter string
}

// New creates a new intake Service.
func New(
	pipelineStore *models.IntakePipelineStore,
	eventStore *models.IntakeEventStore,
	libMatcher LibraryMatcher,
	arrLookup ExternalIDLookup,
) *Service {
	return &Service{
		pipelineStore: pipelineStore,
		eventStore:    eventStore,
		libMatcher:    libMatcher,
		arrLookup:     arrLookup,
		parser:        releases.NewDefaultParser(),
		progCache:     make(map[int]*cachedProg),
	}
}

// WithMetadataEnricher attaches an optional metadata enricher (e.g. arr.Service)
// used to fetch ratings/genres when the title is not found in the native library
// or when the library entry has no ratings.
func (s *Service) WithMetadataEnricher(me MetadataEnricher) *Service {
	s.metaEnricher = me
	return s
}

// Process evaluates a release through the intake pipeline and records the event.
func (s *Service) Process(ctx context.Context, req Request) (*Result, error) {
	if req.ReleaseName == "" {
		return nil, fmt.Errorf("release name must not be empty")
	}

	event := &models.IntakeEvent{
		OwnerID:     req.OwnerID,
		PipelineID:  req.PipelineID,
		ReleaseName: req.ReleaseName,
		Status:      models.IntakeStatusPending,
	}

	result, processErr := s.process(ctx, event, req)
	if processErr != nil {
		event.Status = models.IntakeStatusError
		event.Error = processErr.Error()
	}

	now := time.Now()
	event.ProcessedAt = &now

	saved, saveErr := s.eventStore.Insert(ctx, event)
	if saveErr != nil {
		log.Warn().Err(saveErr).Str("release", req.ReleaseName).Msg("[INTAKE] failed to persist event")
	} else {
		event = saved
	}

	if processErr != nil {
		return &Result{Event: event}, processErr
	}
	if result != nil {
		result.Event = event
	}
	return result, nil
}

// process does the pipeline work, mutating event in place and returning a Result.
func (s *Service) process(ctx context.Context, event *models.IntakeEvent, req Request) (*Result, error) {
	// 1. Parse release name.
	parsed := s.parser.Parse(req.ReleaseName)
	ctInfo := releases.DetermineContentType(parsed)
	ct := ctInfo.ContentType
	event.ContentType = ct
	event.ParsedTitle = parsed.Title
	event.ParsedYear = parsed.Year

	// 2. Resolve external IDs from arr instances (arrs as info providers only).
	if parsed.Title != "" {
		idResult, err := s.arrLookup.LookupExternalIDs(ctx, parsed.Title, arr.ContentType(ct))
		if err != nil {
			log.Debug().Err(err).Str("title", parsed.Title).Msg("[INTAKE] arr id lookup failed")
		} else if idResult != nil && idResult.IDs != nil {
			ids := idResult.IDs
			event.IMDbID = ids.IMDbID
			event.TMDbID = ids.TMDbID
			event.TVDbID = ids.TVDbID
			event.ArrInstanceID = idResult.ArrInstanceID
		}
	}

	// 3. Match against the native library.
	matches, err := s.libMatcher.Match(ctx, req.OwnerID, req.ReleaseName)
	if err != nil {
		log.Debug().Err(err).Msg("[INTAKE] library match failed")
	}
	var bestMatch *library.TitleMatch
	if len(matches) > 0 {
		bestMatch = &matches[0]
		event.MatchedLibraryID = &bestMatch.Title.ID
		event.MatchedTitle = bestMatch.Title.Title
		event.Status = models.IntakeStatusMatched
	}

	// 4. Evaluate pipeline rules.
	pipelines, err := s.getPipelines(ctx, req)
	if err != nil {
		return nil, err
	}

	mc := buildMatchContext(event, bestMatch)

	// 3b. Enrich ratings/genres via metadata provider when the library match
	// has no rating data (e.g. title never synced with ratings, or no match).
	if s.metaEnricher != nil && parsed.Title != "" && mc.RottenTomatoesRating == 0 && mc.IMDbRating == 0 {
		meta, err := s.metaEnricher.LookupMetadata(ctx, parsed.Title, ct)
		if err != nil {
			log.Debug().Err(err).Str("title", parsed.Title).Msg("[INTAKE] metadata enrich failed")
		} else if meta != nil {
			mc.IMDbRating = meta.IMDbRating
			mc.TMDbRating = meta.TMDbRating
			mc.MetacriticRating = meta.MetacriticRating
			mc.RottenTomatoesRating = meta.RottenTomatoesRating
			mc.AudienceRating = meta.AudienceRating
			if meta.Genres != "" {
				mc.Genres = meta.Genres
			}
		}
	}

	for _, pipeline := range pipelines {
		if !pipelineAcceptsContentType(pipeline, ct) {
			continue
		}
		ruleResult, err := s.evalPipelineRules(ctx, pipeline, mc)
		if err != nil {
			log.Warn().Err(err).Int("pipelineID", pipeline.ID).Msg("[INTAKE] rule eval error")
			continue
		}
		if ruleResult == nil {
			continue
		}
		// First matching rule wins.
		event.MatchedRuleID = &ruleResult.RuleID
		event.MatchedRuleName = ruleResult.RuleName
		event.Action = string(ruleResult.Action)
		event.Status = actionToStatus(ruleResult.Action)
		pid := pipeline.ID
		event.PipelineID = &pid

		return &Result{Action: ruleResult.Action, Matched: true}, nil
	}

	// No rule fired.
	if event.Status == models.IntakeStatusMatched {
		event.Status = models.IntakeStatusSkipped
	}
	return &Result{Matched: bestMatch != nil}, nil
}

// getPipelines returns the pipeline slice to run for this request.
func (s *Service) getPipelines(ctx context.Context, req Request) ([]*models.IntakePipeline, error) {
	if req.PipelineID != nil {
		p, err := s.pipelineStore.GetPipeline(ctx, *req.PipelineID)
		if err != nil {
			return nil, err
		}
		return []*models.IntakePipeline{p}, nil
	}
	return s.pipelineStore.ListEnabledPipelines(ctx, req.OwnerID)
}

// evalPipelineRules evaluates rules in sort order; returns on first match.
func (s *Service) evalPipelineRules(ctx context.Context, pipeline *models.IntakePipeline, mc intakeMatchCtx) (*library.RuleResult, error) {
	rules, err := s.pipelineStore.ListEnabledRules(ctx, pipeline.ID)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		// Empty filter means "match everything".
		if rule.ExprFilter == "" {
			return &library.RuleResult{Action: rule.Action, RuleID: rule.ID, RuleName: rule.Name}, nil
		}
		prog, err := s.compileRule(rule)
		if err != nil {
			log.Warn().Err(err).Int("ruleID", rule.ID).Msg("[INTAKE] compile rule failed")
			continue
		}
		out, err := vm.Run(prog, mc)
		if err != nil {
			log.Warn().Err(err).Int("ruleID", rule.ID).Msg("[INTAKE] run rule failed")
			continue
		}
		if matched, ok := out.(bool); ok && matched {
			return &library.RuleResult{Action: rule.Action, RuleID: rule.ID, RuleName: rule.Name}, nil
		}
	}
	return nil, nil
}

func (s *Service) compileRule(rule *models.IntakeRule) (*vm.Program, error) {
	if c, ok := s.progCache[rule.ID]; ok && c.filter == rule.ExprFilter {
		return c.prog, nil
	}
	prog, err := expr.Compile(rule.ExprFilter,
		expr.AsBool(),
		expr.Env(intakeMatchCtx{}),
	)
	if err != nil {
		return nil, err
	}
	s.progCache[rule.ID] = &cachedProg{prog: prog, filter: rule.ExprFilter}
	return prog, nil
}

// ─── intakeMatchCtx ───────────────────────────────────────────────────────────
// All fields must be exported for expr-lang reflection.

type intakeMatchCtx struct {
	// From rls parse
	Title       string
	Year        int
	Season      int
	Episode     int
	Group       string
	Source      string
	Resolution  string
	ContentType string

	// From library match
	LibraryTitle  string
	LibraryYear   int
	LibraryIMDbID string
	LibraryTMDbID int
	LibraryTVDbID int
	HasFile       bool
	MatchScore    int

	// From arr ID resolution
	IMDbID string
	TMDbID int
	TVDbID int

	// Ratings from the information provider.
	// IMDbRating and TMDbRating are on a 0–10 scale.
	// MetacriticRating is 0–100 (Metacritic raw score).
	// RottenTomatoesRating is 0–100 (percent fresh).
	// AudienceRating is 0–10.
	IMDbRating           float64
	TMDbRating           float64
	MetacriticRating     float64
	RottenTomatoesRating float64
	AudienceRating       float64
	// Genres is a comma-separated list ("Action, Adventure, Sci-Fi").
	Genres string
}

func buildMatchContext(event *models.IntakeEvent, match *library.TitleMatch) intakeMatchCtx {
	mc := intakeMatchCtx{
		Title:       event.ParsedTitle,
		Year:        event.ParsedYear,
		ContentType: event.ContentType,
		IMDbID:      event.IMDbID,
		TMDbID:      event.TMDbID,
		TVDbID:      event.TVDbID,
	}
	if match != nil {
		mc.LibraryTitle = match.Title.Title
		mc.LibraryIMDbID = match.Title.IMDbID
		mc.LibraryTMDbID = match.Title.TMDbID
		mc.LibraryTVDbID = match.Title.TVDbID
		mc.HasFile = match.Title.HasFile
		mc.MatchScore = match.Score
		mc.Source = match.Context.Source
		mc.Resolution = match.Context.Resolution
		mc.Group = match.Context.Group
		mc.Season = match.Context.Season
		mc.Episode = match.Context.Episode
		if match.Title.Year != nil {
			mc.LibraryYear = *match.Title.Year
		}
		// Populate ratings from the matched library title (synced from ARR).
		mc.IMDbRating = match.Title.IMDbRating
		mc.TMDbRating = match.Title.TMDbRating
		mc.MetacriticRating = match.Title.MetacriticRating
		mc.RottenTomatoesRating = match.Title.RottenTomatoesRating
		mc.AudienceRating = match.Title.AudienceRating
		mc.Genres = match.Title.Genres
	}
	return mc
}

// ─── misc helpers ─────────────────────────────────────────────────────────────

func pipelineAcceptsContentType(p *models.IntakePipeline, ct string) bool {
	if len(p.ContentTypes) == 0 {
		return true
	}
	for _, t := range p.ContentTypes {
		if t == ct {
			return true
		}
	}
	return false
}

func actionToStatus(action models.LibraryAction) models.IntakeStatus {
	switch action {
	case models.LibraryActionGrab:
		return models.IntakeStatusGrabbed
	case models.LibraryActionSkip:
		return models.IntakeStatusSkipped
	case models.LibraryActionRemove:
		return models.IntakeStatusRemoved
	default:
		return models.IntakeStatusMatched
	}
}
