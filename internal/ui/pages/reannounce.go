// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package pages

import (
	"strings"

	"github.com/autogrr/rui/internal/models"
)

// ──────────────────────────────────────────────────────────────────────────────
// Reannounce types
// ──────────────────────────────────────────────────────────────────────────────

// ReannounceSettings is the page-level view of InstanceReannounceSettings.
type ReannounceSettings struct {
	Enabled                   bool
	Aggressive                bool
	MonitorAll                bool
	InitialWaitSeconds        int
	ReannounceIntervalSeconds int
	MaxAgeSeconds             int
	MaxRetries                int
	ExcludeCategories         bool
	Categories                string // newline-joined list
	ExcludeTags               bool
	Tags                      string
	ExcludeTrackers           bool
	Trackers                  string
}

// ReannounceSettingsProps carries all data for the reannounce settings card.
type ReannounceSettingsProps struct {
	BaseURL    string
	InstanceID int
	Settings   ReannounceSettings
	Success    bool
	Error      string
}

// ReannounceSettingsFromModel converts a model to the page view struct.
func ReannounceSettingsFromModel(s *models.InstanceReannounceSettings) ReannounceSettings {
	if s == nil {
		return ReannounceSettings{}
	}
	return ReannounceSettings{
		Enabled:                   s.Enabled,
		Aggressive:                s.Aggressive,
		MonitorAll:                s.MonitorAll,
		InitialWaitSeconds:        s.InitialWaitSeconds,
		ReannounceIntervalSeconds: s.ReannounceIntervalSeconds,
		MaxAgeSeconds:             s.MaxAgeSeconds,
		MaxRetries:                s.MaxRetries,
		ExcludeCategories:         s.ExcludeCategories,
		Categories:                strings.Join(s.Categories, "\n"),
		ExcludeTags:               s.ExcludeTags,
		Tags:                      strings.Join(s.Tags, "\n"),
		ExcludeTrackers:           s.ExcludeTrackers,
		Trackers:                  strings.Join(s.Trackers, "\n"),
	}
}

// ReannounceActivityEvent is the page-level view of a single reannounce event.
type ReannounceActivityEvent struct {
	Hash        string
	TorrentName string
	Trackers    string
	Outcome     string // "skipped" | "failed" | "succeeded"
	Reason      string
	Timestamp   string
}

// ReannounceActivityProps carries all data for the reannounce activity partial.
type ReannounceActivityProps struct {
	BaseURL    string
	InstanceID int
	Events     []ReannounceActivityEvent
}

// ──────────────────────────────────────────────────────────────────────────────
// Orphan Scan types
// ──────────────────────────────────────────────────────────────────────────────

// OrphanScanSettings is the page-level view of OrphanScanSettings.
type OrphanScanSettings struct {
	Enabled             bool
	AutoCleanupEnabled  bool
	GracePeriodMinutes  int
	ScanIntervalHours   int
	MaxFilesPerRun      int
	AutoCleanupMaxFiles int
	IgnorePaths         string // newline-joined
}

// OrphanScanRun is the page-level view of a single orphan scan run.
type OrphanScanRun struct {
	ID             int64
	Status         string
	TriggeredBy    string
	FilesFound     int
	FilesDeleted   int
	FoldersDeleted int
	BytesReclaimed int64
	Truncated      bool
	ErrorMessage   string
	StartedAt      string
	CompletedAt    string
	IsActive       bool
}

// OrphanScanProps carries all data for orphan scan cards.
type OrphanScanProps struct {
	BaseURL    string
	InstanceID int
	Settings   OrphanScanSettings
	Runs       []OrphanScanRun
	Success    bool
	Error      string
}

// OrphanScanSettingsFromModel converts a model to the page view struct.
func OrphanScanSettingsFromModel(s *models.OrphanScanSettings) OrphanScanSettings {
	if s == nil {
		return OrphanScanSettings{}
	}
	return OrphanScanSettings{
		Enabled:             s.Enabled,
		AutoCleanupEnabled:  s.AutoCleanupEnabled,
		GracePeriodMinutes:  s.GracePeriodMinutes,
		ScanIntervalHours:   s.ScanIntervalHours,
		MaxFilesPerRun:      s.MaxFilesPerRun,
		AutoCleanupMaxFiles: s.AutoCleanupMaxFiles,
		IgnorePaths:         strings.Join(s.IgnorePaths, "\n"),
	}
}

// OrphanScanRunFromModel converts a store OrphanScanRun to the page view struct.
func OrphanScanRunFromModel(r *models.OrphanScanRun) OrphanScanRun {
	if r == nil {
		return OrphanScanRun{}
	}
	completedAt := ""
	if r.CompletedAt != nil {
		completedAt = r.CompletedAt.Format("2006-01-02 15:04:05")
	}
	active := r.Status == "scanning" || r.Status == "queued" || r.Status == "deleting"
	return OrphanScanRun{
		ID:             r.ID,
		Status:         r.Status,
		TriggeredBy:    r.TriggeredBy,
		FilesFound:     r.FilesFound,
		FilesDeleted:   r.FilesDeleted,
		FoldersDeleted: r.FoldersDeleted,
		BytesReclaimed: r.BytesReclaimed,
		Truncated:      r.Truncated,
		ErrorMessage:   r.ErrorMessage,
		StartedAt:      r.StartedAt.Format("2006-01-02 15:04:05"),
		CompletedAt:    completedAt,
		IsActive:       active,
	}
}
