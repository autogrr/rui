// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents_sse.go implements the torrent live-update SSE endpoint for the UI.
// The endpoint streams "torrents-data" events containing a full JSON snapshot
// (rows, total, instance ID, base URL) for the current filter/search/sort
// query. Clients reconnect with updated query params when filters change.

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/ui/pages"
)

// StreamTorrentsSSE streams torrent snapshots via Server-Sent Events.
//
// The handler:
//  1. Resolves the target instance (falls back to first active if instance_id=0).
//  2. Sends an initial "connected" event.
//  3. Polls the SyncManager cache every 3 s and emits "torrents-data" when the
//     instance fingerprint changes.
//  4. Emits a keepalive comment every 15 s so proxies do not close the connection.
//
// Route: GET /ui/sse/torrents
func (h *Handler) StreamTorrentsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), 0)
	search := strings.TrimSpace(q.Get("search"))
	status := q.Get("status")
	category := q.Get("category")
	tag := q.Get("tag")
	tracker := q.Get("tracker")
	savepath := q.Get("savepath")
	expr := strings.TrimSpace(q.Get("expr"))
	sortCol := q.Get("sort")
	sortOrder := q.Get("order")
	if sortCol == "" {
		sortCol = "added_on"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Resolve the instance to poll.
	if instanceID == 0 && h.instanceStore != nil {
		if insts, err := h.instanceStore.List(ctx); err == nil {
			for _, inst := range insts {
				if inst.IsActive {
					instanceID = inst.ID
					break
				}
			}
		}
	}

	// Set SSE headers before writing anything.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	// Send connected event so the client knows the stream is live.
	if _, err := fmt.Fprintf(w, "event: connected\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if h.syncManager == nil {
		// No usable instance — keep the connection alive but never send updates.
		<-ctx.Done()
		return
	}

	var lastFP string

	sendUpdate := func(force bool) bool {
		rows, total, resolvedID := h.fetchTorrentRows(ctx, instanceID, search, status, category, tag, tracker, savepath, expr, sortCol, sortOrder)

		fp := ""
		if resolvedID > 0 {
			if client, err := h.syncManager.GetClientOffline(ctx, resolvedID); err == nil && client != nil {
				if cached := client.GetCachedTorrentFP(); cached != 0 {
					fp = strconv.FormatUint(cached, 16)
				}
			}
		}
		if fp == "" {
			fp = fmt.Sprintf("%d:%d", resolvedID, total)
		}

		if !force && fp == lastFP {
			return true
		}
		lastFP = fp

		payload, err := json.Marshal(struct {
			Rows       []pages.TorrentRow `json:"rows"`
			BaseURL    string             `json:"baseURL"`
			InstanceID int                `json:"instanceID"`
			Total      int                `json:"total"`
		}{
			Rows:       rows,
			BaseURL:    h.baseURL(),
			InstanceID: resolvedID,
			Total:      total,
		})
		if err != nil {
			return false
		}

		if _, err := fmt.Fprintf(w, "event: torrents-data\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !sendUpdate(true) {
		return
	}

	pollTicker := time.NewTicker(3 * time.Second)
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer pollTicker.Stop()
	defer keepaliveTicker.Stop()

	log.Debug().Int("instanceID", instanceID).Msg("torrent SSE client connected")
	defer log.Debug().Int("instanceID", instanceID).Msg("torrent SSE client disconnected")

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			if !sendUpdate(false) {
				return
			}
		case <-keepaliveTicker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
