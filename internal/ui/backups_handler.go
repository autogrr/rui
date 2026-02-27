// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/backups — full page
// ------------------------------------------------------------------

func (h *Handler) GetBackups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	insts := h.navInstances(r)
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	if instanceID == 0 && len(insts) > 0 {
		instanceID = insts[0].ID
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = "settings"
	}

	p := pages.BackupsProps{
		BaseURL:    h.baseURL(),
		Username:   UsernameFromContext(ctx),
		Version:    h.version,
		Instances:  insts,
		InstanceID: instanceID,
		ActiveTab:  activeTab,
	}

	if h.backupsService != nil && instanceID > 0 {
		if settings, err := h.backupsService.GetSettings(ctx, instanceID); err == nil {
			p.Settings = settings
		}
		if runs, err := h.backupsService.ListRuns(ctx, instanceID, 20, 0); err == nil {
			for _, run := range runs {
				p.Runs = append(p.Runs, pages.BackupRunFromModel(run))
			}
			p.RunsTotal = len(p.Runs)
		}
	}

	render(w, r, http.StatusOK, pages.Backups(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/backups/settings/{instanceId}
// ------------------------------------------------------------------

func (h *Handler) GetBackupSettingsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)

	p := pages.BackupsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
	}

	if h.backupsService != nil {
		if settings, err := h.backupsService.GetSettings(ctx, instanceID); err != nil {
			p.BackupsError = err.Error()
		} else {
			p.Settings = settings
		}
	}

	render(w, r, http.StatusOK, pages.BackupSettingsPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/backups/settings/{instanceId}
// ------------------------------------------------------------------

func (h *Handler) PostBackupSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)

	settings := &models.BackupSettings{
		InstanceID:        instanceID,
		Enabled:           r.FormValue("enabled") == "on",
		HourlyEnabled:     r.FormValue("hourly_enabled") == "on",
		DailyEnabled:      r.FormValue("daily_enabled") == "on",
		WeeklyEnabled:     r.FormValue("weekly_enabled") == "on",
		MonthlyEnabled:    r.FormValue("monthly_enabled") == "on",
		KeepHourly:        intParam(r.FormValue("keep_hourly"), 0),
		KeepDaily:         intParam(r.FormValue("keep_daily"), 7),
		KeepWeekly:        intParam(r.FormValue("keep_weekly"), 4),
		KeepMonthly:       intParam(r.FormValue("keep_monthly"), 12),
		IncludeCategories: r.FormValue("include_categories") == "on",
		IncludeTags:       r.FormValue("include_tags") == "on",
	}

	if cp := r.FormValue("custom_path"); cp != "" {
		settings.CustomPath = &cp
	}

	p := pages.BackupsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Settings:   settings,
	}

	if h.backupsService != nil {
		if err := h.backupsService.UpdateSettings(ctx, settings); err != nil {
			p.BackupsError = err.Error()
		} else if saved, err := h.backupsService.GetSettings(ctx, instanceID); err == nil {
			p.Settings = saved
		}
	}

	render(w, r, http.StatusOK, pages.BackupSettingsPartial(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/backups/runs/{instanceId}
// ------------------------------------------------------------------

func (h *Handler) GetBackupRunsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	page := intParam(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}

	const pageSize = 20
	offset := (page - 1) * pageSize

	p := pages.BackupsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		RunsPage:   page,
	}

	if h.backupsService != nil {
		if runs, err := h.backupsService.ListRuns(ctx, instanceID, pageSize, offset); err != nil {
			p.BackupsError = err.Error()
		} else {
			for _, run := range runs {
				p.Runs = append(p.Runs, pages.BackupRunFromModel(run))
			}
			p.RunsTotal = len(p.Runs)
		}
	}

	render(w, r, http.StatusOK, pages.BackupRunsPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/backups/runs/{instanceId}/trigger
// ------------------------------------------------------------------

func (h *Handler) PostTriggerBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	username := UsernameFromContext(ctx)

	if h.backupsService != nil {
		if _, err := h.backupsService.QueueRun(ctx, instanceID, models.BackupRunKindManual, username); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	const pageSize = 20

	p := pages.BackupsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		RunsPage:   1,
	}

	if h.backupsService != nil {
		if runs, err := h.backupsService.ListRuns(ctx, instanceID, pageSize, 0); err == nil {
			for _, run := range runs {
				p.Runs = append(p.Runs, pages.BackupRunFromModel(run))
			}
			p.RunsTotal = len(p.Runs)
		}
	}

	render(w, r, http.StatusOK, pages.BackupRunsPartial(p))
}

// ------------------------------------------------------------------
// DELETE /ui/partials/backups/runs/{runId}
// ------------------------------------------------------------------

func (h *Handler) DeleteBackupRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	runID, err := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if h.backupsService != nil {
		if err := h.backupsService.DeleteRun(ctx, runID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Empty response — hx-swap="outerHTML" removes the row.
	w.WriteHeader(http.StatusOK)
}

// ------------------------------------------------------------------
// GET /ui/backups — full page
