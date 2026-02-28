// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package qbittorrent provides compatibility type definitions and constants that
// existed in github.com/autobrr/go-qbittorrent but were removed or renamed in
// github.com/autogrr/go-qbittorrent.
package qbittorrent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	qbt "github.com/autogrr/go-qbittorrent"
)

// TorrentFiles is a slice of TorrentFile, preserved for backward compatibility.
// The upstream autogrr/go-qbittorrent dropped the named TorrentFiles slice type
// in favour of plain []qbt.TorrentFile.
type TorrentFiles = []qbt.TorrentFile

// PieceState represents the download state of a torrent piece.
// The upstream autogrr/go-qbittorrent returns []int from GetTorrentPieceStates,
// so PieceState is aliased to int for backward compatibility.
type PieceState = int

const (
	// PieceStateNotDownloadYet indicates the piece has not been queued yet.
	PieceStateNotDownloadYet PieceState = 0
	// PieceStateNowDownloading indicates the piece is currently being downloaded.
	PieceStateNowDownloading PieceState = 1
	// PieceStateAlreadyDownloaded indicates the piece has been fully downloaded.
	PieceStateAlreadyDownloaded PieceState = 2
)

// TrackerStatus constants – the upstream module renamed these constants.
// Provide the old names as aliases so existing code keeps compiling.
const (
	TrackerStatusDisabled     = qbt.TrackerDisabled
	TrackerStatusNotContacted = qbt.TrackerNotContacted
	TrackerStatusOK           = qbt.TrackerWorking
	TrackerStatusUpdating     = qbt.TrackerUpdating
	TrackerStatusNotWorking   = qbt.TrackerNotWorking
)

// RSS type aliases – the upstream module uses concrete map/slice types instead
// of named type wrappers.
type (
	// RSSItems is the hierarchical RSS feed/article tree returned by GetRSSItems.
	RSSItems = map[string]any
	// RSSRules is the map of rule-name to rule returned by GetRSSRules.
	RSSRules = map[string]qbt.RSSAutoDownloadRule
	// RSSMatchingArticles is the map of rule-name to matching article titles.
	RSSMatchingArticles = map[string][]string
)

// TorrentCreation compat types – the upstream module simplified the creation
// API: CreateTorrent now returns an int (task ID) and GetTorrentCreationStatus
// takes/returns single-task semantics instead of a slice.
type (
	// TorrentCreationTask is an alias for the new TorrentCreationStatus type.
	TorrentCreationTask = qbt.TorrentCreationStatus
	// TorrentCreationTaskResponse is an alias for TorrentCreationStatus, kept
	// for callers that referenced the old response-wrapper type.
	TorrentCreationTaskResponse = qbt.TorrentCreationStatus
)

// TorrentCreationStatus string constants (the upstream uses *string now).
const (
	TorrentCreationStatusRunning  = "Running"
	TorrentCreationStatusQueued   = "Queued"
	TorrentCreationStatusFinished = "Finished"
	TorrentCreationStatusFailed   = "Failed"
)

// Sentinel errors from the old module that are no longer present in the new
// module. These are mapped to equivalent new errors where possible, or to
// generic error values so existing errors.Is checks continue to compile.
var (
	// ErrTorrentCreationTooManyActiveTasks is returned when the creation queue
	// is full (maps to the generic conflict error from the new module).
	ErrTorrentCreationTooManyActiveTasks = qbt.ErrConflict

	// ErrTorrentCreationTaskNotFound is returned when the requested task ID does
	// not exist (mapped to the not-found error).
	ErrTorrentCreationTaskNotFound = qbt.ErrNotFound

	// ErrTorrentCreationUnfinished is returned when the torrent file is not yet
	// ready (mapped to generic conflict).
	ErrTorrentCreationUnfinished = fmt.Errorf("torrent creation is still in progress: %w", qbt.ErrConflict)

	// ErrTorrentCreationFailed is returned when creation completed with an error.
	ErrTorrentCreationFailed = errors.New("torrent creation failed")

	// ErrUnsupportedVersion is returned when the connected qBittorrent instance
	// is too old to support the requested feature.
	ErrUnsupportedVersion = errors.New("unsupported qBittorrent version")

	// ErrInvalidPriority is returned when an invalid file priority value is used.
	ErrInvalidPriority = fmt.Errorf("invalid file priority: %w", qbt.ErrBadRequest)

	// ErrTorrentMetadataNotDownloadedYet is returned when torrent metadata is
	// not yet available (e.g. magnet links still fetching metadata).
	ErrTorrentMetadataNotDownloadedYet = fmt.Errorf("torrent metadata not yet downloaded: %w", qbt.ErrConflict)
)

// ptrStr safely dereferences a *string, returning "" when nil.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrInt64 safely dereferences a *int64, returning 0 when nil.
func ptrInt64(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// ptrInt safely dereferences a *int, returning 0 when nil.
func ptrInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

// ptrFloat64 safely dereferences a *float64, returning 0 when nil.
func ptrFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// ptrBool safely dereferences a *bool, returning false when nil.
func ptrBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// ptrTorrentState safely dereferences a *qbt.TorrentState, returning "" when nil.
func ptrTorrentState(s *qbt.TorrentState) qbt.TorrentState {
	if s == nil {
		return ""
	}
	return *s
}

// ptrTrackerStatus safely dereferences a *qbt.TrackerStatus, returning 0 when nil.
func ptrTrackerStatus(s *qbt.TrackerStatus) qbt.TrackerStatus {
	if s == nil {
		return 0
	}
	return *s
}

// ptrUint64 safely dereferences a *uint64, returning 0 when nil.
func ptrUint64(n *uint64) uint64 {
	if n == nil {
		return 0
	}
	return *n
}

// mapToTorrentAddOptions converts a legacy map[string]string torrent add options
// (used by the old autobrr/go-qbittorrent API) into the new qbt.TorrentAddOptions struct.
// Keys recognised: savepath, category, tags, rename, contentLayout, paused, stopped,
// skip_checking, sequentialDownload, firstLastPiecePrio, upLimit, dlLimit,
// ratioLimit, seedingTimeLimit.
func mapToTorrentAddOptions(m map[string]string) qbt.TorrentAddOptions {
	if len(m) == 0 {
		return qbt.TorrentAddOptions{}
	}

	opts := qbt.TorrentAddOptions{}

	if v, ok := m["savepath"]; ok {
		opts.SavePath = v
	}
	if v, ok := m["category"]; ok {
		opts.Category = v
	}
	if v, ok := m["tags"]; ok && v != "" {
		// Old API joined tags with commas; split them back out.
		for _, tag := range splitComma(v) {
			if tag != "" {
				opts.Tags = append(opts.Tags, tag)
			}
		}
	}
	if v, ok := m["rename"]; ok {
		opts.Rename = v
	}
	if v, ok := m["contentLayout"]; ok {
		opts.ContentLayout = qbt.ContentLayout(v)
	}
	if m["paused"] == "true" || m["stopped"] == "true" {
		opts.Stopped = true
	}
	if m["skip_checking"] == "true" {
		opts.SkipHashCheck = true
	}
	if m["sequentialDownload"] == "true" {
		opts.SequentialDownload = true
	}
	if m["firstLastPiecePrio"] == "true" {
		opts.FirstLastPiecePrio = true
	}
	if v, ok := m["upLimit"]; ok && v != "" {
		if n, err := parseint64(v); err == nil {
			opts.UploadLimit = n
		}
	}
	if v, ok := m["dlLimit"]; ok && v != "" {
		if n, err := parseint64(v); err == nil {
			opts.DownloadLimit = n
		}
	}
	if v, ok := m["ratioLimit"]; ok && v != "" {
		if f, err := parsefloat64(v); err == nil {
			opts.RatioLimit = f
		}
	}
	if v, ok := m["seedingTimeLimit"]; ok && v != "" {
		if n, err := parseint64(v); err == nil {
			opts.SeedingTimeLimit = n
		}
	}

	return opts
}

func splitComma(s string) []string { return strings.Split(s, ",") }

// splitLines splits s on newlines, trims whitespace, and discards empty entries.
func splitLines(s string) []string {
	parts := strings.Split(s, "\n")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseint64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}
func parsefloat64(s string) (float64, error) { return strconv.ParseFloat(strings.TrimSpace(s), 64) }
