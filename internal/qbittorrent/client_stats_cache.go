// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// client_stats_cache.go — zero-allocation dashboard stats cached on every sync update.
//
// Rather than copying the entire torrent list on every HTTP request (which
// costs 30–230 ms for large instances), we compute counts and per-tracker
// aggregates once inside the qbt.SyncManager OnUpdate callback and store them
// as atomically-swapped pointers. Dashboard reads become nanosecond-scale
// pointer loads with no lock contention.

package qbittorrent

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync/atomic"

	qbt "github.com/autogrr/go-qbittorrent"
)

// CachedTorrentCounts holds aggregate torrent state counts from the last sync.
type CachedTorrentCounts struct {
	Total       int
	Downloading int
	Seeding     int
}

// CachedTrackerRow holds per-tracker aggregated stats from the last sync.
type CachedTrackerRow struct {
	Domain      string
	Seeding     int
	Downloading int
	Count       int
	UpSpeed     uint64
	DlSpeed     uint64
	TotalSize   int64
}

// clientStatsCache holds the atomically-swapped cached stats on a Client.
// Both fields are always swapped together when a sync update arrives.
type clientStatsCache struct {
	counts   atomic.Pointer[CachedTorrentCounts]
	trackers atomic.Pointer[[]CachedTrackerRow]
	// torrentFP is a cheap FNV-64 fingerprint of the torrent list, updated
	// on every OnUpdate. Torrents SSE reads it atomically without copying
	// the full torrent list.
	torrentFP atomic.Uint64
	// domainExtractor is set once by pool.go before StartSyncManager is called,
	// so the very first OnUpdate can already compute tracker breakdown rows.
	domainExtractor atomic.Pointer[func(string) string]
}

// SetDomainExtractor stores the function used to resolve tracker domains.
// Called once by SyncManager after the client is placed in the pool.
func (c *Client) SetDomainExtractor(fn func(string) string) {
	c.statsCache.domainExtractor.Store(&fn)
}

// GetCachedTorrentCounts returns the last computed torrent counts.
// Returns nil if no sync has completed yet.
func (c *Client) GetCachedTorrentCounts() *CachedTorrentCounts {
	return c.statsCache.counts.Load()
}

// GetCachedTorrentFP returns the last computed torrent-list fingerprint.
// Returns 0 if no sync has completed yet.
func (c *Client) GetCachedTorrentFP() uint64 {
	return c.statsCache.torrentFP.Load()
}

// GetCachedTrackerRows returns the last computed per-tracker breakdown.
// Returns nil if no sync has completed yet or no domain extractor is set.
func (c *Client) GetCachedTrackerRows() []CachedTrackerRow {
	p := c.statsCache.trackers.Load()
	if p == nil {
		return nil
	}
	return *p
}

// updateCachedStats is called from the OnUpdate callback.
// It iterates data.Torrents once and atomically replaces both cached values.
func (c *Client) updateCachedStats(data *qbt.MainData) {
	var extractFn func(string) string
	if p := c.statsCache.domainExtractor.Load(); p != nil {
		extractFn = *p
	}

	counts := &CachedTorrentCounts{}
	var trackerAgg map[string]*CachedTrackerRow
	if extractFn != nil {
		trackerAgg = make(map[string]*CachedTrackerRow, 64)
	}

	for _, t := range data.Torrents {
		counts.Total++

		switch ptrTorrentState(t.State) {
		case qbt.StateDownloading, qbt.StateStalledDL,
			qbt.StateMetaDL, qbt.StateCheckingDL,
			qbt.StateAllocating, qbt.StateForcedDL:
			counts.Downloading++
		case qbt.StateUploading, qbt.StateStalledUP,
			qbt.StateForcedUP, qbt.StateCheckingUP:
			counts.Seeding++
		}

		if trackerAgg == nil {
			continue
		}

		domain := extractFn(ptrStr(t.Tracker))
		if domain == "" {
			domain = "Unknown"
		}
		row, ok := trackerAgg[domain]
		if !ok {
			row = &CachedTrackerRow{Domain: domain}
			trackerAgg[domain] = row
		}
		row.Count++
		row.TotalSize += ptrInt64(t.Size)
		if ptrInt64(t.UpSpeed) > 0 {
			row.UpSpeed += uint64(ptrInt64(t.UpSpeed))
		}
		if ptrInt64(t.DlSpeed) > 0 {
			row.DlSpeed += uint64(ptrInt64(t.DlSpeed))
		}
		switch ptrTorrentState(t.State) {
		case qbt.StateUploading, qbt.StateStalledUP,
			qbt.StateQueuedUP, qbt.StateCheckingUP,
			qbt.StateForcedUP:
			row.Seeding++
		case qbt.StateDownloading, qbt.StateStalledDL,
			qbt.StateMetaDL, qbt.StateQueuedDL,
			qbt.StateAllocating, qbt.StateCheckingDL,
			qbt.StateForcedDL:
			row.Downloading++
		}
	}

	c.statsCache.counts.Store(counts)

	// Compute a stable torrent-list fingerprint using sorted hashes so the FP
	// only changes when actual data changes (state, speed bucket), not due to
	// random map iteration order.
	h64 := fnv.New64a()
	fmt.Fprintf(h64, "%d", counts.Total) //nolint:errcheck
	const fpSpeedBucket = 100 * 1024 // 100 KiB/s buckets to suppress noise
	if len(data.Torrents) > 0 {
		hashes := make([]string, 0, len(data.Torrents))
		for h := range data.Torrents {
			hashes = append(hashes, h)
		}
		sort.Strings(hashes)
		if len(hashes) > 20 {
			hashes = hashes[:20]
		}
		for _, hash := range hashes {
			t := data.Torrents[hash]
			fmt.Fprintf(h64, "%s%s%d%d", hash, ptrTorrentState(t.State), //nolint:errcheck
				uint64(ptrInt64(t.DlSpeed))/fpSpeedBucket,
				uint64(ptrInt64(t.UpSpeed))/fpSpeedBucket,
			)
		}
	}
	c.statsCache.torrentFP.Store(h64.Sum64())

	if trackerAgg != nil {
		rows := make([]CachedTrackerRow, 0, len(trackerAgg))
		for _, r := range trackerAgg {
			rows = append(rows, *r)
		}
		// Sort by domain so the stored slice has a deterministic order.
		// This ensures the dashboard SSE fingerprint only changes when actual
		// tracker data changes, not due to random map iteration.
		sort.Slice(rows, func(i, j int) bool { return rows[i].Domain < rows[j].Domain })
		c.statsCache.trackers.Store(&rows)
	}
}
