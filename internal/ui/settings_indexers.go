// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// settings_indexers.go contains HTMX partial handlers for the Indexers and
// Search Cache settings sections.

package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/jackett"
	"github.com/autogrr/rui/internal/ui/pages"
	gojackett "github.com/autogrr/rui/pkg/gojackett"
	"github.com/autogrr/rui/pkg/prowlarr"
)

// ──────────────────────────────────────────────────────────────────
// Indexers section
//
//	GET  /ui/partials/settings/indexers              → list fragment
//	GET  /ui/partials/settings/indexers/form         → new indexer form
//	GET  /ui/partials/settings/indexers/form/{id}    → edit indexer form
//	POST /ui/partials/settings/indexers              → create indexer
//	PUT  /ui/partials/settings/indexers/{id}         → update indexer
//	DELETE /ui/partials/settings/indexers/{id}       → delete indexer
//	POST /ui/partials/settings/indexers/{id}/test    → test connectivity
//	POST /ui/partials/settings/search-cache          → update cache TTL
// ──────────────────────────────────────────────────────────────────

// GetIndexersListPartial returns the indexers list fragment.
func (h *Handler) GetIndexersListPartial(w http.ResponseWriter, r *http.Request) {
	indexers, err := h.indexerStore.List(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list indexers")
		indexers = nil
	}
	render(w, r, http.StatusOK, pages.IndexerListPartial(indexers, h.baseURL()))
}

// GetIndexerForm returns the new-indexer or edit-indexer form fragment.
func (h *Handler) GetIndexerForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	base := h.baseURL()

	if idStr == "" {
		render(w, r, http.StatusOK, pages.IndexerFormNew(base, ""))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		render(w, r, http.StatusOK, pages.IndexerFormNew(base, "Invalid indexer ID"))
		return
	}

	idx, err := h.indexerStore.Get(r.Context(), id)
	if err != nil {
		render(w, r, http.StatusOK, pages.IndexerFormNew(base, "Indexer not found"))
		return
	}

	render(w, r, http.StatusOK, pages.IndexerFormEdit(*idx, base, ""))
}

// PostIndexer creates a new indexer from a form submission.
func (h *Handler) PostIndexer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	indexerID := strings.TrimSpace(r.FormValue("indexer_id"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	backendStr := r.FormValue("backend")
	enabled := r.FormValue("enabled") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "Name is required"))
		return
	}
	if baseURL == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "Base URL is required"))
		return
	}
	if apiKey == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "API key is required"))
		return
	}

	backend, err := models.ParseTorznabBackend(backendStr)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "Invalid backend"))
		return
	}

	priority, _ := strconv.Atoi(r.FormValue("priority"))
	timeout, _ := strconv.Atoi(r.FormValue("timeout_seconds"))
	if timeout <= 0 {
		timeout = 30
	}

	ctx := r.Context()
	ownerID := h.sessionManager.GetInt(ctx, "user_id")
	_, err = h.indexerStore.CreateWithIndexerID(ctx, ownerID, name, baseURL, indexerID, apiKey, nil, nil, enabled, priority, timeout, backend)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormNew(h.baseURL(), "Failed to create indexer: "+err.Error()))
		return
	}

	indexers, _ := h.indexerStore.List(ctx)
	render(w, r, http.StatusOK, pages.IndexerFormSuccess(indexers, h.baseURL()))
}

// PutIndexer updates an existing indexer from a form submission.
func (h *Handler) PutIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid indexer ID", http.StatusBadRequest)
		return
	}

	existing, err := h.indexerStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Indexer not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormEdit(*existing, h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	indexerID := strings.TrimSpace(r.FormValue("indexer_id"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	backendStr := r.FormValue("backend")
	enabled := r.FormValue("enabled") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormEdit(*existing, h.baseURL(), "Name is required"))
		return
	}
	if baseURL == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormEdit(*existing, h.baseURL(), "Base URL is required"))
		return
	}

	priority, _ := strconv.Atoi(r.FormValue("priority"))
	timeout, _ := strconv.Atoi(r.FormValue("timeout_seconds"))
	if timeout <= 0 {
		timeout = 30
	}

	params := models.TorznabIndexerUpdateParams{
		Name:    name,
		BaseURL: baseURL,
		Enabled: &enabled,
	}

	if apiKey != "" {
		params.APIKey = apiKey
	}

	if indexerID != "" {
		params.IndexerID = &indexerID
	}

	if backendStr != "" {
		backend, err := models.ParseTorznabBackend(backendStr)
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormEdit(*existing, h.baseURL(), "Invalid backend"))
			return
		}
		params.Backend = &backend
	}

	params.TimeoutSeconds = &timeout
	params.Priority = &priority

	ctx := r.Context()
	_, err = h.indexerStore.Update(ctx, id, params)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerFormEdit(*existing, h.baseURL(), "Failed to update indexer: "+err.Error()))
		return
	}

	indexers, _ := h.indexerStore.List(ctx)
	render(w, r, http.StatusOK, pages.IndexerFormSuccess(indexers, h.baseURL()))
}

// DeleteIndexer deletes an indexer; returns empty body for outerHTML swap.
func (h *Handler) DeleteIndexer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid indexer ID", http.StatusBadRequest)
		return
	}

	if err := h.indexerStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete indexer")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// PostIndexerTest tests indexer connectivity via capability sync.
func (h *Handler) PostIndexerTest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid indexer ID", http.StatusBadRequest)
		return
	}

	if h.jackettService == nil {
		http.Error(w, "Indexer service not available", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, testErr := h.jackettService.SyncIndexerCaps(ctx, id)
	if testErr != nil {
		log.Debug().Err(testErr).Int("indexer_id", id).Msg("ui: indexer test failed")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<span class="text-destructive text-xs">Connection failed: ` + testErr.Error() + `</span>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<span class="text-green-600 text-xs">Connection OK</span>`))
}

// ──────────────────────────────────────────────────────────────────
// Search Cache section
// ──────────────────────────────────────────────────────────────────

// PostSearchCacheTTL updates the search cache TTL setting.
func (h *Handler) PostSearchCacheTTL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.SearchCacheResultPartial(false, "Invalid form data"))
		return
	}

	ttl, err := strconv.Atoi(r.FormValue("ttl_minutes"))
	if err != nil || ttl < jackett.MinSearchCacheTTLMinutes {
		render(w, r, http.StatusUnprocessableEntity, pages.SearchCacheResultPartial(false,
			"TTL must be at least "+strconv.Itoa(jackett.MinSearchCacheTTLMinutes)+" minutes"))
		return
	}

	if h.jackettService == nil {
		render(w, r, http.StatusUnprocessableEntity, pages.SearchCacheResultPartial(false, "Indexer service not available"))
		return
	}

	ctx := r.Context()
	if _, err := h.jackettService.UpdateSearchCacheSettings(ctx, ttl); err != nil {
		log.Error().Err(err).Msg("ui: failed to update search cache settings")
		render(w, r, http.StatusUnprocessableEntity, pages.SearchCacheResultPartial(false, "Failed to update cache settings"))
		return
	}

	render(w, r, http.StatusOK, pages.SearchCacheResultPartial(true, "Cache settings updated"))
}

// ──────────────────────────────────────────────────────────────────
// Indexer Discovery
//
//	GET  /ui/partials/settings/indexers/discover   → discovery form
//	POST /ui/partials/settings/indexers/discover   → fetch remote indexers
//	POST /ui/partials/settings/indexers/import     → bulk create indexers
// ──────────────────────────────────────────────────────────────────

// GetIndexerDiscoverForm returns the discover-indexers form fragment.
func (h *Handler) GetIndexerDiscoverForm(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.IndexerDiscoverForm(h.baseURL(), ""))
}

// PostIndexerDiscoverList fetches all configured indexers from a remote Jackett or
// Prowlarr instance and returns the selectable list fragment.
func (h *Handler) PostIndexerDiscoverList(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "Invalid form data"))
		return
	}

	backend := r.FormValue("backend")
	baseURL := strings.TrimRight(strings.TrimSpace(r.FormValue("base_url")), "/")
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	if baseURL == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "Base URL is required"))
		return
	}
	if apiKey == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "API Key is required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// Build a set of already-added indexer IDs for this backend so we can mark duplicates.
	addedIDs := map[string]struct{}{}
	if h.indexerStore != nil {
		existing, _ := h.indexerStore.List(ctx)
		for _, idx := range existing {
			if string(idx.Backend) == backend && idx.IndexerID != "" {
				addedIDs[idx.IndexerID] = struct{}{}
			}
		}
	}

	var items []pages.DiscoveredIndexer

	switch backend {
	case "jackett":
		client := gojackett.NewClient(gojackett.Config{
			Host:   baseURL,
			APIKey: apiKey,
		})
		indexers, err := client.GetIndexersCtx(ctx)
		if err != nil {
			log.Warn().Err(err).Str("base_url", baseURL).Msg("ui: jackett discover failed")
			render(w, r, http.StatusOK, pages.IndexerDiscoverForm(h.baseURL(),
				"Failed to connect to Jackett: "+err.Error()))
			return
		}
		for _, idx := range indexers.Indexer {
			_, alreadyAdded := addedIDs[idx.ID]
			items = append(items, pages.DiscoveredIndexer{
				ID:           idx.ID,
				Name:         idx.Title,
				Description:  idx.Description,
				Protocol:     "torrent",
				Type:         idx.Type,
				Configured:   idx.Configured == "true",
				AlreadyAdded: alreadyAdded,
			})
		}

	case "prowlarr":
		client := prowlarr.NewClient(prowlarr.Config{
			Host:   baseURL,
			APIKey: apiKey,
		})
		indexers, err := client.GetIndexers(ctx)
		if err != nil {
			log.Warn().Err(err).Str("base_url", baseURL).Msg("ui: prowlarr discover failed")
			render(w, r, http.StatusOK, pages.IndexerDiscoverForm(h.baseURL(),
				"Failed to connect to Prowlarr: "+err.Error()))
			return
		}
		for _, idx := range indexers {
			if idx.Protocol != "torrent" {
				continue // skip usenet-only indexers
			}
			idStr := strconv.Itoa(idx.ID)
			_, alreadyAdded := addedIDs[idStr]
			items = append(items, pages.DiscoveredIndexer{
				ID:           idStr,
				Name:         idx.Name,
				Description:  idx.Description,
				Protocol:     idx.Protocol,
				Enabled:      idx.Enable,
				AlreadyAdded: alreadyAdded,
			})
		}

	default:
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "Unsupported backend: "+backend))
		return
	}

	render(w, r, http.StatusOK, pages.IndexerDiscoverListPartial(items, h.baseURL(), baseURL, apiKey, backend))
}

// PostIndexerBulkImport creates all selected discovered indexers.
func (h *Handler) PostIndexerBulkImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "Invalid form data"))
		return
	}

	backend := r.FormValue("backend")
	baseURL := strings.TrimRight(strings.TrimSpace(r.FormValue("base_url")), "/")
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	selectedIDs := r.Form["indexer_id[]"]

	if len(selectedIDs) == 0 {
		indexers, _ := h.indexerStore.List(r.Context())
		render(w, r, http.StatusOK, pages.IndexerImportResultPartial(0, 0, "No indexers selected.", indexers, h.baseURL()))
		return
	}

	parsedBackend, err := models.ParseTorznabBackend(backend)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IndexerDiscoverForm(h.baseURL(), "Invalid backend"))
		return
	}

	ctx := r.Context()
	ownerID := h.sessionManager.GetInt(ctx, "user_id")
	imported := 0
	skipped := 0

	for _, idStr := range selectedIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}

		// Build a user-friendly name: for Jackett use the ID as default name (caps sync
		// will populate the real name later); for Prowlarr we only have the ID here.
		name := idStr

		_, err := h.indexerStore.CreateWithIndexerID(
			ctx, ownerID, name, baseURL, idStr, apiKey,
			nil, nil, true, 0, 30, parsedBackend,
		)
		if err != nil {
			log.Warn().Err(err).Str("indexer_id", idStr).Msg("ui: bulk import: failed to create indexer")
			skipped++
			continue
		}
		imported++

		// Best-effort capability sync so the name + categories are populated immediately.
		if h.jackettService != nil {
			go func(id string) {
				// Get the newly created indexer to find its database ID.
				all, _ := h.indexerStore.List(context.Background())
				for _, idx := range all {
					if idx.IndexerID == id && string(idx.Backend) == backend {
						syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if _, err := h.jackettService.SyncIndexerCaps(syncCtx, idx.ID); err != nil {
							log.Debug().Err(err).Str("indexer_id", id).Msg("ui: bulk import: caps sync failed")
						}
						return
					}
				}
			}(idStr)
		}
	}

	indexers, _ := h.indexerStore.List(ctx)
	render(w, r, http.StatusOK, pages.IndexerImportResultPartial(imported, skipped, "", indexers, h.baseURL()))
}
