// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package pages

import (
	"strings"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/layouts"
)

// ──────────────────────────────────────────────────────────────────────────────
// DirScan types
// ──────────────────────────────────────────────────────────────────────────────

// DirScanDirectoryItem is the page-level view of a scan directory.
type DirScanDirectoryItem struct {
	ID                  int
	Path                string
	QbitPathPrefix      string
	Category            string
	Tags                string // comma-joined
	Enabled             bool
	ArrInstanceID       int // 0 = none
	TargetInstanceID    int
	TargetInstanceName  string
	ScanIntervalMinutes int
	LastScanAt          string
}

// DirScanSettings is the page-level view of global dir scan settings.
type DirScanSettings struct {
	Enabled                      bool
	MatchMode                    string
	SizeTolerancePercent         float64
	MinPieceRatio                float64
	MaxSearcheesPerRun           int
	MaxSearcheeAgeDays           int
	AllowPartial                 bool
	SkipPieceBoundarySafetyCheck bool
	StartPaused                  bool
	Category                     string
	Tags                         string // comma-joined
}

// DirScanRun is the page-level view of a dir scan run.
type DirScanRun struct {
	ID            int64
	DirectoryID   int
	Status        string
	TriggeredBy   string
	FilesFound    int
	FilesSkipped  int
	MatchesFound  int
	TorrentsAdded int
	ErrorMessage  string
	StartedAt     string
	CompletedAt   string
	IsActive      bool
}

// DirScanProps holds all data for the full Dir Scan page.
type DirScanProps struct {
	BaseURL   string
	Username  string
	Version   string
	Instances []layouts.Instance

	// Active tab: "directories" | "settings"
	ActiveTab string

	// Global settings
	Settings DirScanSettings

	// Directories list
	Directories []DirScanDirectoryItem

	// Current form (nil = new, non-nil = edit)
	DirectoryForm *DirScanDirectoryItem

	// Recent runs (for a specific directory)
	Runs         []DirScanRun
	RunsForDirID int

	// Error message
	Error string
}

// DirScanDirectoryItemFromModel converts a model to the page view struct.
func DirScanDirectoryItemFromModel(d *models.DirScanDirectory, instances []layouts.Instance) DirScanDirectoryItem {
	if d == nil {
		return DirScanDirectoryItem{}
	}
	arrID := 0
	if d.ArrInstanceID != nil {
		arrID = *d.ArrInstanceID
	}
	lastScan := ""
	if d.LastScanAt != nil {
		lastScan = d.LastScanAt.Format("2006-01-02 15:04")
	}
	targetName := ""
	for _, inst := range instances {
		if inst.ID == d.TargetInstanceID {
			targetName = inst.Name
			break
		}
	}
	return DirScanDirectoryItem{
		ID:                  d.ID,
		Path:                d.Path,
		QbitPathPrefix:      d.QbitPathPrefix,
		Category:            d.Category,
		Tags:                strings.Join(d.Tags, ", "),
		Enabled:             d.Enabled,
		ArrInstanceID:       arrID,
		TargetInstanceID:    d.TargetInstanceID,
		TargetInstanceName:  targetName,
		ScanIntervalMinutes: d.ScanIntervalMinutes,
		LastScanAt:          lastScan,
	}
}

// DirScanSettingsFromModel converts a model to the page view struct.
func DirScanSettingsFromModel(s *models.DirScanSettings) DirScanSettings {
	if s == nil {
		return DirScanSettings{
			MatchMode:            string(models.MatchModeStrict),
			SizeTolerancePercent: 5.0,
			MinPieceRatio:        98.0,
		}
	}
	return DirScanSettings{
		Enabled:                      s.Enabled,
		MatchMode:                    string(s.MatchMode),
		SizeTolerancePercent:         s.SizeTolerancePercent,
		MinPieceRatio:                s.MinPieceRatio,
		MaxSearcheesPerRun:           s.MaxSearcheesPerRun,
		MaxSearcheeAgeDays:           s.MaxSearcheeAgeDays,
		AllowPartial:                 s.AllowPartial,
		SkipPieceBoundarySafetyCheck: s.SkipPieceBoundarySafetyCheck,
		StartPaused:                  s.StartPaused,
		Category:                     s.Category,
		Tags:                         strings.Join(s.Tags, ", "),
	}
}

// DirScanRunFromModel converts a store DirScanRun to the page view struct.
func DirScanRunFromModel(r *models.DirScanRun) DirScanRun {
	if r == nil {
		return DirScanRun{}
	}
	completedAt := ""
	if r.CompletedAt != nil {
		completedAt = r.CompletedAt.Format("2006-01-02 15:04:05")
	}
	status := string(r.Status)
	active := status == "queued" || status == "scanning" || status == "searching" || status == "injecting"
	return DirScanRun{
		ID:            r.ID,
		DirectoryID:   r.DirectoryID,
		Status:        status,
		TriggeredBy:   r.TriggeredBy,
		FilesFound:    r.FilesFound,
		FilesSkipped:  r.FilesSkipped,
		MatchesFound:  r.MatchesFound,
		TorrentsAdded: r.TorrentsAdded,
		ErrorMessage:  r.ErrorMessage,
		StartedAt:     r.StartedAt.Format("2006-01-02 15:04:05"),
		CompletedAt:   completedAt,
		IsActive:      active,
	}
}
