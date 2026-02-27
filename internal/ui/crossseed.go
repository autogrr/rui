// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/crossseed"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/cross-seed  — Full cross-seed page
// ------------------------------------------------------------------

func (h *Handler) GetCrossSeed(w http.ResponseWriter, r *http.Request) {
	p := h.buildCrossSeedProps(r)
	render(w, r, http.StatusOK, pages.CrossSeed(p))
}

// buildCrossSeedProps constructs the cross-seed page props.
func (h *Handler) buildCrossSeedProps(r *http.Request) pages.CrossSeedProps {
	ctx := r.Context()
	q := r.URL.Query()

	navInsts := h.navInstances(r)
	p := pages.CrossSeedProps{
		BaseURL:    h.baseURL(),
		Username:   UsernameFromContext(ctx),
		Version:    h.version,
		Instances:  navInsts,
		ActiveTab:  q.Get("tab"),
		InstanceID: intParam(q.Get("instance_id"), navFirstInstanceID(navInsts)),
		RunsPage:   intParam(q.Get("page"), 1),
	}

	if h.crossSeedService == nil {
		p.CrossSeedError = "Cross-seed service not configured."
		return p
	}

	// Automation settings.
	settings, err := h.crossSeedService.GetAutomationSettings(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("crossseed: get automation settings")
	}
	p.AutomationSettings = settings

	// Search settings.
	searchSettings, err := h.crossSeedService.GetSearchSettings(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("crossseed: get search settings")
	}
	p.SearchSettings = searchSettings

	// Runs (for runs tab).
	if p.ActiveTab == "runs" {
		const pageSize = 25
		offset := (p.RunsPage - 1) * pageSize
		runs, err := h.crossSeedService.ListAutomationRuns(ctx, pageSize, offset)
		if err != nil {
			log.Warn().Err(err).Msg("crossseed: list runs")
		} else {
			for _, run := range runs {
				p.Runs = append(p.Runs, crossSeedRunFromModel(run))
			}
		}
	}

	// Blocklist (for blocklist tab).
	if p.ActiveTab == "blocklist" {
		entries, err := h.crossSeedService.ListBlocklist(ctx, p.InstanceID)
		if err != nil {
			log.Warn().Err(err).Msg("crossseed: list blocklist")
		} else {
			for _, e := range entries {
				p.Blocklist = append(p.Blocklist, pages.CrossSeedBlocklistItem{
					InstanceID: e.InstanceID,
					InfoHash:   e.InfoHash,
				})
			}
		}
	}

	return p
}

// crossSeedRunFromModel converts a models.CrossSeedRun to a page-level item.
func crossSeedRunFromModel(r *models.CrossSeedRun) pages.CrossSeedRunItem {
	item := pages.CrossSeedRunItem{
		ID:              r.ID,
		TriggeredBy:     string(r.TriggeredBy),
		Mode:            string(r.Mode),
		Status:          string(r.Status),
		TotalFeedItems:  r.TotalFeedItems,
		CandidatesFound: r.CandidatesFound,
		TorrentsAdded:   r.TorrentsAdded,
		TorrentsFailed:  r.TorrentsFailed,
		TorrentsSkipped: r.TorrentsSkipped,
	}
	if !r.StartedAt.IsZero() {
		item.StartedAt = r.StartedAt.Format("2006-01-02 15:04")
	}
	if r.CompletedAt != nil {
		item.CompletedAt = r.CompletedAt.Format("2006-01-02 15:04")
	}
	if r.Message != nil {
		item.Message = *r.Message
	}
	if r.ErrorMessage != nil {
		item.ErrorMessage = *r.ErrorMessage
	}
	return item
}

// ------------------------------------------------------------------
// GET /ui/partials/cross-seed/runs  — HTMX runs fragment
// ------------------------------------------------------------------

func (h *Handler) GetCrossSeedRunsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	page := intParam(q.Get("page"), 1)
	const pageSize = 25
	offset := (page - 1) * pageSize

	p := pages.CrossSeedProps{
		BaseURL:   h.baseURL(),
		Instances: h.navInstances(r),
		RunsPage:  page,
		ActiveTab: "runs",
	}

	if h.crossSeedService != nil {
		runs, err := h.crossSeedService.ListAutomationRuns(ctx, pageSize, offset)
		if err != nil {
			log.Warn().Err(err).Msg("crossseed: list runs partial")
		} else {
			for _, run := range runs {
				p.Runs = append(p.Runs, crossSeedRunFromModel(run))
			}
		}
	}

	render(w, r, http.StatusOK, pages.CrossSeedRunsPartial(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/cross-seed/blocklist  — HTMX blocklist fragment
// ------------------------------------------------------------------

func (h *Handler) GetCrossSeedBlocklistPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), navFirstInstanceID(navInsts))

	p := pages.CrossSeedProps{
		BaseURL:    h.baseURL(),
		Instances:  navInsts,
		InstanceID: instanceID,
		ActiveTab:  "blocklist",
	}

	if h.crossSeedService != nil {
		entries, err := h.crossSeedService.ListBlocklist(ctx, instanceID)
		if err != nil {
			log.Warn().Err(err).Msg("crossseed: list blocklist partial")
		} else {
			for _, e := range entries {
				p.Blocklist = append(p.Blocklist, pages.CrossSeedBlocklistItem{
					InstanceID: e.InstanceID,
					InfoHash:   e.InfoHash,
				})
			}
		}
	}

	render(w, r, http.StatusOK, pages.CrossSeedBlocklistPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/cross-seed/automation-settings  — Save automation settings
// ------------------------------------------------------------------

func (h *Handler) PostCrossSeedAutomationSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	settings := &models.CrossSeedAutomationSettings{
		Enabled:                r.FormValue("enabled") == "on",
		RunIntervalMinutes:     intParam(r.FormValue("run_interval_minutes"), 60),
		UseCustomCategory:      r.FormValue("use_custom_category") == "on",
		CustomCategory:         r.FormValue("custom_category"),
		UseCategoryFromIndexer: r.FormValue("use_category_from_indexer") == "on",
		FindIndividualEpisodes: r.FormValue("find_individual_episodes") == "on",
		RedactedAPIKey:         r.FormValue("redacted_api_key"),
		OrpheusAPIKey:          r.FormValue("orpheus_api_key"),
	}

	if v := r.FormValue("size_mismatch_tolerance_percent"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			settings.SizeMismatchTolerancePercent = f
		}
	}

	var errMsg string
	if h.crossSeedService != nil {
		if _, err := h.crossSeedService.UpdateAutomationSettings(ctx, settings); err != nil {
			log.Error().Err(err).Msg("crossseed: update automation settings")
			errMsg = fmt.Sprintf("Failed to save settings: %v", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		fmt.Fprintf(w, `<span class="text-destructive text-xs">%s</span>`, errMsg)
	} else {
		fmt.Fprint(w, `<span class="text-green-600 text-xs">Settings saved</span>`)
	}
}

// ------------------------------------------------------------------
// POST /ui/partials/cross-seed/search-settings  — Save search settings
// ------------------------------------------------------------------

func (h *Handler) PostCrossSeedSearchSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if h.crossSeedService == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}

	patch := crossseed.SearchSettingsPatch{}
	if v := r.FormValue("interval_seconds"); v != "" {
		n := intParam(v, 0)
		patch.IntervalSeconds = &n
	}
	if v := r.FormValue("cooldown_minutes"); v != "" {
		n := intParam(v, 0)
		patch.CooldownMinutes = &n
	}

	var searchErrMsg string
	if _, err := h.crossSeedService.PatchSearchSettings(r.Context(), patch); err != nil {
		log.Error().Err(err).Msg("crossseed: patch search settings")
		searchErrMsg = fmt.Sprintf("Failed to save settings: %v", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if searchErrMsg != "" {
		fmt.Fprintf(w, `<span class="text-destructive text-xs">%s</span>`, searchErrMsg)
	} else {
		fmt.Fprint(w, `<span class="text-green-600 text-xs">Settings saved</span>`)
	}
}

// ------------------------------------------------------------------
// POST /ui/partials/cross-seed/run  — Trigger manual automation run
// ------------------------------------------------------------------

func (h *Handler) PostCrossSeedRun(w http.ResponseWriter, r *http.Request) {
	if h.crossSeedService == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}

	go func() {
		_, err := h.crossSeedService.RunAutomation(r.Context(), crossseed.AutomationRunOptions{})
		if err != nil {
			log.Error().Err(err).Msg("crossseed: manual run")
		}
	}()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<span class="text-green-600 text-xs">Run queued</span>`)
}

// ------------------------------------------------------------------
// DELETE /ui/partials/cross-seed/blocklist  — Remove blocklist entry
// ------------------------------------------------------------------

func (h *Handler) DeleteCrossSeedBlocklistEntry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), 0)
	infoHash := strings.TrimSpace(q.Get("info_hash"))

	if infoHash == "" {
		http.Error(w, "info_hash required", http.StatusBadRequest)
		return
	}

	if h.crossSeedService != nil {
		if err := h.crossSeedService.DeleteBlocklistEntry(r.Context(), instanceID, infoHash); err != nil {
			log.Error().Err(err).Msg("crossseed: delete blocklist entry")
		}
	}

	// Return empty row (removed via hx-swap outerHTML).
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// boolPtr is a tiny helper for bool pointers.
func boolPtr(b bool) *bool { return &b }
