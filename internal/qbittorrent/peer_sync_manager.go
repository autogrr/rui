// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// PeerSyncManager was removed from github.com/autogrr/go-qbittorrent.
// This file provides a local implementation that uses the lower-level
// SyncTorrentPeers API available in autogrr/go-qbittorrent.

package qbittorrent

import (
	"context"
	"sync"
	"time"

	qbt "github.com/autogrr/go-qbittorrent"
)

// PeerSyncOptions configures the behaviour of the PeerSyncManager.
type PeerSyncOptions struct {
	AutoSync     bool
	SyncInterval time.Duration
	OnUpdate     func(*qbt.TorrentPeersResponse)
	OnError      func(error)
}

// DefaultPeerSyncOptions returns sensible defaults.
func DefaultPeerSyncOptions() PeerSyncOptions {
	return PeerSyncOptions{
		AutoSync:     false,
		SyncInterval: 5 * time.Second,
	}
}

// PeerSyncManager maintains an incrementally-updated local copy of peer data
// for a single torrent by issuing sync/torrentPeers requests.
type PeerSyncManager struct {
	client  *qbt.Client
	mu      sync.RWMutex
	hash    string
	data    *qbt.TorrentPeersResponse
	options PeerSyncOptions
}

// newPeerSyncManager creates a PeerSyncManager for the given torrent hash.
func newPeerSyncManager(client *qbt.Client, hash string, opts PeerSyncOptions) *PeerSyncManager {
	if opts.SyncInterval == 0 {
		opts.SyncInterval = 5 * time.Second
	}
	return &PeerSyncManager{
		client:  client,
		hash:    hash,
		options: opts,
		data: &qbt.TorrentPeersResponse{
			Peers: make(map[string]qbt.TorrentPeer),
		},
	}
}

// Sync fetches the latest peer delta from qBittorrent and merges it.
func (psm *PeerSyncManager) Sync(ctx context.Context) error {
	psm.mu.Lock()
	rid := psm.data.Rid
	psm.mu.Unlock()

	update, err := psm.client.SyncTorrentPeers(ctx, psm.hash, rid)
	if err != nil {
		if psm.options.OnError != nil {
			psm.options.OnError(err)
		}
		return err
	}

	psm.mu.Lock()
	mergePeersResponse(psm.data, update)
	psm.mu.Unlock()

	if psm.options.OnUpdate != nil {
		psm.options.OnUpdate(psm.GetPeers())
	}
	return nil
}

// GetPeers returns a snapshot of the current peer data.
func (psm *PeerSyncManager) GetPeers() *qbt.TorrentPeersResponse {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	peers := make(map[string]qbt.TorrentPeer, len(psm.data.Peers))
	for k, v := range psm.data.Peers {
		peers[k] = v
	}
	removed := make([]string, len(psm.data.PeersRemoved))
	copy(removed, psm.data.PeersRemoved)

	return &qbt.TorrentPeersResponse{
		Peers:        peers,
		PeersRemoved: removed,
		Rid:          psm.data.Rid,
		FullUpdate:   psm.data.FullUpdate,
		ShowFlags:    psm.data.ShowFlags,
	}
}

// GetPeerCount returns the number of currently tracked peers.
func (psm *PeerSyncManager) GetPeerCount() int {
	psm.mu.RLock()
	defer psm.mu.RUnlock()
	return len(psm.data.Peers)
}

// mergePeersResponse applies an incremental or full peer update in-place.
func mergePeersResponse(dst, update *qbt.TorrentPeersResponse) {
	if update == nil {
		return
	}
	if update.FullUpdate {
		dst.Peers = update.Peers
		dst.PeersRemoved = nil
		dst.Rid = update.Rid
		dst.ShowFlags = update.ShowFlags
		return
	}

	if dst.Peers == nil {
		dst.Peers = make(map[string]qbt.TorrentPeer)
	}

	for key, p := range update.Peers {
		if existing, ok := dst.Peers[key]; ok {
			dst.Peers[key] = mergeSinglePeer(existing, p)
		} else {
			dst.Peers[key] = p
		}
	}

	for _, removed := range update.PeersRemoved {
		delete(dst.Peers, removed)
	}

	dst.Rid = update.Rid
	if update.ShowFlags {
		dst.ShowFlags = update.ShowFlags
	}
}

// mergeSinglePeer applies non-nil fields from update onto existing.
func mergeSinglePeer(existing, update qbt.TorrentPeer) qbt.TorrentPeer {
	if update.IP != nil {
		existing.IP = update.IP
	}
	if update.Connection != nil {
		existing.Connection = update.Connection
	}
	if update.Flags != nil {
		existing.Flags = update.Flags
	}
	if update.FlagsDesc != nil {
		existing.FlagsDesc = update.FlagsDesc
	}
	if update.Client != nil {
		existing.Client = update.Client
	}
	if update.Files != nil {
		existing.Files = update.Files
	}
	if update.Country != nil {
		existing.Country = update.Country
	}
	if update.CountryCode != nil {
		existing.CountryCode = update.CountryCode
	}
	if update.PeerIDClient != nil {
		existing.PeerIDClient = update.PeerIDClient
	}
	if update.Port != nil {
		existing.Port = update.Port
	}
	// Speed fields – always update (can legitimately be 0)
	existing.DlSpeed = update.DlSpeed
	existing.UpSpeed = update.UpSpeed
	// Progress – only present in payload when explicitly set
	if update.HasProgress() {
		existing.Progress = update.Progress
	}
	existing.Relevance = update.Relevance
	if update.Downloaded != nil {
		existing.Downloaded = update.Downloaded
	}
	if update.Uploaded != nil {
		existing.Uploaded = update.Uploaded
	}
	return existing
}
