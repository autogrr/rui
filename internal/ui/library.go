// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// GetLibrary renders the library management page.
func (h *Handler) GetLibrary(w http.ResponseWriter, r *http.Request) {
	username := UsernameFromContext(r.Context())
	ctx := r.Context()

	insts, _ := h.instanceStore.List(ctx)
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	props := pages.LibraryProps{
		BaseURL:     h.baseURL(),
		Username:    username,
		Version:     h.version,
		CurrentPath: "/ui/library",
		Instances:   navInsts,
	}

	if h.libraryTitleStore != nil {
		titles, err := h.libraryTitleStore.List(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] library: failed to list titles")
		} else {
			props.Titles = titles
		}
	}

	if h.libraryRuleStore != nil {
		rules, err := h.libraryRuleStore.List(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] library: failed to list rules")
		} else {
			props.Rules = rules
		}
	}

	render(w, r, http.StatusOK, pages.Library(props))
}

// PostLibrarySync triggers a library sync for all *arr instances and returns
// the updated #library-titles-table partial for HTMX outerHTML swap.
func (h *Handler) PostLibrarySync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.libraryService == nil {
		setToast(w, "error", "Library service not available")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if h.arrInstanceStore == nil {
		setToast(w, "warning", "No *arr instances configured — add a Sonarr or Radarr instance first")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	instances, err := h.arrInstanceStore.List(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("[UI] library sync: failed to list arr instances")
		setToast(w, "error", "Failed to list *arr instances")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	if len(instances) == 0 {
		setToast(w, "warning", "No *arr instances configured")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	var totalUpserted, totalErrors int
	for _, inst := range instances {
		if sr, err := h.libraryService.SyncSeries(ctx, 1, inst.ID); err != nil {
			log.Warn().Err(err).Int("instanceID", inst.ID).Msg("[UI] library sync series failed")
			totalErrors++
		} else if sr != nil {
			totalUpserted += sr.Upserted
			totalErrors += sr.Errors
		}
		if sr, err := h.libraryService.SyncMovies(ctx, 1, inst.ID); err != nil {
			log.Warn().Err(err).Int("instanceID", inst.ID).Msg("[UI] library sync movies failed")
			totalErrors++
		} else if sr != nil {
			totalUpserted += sr.Upserted
			totalErrors += sr.Errors
		}
	}

	if totalErrors > 0 && totalUpserted == 0 {
		setToast(w, "error", fmt.Sprintf("Sync failed for all instances (%d error(s))", totalErrors))
	} else if totalErrors > 0 {
		setToast(w, "warning", fmt.Sprintf("Synced %d title(s) with %d error(s)", totalUpserted, totalErrors))
	} else {
		setToast(w, "success", fmt.Sprintf("Synced %d title(s) from *arr", totalUpserted))
	}

	var titles []*models.LibraryTitle
	if h.libraryTitleStore != nil {
		var err error
		titles, err = h.libraryTitleStore.List(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] library: failed to list titles after sync")
		}
	}

	render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
}

// PostLibraryScanClients scans all connected qBittorrent instances and upserts
// torrent-sourced titles into the library. Returns the updated titles table partial.
func (h *Handler) PostLibraryScanClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.libraryService == nil {
		setToast(w, "error", "Library service not available")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if h.syncManager == nil {
		setToast(w, "error", "qBittorrent sync manager not available")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	instances, err := h.instanceStore.List(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("[UI] library scan: failed to list instances")
		setToast(w, "error", "Failed to list qBittorrent instances")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	// Only scan active instances.
	var active []*models.Instance
	for _, inst := range instances {
		if inst.IsActive {
			active = append(active, inst)
		}
	}

	if len(active) == 0 {
		setToast(w, "warning", "No active qBittorrent instances found — add an instance in Settings")
		var titles []*models.LibraryTitle
		if h.libraryTitleStore != nil {
			titles, _ = h.libraryTitleStore.List(ctx, 1)
		}
		render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
		return
	}

	var totalUpserted, totalErrors, totalTorrents int
	for _, inst := range active {
		sr, err := h.libraryService.ScanTorrentClients(ctx, 1, inst.ID, h.syncManager)
		if err != nil {
			log.Warn().Err(err).Int("instanceID", inst.ID).Str("instance", inst.Name).Msg("[UI] library scan clients failed")
			totalErrors++
			continue
		}
		if sr != nil {
			totalUpserted += sr.Upserted
			totalErrors += sr.Errors
			totalTorrents += sr.TotalTorrents
		}
	}

	if totalErrors > 0 && totalUpserted == 0 {
		setToast(w, "error", fmt.Sprintf("Scan failed — could not read from %d instance(s). Is qBittorrent connected?", totalErrors))
	} else if totalTorrents == 0 {
		setToast(w, "warning", "No torrents found — is the SyncManager connected to qBittorrent?")
	} else if totalErrors > 0 {
		setToast(w, "warning", fmt.Sprintf("Found %d unique title(s) from %d torrent(s) with %d error(s)", totalUpserted, totalTorrents, totalErrors))
	} else {
		setToast(w, "success", fmt.Sprintf("Found %d unique title(s) from %d torrent(s) across %d instance(s)", totalUpserted, totalTorrents, len(active)))
	}

	var titles []*models.LibraryTitle
	if h.libraryTitleStore != nil {
		var err error
		titles, err = h.libraryTitleStore.List(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] library: failed to list titles after scan")
		}
	}

	render(w, r, http.StatusOK, pages.LibraryTitlesTablePartial(titles))
}

// setToast writes an HX-Trigger header that fires a toast notification on the client.
// typ is one of: success, error, warning, info.
func setToast(w http.ResponseWriter, typ, msg string) {
	type toastPayload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	type triggerPayload struct {
		Toast toastPayload `json:"toast"`
	}
	b, err := json.Marshal(triggerPayload{Toast: toastPayload{Type: typ, Message: msg}})
	if err != nil {
		return
	}
	w.Header().Set("HX-Trigger", string(b))
}

// libraryRulesPartial fetches the fresh rule list and renders the partial.
func (h *Handler) libraryRulesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var rules []*models.LibraryRule
	if h.libraryRuleStore != nil {
		var err error
		rules, err = h.libraryRuleStore.List(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] library: failed to list rules")
		}
	}

	baseURL := h.baseURL()
	render(w, r, http.StatusOK, pages.LibraryRulesListPartial(rules, baseURL))
}

// GetLibraryRuleForm renders the blank create-rule form (HTMX partial).
func (h *Handler) GetLibraryRuleForm(w http.ResponseWriter, r *http.Request) {
	blank := &models.LibraryRule{}
	render(w, r, http.StatusOK, pages.LibraryRuleFormPartial(blank, h.baseURL(), false))
}

// GetLibraryRuleFormEdit renders the edit-rule form pre-populated with existing data.
func (h *Handler) GetLibraryRuleFormEdit(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if h.libraryRuleStore == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	rule, err := h.libraryRuleStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	render(w, r, http.StatusOK, pages.LibraryRuleFormPartial(rule, h.baseURL(), true))
}

// PostLibraryRule creates a new library rule.
func (h *Handler) PostLibraryRule(w http.ResponseWriter, r *http.Request) {
	if h.libraryRuleStore == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	rule := &models.LibraryRule{
		OwnerID:      1,
		Name:         r.FormValue("name"),
		Enabled:      r.FormValue("enabled") == "on",
		SortOrder:    intParam(r.FormValue("sort_order"), 10),
		ContentTypes: r.Form["content_types"],
		ExprFilter:   r.FormValue("expr_filter"),
		Action:       models.LibraryAction(r.FormValue("action")),
	}

	if _, err := h.libraryRuleStore.Create(r.Context(), rule); err != nil {
		log.Warn().Err(err).Msg("[UI] failed to create library rule")
		http.Error(w, "failed to create rule", http.StatusInternalServerError)
		return
	}

	h.libraryRulesPartial(w, r)
}

// PutLibraryRule updates an existing library rule.
func (h *Handler) PutLibraryRule(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.libraryRuleStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	existing, err := h.libraryRuleStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	existing.Name = r.FormValue("name")
	existing.Enabled = r.FormValue("enabled") == "on"
	existing.SortOrder = intParam(r.FormValue("sort_order"), existing.SortOrder)
	existing.ContentTypes = r.Form["content_types"]
	existing.ExprFilter = r.FormValue("expr_filter")
	existing.Action = models.LibraryAction(r.FormValue("action"))

	if _, err := h.libraryRuleStore.Update(r.Context(), existing); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to update library rule")
		http.Error(w, "failed to update rule", http.StatusInternalServerError)
		return
	}

	h.libraryRulesPartial(w, r)
}

// DeleteLibraryRule removes a library rule.
func (h *Handler) DeleteLibraryRule(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.libraryRuleStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.libraryRuleStore.Delete(r.Context(), id); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to delete library rule")
		http.Error(w, "failed to delete rule", http.StatusInternalServerError)
		return
	}

	h.libraryRulesPartial(w, r)
}

// PostLibraryRuleToggle toggles the enabled state of a library rule.
func (h *Handler) PostLibraryRuleToggle(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.libraryRuleStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	rule, err := h.libraryRuleStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	rule.Enabled = !rule.Enabled
	if _, err := h.libraryRuleStore.Update(r.Context(), rule); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to toggle library rule")
		http.Error(w, "failed to toggle rule", http.StatusInternalServerError)
		return
	}

	h.libraryRulesPartial(w, r)
}
