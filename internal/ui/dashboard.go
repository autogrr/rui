// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// dashboard.go contains the live-data dashboard handler and its HTMX partial.

package ui

import (
	"net/http"

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

// GetDashboardPartial returns the live instance-card HTML fragment polled by HTMX.
// Route: GET /ui/partials/dashboard
func (h *Handler) GetDashboardPartial(w http.ResponseWriter, r *http.Request) {
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
				}
			}
			// Populate torrent counts from the sync cache.
			if resp, err := h.syncManager.GetTorrentsWithFilters(
				ctx, inst.ID, 0, 0, "", "", "", qbittorrent.FilterOptions{},
			); err == nil && resp != nil && resp.Stats != nil {
				di.Downloading = resp.Stats.Downloading
				di.Seeding = resp.Stats.Seeding
				di.Total = resp.Stats.Total
				// IsConnected can also be inferred from non-nil stats.
				di.IsConnected = true
			}
		}

		dashInsts = append(dashInsts, di)
	}

	render(w, r, http.StatusOK, pages.DashboardStatsPartial(dashInsts, h.baseURL()))
}

// max64 returns the larger of two int64 values.
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
