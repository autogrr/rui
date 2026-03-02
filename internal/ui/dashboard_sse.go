// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// dashboard_sse.go implements the dashboard live-update SSE endpoint.
// It streams rendered HTML fragments directly:
//   - event: dashboard-data     (global stats + instance cards)
//   - event: dashboard-tracker  (tracker breakdown card)
// whenever aggregate instance stats change.

package ui

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"
	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/ui/pages"
)

// StreamDashboardSSE pushes dashboard HTML fragments via Server-Sent Events.
//
// The handler:
//  1. Sends an initial "connected" event.
//  2. Polls all active instances every 3 s and emits dashboard fragment events when
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
	baseURL := h.baseURL()

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
			var total, downloading, seeding, trackerDown int
			var altSpeed bool

			// All reads are atomic pointer loads — no torrent list copy.
			if client, cerr := h.syncManager.GetClientOffline(ctx, inst.ID); cerr == nil && client != nil {
				if ss := client.GetCachedServerState(); ss != nil {
					connected = 1
					altSpeed = qbt.Deref(ss.UseAltSpeedLimits)
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
				// Include per-tracker row counts so tracker breakdown
				// changes also trigger a dashboard-update event.
				// Sort first so hash input is deterministic across polls.
				trackerRows := client.GetCachedTrackerRows()
				sort.Slice(trackerRows, func(i, j int) bool {
					return trackerRows[i].Domain < trackerRows[j].Domain
				})
				for _, row := range trackerRows {
					fmt.Fprintf(h64, "t|%s|%d|", row.Domain, row.Count) //nolint:errcheck
				}
			}
			// Tracker-down count from health cache.
			if hc := h.syncManager.GetTrackerHealthCounts(inst.ID); hc != nil {
				trackerDown = hc.TrackerDown
			}

			fmt.Fprintf(h64, "%d|%d|%d|%d|%d|%d|%d|%v|", //nolint:errcheck
				inst.ID, connected, total, downloading, seeding, dlSpeed+upSpeed, trackerDown, altSpeed)
		}

		return fmt.Sprintf("%x", h64.Sum64())
	}

	sendUpdate := func() bool {
		fp := fingerprint()
		if fp == lastFP {
			return true
		}
		lastFP = fp

		dashInsts := h.buildDashboardInstances(r)
		statsHTML, err := renderComponentHTML(ctx, pages.DashboardStatsPartial(dashInsts, baseURL))
		if err != nil {
			return false
		}
		trackerRows := h.buildDashboardTrackerBreakdownRows(ctx)
		trackerHTML, err := renderComponentHTML(ctx, pages.DashboardTrackerBreakdown(trackerRows, baseURL))
		if err != nil {
			return false
		}

		if err := writeSSEEvent(w, "dashboard-data", statsHTML); err != nil {
			return false
		}
		if err := writeSSEEvent(w, "dashboard-tracker", trackerHTML); err != nil {
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

	if !sendUpdate() {
		return
	}

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

func renderComponentHTML(ctx context.Context, c templ.Component) (string, error) {
	if c == nil {
		return "", nil
	}
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeSSEEvent(w http.ResponseWriter, event, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if data == "" {
		if _, err := fmt.Fprint(w, "data: {}\n\n"); err != nil {
			return err
		}
		return nil
	}
	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}
