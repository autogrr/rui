// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents_sse.go implements the torrent live-update SSE endpoint for the UI.
// The endpoint streams "torrent-update" events whenever the torrent list for an
// instance changes. Clients (HTMX SSE extension) use these events as a trigger
// to re-fetch the table body with their current filter state, so the search /
// filter inputs are never clobbered by the refresh.

package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// StreamTorrentsSSE streams torrent change events via Server-Sent Events.
//
// The handler:
//  1. Resolves the target instance (falls back to first active if instance_id=0).
//  2. Sends an initial "connected" event.
//  3. Polls the SyncManager cache every 3 s and emits "torrent-update" when the
//     fingerprint changes (count, state, or speeds bucketed to 100 KiB/s steps).
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
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)

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

	if instanceID == 0 || h.syncManager == nil {
		// No usable instance — keep the connection alive but never send updates.
		<-ctx.Done()
		return
	}

	var lastFP string

	// fingerprint reads the atomically-cached torrent fingerprint. This is a
	// nanosecond-scale operation — no locking, no torrent copying, no HTTP calls.
	fingerprint := func() string {
		client, err := h.syncManager.GetClientOffline(ctx, instanceID)
		if err != nil || client == nil {
			return lastFP // no change on error — don't spam events
		}
		fp := client.GetCachedTorrentFP()
		if fp == 0 {
			return lastFP // no sync completed yet
		}
		return strconv.FormatUint(fp, 16)
	}

	sendUpdate := func() bool {
		fp := fingerprint()
		if fp == lastFP {
			return true // no change, still alive
		}
		lastFP = fp
		if _, err := fmt.Fprintf(w, "event: torrent-update\ndata: {}\n\n"); err != nil {
			return false // write error → close connection
		}
		flusher.Flush()
		return true
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
			if !sendUpdate() {
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
