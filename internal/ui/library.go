// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"net/http"

	"github.com/rs/zerolog/log"

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

// PostLibrarySync triggers a library sync for all *arr instances.
func (h *Handler) PostLibrarySync(w http.ResponseWriter, r *http.Request) {
	if h.libraryService == nil {
		http.Error(w, "library service not available", http.StatusServiceUnavailable)
		return
	}
	// Re-render titles table after sync (returns HTMX partial).
	// Sync is best-effort; partial re-render is unconditional.
	if h.arrInstanceStore != nil {
		ctx := r.Context()
		instances, err := h.arrInstanceStore.List(ctx)
		if err == nil {
			for _, inst := range instances {
				if _, err := h.libraryService.SyncSeries(ctx, 1, inst.ID); err != nil {
					log.Warn().Err(err).Int("instanceID", inst.ID).Msg("[UI] library sync series failed")
				}
				if _, err := h.libraryService.SyncMovies(ctx, 1, inst.ID); err != nil {
					log.Warn().Err(err).Int("instanceID", inst.ID).Msg("[UI] library sync movies failed")
				}
			}
		}
	}
	// Redirect back to library page so HTMX gets the refreshed table.
	http.Redirect(w, r, h.baseURL()+"/ui/library", http.StatusSeeOther)
}
