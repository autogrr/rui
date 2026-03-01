// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// TrackerManager was removed from github.com/autogrr/go-qbittorrent.
// This file provides a local implementation that delegates to the qBittorrent
// client API for tracker metadata hydration and caching.

package qbittorrent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/autogrr/go-ttlcache/pkg/ttlcache"
	qbt "github.com/autogrr/go-qbittorrent"
)

const (
	trackerCacheTTL         = 5 * time.Minute
	trackerIncludeChunkSize = 100
)

// trackerAPI describes the subset of qbt.Client functionality required by
// TrackerManager.
type trackerAPI interface {
	GetTorrents(ctx context.Context, opts qbt.TorrentFilterOptions) ([]qbt.Torrent, error)
	GetTorrentTrackers(ctx context.Context, hash string) ([]qbt.TorrentTracker, error)
}

// TrackerManager coordinates tracker metadata hydration with caching.
type TrackerManager struct {
	api                trackerAPI
	cache              *ttlcache.Cache[string, []qbt.TorrentTracker]
	useIncludeTrackers atomic.Bool
}

// NewTrackerManager constructs a TrackerManager backed by the given client.
func NewTrackerManager(api trackerAPI) *TrackerManager {
	return &TrackerManager{
		api: api,
		cache: ttlcache.New(ttlcache.Options[string, []qbt.TorrentTracker]{}.
			SetDefaultTTL(trackerCacheTTL).
			DisableUpdateTime(true)),
	}
}

// HydrateTorrents enriches the provided torrents with tracker metadata from
// cache. It returns the original slice (unchanged) and a map of tracker lists
// keyed by hash.
//
// NOTE: Tracker data is returned in trackerMap rather than written back to
// Torrent.Trackers. The inline field is populated only when IncludeTrackers is
// set on a GetTorrents call; torrents from the sync/maindata path do not carry
// inline trackers. Callers must use the returned trackerMap.
func (tm *TrackerManager) HydrateTorrents(ctx context.Context, torrents []qbt.Torrent) ([]qbt.Torrent, map[string][]qbt.TorrentTracker) {
	if tm == nil || len(torrents) == 0 {
		return torrents, nil
	}

	trackerMap := make(map[string][]qbt.TorrentTracker, len(torrents))
	hashesToFetch := make([]string, 0)
	hashToIndex := make(map[string]int, len(torrents))

	for i := range torrents {
		hash := strings.TrimSpace(ptrStr(torrents[i].Hash))
		if hash == "" {
			continue
		}
		hashToIndex[hash] = i

		if trackers, ok := tm.cache.Get(hash); ok {
			trackerMap[hash] = trackers
			continue
		}
		hashesToFetch = append(hashesToFetch, hash)
	}

	if len(hashesToFetch) == 0 {
		return torrents, trackerMap
	}

	if tm.SupportsIncludeTrackers() {
		tm.hydrateWithIncludeTrackers(ctx, trackerMap, hashesToFetch, hashToIndex)
	} else {
		tm.hydrateIndividually(ctx, trackerMap, hashesToFetch)
	}

	return torrents, trackerMap
}

func (tm *TrackerManager) hydrateWithIncludeTrackers(
	ctx context.Context,
	trackerMap map[string][]qbt.TorrentTracker,
	hashes []string,
	hashToIndex map[string]int,
) {
	pending := make(map[string]struct{}, len(hashes))
	ordered := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" || pending[hash] != (struct{}{}) {
			continue
		}
		if _, exists := pending[hash]; exists {
			continue
		}
		pending[hash] = struct{}{}
		ordered = append(ordered, hash)
	}
	if len(pending) == 0 {
		return
	}

	applyFetched := func(fetched []qbt.Torrent) int {
		progress := 0
		for _, t := range fetched {
			hash := strings.TrimSpace(ptrStr(t.Hash))
			if hash == "" {
				continue
			}
			if _, ok := hashToIndex[hash]; ok {
				if _, wasPending := pending[hash]; wasPending {
					// Use inline tracker data when available (qBittorrent >= 5.1 with IncludeTrackers).
					if len(t.Trackers) > 0 {
						trackerMap[hash] = t.Trackers
						tm.cache.Set(hash, t.Trackers, trackerCacheTTL)
					}
					delete(pending, hash)
					progress++
				}
			}
		}
		return progress
	}

	fetchAll := func() {
		if fetched, err := tm.api.GetTorrents(ctx, qbt.TorrentFilterOptions{IncludeTrackers: true}); err == nil {
			applyFetched(fetched)
		}
	}

	if fetched, err := tm.api.GetTorrents(ctx, qbt.TorrentFilterOptions{Hashes: ordered, IncludeTrackers: true}); err == nil {
		if applyFetched(fetched) > 0 && len(pending) == 0 {
			return
		}
	}

	for len(pending) > 0 {
		chunk := make([]string, 0, min(len(pending), trackerIncludeChunkSize))
		for hash := range pending {
			chunk = append(chunk, hash)
			if len(chunk) >= trackerIncludeChunkSize {
				break
			}
		}
		fetched, err := tm.api.GetTorrents(ctx, qbt.TorrentFilterOptions{Hashes: chunk, IncludeTrackers: true})
		if err != nil {
			fetchAll()
			return
		}
		if applyFetched(fetched) == 0 {
			fetchAll()
			return
		}
	}

	// Fetch trackers individually for all hashes still missing from trackerMap.
	remaining := make([]string, 0, len(hashes))
	for _, h := range hashes {
		if _, ok := trackerMap[h]; !ok {
			remaining = append(remaining, h)
		}
	}
	if len(remaining) > 0 {
		tm.hydrateIndividually(ctx, trackerMap, remaining)
	}
}

func (tm *TrackerManager) hydrateIndividually(
	ctx context.Context,
	trackerMap map[string][]qbt.TorrentTracker,
	hashes []string,
) {
	type result struct {
		hash     string
		trackers []qbt.TorrentTracker
		err      error
	}

	results := make(chan result, len(hashes))
	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup

	wg.Add(len(hashes))
	for _, h := range hashes {
		sem <- struct{}{}
		go func(hash string) {
			defer wg.Done()
			defer func() { <-sem }()

			select {
			case <-ctx.Done():
				results <- result{hash: hash, err: ctx.Err()}
				return
			default:
			}

			trackers, err := tm.fetchTrackersForHash(ctx, hash)
			results <- result{hash: hash, trackers: trackers, err: err}
		}(h)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err == nil && len(r.trackers) > 0 {
			trackerMap[r.hash] = r.trackers
			tm.cache.Set(r.hash, r.trackers, trackerCacheTTL)
		}
	}
}

// Invalidate clears cached tracker metadata for the given hashes.
// When no hashes are provided the entire cache is purged.
func (tm *TrackerManager) Invalidate(hashes ...string) {
	if tm == nil || tm.cache == nil {
		return
	}
	if len(hashes) == 0 {
		for _, key := range tm.cache.GetKeys() {
			if key != "" {
				tm.cache.Delete(key)
			}
		}
		return
	}
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash != "" {
			tm.cache.Delete(hash)
		}
	}
}

// SetUseIncludeTrackers configures whether to use the bulk IncludeTrackers API.
func (tm *TrackerManager) SetUseIncludeTrackers(use bool) {
	if tm != nil {
		tm.useIncludeTrackers.Store(use)
	}
}

// SupportsIncludeTrackers reports whether bulk tracker fetching is enabled.
func (tm *TrackerManager) SupportsIncludeTrackers() bool {
	return tm != nil && tm.useIncludeTrackers.Load()
}

func (tm *TrackerManager) fetchTrackersForHash(ctx context.Context, hash string) ([]qbt.TorrentTracker, error) {
	if tm == nil || tm.api == nil {
		return nil, fmt.Errorf("tracker manager not initialized")
	}
	return tm.api.GetTorrentTrackers(ctx, hash)
}
