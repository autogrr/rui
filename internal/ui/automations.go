// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/automations — full page
// ------------------------------------------------------------------

func (h *Handler) GetAutomations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	insts := h.navInstances(r)
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	if instanceID == 0 && len(insts) > 0 {
		instanceID = insts[0].ID
	}

	p := pages.AutomationsProps{
		BaseURL:    h.baseURL(),
		Username:   UsernameFromContext(ctx),
		Version:    h.version,
		Instances:  insts,
		InstanceID: instanceID,
	}

	h.fillAutomationsData(ctx, &p, instanceID, insts)

	render(w, r, http.StatusOK, pages.Automations(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/automations — HTMX table fragment
// ------------------------------------------------------------------

func (h *Handler) GetAutomationsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	insts := h.navInstances(r)
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	if instanceID == 0 && len(insts) > 0 {
		instanceID = insts[0].ID
	}

	p := pages.AutomationsProps{
		BaseURL:    h.baseURL(),
		Instances:  insts,
		InstanceID: instanceID,
	}

	h.fillAutomationsData(ctx, &p, instanceID, insts)

	render(w, r, http.StatusOK, pages.AutomationsPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/automations/{instanceId}/rules/{id}/toggle
// ------------------------------------------------------------------

func (h *Handler) PostAutomationToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	id := intParam(chi.URLParam(r, "id"), 0)

	a, err := h.automationStore.Get(ctx, instanceID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	a.Enabled = !a.Enabled

	updated, err := h.automationStore.Update(ctx, a)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}

	insts := h.navInstances(r)
	instanceName := ""
	for _, inst := range insts {
		if inst.ID == instanceID {
			instanceName = inst.Name
			break
		}
	}

	item := pages.AutomationFromModel(updated, instanceName)
	render(w, r, http.StatusOK, pages.AutomationRowTempl(item, h.baseURL()))
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func (h *Handler) fillAutomationsData(ctx context.Context, p *pages.AutomationsProps, instanceID int, insts []layouts.Instance) {
	if h.automationStore == nil || instanceID == 0 {
		return
	}

	automations, err := h.automationStore.ListByInstance(ctx, instanceID)
	if err != nil {
		p.AutomationsError = err.Error()
	} else {
		instanceName := ""
		for _, inst := range insts {
			if inst.ID == instanceID {
				instanceName = inst.Name
				break
			}
		}
		for _, a := range automations {
			p.Automations = append(p.Automations, pages.AutomationFromModel(a, instanceName))
		}
	}

	if h.automationActivityStore == nil {
		return
	}

	activities, err := h.automationActivityStore.ListByInstance(ctx, instanceID, 50)
	if err == nil {
		for _, act := range activities {
			p.Activity = append(p.Activity, automationActivityItemFromModel(act))
		}
	}
}

func automationActivityItemFromModel(a *models.AutomationActivity) pages.AutomationActivityItem {
	return pages.AutomationActivityItem{
		ID: int64(a.ID),
		AutomationID: func() int {
			if a.RuleID != nil {
				return *a.RuleID
			}
			return 0
		}(),
		Name:        a.RuleName,
		TorrentHash: a.Hash,
		TorrentName: a.TorrentName,
		Action:      a.Action,
		Message:     a.Reason,
		CreatedAt:   a.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ------------------------------------------------------------------
// Reannounce handlers
// ------------------------------------------------------------------

// GetReannounceSettingsPartial returns the reannounce settings card for an instance.
// GET /ui/partials/automations/reannounce/settings/{instanceId}
func (h *Handler) GetReannounceSettingsPartial(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	var settings *models.InstanceReannounceSettings
	if h.reannounceStore != nil {
		s, err := h.reannounceStore.Get(r.Context(), instanceID)
		if err == nil {
			settings = s
		}
	}
	if settings == nil {
		settings = models.DefaultInstanceReannounceSettings(instanceID)
	}

	render(w, r, http.StatusOK, pages.ReannounceSettingsCard(pages.ReannounceSettingsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Settings:   pages.ReannounceSettingsFromModel(settings),
	}))
}

// PostReannounceSettings saves reannounce settings for an instance.
// POST /ui/partials/automations/reannounce/settings/{instanceId}
func (h *Handler) PostReannounceSettings(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Fetch existing or default.
	var existing *models.InstanceReannounceSettings
	if h.reannounceStore != nil {
		s, err := h.reannounceStore.Get(r.Context(), instanceID)
		if err == nil {
			existing = s
		}
	}
	if existing == nil {
		existing = models.DefaultInstanceReannounceSettings(instanceID)
	}

	// Apply form values.
	existing.Enabled = r.FormValue("enabled") == "on"
	existing.Aggressive = r.FormValue("aggressive") == "on"
	existing.MonitorAll = r.FormValue("monitor_all") == "on"
	existing.InitialWaitSeconds = intParam(r.FormValue("initial_wait_seconds"), existing.InitialWaitSeconds)
	existing.ReannounceIntervalSeconds = intParam(r.FormValue("reannounce_interval_seconds"), existing.ReannounceIntervalSeconds)
	existing.MaxAgeSeconds = intParam(r.FormValue("max_age_seconds"), existing.MaxAgeSeconds)
	existing.MaxRetries = intParam(r.FormValue("max_retries"), existing.MaxRetries)

	// Parse filter lists (newline-separated).
	existing.Categories = splitLines(r.FormValue("categories"))
	existing.Tags = splitLines(r.FormValue("tags"))
	existing.Trackers = splitLines(r.FormValue("trackers"))
	existing.ExcludeCategories = r.FormValue("exclude_categories") == "on"
	existing.ExcludeTags = r.FormValue("exclude_tags") == "on"
	existing.ExcludeTrackers = r.FormValue("exclude_trackers") == "on"

	errMsg := ""
	if h.reannounceStore != nil {
		if _, err := h.reannounceStore.Upsert(r.Context(), existing); err != nil {
			errMsg = "Failed to save: " + err.Error()
		}
	}

	render(w, r, http.StatusOK, pages.ReannounceSettingsCard(pages.ReannounceSettingsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Settings:   pages.ReannounceSettingsFromModel(existing),
		Success:    errMsg == "",
		Error:      errMsg,
	}))
}

// GetReannounceActivityPartial returns recent reannounce activity for an instance.
// GET /ui/partials/automations/reannounce/activity/{instanceId}
func (h *Handler) GetReannounceActivityPartial(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)

	var events []pages.ReannounceActivityEvent
	if h.reannounceService != nil && instanceID > 0 {
		for _, e := range h.reannounceService.GetActivity(instanceID, 50) {
			events = append(events, pages.ReannounceActivityEvent{
				Hash:        e.Hash,
				TorrentName: e.TorrentName,
				Trackers:    e.Trackers,
				Outcome:     string(e.Outcome),
				Reason:      e.Reason,
				Timestamp:   e.Timestamp.Format("2006-01-02 15:04:05"),
			})
		}
	}

	render(w, r, http.StatusOK, pages.ReannounceActivityPartial(pages.ReannounceActivityProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Events:     events,
	}))
}

// ------------------------------------------------------------------
// Orphan Scan handlers
// ------------------------------------------------------------------

// GetOrphanScanSettingsPartial returns orphan scan settings for an instance.
// GET /ui/partials/automations/orphan-scan/settings/{instanceId}
func (h *Handler) GetOrphanScanSettingsPartial(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	var settings *models.OrphanScanSettings
	if h.orphanScanStore != nil {
		s, err := h.orphanScanStore.GetSettings(r.Context(), instanceID)
		if err == nil && s != nil {
			settings = s
		}
	}
	if settings == nil {
		settings = &models.OrphanScanSettings{InstanceID: instanceID}
	}

	render(w, r, http.StatusOK, pages.OrphanScanSettingsCard(pages.OrphanScanProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Settings:   pages.OrphanScanSettingsFromModel(settings),
	}))
}

// PostOrphanScanSettings saves orphan scan settings for an instance.
// POST /ui/partials/automations/orphan-scan/settings/{instanceId}
func (h *Handler) PostOrphanScanSettings(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Fetch or default.
	existing := &models.OrphanScanSettings{InstanceID: instanceID}
	if h.orphanScanStore != nil {
		if s, err := h.orphanScanStore.GetSettings(r.Context(), instanceID); err == nil && s != nil {
			existing = s
		}
	}

	existing.Enabled = r.FormValue("enabled") == "on"
	existing.AutoCleanupEnabled = r.FormValue("auto_cleanup_enabled") == "on"
	existing.GracePeriodMinutes = intParam(r.FormValue("grace_period_minutes"), existing.GracePeriodMinutes)
	existing.ScanIntervalHours = intParam(r.FormValue("scan_interval_hours"), existing.ScanIntervalHours)
	existing.MaxFilesPerRun = intParam(r.FormValue("max_files_per_run"), existing.MaxFilesPerRun)
	existing.AutoCleanupMaxFiles = intParam(r.FormValue("auto_cleanup_max_files"), existing.AutoCleanupMaxFiles)
	existing.IgnorePaths = splitLines(r.FormValue("ignore_paths"))

	errMsg := ""
	if h.orphanScanStore != nil {
		if _, err := h.orphanScanStore.UpsertSettings(r.Context(), existing); err != nil {
			errMsg = "Failed to save: " + err.Error()
		}
	}

	render(w, r, http.StatusOK, pages.OrphanScanSettingsCard(pages.OrphanScanProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Settings:   pages.OrphanScanSettingsFromModel(existing),
		Success:    errMsg == "",
		Error:      errMsg,
	}))
}

// GetOrphanScanRunsPartial returns recent orphan scan runs for an instance.
// GET /ui/partials/automations/orphan-scan/runs/{instanceId}
func (h *Handler) GetOrphanScanRunsPartial(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)

	var runs []pages.OrphanScanRun
	if h.orphanScanStore != nil && instanceID > 0 {
		raw, err := h.orphanScanStore.ListRuns(r.Context(), instanceID, 10)
		if err == nil {
			for _, run := range raw {
				runs = append(runs, pages.OrphanScanRunFromModel(run))
			}
		}
	}

	render(w, r, http.StatusOK, pages.OrphanScanRunsPartial(pages.OrphanScanProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Runs:       runs,
	}))
}

// PostTriggerOrphanScan triggers an orphan scan for an instance.
// POST /ui/partials/automations/orphan-scan/trigger/{instanceId}
func (h *Handler) PostTriggerOrphanScan(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	errMsg := ""
	if h.orphanScanService != nil {
		if _, err := h.orphanScanService.TriggerScan(r.Context(), instanceID, "manual"); err != nil {
			errMsg = err.Error()
		}
	} else {
		errMsg = "Orphan scan service unavailable"
	}

	var runs []pages.OrphanScanRun
	if h.orphanScanStore != nil {
		if raw, err := h.orphanScanStore.ListRuns(r.Context(), instanceID, 10); err == nil {
			for _, run := range raw {
				runs = append(runs, pages.OrphanScanRunFromModel(run))
			}
		}
	}

	render(w, r, http.StatusOK, pages.OrphanScanRunsPartial(pages.OrphanScanProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Runs:       runs,
		Error:      errMsg,
	}))
}

// splitLines splits a newline- or comma-separated string into trimmed non-empty pieces.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.Trim(line, ","))
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ------------------------------------------------------------------
// Automation rule CRUD handlers
// ------------------------------------------------------------------

// GetAutomationRuleFormNew renders the create-rule form modal body.
// GET /ui/partials/automations/rules/new?instance_id={id}
func (h *Handler) GetAutomationRuleFormNew(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	insts := h.navInstances(r)
	render(w, r, http.StatusOK, pages.AutomationRuleFormNew(insts, instanceID, h.baseURL(), ""))
}

// GetAutomationRuleFormEdit renders the edit-rule form modal body.
// GET /ui/partials/automations/{instanceId}/rules/{id}/edit
func (h *Handler) GetAutomationRuleFormEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	id := intParam(chi.URLParam(r, "id"), 0)

	a, err := h.automationStore.Get(ctx, instanceID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	item := pages.AutomationRuleFormItem{
		ID:             a.ID,
		InstanceID:     a.InstanceID,
		Name:           a.Name,
		TrackerPattern: a.TrackerPattern,
		DryRun:         a.DryRun,
		Enabled:        a.Enabled,
	}
	render(w, r, http.StatusOK, pages.AutomationRuleFormEdit(item, h.baseURL()))
}

// PostAutomationRule creates a new automation rule.
// POST /ui/partials/automations/{instanceId}/rules
func (h *Handler) PostAutomationRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	if instanceID == 0 {
		instanceID = intParam(r.FormValue("instance_id"), 0)
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		insts := h.navInstances(r)
		render(w, r, http.StatusUnprocessableEntity, pages.AutomationRuleFormNew(insts, instanceID, h.baseURL(), "Name is required"))
		return
	}

	rule := &models.Automation{
		InstanceID:     instanceID,
		Name:           name,
		TrackerPattern: strings.TrimSpace(r.FormValue("tracker_pattern")),
		DryRun:         r.FormValue("dry_run") == "true",
		Enabled:        r.FormValue("enabled") == "true",
	}

	if _, err := h.automationStore.Create(ctx, rule); err != nil {
		log.Error().Err(err).Msg("ui: failed to create automation rule")
		insts := h.navInstances(r)
		render(w, r, http.StatusUnprocessableEntity, pages.AutomationRuleFormNew(insts, instanceID, h.baseURL(), "Failed to create rule: "+err.Error()))
		return
	}

	w.Header().Set("HX-Trigger", "closeModal")
	h.renderAutomationsPartial(w, r, instanceID)
}

// PutAutomationRule updates an existing automation rule.
// PUT /ui/partials/automations/{instanceId}/rules/{id}
func (h *Handler) PutAutomationRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	id := intParam(chi.URLParam(r, "id"), 0)

	a, err := h.automationStore.Get(ctx, instanceID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		item := pages.AutomationRuleFormItem{
			ID: a.ID, InstanceID: a.InstanceID, Name: a.Name,
			TrackerPattern: a.TrackerPattern, DryRun: a.DryRun, Enabled: a.Enabled,
			ErrMsg: "Name is required",
		}
		render(w, r, http.StatusUnprocessableEntity, pages.AutomationRuleFormEdit(item, h.baseURL()))
		return
	}

	a.Name = name
	a.TrackerPattern = strings.TrimSpace(r.FormValue("tracker_pattern"))
	a.DryRun = r.FormValue("dry_run") == "true"
	a.Enabled = r.FormValue("enabled") == "true"

	if _, err := h.automationStore.Update(ctx, a); err != nil {
		log.Error().Err(err).Msg("ui: failed to update automation rule")
		item := pages.AutomationRuleFormItem{
			ID: a.ID, InstanceID: a.InstanceID, Name: a.Name,
			TrackerPattern: a.TrackerPattern, DryRun: a.DryRun, Enabled: a.Enabled,
			ErrMsg: "Failed to update rule: " + err.Error(),
		}
		render(w, r, http.StatusUnprocessableEntity, pages.AutomationRuleFormEdit(item, h.baseURL()))
		return
	}

	w.Header().Set("HX-Trigger", "closeModal")
	h.renderAutomationsPartial(w, r, instanceID)
}

// DeleteAutomationRule deletes an automation rule.
// DELETE /ui/partials/automations/{instanceId}/rules/{id}
func (h *Handler) DeleteAutomationRule(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	id := intParam(chi.URLParam(r, "id"), 0)

	if err := h.automationStore.Delete(r.Context(), instanceID, id); err != nil {
		log.Error().Err(err).Msg("ui: failed to delete automation rule")
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	// Return empty HTML; HTMX outerHTML swap on the row will remove it.
	w.WriteHeader(http.StatusOK)
}

// renderAutomationsPartial fetches all automations data and renders AutomationsPartial.
func (h *Handler) renderAutomationsPartial(w http.ResponseWriter, r *http.Request, instanceID int) {
	insts := h.navInstances(r)
	p := pages.AutomationsProps{
		BaseURL:    h.baseURL(),
		Instances:  insts,
		InstanceID: instanceID,
	}
	h.fillAutomationsData(r.Context(), &p, instanceID, insts)
	render(w, r, http.StatusOK, pages.AutomationsPartial(p))
}
