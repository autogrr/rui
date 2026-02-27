// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// settings_trackers.go contains HTMX handlers for the tracker customizations
// section of the settings page.

package ui

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/pages"
)

// GetTrackerCustomizationFormNew renders the create form into #tracker-form-slot.
// GET /ui/partials/settings/trackers/form
func (h *Handler) GetTrackerCustomizationFormNew(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.TrackerCustomizationFormCreate(h.baseURL(), ""))
}

// GetTrackerCustomizationFormEdit renders the edit form for an existing entry.
// GET /ui/partials/settings/trackers/{id}/form
func (h *Handler) GetTrackerCustomizationFormEdit(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.trackerCustomizationStore == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	c, err := h.trackerCustomizationStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	item := pages.TrackerCustomizationFromModel(c)
	render(w, r, http.StatusOK, pages.TrackerCustomizationFormEdit(item, h.baseURL()))
}

// PostTrackerCustomization creates a new tracker customization.
// POST /ui/partials/settings/trackers
func (h *Handler) PostTrackerCustomization(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.TrackerCustomizationFormCreate(h.baseURL(), "Display name is required"))
		return
	}

	record := &models.TrackerCustomization{
		DisplayName:     displayName,
		Domains:         splitCSV(r.FormValue("domains")),
		IncludedInStats: splitCSV(r.FormValue("included_in_stats")),
	}

	if _, err := h.trackerCustomizationStore.Create(r.Context(), record); err != nil {
		log.Error().Err(err).Msg("ui: failed to create tracker customization")
		render(w, r, http.StatusUnprocessableEntity, pages.TrackerCustomizationFormCreate(h.baseURL(), "Failed to create: "+err.Error()))
		return
	}

	h.renderTrackerCustomizationList(w, r)
}

// PutTrackerCustomization updates an existing tracker customization.
// PUT /ui/partials/settings/trackers/{id}
func (h *Handler) PutTrackerCustomization(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.trackerCustomizationStore == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	c, err := h.trackerCustomizationStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		item := pages.TrackerCustomizationFromModel(c)
		item.ErrMsg = "Display name is required"
		render(w, r, http.StatusUnprocessableEntity, pages.TrackerCustomizationFormEdit(item, h.baseURL()))
		return
	}

	c.DisplayName = displayName
	c.Domains = splitCSV(r.FormValue("domains"))
	c.IncludedInStats = splitCSV(r.FormValue("included_in_stats"))

	updated, err := h.trackerCustomizationStore.Update(r.Context(), c)
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to update tracker customization")
		item := pages.TrackerCustomizationFromModel(c)
		item.ErrMsg = "Failed to update: " + err.Error()
		render(w, r, http.StatusUnprocessableEntity, pages.TrackerCustomizationFormEdit(item, h.baseURL()))
		return
	}

	item := pages.TrackerCustomizationFromModel(updated)
	render(w, r, http.StatusOK, pages.TrackerCustomizationRowTempl(item, h.baseURL()))
}

// DeleteTrackerCustomization deletes a tracker customization entry.
// DELETE /ui/partials/settings/trackers/{id}
func (h *Handler) DeleteTrackerCustomization(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.trackerCustomizationStore == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := h.trackerCustomizationStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Msg("ui: failed to delete tracker customization")
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	// Empty 200 causes HTMX outerHTML swap to delete the row element.
	w.WriteHeader(http.StatusOK)
}

// renderTrackerCustomizationList reloads and renders the full tracker list.
func (h *Handler) renderTrackerCustomizationList(w http.ResponseWriter, r *http.Request) {
	var customs []*models.TrackerCustomization
	if h.trackerCustomizationStore != nil {
		if list, err := h.trackerCustomizationStore.List(r.Context()); err == nil {
			customs = list
		}
	}
	render(w, r, http.StatusOK, pages.TrackerCustomizationListPartial(customs, h.baseURL()))
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
