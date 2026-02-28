// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// instances.go handles HTMX-driven CRUD for qBittorrent instances.

package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/instances
// ------------------------------------------------------------------

// GetInstances renders the full instances management page.
// Static data (name/host/etc.) is rendered immediately; each card's status badge lazy-loads separately.
func (h *Handler) GetInstances(w http.ResponseWriter, r *http.Request) {
	username := UsernameFromContext(r.Context())
	ctx := r.Context()

	insts, err := h.instanceStore.List(ctx)
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list instances")
		insts = []*models.Instance{}
	}

	// Build static items (no connectivity check) — status lazy-loads per card.
	items := make([]pages.InstanceListItem, 0, len(insts))
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		items = append(items, instanceToStaticItem(inst))
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	render(w, r, http.StatusOK, pages.Instances(pages.InstancesPageProps{
		BaseURL:   h.baseURL(),
		Username:  username,
		Version:   h.version,
		Instances: items,
		NavInsts:  navInsts,
	}))
}

// ------------------------------------------------------------------
// GET /ui/partials/instances/form        → empty add form
// GET /ui/partials/instances/form/{id}   → pre-filled edit form
// ------------------------------------------------------------------

// GetInstanceForm returns the add or edit form as an HTMX partial.
// Uses only DB data (instanceToStaticItem) so the form is never blocked by a
// live qBittorrent health-check — connection status is irrelevant for editing.
func (h *Handler) GetInstanceForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		render(w, r, http.StatusOK, pages.InstanceFormNew(h.baseURL(), ""))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	inst, err := h.instanceStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	item := instanceToStaticItem(inst)
	render(w, r, http.StatusOK, pages.InstanceFormEdit(item, h.baseURL(), ""))
}

// ------------------------------------------------------------------
// POST /ui/instances  → create
// ------------------------------------------------------------------

// PostInstance handles instance creation from the HTMX form.
func (h *Handler) PostInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	username := r.FormValue("username")
	password := r.FormValue("password")
	tlsSkipVerify := r.FormValue("tls_skip_verify") == "on"
	localFS := r.FormValue("has_local_filesystem_access") == "on"
	authBypass := r.FormValue("auth_bypass") == "on"
	showBasicAuth := r.FormValue("show_basic_auth") == "on"
	basicUsername := r.FormValue("basic_username")
	basicPassword := r.FormValue("basic_password")

	// Hardlink / Reflink mode fields.
	hardlinkMode := r.FormValue("hardlink_mode") // "regular", "hardlink", "reflink"
	useHardlinks := hardlinkMode == "hardlink"
	useReflinks := hardlinkMode == "reflink"
	hardlinkBaseDir := strings.TrimSpace(r.FormValue("hardlink_base_dir"))
	hardlinkDirPreset := r.FormValue("hardlink_dir_preset")
	if hardlinkDirPreset == "" {
		hardlinkDirPreset = "flat"
	}
	fallbackToRegular := r.FormValue("fallback_to_regular") == "on"

	if name == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.InstanceFormNew(h.baseURL(), "Instance name is required"))
		return
	}
	if host == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.InstanceFormNew(h.baseURL(), "URL is required"))
		return
	}

	if authBypass {
		username = ""
		password = ""
	}

	var basicUserPtr, basicPassPtr *string
	if showBasicAuth && basicUsername != "" {
		basicUserPtr = &basicUsername
		basicPassPtr = &basicPassword
	}

	localFSBool := localFS
	created, err := h.instanceStore.Create(
		r.Context(),
		name, host, username, password,
		basicUserPtr, basicPassPtr,
		tlsSkipVerify, &localFSBool,
	)
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to create instance")
		render(w, r, http.StatusUnprocessableEntity, pages.InstanceFormNew(h.baseURL(), "Failed to create instance: "+err.Error()))
		return
	}

	// Apply hardlink/reflink settings if configured (separate Update call since Create doesn't support them).
	if useHardlinks || useReflinks || hardlinkBaseDir != "" {
		useHL := useHardlinks
		useRL := useReflinks
		fall := fallbackToRegular
		if _, updateErr := h.instanceStore.Update(r.Context(), created.ID, created.Name, created.Host,
			created.Username, "",
			created.BasicUsername, nil,
			&models.InstanceUpdateParams{
				UseHardlinks:          &useHL,
				UseReflinks:           &useRL,
				HardlinkBaseDir:       &hardlinkBaseDir,
				HardlinkDirPreset:     &hardlinkDirPreset,
				FallbackToRegularMode: &fall,
			},
		); updateErr != nil {
			log.Warn().Err(updateErr).Int("id", created.ID).Msg("ui: failed to apply hardlink settings on create")
		}
	}

	h.renderInstanceSuccessPartial(w, r)
}

// ------------------------------------------------------------------
// PUT /ui/instances/{id}  → update
// ------------------------------------------------------------------

// PutInstance handles instance update from the HTMX form.
func (h *Handler) PutInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	username := r.FormValue("username")
	password := r.FormValue("password")
	tlsSkipVerify := r.FormValue("tls_skip_verify") == "on"
	localFS := r.FormValue("has_local_filesystem_access") == "on"
	authBypass := r.FormValue("auth_bypass") == "on"
	showBasicAuth := r.FormValue("show_basic_auth") == "on"
	basicUsername := r.FormValue("basic_username")
	basicPassword := r.FormValue("basic_password")

	// Hardlink / Reflink mode fields.
	hardlinkMode := r.FormValue("hardlink_mode")
	useHardlinks := hardlinkMode == "hardlink"
	useReflinks := hardlinkMode == "reflink"
	hardlinkBaseDir := strings.TrimSpace(r.FormValue("hardlink_base_dir"))
	hardlinkDirPreset := r.FormValue("hardlink_dir_preset")
	if hardlinkDirPreset == "" {
		hardlinkDirPreset = "flat"
	}
	fallbackToRegular := r.FormValue("fallback_to_regular") == "on"

	existing, fetchErr := h.instanceStore.Get(r.Context(), id)
	if fetchErr != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	if name == "" {
		item := instanceToStaticItem(existing)
		render(w, r, http.StatusUnprocessableEntity, pages.InstanceFormEdit(item, h.baseURL(), "Instance name is required"))
		return
	}

	if authBypass {
		username = ""
		password = ""
	}

	var basicUserPtr, basicPassPtr *string
	if showBasicAuth && basicUsername != "" {
		basicUserPtr = &basicUsername
		basicPassPtr = &basicPassword
	} else if !showBasicAuth {
		empty := ""
		basicUserPtr = &empty
		basicPassPtr = &empty
	}

	tlsPtr := tlsSkipVerify
	localFSPtr := localFS
	params := &models.InstanceUpdateParams{
		TLSSkipVerify:            &tlsPtr,
		HasLocalFilesystemAccess: &localFSPtr,
		UseHardlinks:             &useHardlinks,
		UseReflinks:              &useReflinks,
		HardlinkBaseDir:          &hardlinkBaseDir,
		HardlinkDirPreset:        &hardlinkDirPreset,
		FallbackToRegularMode:    &fallbackToRegular,
	}

	_, err = h.instanceStore.Update(
		r.Context(),
		id, name, host, username, password,
		basicUserPtr, basicPassPtr,
		params,
	)
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to update instance")
		existing2, _ := h.instanceStore.Get(r.Context(), id)
		var item pages.InstanceListItem
		if existing2 != nil {
			item = instanceToStaticItem(existing2)
		}
		render(w, r, http.StatusUnprocessableEntity, pages.InstanceFormEdit(item, h.baseURL(), "Failed to update instance: "+err.Error()))
		return
	}

	h.renderInstanceSuccessPartial(w, r)
}

// ------------------------------------------------------------------
// DELETE /ui/instances/{id}  → delete (returns empty to remove row)
// ------------------------------------------------------------------

// DeleteInstance handles instance deletion via HTMX.
func (h *Handler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.instanceStore.Delete(r.Context(), id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to delete instance")
		http.Error(w, "failed to delete instance", http.StatusInternalServerError)
		return
	}

	// Return empty to remove the instance row via outerHTML swap
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// ------------------------------------------------------------------
// POST /ui/instances/{id}/toggle  → enable/disable
// ------------------------------------------------------------------

// PostInstanceToggle toggles the IsActive state of an instance.
// The response card uses instanceToStaticItem (no live connectivity check);
// the status badge lazy-loads from /ui/partials/instances/{id}/status.
func (h *Handler) PostInstanceToggle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	inst, err := h.instanceStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	updated, err := h.instanceStore.SetActiveState(r.Context(), id, !inst.IsActive)
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("ui: failed to toggle instance active state")
		http.Error(w, "failed to update instance", http.StatusInternalServerError)
		return
	}

	item := instanceToStaticItem(updated)
	render(w, r, http.StatusOK, pages.InstanceRowCard(item, h.baseURL()))
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

// GetInstancesListPartial returns just the instance list HTML (for OOB refreshes after add/edit/delete).
func (h *Handler) GetInstancesListPartial(w http.ResponseWriter, r *http.Request) {
	insts, err := h.instanceStore.List(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to list instances for partial")
		insts = []*models.Instance{}
	}
	items := make([]pages.InstanceListItem, 0, len(insts))
	for _, inst := range insts {
		items = append(items, instanceToStaticItem(inst))
	}
	render(w, r, http.StatusOK, pages.InstanceListPartial(items, h.baseURL()))
}

// GetInstanceStatusPartial checks live connectivity and returns the status badge fragment for one card.
func (h *Handler) GetInstanceStatusPartial(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	inst, err := h.instanceStore.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	item := instanceToListItem(r.Context(), inst, h.syncManager)
	render(w, r, http.StatusOK, pages.InstanceStatusPartial(item))
}

// renderInstanceSuccessPartial replaces the form slot and OOB-swaps the instance list after a save.
func (h *Handler) renderInstanceSuccessPartial(w http.ResponseWriter, r *http.Request) {
	insts, err := h.instanceStore.List(r.Context())
	if err != nil {
		insts = []*models.Instance{}
	}
	items := make([]pages.InstanceListItem, 0, len(insts))
	for _, inst := range insts {
		items = append(items, instanceToStaticItem(inst))
	}
	render(w, r, http.StatusOK, pages.InstanceFormSuccessOOB(items, h.baseURL()))
}

// instanceToStaticItem builds a list item with only DB data — no connectivity check.
// The card's status badge is loaded lazily from GetInstanceStatusPartial.
func instanceToStaticItem(inst *models.Instance) pages.InstanceListItem {
	item := pages.InstanceListItem{
		ID:                       inst.ID,
		Name:                     inst.Name,
		Host:                     inst.Host,
		Username:                 inst.Username,
		TLSSkipVerify:            inst.TLSSkipVerify,
		HasLocalFilesystemAccess: inst.HasLocalFilesystemAccess,
		IsActive:                 inst.IsActive,
		HasBasicAuth:             inst.BasicUsername != nil && *inst.BasicUsername != "",
		UseHardlinks:             inst.UseHardlinks,
		UseReflinks:              inst.UseReflinks,
		HardlinkBaseDir:          inst.HardlinkBaseDir,
		HardlinkDirPreset:        inst.HardlinkDirPreset,
		FallbackToRegularMode:    inst.FallbackToRegularMode,
	}
	if inst.BasicUsername != nil {
		item.BasicUsername = *inst.BasicUsername
	}
	return item
}

// instanceToListItem converts a single Instance model to the page view type.
func instanceToListItem(ctx context.Context, inst *models.Instance, sm *qbittorrent.SyncManager) pages.InstanceListItem {
	item := pages.InstanceListItem{
		ID:                       inst.ID,
		Name:                     inst.Name,
		Host:                     inst.Host,
		Username:                 inst.Username,
		TLSSkipVerify:            inst.TLSSkipVerify,
		HasLocalFilesystemAccess: inst.HasLocalFilesystemAccess,
		IsActive:                 inst.IsActive,
		HasBasicAuth:             inst.BasicUsername != nil && *inst.BasicUsername != "",
		UseHardlinks:             inst.UseHardlinks,
		UseReflinks:              inst.UseReflinks,
		HardlinkBaseDir:          inst.HardlinkBaseDir,
		HardlinkDirPreset:        inst.HardlinkDirPreset,
		FallbackToRegularMode:    inst.FallbackToRegularMode,
	}
	if inst.BasicUsername != nil {
		item.BasicUsername = *inst.BasicUsername
	}

	if !inst.IsActive {
		item.ConnectionStatus = "disabled"
		return item
	}

	// Check live connection status from sync manager cache (non-blocking if not in pool).
	if sm != nil {
		if client, err := sm.GetClient(ctx, inst.ID); err == nil && client != nil {
			item.IsConnected = client.IsHealthy()
			if status := strings.TrimSpace(client.GetCachedConnectionStatus()); status != "" {
				item.ConnectionStatus = strings.ToLower(status)
			}
		}
	}

	return item
}
