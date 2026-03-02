// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package releases

import (
	"sort"
	"strings"
)

// videoCodecAliases maps equivalent video codec names to a canonical form.
// x264, H.264, H264, and AVC all refer to the same underlying codec (AVC/H.264).
// x265, H.265, H265, and HEVC all refer to the same underlying codec (HEVC/H.265).
var videoCodecAliases = map[string]string{
	"X264":  "AVC",
	"H.264": "AVC",
	"H264":  "AVC",
	"AVC":   "AVC",
	"X265":  "HEVC",
	"H.265": "HEVC",
	"H265":  "HEVC",
	"HEVC":  "HEVC",
}

// NormalizeVideoCodec converts a video codec string to its canonical form.
// Returns the original (uppercased) string if no alias mapping exists.
func NormalizeVideoCodec(codec string) string {
	upper := strings.ToUpper(strings.TrimSpace(codec))
	if canonical, ok := videoCodecAliases[upper]; ok {
		return canonical
	}
	return upper
}

// JoinNormalizedCodecSlice converts a codec slice to a normalized string for comparison.
// Applies codec aliasing so that x264, H.264, H264, and AVC are treated as equivalent.
func JoinNormalizedCodecSlice(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(slice))
	normalized := make([]string, 0, len(slice))
	for _, codec := range slice {
		n := NormalizeVideoCodec(codec)
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		normalized = append(normalized, n)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, " ")
}

// sourceAliases maps source names to a canonical form for comparison.
// WEB-DL variants normalize to WEBDL, WEBRip variants to WEBRIP.
// Plain "WEB" stays as "WEB" and is treated as ambiguous (matches both).
var sourceAliases = map[string]string{
	"WEB-DL":  "WEBDL",
	"WEBDL":   "WEBDL",
	"WEB-RIP": "WEBRIP",
	"WEBRIP":  "WEBRIP",
	"WEB":     "WEB",
}

// NormalizeSource converts a source string to its canonical form.
// Returns the original (uppercased) string if no alias mapping exists.
func NormalizeSource(source string) string {
	upper := strings.ToUpper(strings.TrimSpace(source))
	if canonical, ok := sourceAliases[upper]; ok {
		return canonical
	}
	return upper
}

// JoinNormalizedSlice converts a string slice to an uppercase, sorted,
// space-joined string. Useful for HDR tags, audio tracks, language lists, etc.
func JoinNormalizedSlice(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	normalized := make([]string, len(slice))
	for i, s := range slice {
		normalized[i] = strings.ToUpper(strings.TrimSpace(s))
	}
	sort.Strings(normalized)
	return strings.Join(normalized, " ")
}

// SourcesCompatible reports whether two normalised source strings are compatible
// for cross-seed matching. Plain "WEB" is ambiguous and matches both "WEBDL" and
// "WEBRIP"; "WEBDL" and "WEBRIP" do not match each other. Empty strings are
// treated as always compatible (unknown source cannot be ruled out).
func SourcesCompatible(source, candidate string) bool {
	if source == "" || candidate == "" {
		return true
	}
	if source == candidate {
		return true
	}
	isWebSource := func(s string) bool {
		switch s {
		case "WEB", "WEBDL", "WEBRIP":
			return true
		default:
			return false
		}
	}
	if !isWebSource(source) || !isWebSource(candidate) {
		return false
	}
	// Both are web sources but different — only allow if one side is the
	// ambiguous "WEB" (which can be either WEB-DL or WEBRip).
	return source == "WEB" || candidate == "WEB"
}
