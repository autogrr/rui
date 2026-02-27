// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents.go contains the HTMX partial handlers for the torrents page:
//   - detail panel (GET /ui/partials/torrents/{hash})
//   - bulk actions (POST /ui/partials/torrents/action)
//   - add torrent  (POST /ui/partials/torrents/add)

package ui

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autogrr/rui/internal/ui/pages"
)

// GetTorrentDetailPartial loads properties, files, trackers and peers for one
// torrent and renders the TorrentDetailPanel fragment.
// Route: GET /ui/partials/torrents/{hash}
func (h *Handler) GetTorrentDetailPartial(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)

	if h.syncManager == nil || hash == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	// Resolve instance to query.
	targetID := instanceID
	if targetID == 0 {
		if insts, err := h.instanceStore.List(ctx); err == nil {
			for _, inst := range insts {
				if inst.IsActive {
					targetID = inst.ID
					break
				}
			}
		}
	}
	if targetID == 0 {
		http.Error(w, "no active instance", http.StatusServiceUnavailable)
		return
	}

	// Pull basic metadata (name, state, category, tags) from the cache.
	var name, state, category, tags string
	if torrents, err := h.syncManager.GetTorrents(ctx, targetID, qbt.TorrentFilterOptions{
		Hashes: []string{hash},
	}); err == nil {
		for _, t := range torrents {
			name = t.Name
			state = string(t.State)
			category = t.Category
			tags = t.Tags
			break
		}
	}

	// Fetch detail data; individual failures are non-fatal.
	props, _ := h.syncManager.GetTorrentProperties(ctx, targetID, hash)
	files, _ := h.syncManager.GetTorrentFiles(ctx, targetID, hash)
	trackers, _ := h.syncManager.GetTorrentTrackers(ctx, targetID, hash)
	webSeeds, _ := h.syncManager.GetTorrentWebSeeds(ctx, targetID, hash)

	var peers []qbt.TorrentPeer
	if peersResp, err := h.syncManager.GetTorrentPeers(ctx, targetID, hash); err == nil && peersResp != nil {
		peers = make([]qbt.TorrentPeer, 0, len(peersResp.Peers))
		for _, peer := range peersResp.Peers {
			peers = append(peers, peer)
		}
	}

	render(w, r, http.StatusOK, pages.TorrentDetailPanel(pages.TorrentDetailProps{
		BaseURL:    h.baseURL(),
		InstanceID: targetID,
		Hash:       hash,
		Name:       name,
		State:      state,
		Category:   category,
		Tags:       tags,
		Properties: props,
		Files:      files,
		Trackers:   trackers,
		Peers:      peers,
		WebSeeds:   webSeeds,
	}))
}

// PostTorrentsAction executes a bulk torrent action (pause, resume, delete,
// deleteWithFiles, recheck, reannounce) and returns a refreshed table body.
// Route: POST /ui/partials/torrents/action
func (h *Handler) PostTorrentsAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	hashes := r.Form["hashes"]
	instanceID := intParam(r.FormValue("instance_id"), 0)

	ctx := r.Context()

	// Resolve instance ID.
	if instanceID == 0 {
		if insts, err := h.instanceStore.List(ctx); err == nil {
			for _, inst := range insts {
				if inst.IsActive {
					instanceID = inst.ID
					break
				}
			}
		}
	}

	if h.syncManager != nil && len(hashes) > 0 && action != "" && instanceID > 0 {
		_ = h.syncManager.BulkAction(ctx, instanceID, hashes, action)
	}

	// Re-render the table body with the current instance (no filters — bulk bar
	// does not carry filter state).
	rows, total, _ := h.fetchTorrentRows(ctx, instanceID, 1, defaultPageSize, "", "", "", "")
	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Page:       1,
		PageSize:   defaultPageSize,
		Rows:       rows,
		Total:      total,
	}))
}

// PostAddTorrent accepts a .torrent file upload or a magnet/URL string, forwards
// it to the chosen qBittorrent instance and returns a refreshed table body.
// Route: POST /ui/partials/torrents/add
func (h *Handler) PostAddTorrent(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 64 << 20 // 64 MiB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		// Fall back to regular form.
		if err2 := r.ParseForm(); err2 != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	// Resolve instance ID.
	if instanceID == 0 {
		if insts, err := h.instanceStore.List(ctx); err == nil {
			for _, inst := range insts {
				if inst.IsActive {
					instanceID = inst.ID
					break
				}
			}
		}
	}

	if h.syncManager != nil && instanceID > 0 {
		// Build options map.
		opts := map[string]string{}
		if cat := strings.TrimSpace(r.FormValue("category")); cat != "" {
			opts["category"] = cat
		}
		if sp := strings.TrimSpace(r.FormValue("savepath")); sp != "" {
			opts["savePath"] = sp
		}
		if r.FormValue("paused") == "true" {
			opts["paused"] = "true"
		}

		// Try .torrent file first.
		if r.MultipartForm != nil {
			if fhs := r.MultipartForm.File["torrentfile"]; len(fhs) > 0 {
				f, err := fhs[0].Open()
				if err == nil {
					defer f.Close() //nolint:errcheck
					data, readErr := io.ReadAll(f)
					if readErr == nil && len(data) > 0 {
						_ = h.syncManager.AddTorrent(ctx, instanceID, data, opts)
					}
				}
			}
		}

		// Also process URL/magnet if provided.
		if rawURLs := strings.TrimSpace(r.FormValue("urls")); rawURLs != "" {
			var urls []string
			for _, u := range strings.Split(rawURLs, "\n") {
				u = strings.TrimSpace(u)
				if u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				_ = h.syncManager.AddTorrentFromURLs(ctx, instanceID, urls, opts)
			}
		}
	}

	// Re-render the table body so newly added torrents appear immediately.
	rows, total, _ := h.fetchTorrentRows(ctx, instanceID, 1, defaultPageSize, "", "", "", "")
	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Page:       1,
		PageSize:   defaultPageSize,
		Rows:       rows,
		Total:      total,
	}))
}
