// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// QBTSyncManager wraps *qbt.SyncManager and adds backward-compatible methods
// that existed in github.com/autobrr/go-qbittorrent but were removed or
// significantly changed in github.com/autogrr/go-qbittorrent.

package qbittorrent

import (
	"context"
	"strings"
	"sync"
	"time"

	qbt "github.com/autogrr/go-qbittorrent"
)

// QBTSyncManager wraps the qBittorrent sync manager from the upstream library
// and restores the richer API surface (GetTorrents with filter, GetTorrentMap,
// LastSyncTime, Trackers, etc.) that older code depends on.
type QBTSyncManager struct {
	*qbt.SyncManager

	trackerMgr *TrackerManager

	mu       sync.RWMutex
	lastSync time.Time

	// prevHashes is used to compute TorrentsRemoved for legacy callbacks.
	prevHashes map[string]struct{}

	// lastTrackers and lastCategories cache a snapshot from the most recent
	// OnUpdate call for use in GetData(). Protected by mu.
	lastTrackers   map[string][]string
	lastCategories map[string]qbt.Category

	// triggerAfterSync is set in newQBTSyncManager to the wrapped OnUpdate
	// function. It is invoked once in Start() after the initial synchronous
	// Sync() call so that GetCachedServerState / GetCachedTorrentCounts are
	// populated before the background loop's first tick.
	triggerAfterSync func(*qbt.SyncState)
}

// newQBTSyncManager constructs a QBTSyncManager.
// It intercepts SyncOptions.OnUpdate so it can track lastSync and compute
// removed-torrent lists for legacy delta-style callbacks.
func newQBTSyncManager(
	client *qbt.Client,
	opts qbt.SyncOptions,
	trackerMgr *TrackerManager,
	legacyOnUpdate func(*qbt.MainData),
	legacyOnError func(error),
) *QBTSyncManager {
	sm := &QBTSyncManager{
		trackerMgr:     trackerMgr,
		lastSync:       time.Now(),
		prevHashes:     make(map[string]struct{}),
		lastTrackers:   make(map[string][]string),
		lastCategories: make(map[string]qbt.Category),
	}

	// Wrap OnUpdate: synthesise a *MainData so existing handlers keep working.
	wrappedOnUpdate := func(state *qbt.SyncState) {
		if state == nil {
			return
		}

		currTorrents := state.GetTorrents()
		currCategories := state.GetCategories()

		// Snapshot Trackers. state.Trackers is a public field guarded internally
		// by state.mu, which we cannot acquire from outside the package. This
		// copy is taken immediately after apply() finishes; concurrent user Sync()
		// calls are coalesced via singleflight so in practice no concurrent write
		// occurs during this window.
		currTrackers := make(map[string][]string, len(state.Trackers))
		for k, v := range state.Trackers {
			currTrackers[k] = v
		}

		// Compute removed hashes by diffing against the previous set.
		sm.mu.Lock()
		var removed []string
		for hash := range sm.prevHashes {
			if _, ok := currTorrents[hash]; !ok {
				removed = append(removed, hash)
			}
		}
		newPrev := make(map[string]struct{}, len(currTorrents))
		for hash := range currTorrents {
			newPrev[hash] = struct{}{}
		}
		sm.prevHashes = newPrev
		sm.lastSync = time.Now()
		sm.lastTrackers = currTrackers
		sm.lastCategories = currCategories
		sm.mu.Unlock()

		if legacyOnUpdate == nil {
			return
		}

		serverState := state.GetServerState()
		legacyOnUpdate(&qbt.MainData{
			Torrents:        currTorrents,
			TorrentsRemoved: removed,
			ServerState:     serverState,
			Categories:      currCategories,
			Trackers:        currTrackers,
		})
	}
	opts.OnUpdate = wrappedOnUpdate
	sm.triggerAfterSync = wrappedOnUpdate

	// Wrap OnError: old signature is func(error), new is func(error) bool.
	if legacyOnError != nil {
		opts.OnError = func(err error) bool {
			legacyOnError(err)
			return false // continue syncing
		}
	}

	// Ensure DynamicInterval (renamed from DynamicSync) is enabled.
	opts.DynamicInterval = true

	sm.SyncManager = client.NewSyncManager(opts)
	return sm
}

// Start performs an initial synchronous sync (returning any error) then
// launches the background polling loop. This mirrors the old
// autobrr/go-qbittorrent SyncManager.Start signature.
func (sm *QBTSyncManager) Start(ctx context.Context) error {
	if _, err := sm.SyncManager.Sync(ctx); err != nil {
		return err
	}
	// Immediately fire the OnUpdate-equivalent callback with the initial state
	// so that GetCachedServerState / GetCachedTorrentCounts / GetCachedTrackerRows
	// are populated before the background polling loop's first tick (~1 s).
	if state := sm.SyncManager.State(); state != nil && sm.triggerAfterSync != nil {
		sm.triggerAfterSync(state)
	}
	sm.SyncManager.Start(ctx)
	return nil
}

// Sync performs one synchronisation and returns only the error, discarding the
// *SyncState (old callers relied on this simpler signature).
func (sm *QBTSyncManager) Sync(ctx context.Context) error {
	_, err := sm.SyncManager.Sync(ctx)
	if err == nil {
		sm.mu.Lock()
		sm.lastSync = time.Now()
		sm.mu.Unlock()
	}
	return err
}

// GetTorrents returns torrents filtered by opts.Hashes (if non-empty) from the
// in-memory state. Other filter fields are not applied (the real filtering
// happens in SyncManager above this layer).
func (sm *QBTSyncManager) GetTorrents(opts qbt.TorrentFilterOptions) []qbt.Torrent {
	state := sm.State()
	if state == nil {
		return nil
	}
	torrents := state.GetTorrentSlice()
	if len(opts.Hashes) == 0 {
		return torrents
	}

	set := make(map[string]struct{}, len(opts.Hashes))
	for _, h := range opts.Hashes {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	filtered := make([]qbt.Torrent, 0, len(opts.Hashes))
	for _, t := range torrents {
		if t.Hash == nil {
			continue
		}
		if _, ok := set[strings.ToLower(*t.Hash)]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// GetTorrentsUnchecked is identical to GetTorrents for the new module; the
// "unchecked" distinction (bypassing freshness validation) no longer applies.
func (sm *QBTSyncManager) GetTorrentsUnchecked(opts qbt.TorrentFilterOptions) []qbt.Torrent {
	return sm.GetTorrents(opts)
}

// GetTorrentMap returns a hash→Torrent map, optionally filtered by opts.Hashes.
func (sm *QBTSyncManager) GetTorrentMap(opts qbt.TorrentFilterOptions) map[string]qbt.Torrent {
	torrents := sm.GetTorrents(opts)
	result := make(map[string]qbt.Torrent, len(torrents))
	for _, t := range torrents {
		if t.Hash != nil {
			result[*t.Hash] = t
		}
	}
	return result
}

// GetTorrent returns a single torrent by hash from the in-memory state.
func (sm *QBTSyncManager) GetTorrent(hash string) (qbt.Torrent, bool) {
	if sm.State() == nil {
		return qbt.Torrent{}, false
	}
	return sm.State().GetTorrent(hash)
}

// GetCategories returns all categories from the in-memory state.
func (sm *QBTSyncManager) GetCategories() map[string]qbt.Category {
	if sm.State() == nil {
		return nil
	}
	return sm.State().GetCategories()
}

// GetTags returns all tags from the in-memory state.
func (sm *QBTSyncManager) GetTags() []string {
	if sm.State() == nil {
		return nil
	}
	return sm.State().GetTags()
}

// LastSyncTime returns the time of the last successful sync that changed state.
func (sm *QBTSyncManager) LastSyncTime() time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastSync
}

// GetServerState returns the most recent qBittorrent server state.
func (sm *QBTSyncManager) GetServerState() qbt.ServerState {
	if sm.State() == nil {
		return qbt.ServerState{}
	}
	return sm.State().GetServerState()
}

// Trackers returns the TrackerManager associated with this sync manager.
func (sm *QBTSyncManager) Trackers() *TrackerManager {
	return sm.trackerMgr
}

// GetData synthesises a *qbt.MainData from the cached snapshot for callers
// that still use the old SyncManager.GetData() pattern. Categories and Trackers
// are taken from the snapshot populated during the most recent OnUpdate; Torrents
// and ServerState are read fresh from the live SyncState.
func (sm *QBTSyncManager) GetData() *qbt.MainData {
	state := sm.SyncManager.State()
	if state == nil {
		return nil
	}

	sm.mu.RLock()
	trackers := sm.lastTrackers
	categories := sm.lastCategories
	sm.mu.RUnlock()

	return &qbt.MainData{
		Torrents:    state.GetTorrents(),
		Categories:  categories,
		Tags:        state.GetTags(),
		Trackers:    trackers,
		ServerState: state.GetServerState(),
	}
}

// GetTrackerMap returns the cached tracker→[]hash map from the most recent sync.
func (sm *QBTSyncManager) GetTrackerMap() map[string][]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastTrackers
}
