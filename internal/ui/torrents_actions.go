// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents_actions.go implements individual torrent action handlers for the
// context menu and detail panel operations:
//   - set category   (POST /ui/partials/torrents/set-category)
//   - add tags       (POST /ui/partials/torrents/add-tags)
//   - remove tags    (POST /ui/partials/torrents/remove-tags)
//   - set location   (POST /ui/partials/torrents/set-location)
//   - set limits     (POST /ui/partials/torrents/set-limits)
//   - rename         (POST /ui/partials/torrents/rename)
//   - file priority  (POST /ui/partials/torrents/file-priority)
//   - export torrent (GET  /ui/partials/torrents/export/{hash})

package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// PostTorrentSetCategory sets the category for the given torrent hashes.
// Route: POST /ui/partials/torrents/set-category
func (h *Handler) PostTorrentSetCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hashes := r.Form["hashes"]
	category := r.FormValue("category") // empty string = clear category
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || len(hashes) == 0 {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.SetCategory(ctx, instanceID, hashes, category); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentAddTags adds tags to the given torrent hashes.
// Route: POST /ui/partials/torrents/add-tags
func (h *Handler) PostTorrentAddTags(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hashes := r.Form["hashes"]
	tags := strings.TrimSpace(r.FormValue("tags")) // comma-separated
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || len(hashes) == 0 || tags == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.AddTags(ctx, instanceID, hashes, tags); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentRemoveTags removes tags from the given torrent hashes.
// Route: POST /ui/partials/torrents/remove-tags
func (h *Handler) PostTorrentRemoveTags(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hashes := r.Form["hashes"]
	tags := strings.TrimSpace(r.FormValue("tags")) // comma-separated
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || len(hashes) == 0 || tags == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.RemoveTags(ctx, instanceID, hashes, tags); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentSetLocation moves torrents to a new save path.
// Route: POST /ui/partials/torrents/set-location
func (h *Handler) PostTorrentSetLocation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hashes := r.Form["hashes"]
	location := strings.TrimSpace(r.FormValue("location"))
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || len(hashes) == 0 || location == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.SetLocation(ctx, instanceID, hashes, location); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentSetLimits sets download/upload speed limits and share ratio/time limits.
// Route: POST /ui/partials/torrents/set-limits
func (h *Handler) PostTorrentSetLimits(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hashes := r.Form["hashes"]
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || len(hashes) == 0 {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	// Speed limits (KiB/s; -1 = use global, 0 = unlimited).
	if dlStr := r.FormValue("dl_limit"); dlStr != "" {
		dlLimit, err := strconv.ParseInt(dlStr, 10, 64)
		if err == nil {
			_ = h.syncManager.SetTorrentDownloadLimit(ctx, instanceID, hashes, dlLimit)
		}
	}
	if ulStr := r.FormValue("up_limit"); ulStr != "" {
		ulLimit, err := strconv.ParseInt(ulStr, 10, 64)
		if err == nil {
			_ = h.syncManager.SetTorrentUploadLimit(ctx, instanceID, hashes, ulLimit)
		}
	}

	// Share limits.
	ratioStr := r.FormValue("ratio_limit")
	seedTimeStr := r.FormValue("seed_time_limit")
	if ratioStr != "" || seedTimeStr != "" {
		ratio := float64(-2)  // -2 = use global
		seedTime := int64(-2) // -2 = use global
		if ratioStr != "" {
			if v, err := strconv.ParseFloat(ratioStr, 64); err == nil {
				ratio = v
			}
		}
		if seedTimeStr != "" {
			if v, err := strconv.ParseInt(seedTimeStr, 10, 64); err == nil {
				seedTime = v
			}
		}
		_ = h.syncManager.SetTorrentShareLimit(ctx, instanceID, hashes, ratio, seedTime, -2)
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentRename renames a torrent's display name.
// Route: POST /ui/partials/torrents/rename
func (h *Handler) PostTorrentRename(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	name := strings.TrimSpace(r.FormValue("name"))
	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || hash == "" || name == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.syncManager.RenameTorrent(ctx, instanceID, hash, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentFilePriority sets file priority for specific file indices.
// Route: POST /ui/partials/torrents/file-priority
func (h *Handler) PostTorrentFilePriority(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := intParam(r.FormValue("instance_id"), 0)
	priority := intParam(r.FormValue("priority"), 1)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || hash == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	// Parse comma-separated indices.
	idxStrs := strings.Split(r.FormValue("indices"), ",")
	indices := make([]int, 0, len(idxStrs))
	for _, s := range idxStrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if idx, err := strconv.Atoi(s); err == nil {
			indices = append(indices, idx)
		}
	}
	if len(indices) == 0 {
		http.Error(w, "no file indices", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.SetTorrentFilePriority(ctx, instanceID, hash, indices, priority); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTorrentExport serves the .torrent file for download.
// Route: GET /ui/partials/torrents/export/{hash}
func (h *Handler) GetTorrentExport(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	ctx := r.Context()

	instanceID = h.resolveInstanceID(ctx, instanceID)
	if h.syncManager == nil || instanceID == 0 || hash == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	data, filename, contentType, err := h.syncManager.ExportTorrent(ctx, instanceID, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if filename == "" {
		filename = hash + ".torrent"
	}
	if contentType == "" {
		contentType = "application/x-bittorrent"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// resolveInstanceID resolves an instance ID, falling back to first active.
func (h *Handler) resolveInstanceID(ctx context.Context, instanceID int) int {
	if instanceID > 0 {
		return instanceID
	}
	if h.instanceStore == nil {
		return 0
	}
	if insts, err := h.instanceStore.List(ctx); err == nil {
		for _, inst := range insts {
			if inst.IsActive {
				return inst.ID
			}
		}
	}
	return 0
}

// PostTorrentAddTrackers adds tracker URLs to a torrent.
// Route: POST /ui/partials/torrents/add-trackers
func (h *Handler) PostTorrentAddTrackers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	urls := strings.TrimSpace(r.FormValue("urls"))

	if h.syncManager == nil || hash == "" || instanceID == 0 || urls == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.BulkAddTrackers(r.Context(), instanceID, []string{hash}, urls); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentRemoveTrackers removes tracker URLs from a torrent.
// Route: POST /ui/partials/torrents/remove-trackers
func (h *Handler) PostTorrentRemoveTrackers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	url := strings.TrimSpace(r.FormValue("url"))

	if h.syncManager == nil || hash == "" || instanceID == 0 || url == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.BulkRemoveTrackers(r.Context(), instanceID, []string{hash}, url); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentEditTracker edits a tracker URL for a torrent.
// Route: POST /ui/partials/torrents/edit-tracker
func (h *Handler) PostTorrentEditTracker(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	oldURL := strings.TrimSpace(r.FormValue("old_url"))
	newURL := strings.TrimSpace(r.FormValue("new_url"))

	if h.syncManager == nil || hash == "" || instanceID == 0 || oldURL == "" || newURL == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.BulkEditTrackers(r.Context(), instanceID, []string{hash}, oldURL, newURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentRenameFile renames a file inside a torrent.
// Route: POST /ui/partials/torrents/rename-file
func (h *Handler) PostTorrentRenameFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	oldPath := strings.TrimSpace(r.FormValue("old_path"))
	newPath := strings.TrimSpace(r.FormValue("new_path"))

	if h.syncManager == nil || hash == "" || instanceID == 0 || oldPath == "" || newPath == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.RenameTorrentFile(r.Context(), instanceID, hash, oldPath, newPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentBanPeers bans the specified peers.
// Route: POST /ui/partials/torrents/ban-peers
func (h *Handler) PostTorrentBanPeers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	peersStr := strings.TrimSpace(r.FormValue("peers"))

	if h.syncManager == nil || instanceID == 0 || peersStr == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	peers := strings.Split(peersStr, "\n")
	cleaned := make([]string, 0, len(peers))
	for _, p := range peers {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		http.Error(w, "no peers specified", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.BanPeers(r.Context(), instanceID, cleaned); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostTorrentAddPeers adds peers to the specified torrent.
// Route: POST /ui/partials/torrents/add-peers
func (h *Handler) PostTorrentAddPeers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hash := r.FormValue("hash")
	instanceID := h.resolveInstanceID(r.Context(), intParam(r.FormValue("instance_id"), 0))
	peersStr := strings.TrimSpace(r.FormValue("peers"))

	if h.syncManager == nil || hash == "" || instanceID == 0 || peersStr == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	peers := strings.Split(peersStr, "\n")
	cleaned := make([]string, 0, len(peers))
	for _, p := range peers {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		http.Error(w, "no peers specified", http.StatusBadRequest)
		return
	}

	if err := h.syncManager.AddPeersToTorrents(r.Context(), instanceID, []string{hash}, cleaned); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
