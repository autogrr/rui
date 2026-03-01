// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/library"
)

// LibraryHandler handles REST endpoints for library management.
type LibraryHandler struct {
	titleStore *models.LibraryTitleStore
	ruleStore  *models.LibraryRuleStore
	libService *library.Service
}

// NewLibraryHandler creates a new LibraryHandler.
func NewLibraryHandler(
	titleStore *models.LibraryTitleStore,
	ruleStore *models.LibraryRuleStore,
	libService *library.Service,
) *LibraryHandler {
	return &LibraryHandler{
		titleStore: titleStore,
		ruleStore:  ruleStore,
		libService: libService,
	}
}

// Routes registers all library routes under r.
func (h *LibraryHandler) Routes(r chi.Router) {
	r.Route("/library", func(r chi.Router) {
		r.Get("/titles", h.ListTitles)
		r.Get("/titles/{id}", h.GetTitle)
		r.Delete("/titles/{id}", h.DeleteTitle)
		r.Get("/titles/search", h.SearchTitles)
		r.Post("/sync/{instanceId}", h.SyncInstance)
		r.Post("/match", h.MatchRelease)

		r.Get("/rules", h.ListRules)
		r.Post("/rules", h.CreateRule)
		r.Get("/rules/{id}", h.GetRule)
		r.Put("/rules/{id}", h.UpdateRule)
		r.Delete("/rules/{id}", h.DeleteRule)
	})
}

// ListTitles handles GET /api/library/titles
func (h *LibraryHandler) ListTitles(w http.ResponseWriter, r *http.Request) {
	ownerID := 1 // single-user: owner is always user ID 1
	ct := r.URL.Query().Get("content_type")

	ctx := r.Context()
	var (
		titles []*models.LibraryTitle
		err    error
	)
	if ct != "" {
		titles, err = h.titleStore.ListByContentType(ctx, ownerID, models.LibraryContentType(ct))
	} else {
		titles, err = h.titleStore.List(ctx, ownerID)
	}
	if err != nil {
		log.Error().Err(err).Msg("[LIBRARY] list titles failed")
		RespondError(w, http.StatusInternalServerError, "failed to list titles")
		return
	}
	RespondJSON(w, http.StatusOK, titles)
}

// GetTitle handles GET /api/library/titles/{id}
func (h *LibraryHandler) GetTitle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	title, err := h.titleStore.Get(r.Context(), id)
	if err != nil {
		if err == models.ErrLibraryTitleNotFound {
			RespondError(w, http.StatusNotFound, "title not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get title")
		return
	}
	RespondJSON(w, http.StatusOK, title)
}

// DeleteTitle handles DELETE /api/library/titles/{id}
func (h *LibraryHandler) DeleteTitle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.titleStore.Delete(r.Context(), id); err != nil {
		if err == models.ErrLibraryTitleNotFound {
			RespondError(w, http.StatusNotFound, "title not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to delete title")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// SearchTitles handles GET /api/library/titles/search?q=…
func (h *LibraryHandler) SearchTitles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		RespondError(w, http.StatusBadRequest, "q parameter required")
		return
	}
	ownerID := 1 // single-user: owner is always user ID 1
	titles, err := h.titleStore.Search(r.Context(), ownerID, q)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "search failed")
		return
	}
	RespondJSON(w, http.StatusOK, titles)
}

// SyncInstance handles POST /api/library/sync/{instanceId}
func (h *LibraryHandler) SyncInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := parseID(w, r, "instanceId")
	if !ok {
		return
	}
	ownerID := 1 // single-user: owner is always user ID 1
	ct := r.URL.Query().Get("content_type")

	ctx := r.Context()
	var result interface{}
	var err error

	switch ct {
	case "movie":
		result, err = h.libService.SyncMovies(ctx, ownerID, instanceID)
	default:
		result, err = h.libService.SyncSeries(ctx, ownerID, instanceID)
	}
	if err != nil {
		log.Error().Err(err).Int("instanceID", instanceID).Msg("[LIBRARY] sync failed")
		RespondError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// MatchRelease handles POST /api/library/match
func (h *LibraryHandler) MatchRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseName string `json:"release_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ReleaseName == "" {
		RespondError(w, http.StatusBadRequest, "release_name required")
		return
	}
	ownerID := 1 // single-user: owner is always user ID 1
	matches, err := h.libService.Match(r.Context(), ownerID, req.ReleaseName)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "match failed")
		return
	}
	RespondJSON(w, http.StatusOK, matches)
}

// ─── Rules ────────────────────────────────────────────────────────────────────

// ListRules handles GET /api/library/rules
func (h *LibraryHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	ownerID := 1 // single-user: owner is always user ID 1
	rules, err := h.ruleStore.List(r.Context(), ownerID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	RespondJSON(w, http.StatusOK, rules)
}

// CreateRule handles POST /api/library/rules
func (h *LibraryHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var rule models.LibraryRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.OwnerID = 1 // single-user: owner is always user ID 1
	created, err := h.ruleStore.Create(r.Context(), &rule)
	if err != nil {
		log.Error().Err(err).Msg("[LIBRARY] create rule failed")
		RespondError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}
	RespondJSON(w, http.StatusCreated, created)
}

// GetRule handles GET /api/library/rules/{id}
func (h *LibraryHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	rule, err := h.ruleStore.Get(r.Context(), id)
	if err != nil {
		if err == models.ErrLibraryRuleNotFound {
			RespondError(w, http.StatusNotFound, "rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get rule")
		return
	}
	RespondJSON(w, http.StatusOK, rule)
}

// UpdateRule handles PUT /api/library/rules/{id}
func (h *LibraryHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var rule models.LibraryRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.ID = id
	updated, err := h.ruleStore.Update(r.Context(), &rule)
	if err != nil {
		if err == models.ErrLibraryRuleNotFound {
			RespondError(w, http.StatusNotFound, "rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}
	RespondJSON(w, http.StatusOK, updated)
}

// DeleteRule handles DELETE /api/library/rules/{id}
func (h *LibraryHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.ruleStore.Delete(r.Context(), id); err != nil {
		if err == models.ErrLibraryRuleNotFound {
			RespondError(w, http.StatusNotFound, "rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseID(w http.ResponseWriter, r *http.Request, param string) (int, bool) {
	idStr := chi.URLParam(r, param)
	if idStr == "" {
		RespondError(w, http.StatusBadRequest, "missing "+param)
		return 0, false
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		RespondError(w, http.StatusBadRequest, "invalid "+param)
		return 0, false
	}
	return id, true
}
