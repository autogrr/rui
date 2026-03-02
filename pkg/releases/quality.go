// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package releases

import (
	"sort"
	"strings"

	"github.com/moistari/rls"
)

// QualityInfo holds technical quality metadata extracted from a parsed rls.Release.
// It covers the fields most relevant for grouping and display: resolution,
// source (origin medium), video codec, and HDR format.
type QualityInfo struct {
	Resolution string // canonical: "4K", "1080p", "720p", "SD", or original uppercase
	Source     string // canonical: "WEBDL", "WEBRIP", "BLURAY", "HDTV", etc.
	VideoCodec string // canonical: "HEVC", "AVC", "AV1", or joined multi-codec string
	HDR        string // joined sorted: "DV", "HDR10", "DV HDR10", etc.
}

// ExtractQuality returns structured quality metadata from a parsed rls.Release.
func ExtractQuality(r *rls.Release) QualityInfo {
	if r == nil {
		return QualityInfo{}
	}
	return QualityInfo{
		Resolution: NormalizeResolution(r.Resolution),
		Source:     NormalizeSource(r.Source),
		VideoCodec: JoinNormalizedCodecSlice(r.Codec),
		HDR:        JoinNormalizedSlice(r.HDR),
	}
}

// Label returns a compact space-separated display string for this QualityInfo.
// e.g. "1080p WEBDL HEVC" or "4K BLURAY HEVC DV HDR10".
// Empty components are omitted; returns "" when all components are empty.
func (q QualityInfo) Label() string {
	parts := make([]string, 0, 4)
	if q.Resolution != "" {
		parts = append(parts, q.Resolution)
	}
	if q.Source != "" {
		parts = append(parts, q.Source)
	}
	if q.VideoCodec != "" {
		parts = append(parts, q.VideoCodec)
	}
	if q.HDR != "" {
		parts = append(parts, q.HDR)
	}
	return strings.Join(parts, " ")
}

// NormalizeResolution converts a resolution string to a canonical upper-case form.
// "2160p"/"UHD"/"4K" → "4K"; "1080p" → "1080p"; "720p" → "720p"; SD variants → "SD".
// Unrecognised values are returned as-is, uppercased and trimmed.
func NormalizeResolution(res string) string {
	switch strings.ToUpper(strings.TrimSpace(res)) {
	case "2160P", "UHD", "4K":
		return "4K"
	case "1080P":
		return "1080p"
	case "720P":
		return "720p"
	case "480P", "576P", "SD":
		return "SD"
	case "":
		return ""
	default:
		return strings.ToUpper(strings.TrimSpace(res))
	}
}

// JoinQualityLabels deduplicates a slice of quality label strings (from
// QualityInfo.Label), sorts them deterministically, and joins them with ", ".
// Empty strings are ignored. Returns "" when the resulting set is empty.
func JoinQualityLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(labels))
	unique := make([]string, 0, len(labels))
	for _, l := range labels {
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		unique = append(unique, l)
	}
	if len(unique) == 0 {
		return ""
	}
	sort.Strings(unique)
	return strings.Join(unique, ", ")
}
