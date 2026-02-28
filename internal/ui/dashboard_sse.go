// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// dashboard_sse.go implements the dashboard live-update SSE endpoint.
// It streams "dashboard-update" events whenever aggregate instance stats
// change (connection state, torrent counts, or transfer speeds shift by a
// meaningful amount). The dashboard page listens with the HTMX SSE extension
// and falls back to slow polling when EventSource is unavailable.

package ui

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"time"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/rs/zerolog/log"
)

// StreamDashboardSSE pushes "dashboard-update" events via Server-Sent Events.
//
// The handler:
//  1. Sends an initial "connected" event.
//  2. Polls all active instances every 3 s and emits "dashboard-update" when
//     aggregate stats change (connection count, torrent counts, or speeds shift
//     by ≥50 KiB/s).
//  3. Emits a keepalive comment every 15 s so proxies do not close the connection.
//
// Route: GET /ui/sse/dashboard
func (h *Handler) StreamDashboardSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintf(w, "event: connected\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if h.syncManager == nil || h.instanceStore == nil {
		<-ctx.Done()
		return
	}

	var lastFP string

	// fingerprint hashes the aggregate dashboard stats across all active instances.
	// Speeds are bucketed to 50 KiB/s steps to avoid spurious events from minor
	// fluctuations.
	fingerprint := func() string {
		insts, err := h.instanceStore.List(ctx)
		if err != nil {
			return lastFP
		}

		h64 := fnv.New64a()
		const speedBucket = 50 * 1024 // 50 KiB/s

		for _, inst := range insts {
			if !inst.IsActive {
				continue
			}

			var connected, dlSpeed, upSpeed int64
			var total, downloading, seeding int

			// All reads are atomic pointer loads — no torrent list copy.
			if client, cerr := h.syncManager.GetClientOffline(ctx, inst.ID); cerr == nil && client != nil {
				if ss := client.GetCachedServerState(); ss != nil {
					connected = 1
					if qbt.Deref(ss.DlInfoSpeed) > 0 {
						dlSpeed = qbt.Deref(ss.DlInfoSpeed) / speedBucket
					}
					if qbt.Deref(ss.UpInfoSpeed) > 0 {
						upSpeed = qbt.Deref(ss.UpInfoSpeed) / speedBucket
					}
				}
				if counts := client.GetCachedTorrentCounts(); counts != nil {
					total = counts.Total
					downloading = counts.Downloading
					seeding = counts.Seeding
				}
			}

			fmt.Fprintf(h64, "%d|%d|%d|%d|%d|%d|", //nolint:errcheck
				inst.ID, connected, total, downloading, seeding, dlSpeed+upSpeed)
		}

		return fmt.Sprintf("%x", h64.Sum64())
	}

	sendUpdate := func() bool {
		fp := fingerprint()
		if fp == lastFP {
			return true
		}
		lastFP = fp
		if _, err := fmt.Fprintf(w, "event: dashboard-update\ndata: {}\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	pollTicker := time.NewTicker(3 * time.Second)
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer pollTicker.Stop()
	defer keepaliveTicker.Stop()

	log.Debug().Msg("dashboard SSE client connected")
	defer log.Debug().Msg("dashboard SSE client disconnected")

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
