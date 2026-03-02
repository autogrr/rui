// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// dashboard.go contains the live-data dashboard handler and its HTMX partials.

package ui

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	qbt "github.com/autogrr/go-qbittorrent"

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

// buildDashboardInstances returns live dashboard stats from the in-memory
// sync cache. All values come from atomically-swapped pointers updated by
// the qbt.SyncManager OnUpdate callback — no torrent list copy, no lock
// contention, no outbound network calls.
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
			// GetClientOffline: RLock map lookup, no HealthCheck.
			if client, err := h.syncManager.GetClientOffline(ctx, inst.ID); err == nil && client != nil {
				if ss := client.GetCachedServerState(); ss != nil {
					di.IsConnected = true
					di.DlSpeed = uint64(max64(qbt.Deref(ss.DlInfoSpeed), 0))
					di.UpSpeed = uint64(max64(qbt.Deref(ss.UpInfoSpeed), 0))
					di.FreeSpaceBytes = qbt.Deref(ss.FreeSpaceOnDisk)
					di.UseAltSpeedLimits = qbt.Deref(ss.UseAltSpeedLimits)
					di.AlltimeDl = qbt.Deref(ss.AllTimeDownload)
					di.AlltimeUl = qbt.Deref(ss.AllTimeUpload)
				}
				// Torrent counts: atomic pointer load, zero allocation.
				// Never override IsConnected from counts — stale post-disconnect
				// counts would incorrectly show the instance as online.
				if counts := client.GetCachedTorrentCounts(); counts != nil {
					di.Total = counts.Total
					di.Downloading = counts.Downloading
					di.Seeding = counts.Seeding
				}
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

func (h *Handler) buildDashboardTrackerBreakdownRows(ctx context.Context) []pages.TrackerBreakdownRow {
	insts, err := h.instanceStore.List(ctx)
	if err != nil {
		insts = []*models.Instance{}
	}

	customizations, _ := h.trackerCustomizationStore.List(ctx)

	agg := make(map[string]*qbittorrent.CachedTrackerRow)

	if h.syncManager != nil {
		for _, inst := range insts {
			if !inst.IsActive {
				continue
			}
			client, err := h.syncManager.GetClientOffline(ctx, inst.ID)
			if err != nil || client == nil {
				continue
			}
			for _, row := range client.GetCachedTrackerRows() {
				cached := row // copy off the slice
				existing, ok := agg[cached.Domain]
				if !ok {
					agg[cached.Domain] = &cached
					continue
				}
				// Merge across multiple instances.
				existing.Count += cached.Count
				existing.Seeding += cached.Seeding
				existing.Downloading += cached.Downloading
				existing.UpSpeed += cached.UpSpeed
				existing.DlSpeed += cached.DlSpeed
				existing.TotalSize += cached.TotalSize
			}
		}
	}

	rows := make([]pages.TrackerBreakdownRow, 0, len(agg))
	for domain, ag := range agg {
		displayName := models.ResolveTrackerDisplayName(domain, "", customizations)
		rows = append(rows, pages.TrackerBreakdownRow{
			DisplayName:  displayName,
			Domain:       domain,
			TorrentCount: ag.Count,
			Seeding:      ag.Seeding,
			Downloading:  ag.Downloading,
			UpSpeed:      ag.UpSpeed,
			DlSpeed:      ag.DlSpeed,
			TotalSize:    ag.TotalSize,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TorrentCount > rows[j].TorrentCount
	})

	return rows
}

// GetDashboardTrackerBreakdown returns per-tracker aggregated stats across all
// active instances, lazy-loaded by the dashboard on first paint.
// Stats are pre-computed by the OnUpdate callback; this handler just reads
// atomic pointers — no torrent list copy, no lock contention.
// Route: GET /ui/partials/dashboard/tracker-breakdown
func (h *Handler) GetDashboardTrackerBreakdown(w http.ResponseWriter, r *http.Request) {
	rows := h.buildDashboardTrackerBreakdownRows(r.Context())
	render(w, r, http.StatusOK, pages.DashboardTrackerBreakdown(rows, h.baseURL()))
}
