// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// stubs.go contains HTTP handlers for UI pages.  Simple placeholder pages
// render the authenticated shell with a "coming soon" banner via renderStub.
// Pages with real implementations (torrents, settings) live here until they
// grow large enough to warrant their own files.

package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/autogrr/rui/internal/domain"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// Stub pages (not yet migrated to full implementations)
// ------------------------------------------------------------------

func (h *Handler) GetSearch(w http.ResponseWriter, r *http.Request) {
	h.renderStub(w, r, "Search", "/ui/search")
}

func (h *Handler) GetCrossSeed(w http.ResponseWriter, r *http.Request) {
	h.renderStub(w, r, "Cross-seed", "/ui/cross-seed")
}

func (h *Handler) GetAutomations(w http.ResponseWriter, r *http.Request) {
	h.renderStub(w, r, "Automations", "/ui/automations")
}

func (h *Handler) GetBackups(w http.ResponseWriter, r *http.Request) {
	h.renderStub(w, r, "Backups", "/ui/backups")
}

func (h *Handler) GetRSS(w http.ResponseWriter, r *http.Request) {
	h.renderStub(w, r, "RSS", "/ui/rss")
}

// renderStub is a shared helper for stub page handlers.
func (h *Handler) renderStub(w http.ResponseWriter, r *http.Request, title, path string) {
	username := UsernameFromContext(r.Context())

	insts, _ := h.instanceStore.List(r.Context())
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	render(w, r, http.StatusOK, pages.Stub(pages.StubProps{
		BaseURL:     h.baseURL(),
		Username:    username,
		Version:     h.version,
		Title:       title,
		CurrentPath: path,
		Instances:   navInsts,
	}))
}

// ------------------------------------------------------------------
// Torrents page
// ------------------------------------------------------------------

const defaultPageSize = 50

// GetTorrents renders the full torrents list page.
func (h *Handler) GetTorrents(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	status := q.Get("status")
	page := intParam(q.Get("page"), 1)
	instanceID := intParam(q.Get("instance_id"), 0)

	rows, total := h.fetchTorrentRows(ctx, instanceID, page, defaultPageSize, search, status)

	render(w, r, http.StatusOK, pages.Torrents(pages.TorrentsProps{
		BaseURL:    h.baseURL(),
		Username:   username,
		Version:    h.version,
		Instances:  navInsts,
		InstanceID: instanceID,
		Search:     search,
		Status:     status,
		Page:       page,
		PageSize:   defaultPageSize,
		Rows:       rows,
		Total:      total,
	}))
}

// GetTorrentsPartial returns just the <tr> rows for an HTMX table-body swap.
// Route: GET /ui/partials/torrents
func (h *Handler) GetTorrentsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	status := q.Get("status")
	page := intParam(q.Get("page"), 1)
	instanceID := intParam(q.Get("instance_id"), 0)

	rows, total := h.fetchTorrentRows(ctx, instanceID, page, defaultPageSize, search, status)

	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		Rows:       rows,
		Total:      total,
		Page:       page,
		PageSize:   defaultPageSize,
		Search:     search,
		Status:     status,
		InstanceID: instanceID,
		BaseURL:    h.baseURL(),
	}))
}

// fetchTorrentRows queries SyncManager and maps the result to []TorrentRow.
// instanceID == 0 means "first active instance" (fallback when none selected).
func (h *Handler) fetchTorrentRows(ctx context.Context, instanceID, page, pageSize int, search, status string) ([]pages.TorrentRow, int) {
	if h.syncManager == nil {
		return nil, 0
	}

	// Resolve which instance to query.
	targetID := instanceID
	if targetID == 0 {
		insts, err := h.instanceStore.List(ctx)
		if err == nil {
			for _, inst := range insts {
				if inst.IsActive {
					targetID = inst.ID
					break
				}
			}
		}
	}
	if targetID == 0 {
		return nil, 0
	}

	filters := qbittorrent.FilterOptions{}
	if status != "" {
		filters.Status = []string{status}
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	resp, err := h.syncManager.GetTorrentsWithFilters(ctx, targetID, pageSize, offset, "name", "asc", search, filters)
	if err != nil || resp == nil {
		return nil, 0
	}

	rows := make([]pages.TorrentRow, 0, len(resp.Torrents))
	for _, tv := range resp.Torrents {
		if tv.Torrent == nil {
			continue
		}
		rows = append(rows, pages.TorrentRow{
			Hash:     tv.Hash,
			Name:     tv.Name,
			State:    string(tv.State),
			SizeB:    tv.TotalSize,
			Progress: tv.Progress,
			DlSpeed:  tv.DlSpeed,
			UpSpeed:  tv.UpSpeed,
			Ratio:    tv.Ratio,
			Category: tv.Category,
			Tags:     tv.Tags,
			ETA:      tv.ETA,
		})
	}

	return rows, resp.Total
}

// intParam parses a string as an int, returning def on error or when empty.
func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ------------------------------------------------------------------
// Settings page
// ------------------------------------------------------------------

// GetSettings renders the settings page, routing to the correct section.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	username := UsernameFromContext(r.Context())
	ctx := r.Context()

	section := chi.URLParam(r, "section")
	if section == "" {
		section = pages.SettingsSectionGeneral
	}

	insts, _ := h.instanceStore.List(ctx)
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	var cfg *domain.Config
	if h.cfg != nil {
		cfg = h.cfg.Config
	}

	props := pages.SettingsProps{
		BaseURL:   h.baseURL(),
		Username:  username,
		Version:   h.version,
		Instances: navInsts,
		Section:   section,
		Config:    cfg,
	}

	// Load API keys for the api-keys section.
	if section == pages.SettingsSectionAPIKeys {
		if keys, err := h.authService.ListAPIKeys(ctx); err == nil {
			props.APIKeys = keys
		}
	}

	// Load section-specific data for the new settings sections.
	switch section {
	case pages.SettingsSectionIndexers:
		if idxs, err := h.indexerStore.List(ctx); err == nil {
			props.Indexers = idxs
		}
	case pages.SettingsSectionSearchCache:
		if h.jackettService != nil {
			if stats, err := h.jackettService.GetSearchCacheStats(ctx); err == nil {
				props.SearchCacheStats = stats
			}
		}
	case pages.SettingsSectionIntegrations:
		if insts, err := h.arrInstanceStore.List(ctx); err == nil {
			props.ArrInstances = insts
		}
	case pages.SettingsSectionClientAPI:
		if keys, err := h.clientAPIKeyStore.GetAll(ctx); err == nil {
			props.ClientAPIKeys = keys
		}
	case pages.SettingsSectionExtPrograms:
		if progs, err := h.extProgramStore.List(ctx); err == nil {
			props.ExtPrograms = progs
		}
	case pages.SettingsSectionNotifications:
		if targets, err := h.notificationTargetStore.List(ctx); err == nil {
			props.NotificationTargets = targets
		}
	}

	render(w, r, http.StatusOK, pages.Settings(props))
}
