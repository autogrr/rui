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
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	qbt "github.com/autogrr/go-qbittorrent"

	"github.com/autogrr/rui/internal/domain"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// renderStub is a shared helper for stub page handlers.
// ------------------------------------------------------------------
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
	category := q.Get("category")
	tag := q.Get("tag")
	tracker := q.Get("tracker")
	savepath := q.Get("savepath")
	instanceID := intParam(q.Get("instance_id"), 0)
	expr := strings.TrimSpace(q.Get("expr"))
	sortCol := q.Get("sort")
	sortOrder := q.Get("order")
	if sortCol == "" {
		sortCol = "added_on"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, total, targetID := h.fetchTorrentRows(ctx, instanceID, search, status, category, tag, tracker, savepath, expr, sortCol, sortOrder)

	// Load sidebar data for the full page render.
	var cats []string
	var tagList []string
	var trackers []string
	var savepaths []string
	if targetID > 0 && h.syncManager != nil {
		if catMap, err := h.syncManager.GetCategories(ctx, targetID); err == nil {
			for name := range catMap {
				cats = append(cats, name)
			}
			sort.Strings(cats)
		}
		if t, err := h.syncManager.GetTags(ctx, targetID); err == nil {
			tagList = t
			sort.Strings(tagList)
		}
		// Derive unique tracker domains and save paths from the full unfiltered list.
		if all, err := h.syncManager.GetAllTorrents(ctx, targetID); err == nil {
			trackerSet := make(map[string]struct{}, 64)
			savepathSet := make(map[string]struct{}, 64)
			for _, t := range all {
				if qbt.Deref(t.Tracker) != "" {
					if domain := h.syncManager.ExtractDomainFromURL(qbt.Deref(t.Tracker)); domain != "" && domain != "Unknown" {
						trackerSet[domain] = struct{}{}
					}
				}
				if qbt.Deref(t.SavePath) != "" {
					sp := strings.ReplaceAll(qbt.Deref(t.SavePath), "\\\\", "/")
					savepathSet[sp] = struct{}{}
				}
			}
			for k := range trackerSet {
				trackers = append(trackers, k)
			}
			for k := range savepathSet {
				savepaths = append(savepaths, k)
			}
			sort.Strings(trackers)
			sort.Strings(savepaths)
		}
	}

	render(w, r, http.StatusOK, pages.Torrents(pages.TorrentsProps{
		BaseURL:        h.baseURL(),
		Username:       username,
		Version:        h.version,
		Instances:      navInsts,
		InstanceID:     targetID,
		Search:         search,
		Status:         status,
		Category:       category,
		Tag:            tag,
		Sort:           sortCol,
		Order:          sortOrder,
		Expr:           expr,
		Rows:           rows,
		Total:          total,
		Categories:     cats,
		Tags:           tagList,
		Trackers:       trackers,
		SavePaths:      savepaths,
		FilterTracker:  tracker,
		FilterSavePath: savepath,
	}))
}

// GetTorrentsPartial returns just the <tr> rows for an HTMX table-body swap.
// Route: GET /ui/partials/torrents
func (h *Handler) GetTorrentsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	status := q.Get("status")
	category := q.Get("category")
	tag := q.Get("tag")
	tracker := q.Get("tracker")
	savepath := q.Get("savepath")
	instanceID := intParam(q.Get("instance_id"), 0)
	expr := strings.TrimSpace(q.Get("expr"))
	sortCol := q.Get("sort")
	sortOrder := q.Get("order")
	if sortCol == "" {
		sortCol = "added_on"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, total, _ := h.fetchTorrentRows(ctx, instanceID, search, status, category, tag, tracker, savepath, expr, sortCol, sortOrder)

	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		Rows:           rows,
		Total:          total,
		Search:         search,
		Status:         status,
		Category:       category,
		Tag:            tag,
		Sort:           sortCol,
		Order:          sortOrder,
		Expr:           expr,
		FilterTracker:  tracker,
		FilterSavePath: savepath,
		InstanceID:     instanceID,
		BaseURL:        h.baseURL(),
	}))
}

// fetchTorrentRows queries SyncManager and maps the result to []TorrentRow.
// instanceID == 0 means "first active instance" (fallback when none selected).
// Returns rows, total count, and the resolved instance ID used for the query.
// limit=0 means unbounded (all matching torrents).
func (h *Handler) fetchTorrentRows(ctx context.Context, instanceID int, search, status, category, tag, tracker, savepath, expr, sortCol, sortOrder string) ([]pages.TorrentRow, int, int) {
	if h.syncManager == nil {
		return nil, 0, 0
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
		return nil, 0, 0
	}

	filters := qbittorrent.FilterOptions{}
	if status != "" {
		filters.Status = []string{status}
	}
	if category != "" {
		filters.Categories = []string{category}
	}
	if tag != "" {
		filters.Tags = []string{tag}
	}
	if tracker != "" {
		filters.Trackers = []string{tracker}
	}
	if savepath != "" {
		filters.SavePaths = []string{savepath}
	}
	if expr != "" {
		filters.Expr = expr
	}

	if sortCol == "" {
		sortCol = "added_on"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// limit=0 means unbounded — virtual scroll handles rendering
	resp, err := h.syncManager.GetTorrentsWithFilters(ctx, targetID, 0, 0, sortCol, sortOrder, search, filters)
	if err != nil || resp == nil {
		return nil, 0, targetID
	}

	rows := make([]pages.TorrentRow, 0, len(resp.Torrents))
	for _, tv := range resp.Torrents {
		if tv.Torrent == nil {
			continue
		}
		rows = append(rows, pages.TorrentRow{
			Hash:          qbt.Deref(tv.Hash),
			Name:          qbt.Deref(tv.Name),
			State:         string(qbt.Deref(tv.State)),
			SizeB:         qbt.Deref(tv.Size),
			TotalSizeB:    qbt.Deref(tv.TotalSize),
			Progress:      qbt.Deref(tv.Progress),
			DlSpeed:       qbt.Deref(tv.DlSpeed),
			UpSpeed:       qbt.Deref(tv.UpSpeed),
			Ratio:         qbt.Deref(tv.Ratio),
			Category:      qbt.Deref(tv.Category),
			Tags:          qbt.Deref(tv.Tags),
			ETA:           qbt.Deref(tv.ETA),
			AddedOn:       qbt.Deref(tv.AddedOn),
			CompletionOn:  qbt.Deref(tv.CompletionOn),
			SavePath:      qbt.Deref(tv.SavePath),
			Tracker:       qbt.Deref(tv.Tracker),
			Uploaded:      qbt.Deref(tv.Uploaded),
			Downloaded:    qbt.Deref(tv.Downloaded),
			NumSeeds:      int64(qbt.Deref(tv.NumSeeds)),
			NumLeechs:     int64(qbt.Deref(tv.NumLeechs)),
			NumComplete:   int64(qbt.Deref(tv.NumComplete)),
			NumIncomplete: int64(qbt.Deref(tv.NumIncomplete)),
			SeedingTime:   qbt.Deref(tv.SeedingTime),
			TimeActive:    qbt.Deref(tv.TimeActive),
			AmountLeft:    qbt.Deref(tv.AmountLeft),
			LastActivity:  qbt.Deref(tv.LastActivity),
			Availability:  qbt.Deref(tv.Availability),
			InfohashV1:    qbt.Deref(tv.InfoHashV1),
			InfohashV2:    qbt.Deref(tv.InfoHashV2),
			Priority:      int64(qbt.Deref(tv.Priority)),
		})
	}

	return rows, resp.Total, targetID
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
	case pages.SettingsSectionTrackers:
		if h.trackerCustomizationStore != nil {
			if customs, err := h.trackerCustomizationStore.List(ctx); err == nil {
				props.TrackerCustomizations = customs
			}
		}
	}

	render(w, r, http.StatusOK, pages.Settings(props))
}
