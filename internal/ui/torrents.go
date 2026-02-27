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
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

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

	// Fetch cross-seed local matches (best-effort, errors are non-fatal).
	var crossSeedMatches []pages.CrossSeedMatch
	if h.crossSeedService != nil {
		if resp, err := h.crossSeedService.FindLocalMatches(ctx, targetID, hash, false); err == nil && resp != nil {
			for _, m := range resp.Matches {
				crossSeedMatches = append(crossSeedMatches, pages.CrossSeedMatch{
					InstanceName: m.InstanceName,
					Name:         m.Name,
					Category:     m.Category,
					Tags:         m.Tags,
					State:        m.State,
					SavePath:     m.SavePath,
					MatchType:    m.MatchType,
					Progress:     m.Progress,
					Size:         m.Size,
				})
			}
		} else if err != nil {
			log.Debug().Err(err).Str("hash", hash).Msg("ui: cross-seed local match check failed (non-fatal)")
		}
	}

	var peers []qbt.TorrentPeer
	if peersResp, err := h.syncManager.GetTorrentPeers(ctx, targetID, hash); err == nil && peersResp != nil {
		peers = make([]qbt.TorrentPeer, 0, len(peersResp.Peers))
		for _, peer := range peersResp.Peers {
			peers = append(peers, peer)
		}
	}

	render(w, r, http.StatusOK, pages.TorrentDetailPanel(pages.TorrentDetailProps{
		BaseURL:          h.baseURL(),
		InstanceID:       targetID,
		Hash:             hash,
		Name:             name,
		State:            state,
		Category:         category,
		Tags:             tags,
		Properties:       props,
		Files:            files,
		Trackers:         trackers,
		Peers:            peers,
		WebSeeds:         webSeeds,
		CrossSeedMatches: crossSeedMatches,
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
	rows, total, _ := h.fetchTorrentRows(ctx, instanceID, "", "", "", "", "", "added_on", "desc")
	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Rows:       rows,
		Total:      total,
	}))
}

// buildTorrentAddOpts constructs the qBittorrent options map from the add form.
// It covers all fields exposed in the expanded add-torrent dialog.
func buildTorrentAddOpts(r *http.Request) map[string]string {
	opts := map[string]string{}

	if cat := strings.TrimSpace(r.FormValue("category")); cat != "" {
		opts["category"] = cat
	}
	if tags := strings.TrimSpace(r.FormValue("tags")); tags != "" {
		opts["tags"] = tags
	}
	if r.FormValue("paused") == "true" {
		opts["paused"] = "true"
	}
	if r.FormValue("skip_hash_check") == "true" {
		opts["skip_checking"] = "true"
	}
	if r.FormValue("sequential_download") == "true" {
		opts["sequentialDownload"] = "true"
	}
	if r.FormValue("first_last_piece_prio") == "true" {
		opts["firstLastPiecePrio"] = "true"
	}
	if atmm := r.FormValue("auto_tmm"); atmm == "true" || atmm == "false" {
		opts["autoTMM"] = atmm
	}
	if sp := strings.TrimSpace(r.FormValue("savepath")); sp != "" {
		opts["savePath"] = sp
	}
	if r.FormValue("use_download_path") == "true" {
		opts["useDownloadPath"] = "true"
		if dp := strings.TrimSpace(r.FormValue("download_path")); dp != "" {
			opts["downloadPath"] = dp
		}
	}
	if dl := strings.TrimSpace(r.FormValue("dl_limit")); dl != "" && dl != "0" {
		if v, err := strconv.ParseInt(dl, 10, 64); err == nil && v > 0 {
			opts["dlLimit"] = strconv.FormatInt(v*1024, 10) // KiB/s → B/s
		}
	}
	if ul := strings.TrimSpace(r.FormValue("up_limit")); ul != "" && ul != "0" {
		if v, err := strconv.ParseInt(ul, 10, 64); err == nil && v > 0 {
			opts["upLimit"] = strconv.FormatInt(v*1024, 10) // KiB/s → B/s
		}
	}
	if rl := strings.TrimSpace(r.FormValue("ratio_limit")); rl != "" && rl != "0" {
		if v, err := strconv.ParseFloat(rl, 64); err == nil && v > 0 {
			opts["ratioLimit"] = strconv.FormatFloat(v, 'f', 2, 64)
		}
	}
	if stl := strings.TrimSpace(r.FormValue("seed_time_limit")); stl != "" && stl != "0" {
		if v, err := strconv.ParseInt(stl, 10, 64); err == nil && v > 0 {
			opts["seedingTimeLimit"] = strconv.FormatInt(v, 10)
		}
	}
	if cl := strings.TrimSpace(r.FormValue("content_layout")); cl != "" {
		opts["contentLayout"] = cl
	}
	if rn := strings.TrimSpace(r.FormValue("rename")); rn != "" {
		opts["rename"] = rn
	}

	return opts
}

// PostAddTorrent accepts one or more .torrent file uploads or magnet/URL lines,
// forwards them to the chosen qBittorrent instance and returns a refreshed table
// body with all form options applied.
// Route: POST /ui/partials/torrents/add
func (h *Handler) PostAddTorrent(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 64 << 20 // 64 MiB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		if err2 := r.ParseForm(); err2 != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	ctx := r.Context()

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
		opts := buildTorrentAddOpts(r)

		// Upload every .torrent file provided (multi-file support).
		if r.MultipartForm != nil {
			for _, fh := range r.MultipartForm.File["torrentfile"] {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				data, readErr := io.ReadAll(f)
				_ = f.Close()
				if readErr == nil && len(data) > 0 {
					_ = h.syncManager.AddTorrent(ctx, instanceID, data, opts)
				}
			}
		}

		if rawURLs := strings.TrimSpace(r.FormValue("urls")); rawURLs != "" {
			var urls []string
			for _, u := range strings.Split(rawURLs, "\n") {
				if u = strings.TrimSpace(u); u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				_ = h.syncManager.AddTorrentFromURLs(ctx, instanceID, urls, opts)
			}
		}
	}

	rows, total, _ := h.fetchTorrentRows(ctx, instanceID, "", "", "", "", "", "added_on", "desc")
	render(w, r, http.StatusOK, pages.TorrentsTableBody(pages.TorrentsProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Sort:       "added_on",
		Order:      "desc",
		Rows:       rows,
		Total:      total,
	}))
}
