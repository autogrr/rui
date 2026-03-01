// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/dir-scan  — Full dir-scan page
// ------------------------------------------------------------------

func (h *Handler) GetDirScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)

	p := pages.DirScanProps{
		BaseURL:   h.baseURL(),
		Username:  UsernameFromContext(ctx),
		Version:   h.version,
		Instances: navInsts,
		ActiveTab: r.URL.Query().Get("tab"),
	}

	if h.dirScanService == nil {
		p.Error = "Dir scan service not configured."
	}

	render(w, r, http.StatusOK, pages.DirScan(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/dir-scan/settings
// ------------------------------------------------------------------

func (h *Handler) GetDirScanSettingsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ownerID := h.sessionManager.GetInt(ctx, "user_id")

	var settings *models.DirScanSettings
	if h.dirScanService != nil {
		s, err := h.dirScanService.GetSettings(ctx, ownerID)
		if err != nil {
			log.Error().Err(err).Msg("dirscan ui: failed to get settings")
		} else {
			settings = s
		}
	}

	render(w, r, http.StatusOK, pages.DirScanSettingsPartial(pages.DirScanSettingsFromModel(settings), h.baseURL()))
}

// ------------------------------------------------------------------
// POST /ui/partials/dir-scan/settings
// ------------------------------------------------------------------

func (h *Handler) PostDirScanSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Fetch existing to preserve fields not in the form.
	ownerID := h.sessionManager.GetInt(ctx, "user_id")
	var existing *models.DirScanSettings
	if h.dirScanService != nil {
		if s, err := h.dirScanService.GetSettings(ctx, ownerID); err == nil && s != nil {
			existing = s
		}
	}
	if existing == nil {
		existing = &models.DirScanSettings{}
	}

	existing.Enabled = r.FormValue("enabled") == "on"
	existing.AllowPartial = r.FormValue("allow_partial") == "on"
	existing.StartPaused = r.FormValue("start_paused") == "on"
	existing.SkipPieceBoundarySafetyCheck = r.FormValue("skip_piece_boundary_safety_check") == "on"
	existing.MatchMode = models.MatchMode(r.FormValue("match_mode"))
	existing.Category = r.FormValue("category")
	existing.SizeTolerancePercent = parseFloat(r.FormValue("size_tolerance_percent"), existing.SizeTolerancePercent)
	existing.MinPieceRatio = parseFloat(r.FormValue("min_piece_ratio"), existing.MinPieceRatio)
	existing.MaxSearcheesPerRun = intParam(r.FormValue("max_searchees_per_run"), existing.MaxSearcheesPerRun)
	existing.MaxSearcheeAgeDays = intParam(r.FormValue("max_searchee_age_days"), existing.MaxSearcheeAgeDays)
	existing.Tags = parseCommaTags(r.FormValue("tags"))

	errMsg := ""
	if h.dirScanService != nil {
		if _, err := h.dirScanService.UpdateSettings(ctx, ownerID, existing); err != nil {
			errMsg = "Failed to save: " + err.Error()
			log.Error().Err(err).Msg("dirscan ui: failed to update settings")
		}
	} else {
		errMsg = "Dir scan service not configured."
	}

	if errMsg != "" {
		w.Header().Set("HX-Trigger", `{"toast":{"type":"error","message":"`+errMsg+`"}}`)
	}

	// Re-fetch to get saved values.
	var saved *models.DirScanSettings
	if h.dirScanService != nil {
		if s, err := h.dirScanService.GetSettings(ctx, ownerID); err == nil {
			saved = s
		}
	}

	render(w, r, http.StatusOK, pages.DirScanSettingsPartial(pages.DirScanSettingsFromModel(saved), h.baseURL()))
}

// ------------------------------------------------------------------
// GET /ui/partials/dir-scan/directories
// ------------------------------------------------------------------

func (h *Handler) GetDirScanDirectoriesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)

	var dirs []pages.DirScanDirectoryItem
	if h.dirScanService != nil {
		raw, err := h.dirScanService.ListDirectories(ctx)
		if err != nil {
			log.Error().Err(err).Msg("dirscan ui: failed to list directories")
		}
		for _, d := range raw {
			dirs = append(dirs, pages.DirScanDirectoryItemFromModel(d, navInsts))
		}
	}

	render(w, r, http.StatusOK, pages.DirScanDirectoriesPartial(dirs, h.baseURL()))
}

// ------------------------------------------------------------------
// GET /ui/partials/dir-scan/directories/form
// GET /ui/partials/dir-scan/directories/form/{id}
// ------------------------------------------------------------------

func (h *Handler) GetDirScanDirectoryForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)

	idStr := chi.URLParam(r, "id")
	id := intParam(idStr, 0)

	var item *pages.DirScanDirectoryItem
	if id > 0 && h.dirScanService != nil {
		dir, err := h.dirScanService.GetDirectory(ctx, id)
		if err != nil {
			log.Error().Err(err).Int("id", id).Msg("dirscan ui: failed to get directory")
		} else {
			v := pages.DirScanDirectoryItemFromModel(dir, navInsts)
			item = &v
		}
	}

	render(w, r, http.StatusOK, pages.DirScanDirectoryForm(item, navInsts, h.baseURL()))
}

// ------------------------------------------------------------------
// POST /ui/partials/dir-scan/directories
// ------------------------------------------------------------------

func (h *Handler) PostDirScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	targetID := intParam(r.FormValue("target_instance_id"), 0)
	dir := &models.DirScanDirectory{
		Path:                r.FormValue("path"),
		QbitPathPrefix:      r.FormValue("qbit_path_prefix"),
		Category:            r.FormValue("category"),
		Tags:                parseCommaTags(r.FormValue("tags")),
		Enabled:             r.FormValue("enabled") == "on",
		TargetInstanceID:    targetID,
		ScanIntervalMinutes: intParam(r.FormValue("scan_interval_minutes"), 0),
	}

	errMsg := ""
	if h.dirScanService != nil {
		if _, err := h.dirScanService.CreateDirectory(ctx, dir); err != nil {
			errMsg = err.Error()
			log.Error().Err(err).Msg("dirscan ui: failed to create directory")
		}
	} else {
		errMsg = "Dir scan service not configured."
	}

	if errMsg != "" {
		w.Header().Set("HX-Trigger", `{"toast":{"type":"error","message":"`+errMsg+`"}}`)
	}

	// Re-render the full directories list.
	var dirs []pages.DirScanDirectoryItem
	if h.dirScanService != nil {
		if raw, err := h.dirScanService.ListDirectories(ctx); err == nil {
			for _, d := range raw {
				dirs = append(dirs, pages.DirScanDirectoryItemFromModel(d, navInsts))
			}
		}
	}

	render(w, r, http.StatusOK, pages.DirScanDirectoriesPartial(dirs, h.baseURL()))
}

// ------------------------------------------------------------------
// PUT /ui/partials/dir-scan/directories/{id}
// ------------------------------------------------------------------

func (h *Handler) PutDirScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	navInsts := h.navInstances(r)
	id := intParam(chi.URLParam(r, "id"), 0)

	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	path := r.FormValue("path")
	qbitPrefix := r.FormValue("qbit_path_prefix")
	category := r.FormValue("category")
	tags := parseCommaTags(r.FormValue("tags"))
	enabled := r.FormValue("enabled") == "on"
	targetID := intParam(r.FormValue("target_instance_id"), 0)
	intervalMins := intParam(r.FormValue("scan_interval_minutes"), 0)

	params := &models.DirScanDirectoryUpdateParams{
		Path:                &path,
		QbitPathPrefix:      &qbitPrefix,
		Category:            &category,
		Tags:                &tags,
		Enabled:             &enabled,
		TargetInstanceID:    &targetID,
		ScanIntervalMinutes: &intervalMins,
	}

	errMsg := ""
	if h.dirScanService != nil {
		if _, err := h.dirScanService.UpdateDirectory(ctx, id, params); err != nil {
			errMsg = err.Error()
			log.Error().Err(err).Int("id", id).Msg("dirscan ui: failed to update directory")
		}
	} else {
		errMsg = "Dir scan service not configured."
	}

	if errMsg != "" {
		w.Header().Set("HX-Trigger", `{"toast":{"type":"error","message":"`+errMsg+`"}}`)
	}

	// Re-render the full directories list.
	var dirs []pages.DirScanDirectoryItem
	if h.dirScanService != nil {
		if raw, err := h.dirScanService.ListDirectories(ctx); err == nil {
			for _, d := range raw {
				dirs = append(dirs, pages.DirScanDirectoryItemFromModel(d, navInsts))
			}
		}
	}

	render(w, r, http.StatusOK, pages.DirScanDirectoriesPartial(dirs, h.baseURL()))
}

// ------------------------------------------------------------------
// DELETE /ui/partials/dir-scan/directories/{id}
// ------------------------------------------------------------------

func (h *Handler) DeleteDirScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := intParam(chi.URLParam(r, "id"), 0)

	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if h.dirScanService != nil {
		if err := h.dirScanService.DeleteDirectory(ctx, id); err != nil {
			log.Error().Err(err).Int("id", id).Msg("dirscan ui: failed to delete directory")
		}
	}

	// Return empty so HTMX can swap out the card.
	w.WriteHeader(http.StatusOK)
}

// ------------------------------------------------------------------
// POST /ui/partials/dir-scan/directories/{id}/scan
// ------------------------------------------------------------------

func (h *Handler) PostDirScanTrigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := intParam(chi.URLParam(r, "id"), 0)

	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	errMsg := ""
	if h.dirScanService != nil {
		if _, err := h.dirScanService.StartManualScan(ctx, id); err != nil {
			if !errors.Is(err, models.ErrDirScanRunAlreadyActive) {
				errMsg = err.Error()
				log.Error().Err(err).Int("directoryID", id).Msg("dirscan ui: failed to start scan")
			}
		}
	} else {
		errMsg = "Dir scan service not configured."
	}

	// Fetch and show runs.
	var runs []pages.DirScanRun
	if h.dirScanService != nil {
		if raw, err := h.dirScanService.ListRuns(ctx, id, 10); err == nil {
			for _, run := range raw {
				runs = append(runs, pages.DirScanRunFromModel(run))
			}
		}
	}

	if errMsg != "" {
		w.Header().Set("HX-Trigger", `{"toast":{"type":"error","message":"`+errMsg+`"}}`)
	}

	render(w, r, http.StatusOK, pages.DirScanRunsPartial(runs, id, h.baseURL()))
}

// ------------------------------------------------------------------
// POST /ui/partials/dir-scan/directories/{id}/cancel
// ------------------------------------------------------------------

func (h *Handler) PostDirScanCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := intParam(chi.URLParam(r, "id"), 0)

	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if h.dirScanService != nil {
		if err := h.dirScanService.CancelScan(ctx, id); err != nil {
			log.Error().Err(err).Int("directoryID", id).Msg("dirscan ui: failed to cancel scan")
		}
	}

	// Fetch and show runs.
	var runs []pages.DirScanRun
	if h.dirScanService != nil {
		if raw, err := h.dirScanService.ListRuns(ctx, id, 10); err == nil {
			for _, run := range raw {
				runs = append(runs, pages.DirScanRunFromModel(run))
			}
		}
	}

	render(w, r, http.StatusOK, pages.DirScanRunsPartial(runs, id, h.baseURL()))
}

// ------------------------------------------------------------------
// GET /ui/partials/dir-scan/runs/{id}
// ------------------------------------------------------------------

func (h *Handler) GetDirScanRunsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirID := intParam(chi.URLParam(r, "id"), 0)

	var runs []pages.DirScanRun
	if h.dirScanService != nil && dirID > 0 {
		if raw, err := h.dirScanService.ListRuns(ctx, dirID, 10); err == nil {
			for _, run := range raw {
				runs = append(runs, pages.DirScanRunFromModel(run))
			}
		}
	}

	render(w, r, http.StatusOK, pages.DirScanRunsPartial(runs, dirID, h.baseURL()))
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

// parseFloat parses a float64 from a string, returning def on failure.
func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// parseCommaTags splits a comma-separated tag string into a trimmed slice.
func parseCommaTags(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
