// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// settings_services.go contains HTMX partial handlers for:
//   - *arr Integrations section
//   - Client API Keys section
//   - External Programs section
//   - Notifications section

package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/notifications"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ──────────────────────────────────────────────────────────────────
// *arr Integrations section
//
//	GET    /ui/partials/settings/integrations          → list fragment
//	GET    /ui/partials/settings/integrations/form     → new form
//	GET    /ui/partials/settings/integrations/form/{id}→ edit form
//	POST   /ui/partials/settings/integrations          → create
//	PUT    /ui/partials/settings/integrations/{id}     → update
//	DELETE /ui/partials/settings/integrations/{id}     → delete
//	POST   /ui/partials/settings/integrations/{id}/test→ test
// ──────────────────────────────────────────────────────────────────

// GetIntegrationsListPartial returns the integrations list fragment.
func (h *Handler) GetIntegrationsListPartial(w http.ResponseWriter, r *http.Request) {
	instances, err := h.arrInstanceStore.List(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list arr instances")
		instances = nil
	}
	render(w, r, http.StatusOK, pages.ArrInstanceListPartial(instances, h.baseURL()))
}

// GetIntegrationForm returns the new or edit integration form fragment.
func (h *Handler) GetIntegrationForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	base := h.baseURL()

	if idStr == "" {
		render(w, r, http.StatusOK, pages.IntegrationFormNew(base, ""))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		render(w, r, http.StatusOK, pages.IntegrationFormNew(base, "Invalid instance ID"))
		return
	}

	inst, err := h.arrInstanceStore.Get(r.Context(), id)
	if err != nil {
		render(w, r, http.StatusOK, pages.IntegrationFormNew(base, "Instance not found"))
		return
	}

	render(w, r, http.StatusOK, pages.IntegrationFormEdit(*inst, base, ""))
}

// PostIntegration creates a new *arr integration.
func (h *Handler) PostIntegration(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	instType := models.ArrInstanceType(r.FormValue("type"))
	enabled := r.FormValue("enabled") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "Name is required"))
		return
	}
	if baseURL == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "Base URL is required"))
		return
	}
	if apiKey == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "API key is required"))
		return
	}
	if instType != models.ArrInstanceTypeSonarr && instType != models.ArrInstanceTypeRadarr {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "Invalid type (must be sonarr or radarr)"))
		return
	}

	timeout, _ := strconv.Atoi(r.FormValue("timeout_seconds"))
	if timeout <= 0 {
		timeout = 15
	}

	ctx := r.Context()
	_, err := h.arrInstanceStore.Create(ctx, instType, name, baseURL, apiKey, nil, nil, enabled, 0, timeout)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormNew(h.baseURL(), "Failed to create integration: "+err.Error()))
		return
	}

	instances, _ := h.arrInstanceStore.List(ctx)
	render(w, r, http.StatusOK, pages.IntegrationFormSuccess(instances, h.baseURL()))
}

// PutIntegration updates an existing *arr integration.
func (h *Handler) PutIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	existing, err := h.arrInstanceStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormEdit(*existing, h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	enabled := r.FormValue("enabled") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormEdit(*existing, h.baseURL(), "Name is required"))
		return
	}
	if baseURL == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormEdit(*existing, h.baseURL(), "Base URL is required"))
		return
	}

	timeout, _ := strconv.Atoi(r.FormValue("timeout_seconds"))
	if timeout <= 0 {
		timeout = 15
	}

	params := &models.ArrInstanceUpdateParams{
		Name:    &name,
		BaseURL: &baseURL,
		Enabled: &enabled,
	}
	if apiKey != "" {
		params.APIKey = &apiKey
	}
	params.TimeoutSeconds = &timeout

	ctx := r.Context()
	_, err = h.arrInstanceStore.Update(ctx, id, params)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.IntegrationFormEdit(*existing, h.baseURL(), "Failed to update integration: "+err.Error()))
		return
	}

	instances, _ := h.arrInstanceStore.List(ctx)
	render(w, r, http.StatusOK, pages.IntegrationFormSuccess(instances, h.baseURL()))
}

// DeleteIntegration deletes an *arr integration; returns empty body for outerHTML swap.
func (h *Handler) DeleteIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	if err := h.arrInstanceStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete arr instance")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// PostIntegrationTest tests connectivity to an *arr instance.
func (h *Handler) PostIntegrationTest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid instance ID", http.StatusBadRequest)
		return
	}

	testErr := h.arrService.TestInstance(r.Context(), id)
	if testErr != nil {
		log.Debug().Err(testErr).Int("id", id).Msg("ui: arr instance test failed")
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
// Client API Keys section
//
//	GET    /ui/partials/settings/client-api/form       → new form
//	POST   /ui/partials/settings/client-api            → create key
//	DELETE /ui/partials/settings/client-api/{id}       → revoke key
// ──────────────────────────────────────────────────────────────────

// GetClientAPIKeyForm renders the generate-key form with instance selector.
func (h *Handler) GetClientAPIKeyForm(w http.ResponseWriter, r *http.Request) {
	insts, _ := h.instanceStore.List(r.Context())
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}
	render(w, r, http.StatusOK, pages.ClientAPIKeyFormNew(navInsts, h.baseURL(), ""))
}

// PostClientAPIKey creates a new client API key.
func (h *Handler) PostClientAPIKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		insts, _ := h.instanceStore.List(r.Context())
		navInsts := make([]layouts.Instance, 0, len(insts))
		for _, inst := range insts {
			navInsts = append(navInsts, layouts.Instance{ID: inst.ID, Name: inst.Name, IsActive: inst.IsActive})
		}
		render(w, r, http.StatusUnprocessableEntity, pages.ClientAPIKeyFormNew(navInsts, h.baseURL(), "Invalid form data"))
		return
	}

	clientName := strings.TrimSpace(r.FormValue("client_name"))
	instanceID, _ := strconv.Atoi(r.FormValue("instance_id"))

	insts, _ := h.instanceStore.List(r.Context())
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{ID: inst.ID, Name: inst.Name, IsActive: inst.IsActive})
	}

	if clientName == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.ClientAPIKeyFormNew(navInsts, h.baseURL(), "Client name is required"))
		return
	}
	if instanceID == 0 {
		render(w, r, http.StatusUnprocessableEntity, pages.ClientAPIKeyFormNew(navInsts, h.baseURL(), "Please select an instance"))
		return
	}

	ctx := r.Context()
	rawKey, _, err := h.clientAPIKeyStore.Create(ctx, clientName, instanceID)
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to create client API key")
		render(w, r, http.StatusUnprocessableEntity, pages.ClientAPIKeyFormNew(navInsts, h.baseURL(), "Failed to create key"))
		return
	}

	keys, _ := h.clientAPIKeyStore.GetAll(ctx)
	render(w, r, http.StatusOK, pages.ClientAPIKeyCreated(rawKey, keys, h.baseURL()))
}

// DeleteClientAPIKey revokes a client API key; returns empty body for outerHTML swap.
func (h *Handler) DeleteClientAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid key ID", http.StatusBadRequest)
		return
	}

	if err := h.clientAPIKeyStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete client API key")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// ──────────────────────────────────────────────────────────────────
// External Programs section
//
//	GET    /ui/partials/settings/external-programs          → list
//	GET    /ui/partials/settings/external-programs/form     → new form
//	GET    /ui/partials/settings/external-programs/form/{id}→ edit form
//	POST   /ui/partials/settings/external-programs          → create
//	PUT    /ui/partials/settings/external-programs/{id}     → update
//	DELETE /ui/partials/settings/external-programs/{id}     → delete
// ──────────────────────────────────────────────────────────────────

// GetExtProgramsListPartial returns the external programs list fragment.
func (h *Handler) GetExtProgramsListPartial(w http.ResponseWriter, r *http.Request) {
	progs, err := h.extProgramStore.List(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list external programs")
		progs = nil
	}
	render(w, r, http.StatusOK, pages.ExtProgramListPartial(progs, h.baseURL()))
}

// GetExtProgramForm returns the new or edit external program form fragment.
func (h *Handler) GetExtProgramForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	base := h.baseURL()

	if idStr == "" {
		render(w, r, http.StatusOK, pages.ExtProgramFormNew(base, ""))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		render(w, r, http.StatusOK, pages.ExtProgramFormNew(base, "Invalid program ID"))
		return
	}

	prog, err := h.extProgramStore.GetByID(r.Context(), id)
	if err != nil {
		render(w, r, http.StatusOK, pages.ExtProgramFormNew(base, "Program not found"))
		return
	}

	render(w, r, http.StatusOK, pages.ExtProgramFormEdit(*prog, base, ""))
}

// PostExtProgram creates a new external program.
func (h *Handler) PostExtProgram(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormNew(h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	path := strings.TrimSpace(r.FormValue("path"))
	argsTemplate := r.FormValue("args_template")
	enabled := r.FormValue("enabled") != ""
	useTerminal := r.FormValue("use_terminal") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormNew(h.baseURL(), "Name is required"))
		return
	}
	if path == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormNew(h.baseURL(), "Path is required"))
		return
	}

	if h.extProgramService != nil && !h.extProgramService.IsPathAllowed(path) {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormNew(h.baseURL(), "Program path is not allowed"))
		return
	}

	req := &models.ExternalProgramCreate{
		Name:         name,
		Path:         path,
		ArgsTemplate: argsTemplate,
		Enabled:      enabled,
		UseTerminal:  useTerminal,
	}

	ctx := r.Context()
	_, err := h.extProgramStore.Create(ctx, req)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormNew(h.baseURL(), "Failed to create program: "+err.Error()))
		return
	}

	progs, _ := h.extProgramStore.List(ctx)
	render(w, r, http.StatusOK, pages.ExtProgramFormSuccess(progs, h.baseURL()))
}

// PutExtProgram updates an existing external program.
func (h *Handler) PutExtProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid program ID", http.StatusBadRequest)
		return
	}

	existing, err := h.extProgramStore.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Program not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormEdit(*existing, h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	path := strings.TrimSpace(r.FormValue("path"))
	argsTemplate := r.FormValue("args_template")
	enabled := r.FormValue("enabled") != ""
	useTerminal := r.FormValue("use_terminal") != ""

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormEdit(*existing, h.baseURL(), "Name is required"))
		return
	}
	if path == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormEdit(*existing, h.baseURL(), "Path is required"))
		return
	}

	if h.extProgramService != nil && !h.extProgramService.IsPathAllowed(path) {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormEdit(*existing, h.baseURL(), "Program path is not allowed"))
		return
	}

	req := &models.ExternalProgramUpdate{
		Name:         name,
		Path:         path,
		ArgsTemplate: argsTemplate,
		Enabled:      enabled,
		UseTerminal:  useTerminal,
	}

	ctx := r.Context()
	_, err = h.extProgramStore.Update(ctx, id, req)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.ExtProgramFormEdit(*existing, h.baseURL(), "Failed to update program: "+err.Error()))
		return
	}

	progs, _ := h.extProgramStore.List(ctx)
	render(w, r, http.StatusOK, pages.ExtProgramFormSuccess(progs, h.baseURL()))
}

// DeleteExtProgram deletes an external program; returns empty body for outerHTML swap.
func (h *Handler) DeleteExtProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid program ID", http.StatusBadRequest)
		return
	}

	if err := h.extProgramStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete external program")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// ──────────────────────────────────────────────────────────────────
// Notifications section
//
//	GET    /ui/partials/settings/notifications          → list
//	GET    /ui/partials/settings/notifications/form     → new form
//	GET    /ui/partials/settings/notifications/form/{id}→ edit form
//	POST   /ui/partials/settings/notifications          → create
//	PUT    /ui/partials/settings/notifications/{id}     → update
//	DELETE /ui/partials/settings/notifications/{id}     → delete
//	POST   /ui/partials/settings/notifications/{id}/test→ test
// ──────────────────────────────────────────────────────────────────

// GetNotificationsListPartial returns the notifications list fragment.
func (h *Handler) GetNotificationsListPartial(w http.ResponseWriter, r *http.Request) {
	targets, err := h.notificationTargetStore.List(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list notification targets")
		targets = nil
	}
	render(w, r, http.StatusOK, pages.NotificationListPartial(targets, h.baseURL()))
}

// GetNotificationForm returns the new or edit notification form fragment.
func (h *Handler) GetNotificationForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	base := h.baseURL()

	if idStr == "" {
		render(w, r, http.StatusOK, pages.NotificationFormNew(base, ""))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		render(w, r, http.StatusOK, pages.NotificationFormNew(base, "Invalid target ID"))
		return
	}

	target, err := h.notificationTargetStore.GetByID(r.Context(), id)
	if err != nil {
		render(w, r, http.StatusOK, pages.NotificationFormNew(base, "Target not found"))
		return
	}

	render(w, r, http.StatusOK, pages.NotificationFormEdit(*target, base, ""))
}

// PostNotification creates a new notification target.
func (h *Handler) PostNotification(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormNew(h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))
	enabled := r.FormValue("enabled") != ""
	eventTypeInputs := r.Form["event_types"]

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormNew(h.baseURL(), "Name is required"))
		return
	}
	if url == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormNew(h.baseURL(), "URL is required"))
		return
	}

	eventTypes, err := notifications.NormalizeEventTypes(eventTypeInputs)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormNew(h.baseURL(), "Invalid event types: "+err.Error()))
		return
	}
	if len(eventTypes) == 0 {
		eventTypes = notifications.AllEventTypeStrings()
	}

	ctx := r.Context()
	_, err = h.notificationTargetStore.Create(ctx, &models.NotificationTargetCreate{
		Name:       name,
		URL:        url,
		Enabled:    enabled,
		EventTypes: eventTypes,
	})
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormNew(h.baseURL(), "Failed to create notification target: "+err.Error()))
		return
	}

	targets, _ := h.notificationTargetStore.List(ctx)
	render(w, r, http.StatusOK, pages.NotificationFormSuccess(targets, h.baseURL()))
}

// PutNotification updates an existing notification target.
func (h *Handler) PutNotification(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid target ID", http.StatusBadRequest)
		return
	}

	existing, err := h.notificationTargetStore.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Target not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormEdit(*existing, h.baseURL(), "Invalid form data"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))
	enabled := r.FormValue("enabled") != ""
	eventTypeInputs := r.Form["event_types"]

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormEdit(*existing, h.baseURL(), "Name is required"))
		return
	}
	if url == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormEdit(*existing, h.baseURL(), "URL is required"))
		return
	}

	eventTypes, err := notifications.NormalizeEventTypes(eventTypeInputs)
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormEdit(*existing, h.baseURL(), "Invalid event types: "+err.Error()))
		return
	}
	if len(eventTypes) == 0 {
		eventTypes = notifications.AllEventTypeStrings()
	}

	ctx := r.Context()
	_, err = h.notificationTargetStore.Update(ctx, id, &models.NotificationTargetUpdate{
		Name:       name,
		URL:        url,
		Enabled:    enabled,
		EventTypes: eventTypes,
	})
	if err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.NotificationFormEdit(*existing, h.baseURL(), "Failed to update target: "+err.Error()))
		return
	}

	targets, _ := h.notificationTargetStore.List(ctx)
	render(w, r, http.StatusOK, pages.NotificationFormSuccess(targets, h.baseURL()))
}

// DeleteNotification deletes a notification target; returns empty body for outerHTML swap.
func (h *Handler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid target ID", http.StatusBadRequest)
		return
	}

	if err := h.notificationTargetStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete notification target")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// PostNotificationTest sends a test notification to a target.
func (h *Handler) PostNotificationTest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid target ID", http.StatusBadRequest)
		return
	}

	target, err := h.notificationTargetStore.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Target not found", http.StatusNotFound)
		return
	}

	testErr := h.notificationService.SendTest(r.Context(), target, "Test notification", "This is a test notification from qui.")
	if testErr != nil {
		log.Debug().Err(testErr).Int("id", id).Msg("ui: notification test failed")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<span class="text-destructive text-xs">Send failed: ` + testErr.Error() + `</span>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<span class="text-green-600 text-xs">Test sent successfully</span>`))
}
