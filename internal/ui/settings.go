// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// settings.go contains HTMX partial handlers for the settings page sections
// that require interactivity (API key management, change password).

package ui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/auth"
	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/partials/settings/api-keys          → list fragment
// GET /ui/partials/settings/api-keys/form     → create form
// POST /ui/partials/settings/api-keys         → create key
// DELETE /ui/partials/settings/api-keys/{id}  → delete key
// POST /ui/partials/settings/security         → change password
// ------------------------------------------------------------------

// GetAPIKeysListPartial returns the API key list HTML fragment.
func (h *Handler) GetAPIKeysListPartial(w http.ResponseWriter, r *http.Request) {
	keys, err := h.authService.ListAPIKeys(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list api keys")
		keys = []*models.APIKey{}
	}
	render(w, r, http.StatusOK, pages.APIKeysListPartial(keys, h.baseURL()))
}

// GetAPIKeyFormPartial returns the create-key form HTML.
func (h *Handler) GetAPIKeyFormPartial(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.APIKeyFormPartial(h.baseURL(), ""))
}

// PostAPIKey creates a new API key and returns the one-time reveal banner.
func (h *Handler) PostAPIKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.APIKeyFormPartial(h.baseURL(), "Key name is required"))
		return
	}

	rawKey, _, err := h.authService.CreateAPIKey(r.Context(), name)
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to create api key")
		render(w, r, http.StatusInternalServerError, pages.APIKeyFormPartial(h.baseURL(), "Failed to create API key"))
		return
	}

	render(w, r, http.StatusOK, pages.APIKeyCreatedBanner(name, rawKey, h.baseURL()))
}

// DeleteAPIKey revokes an API key and returns an empty response (removes the row).
func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.authService.DeleteAPIKey(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete api key")
		http.Error(w, "failed to revoke key", http.StatusInternalServerError)
		return
	}

	// Return empty to remove the row via outerHTML swap.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// PostChangePassword handles the change-password form submission.
func (h *Handler) PostChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword != confirmPassword {
		render(w, r, http.StatusUnprocessableEntity, pages.ChangePasswordFormPartial(h.baseURL(), &pages.ChangePasswordResult{
			Error: "New passwords do not match",
		}))
		return
	}
	if len(newPassword) < 8 {
		render(w, r, http.StatusUnprocessableEntity, pages.ChangePasswordFormPartial(h.baseURL(), &pages.ChangePasswordResult{
			Error: "New password must be at least 8 characters",
		}))
		return
	}

	if err := h.authService.ChangePassword(r.Context(), currentPassword, newPassword); err != nil {
		log.Warn().Err(err).Msg("ui: failed to change password")
		msg := "Failed to change password. Check your current password and try again."
		if errors.Is(err, auth.ErrInvalidCredentials) {
			msg = "Current password is incorrect"
		}
		render(w, r, http.StatusUnprocessableEntity, pages.ChangePasswordFormPartial(h.baseURL(), &pages.ChangePasswordResult{
			Error: msg,
		}))
		return
	}

	render(w, r, http.StatusOK, pages.ChangePasswordFormPartial(h.baseURL(), &pages.ChangePasswordResult{
		Success: true,
	}))
}
