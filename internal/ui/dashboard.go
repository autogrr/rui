// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// dashboard.go contains the live-data dashboard handler and its HTMX partials.

package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// GetDashboard renders the full dashboard page.
// Instance cards are initially shown as skeletons; HTMX polls
// GET /ui/partials/dashboard every 5 s for live stats.
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	username := UsernameFromContext(r.Context())

	insts, err := h.instanceStore.List(r.Context())
	if err != nil {
		insts = []*models.Instance{}
	}

	dashInsts := make([]pages.DashboardInstance, 0, len(insts))
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		dashInsts = append(dashInsts, pages.DashboardInstance{
			ID:       inst.ID,
			Name:     inst.Name,
			Host:     inst.Host,
			IsActive: inst.IsActive,
		})
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	render(w, r, http.StatusOK, pages.Dashboard(pages.DashboardProps{
		BaseURL:   h.baseURL(),
		Username:  username,
		Version:   h.version,
		Instances: dashInsts,
	}))
}

// buildDashboardInstances queries all configured instances and populates live
// statistics into a []pages.DashboardInstance slice.
func (h *Handler) buildDashboardInstances(r *http.Request) []pages.DashboardInstance {
	ctx := r.Context()

	insts, err := h.instanceStore.List(ctx)
	if err != nil {
		insts = []*models.Instance{}
	}

	dashInsts := make([]pages.DashboardInstance, 0, len(insts))
	for _, inst := range insts {
		di := pages.DashboardInstance{
			ID:       inst.ID,
			Name:     inst.Name,
			Host:     inst.Host,
			IsActive: inst.IsActive,
		}

		if h.syncManager != nil && inst.IsActive {
			// Populate live stats from the cached qBittorrent client state.
			if client, err := h.syncManager.GetClient(ctx, inst.ID); err == nil {
				if ss := client.GetCachedServerState(); ss != nil {
					di.IsConnected = true
					di.DlSpeed = uint64(max64(ss.DlInfoSpeed, 0))
					di.UpSpeed = uint64(max64(ss.UpInfoSpeed, 0))
					di.FreeSpaceBytes = ss.FreeSpaceOnDisk
					di.UseAltSpeedLimits = ss.UseAltSpeedLimits
					di.AlltimeDl = ss.AlltimeDl
					di.AlltimeUl = ss.AlltimeUl
				}
			}
			// Populate torrent counts from the sync cache.
			if resp, err := h.syncManager.GetTorrentsWithFilters(
				ctx, inst.ID, 0, 0, "", "", "", qbittorrent.FilterOptions{},
			); err == nil && resp != nil && resp.Stats != nil {
				di.Downloading = resp.Stats.Downloading
				di.Seeding = resp.Stats.Seeding
				di.Total = resp.Stats.Total
				di.IsConnected = true
			}
			// Tracker-down count from health cache (non-blocking).
			if hc := h.syncManager.GetTrackerHealthCounts(inst.ID); hc != nil {
				di.TrackerDown = hc.TrackerDown
			}
		}

		dashInsts = append(dashInsts, di)
	}
	return dashInsts
}

// GetDashboardPartial returns the live instance-card HTML fragment polled by HTMX.
// Route: GET /ui/partials/dashboard
func (h *Handler) GetDashboardPartial(w http.ResponseWriter, r *http.Request) {
	dashInsts := h.buildDashboardInstances(r)
	render(w, r, http.StatusOK, pages.DashboardStatsPartial(dashInsts, h.baseURL()))
}

// PostAltSpeedToggle toggles alternative speed limits for one instance and
// returns the refreshed dashboard cards so the UI updates immediately.
// Route: POST /ui/partials/dashboard/{id}/alt-speed
func (h *Handler) PostAltSpeedToggle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}

	if h.syncManager == nil {
		http.Error(w, "sync manager unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.ToggleAlternativeSpeedLimits(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-render all dashboard cards so alt-speed state reflects the change.
	dashInsts := h.buildDashboardInstances(r)
	render(w, r, http.StatusOK, pages.DashboardStatsPartial(dashInsts, h.baseURL()))
}

// max64 returns the larger of two int64 values.
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
