// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package automations

import (
	"math"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/pkg/releases"
)

const maxConditionDepth = 20

// minContainsNameLength is the minimum name length for CONTAINS_IN matching
// to avoid surprising matches on short names.
const minContainsNameLength = 10

// categoryEntry stores torrent info for category-based lookups.
type categoryEntry struct {
	Hash           string // torrent hash for self-exclusion
	Name           string // lowercased name (for EXISTS_IN exact match)
	NormalizedName string // normalized name for CONTAINS_IN (separators → space)
}

// FreeSpaceSourceState tracks free space projection state for a single source.
// Each source (qBittorrent or path) has its own state to correctly handle
// workflows that target different disks.
type FreeSpaceSourceState struct {
	// FreeSpace is the base free space in bytes from this source.
	FreeSpace int64
	// SpaceToClear is the cumulative disk space that will be freed by deletions.
	SpaceToClear int64
	// FilesToClear tracks cross-seed keys already counted to avoid double-counting.
	FilesToClear map[crossSeedKey]struct{}
	// HardlinkSignaturesToClear tracks hardlink signatures already counted.
	HardlinkSignaturesToClear map[string]struct{}
}

// EvalContext provides additional context for condition evaluation.
type EvalContext struct {
	// UnregisteredSet contains hashes of unregistered torrents (from SyncManager health counts)
	UnregisteredSet map[string]struct{}
	// TrackerDownSet contains hashes of torrents whose trackers are down (from SyncManager health counts)
	TrackerDownSet map[string]struct{}
	// HardlinkScopeByHash maps torrent hash to its hardlink scope (none, torrents_only, outside_qbittorrent)
	HardlinkScopeByHash map[string]string
	// HasMissingFilesByHash maps torrent hash to whether or not it has missing files on disk
	HasMissingFilesByHash map[string]bool
	// InstanceHasLocalAccess indicates whether the instance has local filesystem access
	InstanceHasLocalAccess bool
	// FreeSpace is the free space on the instance's filesystem (current active source)
	FreeSpace int64
	// SpaceToClear is the amount of disk space that will be cleared by the "free space" condition (current active source)
	SpaceToClear int64
	// FilesToClear is a map of cross-seed keys to the amount of disk space that will be cleared by the "free space" condition, ensuring we don't double count cross-seeds (current active source)
	FilesToClear map[crossSeedKey]struct{}
	// HardlinkSignatureByHash maps torrent hash to its hardlink signature (sorted file IDs joined with ";").
	// Only populated when includeHardlinks is enabled for FREE_SPACE rules.
	HardlinkSignatureByHash map[string]string
	// HardlinkSignaturesToClear tracks hardlink signatures already counted in space projection (current active source).
	// Torrents with the same signature share physical files and should only be counted once.
	HardlinkSignaturesToClear map[string]struct{}
	// FreeSpaceStates maps rule keys to their projection state.
	// Rule keys are "sourceKey|rule:<id>" where sourceKey is "qbt" or "path:/some/path".
	// Each rule gets its own state to prevent interference between rules with different thresholds.
	FreeSpaceStates map[string]*FreeSpaceSourceState
	// ActiveFreeSpaceSource is the rule key currently loaded into the top-level fields.
	ActiveFreeSpaceSource string

	// CategoryIndex maps lowercased category → lowercased name → set of hashes.
	// Enables O(1) EXISTS_IN lookups while supporting self-exclusion.
	CategoryIndex map[string]map[string]map[string]struct{}

	// CategoryNames maps lowercased category → slice of categoryEntry.
	// Used for CONTAINS_IN iteration (stores pre-normalized names).
	CategoryNames map[string][]categoryEntry

	// NowUnix is the current Unix timestamp, used for age field evaluation.
	// If zero, time.Now().Unix() is used. Set this for deterministic tests.
	NowUnix int64

	// TrackerDisplayNameByDomain maps lowercase tracker domains to their display names.
	// Used for UseTrackerAsTag with UseDisplayName option.
	TrackerDisplayNameByDomain map[string]string

	// ReleaseParser caches parsed release metadata for RLS-derived fields.
	ReleaseParser *releases.Parser

	// ActiveRuleID is used for rule-scoped evaluation helpers (grouping, free space sources, etc).
	// It is set by the automations processor before evaluating a rule.
	ActiveRuleID int

	// activeGroupIndex is the currently active (rule-scoped) grouping index.
	activeGroupIndex *groupIndex

	// groupIndexCache caches group indices per rule ID + group ID to avoid rebuilding.
	groupIndexCache map[int]map[string]*groupIndex
	// groupConditionUsageByRule caches which grouping IDs are referenced by grouped condition fields.
	groupConditionUsageByRule map[int]groupingConditionUsage
}

// separatorReplacer replaces common torrent name separators with spaces.
var separatorReplacer = strings.NewReplacer(".", " ", "_", " ", "-", " ")

// whitespaceCollapser collapses multiple spaces into one.
var whitespaceCollapser = regexp.MustCompile(`\s+`)

// normalizeName normalizes a torrent name for CONTAINS_IN comparison:
// lowercase + replace . _ - with space + collapse whitespace.
func normalizeName(s string) string {
	s = normalizeLower(s)
	s = separatorReplacer.Replace(s)
	s = whitespaceCollapser.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// BuildCategoryIndex builds the category lookup structures from a list of torrents.
// Returns both the CategoryIndex (for O(1) EXISTS_IN) and CategoryNames (for CONTAINS_IN iteration).
func BuildCategoryIndex(torrents []qbt.Torrent) (map[string]map[string]map[string]struct{}, map[string][]categoryEntry) {
	categoryIndex := make(map[string]map[string]map[string]struct{})
	categoryNames := make(map[string][]categoryEntry)

	for _, t := range torrents {
		// Use lowercased + trimmed category as key (empty string is valid for uncategorized)
		catKey := normalizeLowerTrim(qbt.Deref(t.Category))
		nameLower := normalizeLower(qbt.Deref(t.Name))

		// Build CategoryIndex for O(1) EXISTS_IN lookup
		if categoryIndex[catKey] == nil {
			categoryIndex[catKey] = make(map[string]map[string]struct{})
		}
		if categoryIndex[catKey][nameLower] == nil {
			categoryIndex[catKey][nameLower] = make(map[string]struct{})
		}
		categoryIndex[catKey][nameLower][qbt.Deref(t.Hash)] = struct{}{}

		// Build CategoryNames for CONTAINS_IN iteration
		categoryNames[catKey] = append(categoryNames[catKey], categoryEntry{
			Hash:           qbt.Deref(t.Hash),
			Name:           nameLower,
			NormalizedName: normalizeName(qbt.Deref(t.Name)),
		})
	}

	return categoryIndex, categoryNames
}

// existsInCategory checks if a different torrent with the exact same name exists in the target category.
func existsInCategory(torrentHash, torrentName, targetCategory string, ctx *EvalContext) bool {
	if ctx == nil || ctx.CategoryIndex == nil {
		return false
	}

	// Normalize inputs
	catKey := normalizeLowerTrim(targetCategory)
	// Treat all-whitespace as "no match" (but empty string is valid for uncategorized)
	if targetCategory != "" && catKey == "" {
		return false
	}
	nameLower := normalizeLower(torrentName)

	// Lookup category
	nameMap, ok := ctx.CategoryIndex[catKey]
	if !ok {
		return false
	}

	// Lookup name
	hashSet, ok := nameMap[nameLower]
	if !ok {
		return false
	}

	// Check if any hash in the set is different from the current torrent
	for hash := range hashSet {
		if hash != torrentHash {
			return true
		}
	}
	return false
}

// containsInCategory checks if a different torrent with a similar name exists in the target category.
// Uses bidirectional contains matching with normalization.
func containsInCategory(torrentHash, torrentName, targetCategory string, ctx *EvalContext) bool {
	if ctx == nil || ctx.CategoryNames == nil {
		return false
	}

	// Normalize inputs
	catKey := normalizeLowerTrim(targetCategory)
	// Treat all-whitespace as "no match" (but empty string is valid for uncategorized)
	if targetCategory != "" && catKey == "" {
		return false
	}

	// Skip if current torrent name is too short
	normalizedCurrent := normalizeName(torrentName)
	if len(normalizedCurrent) < minContainsNameLength {
		return false
	}

	// Lookup category entries
	entries, ok := ctx.CategoryNames[catKey]
	if !ok {
		return false
	}

	// Check each entry for bidirectional contains match
	for _, entry := range entries {
		// Skip self
		if entry.Hash == torrentHash {
			continue
		}
		// Skip entries with short normalized names
		if len(entry.NormalizedName) < minContainsNameLength {
			continue
		}
		// Bidirectional contains: either contains the other
		if strings.Contains(normalizedCurrent, entry.NormalizedName) ||
			strings.Contains(entry.NormalizedName, normalizedCurrent) {
			return true
		}
	}
	return false
}

// ConditionUsesField checks if a condition tree references a specific field.
func ConditionUsesField(cond *RuleCondition, field ConditionField) bool {
	if cond == nil {
		return false
	}
	if cond.Field == field {
		return true
	}
	for _, child := range cond.Conditions {
		if ConditionUsesField(child, field) {
			return true
		}
	}
	return false
}

// EvaluateCondition recursively evaluates a condition against a torrent.
// Returns true if the torrent matches the condition.
// For conditions that require additional context (like isUnregistered), use EvaluateConditionWithContext.
func EvaluateCondition(cond *RuleCondition, torrent qbt.Torrent, depth int) bool {
	return EvaluateConditionWithContext(cond, torrent, nil, depth)
}

// EvaluateConditionWithContext recursively evaluates a condition against a torrent with optional context.
// Returns true if the torrent matches the condition.
func EvaluateConditionWithContext(cond *RuleCondition, torrent qbt.Torrent, ctx *EvalContext, depth int) bool {
	if cond == nil || depth > maxConditionDepth {
		return false
	}

	// Compile regex if needed, but skip for EXISTS_IN/CONTAINS_IN operators
	// (cond.Value is a category name, not a pattern)
	if cond.Operator != OperatorExistsIn && cond.Operator != OperatorContainsIn {
		if cond.Regex || cond.Operator == OperatorMatches {
			if err := cond.CompileRegex(); err != nil {
				log.Debug().
					Err(err).
					Str("field", string(cond.Field)).
					Str("pattern", cond.Value).
					Msg("automations: regex compilation failed")
				return false
			}
		}
	}

	var result bool

	// Handle logical operators (AND/OR) with child conditions
	if cond.IsGroup() {
		switch cond.Operator {
		case OperatorOr:
			// OR: at least one child must match
			result = false
			for _, child := range cond.Conditions {
				if EvaluateConditionWithContext(child, torrent, ctx, depth+1) {
					result = true
					break
				}
			}
		case OperatorAnd:
			// AND: all children must match
			result = true
			for _, child := range cond.Conditions {
				if !EvaluateConditionWithContext(child, torrent, ctx, depth+1) {
					result = false
					break
				}
			}
		}
	} else {
		// Leaf condition: evaluate against the torrent
		result = evaluateLeaf(cond, torrent, ctx)
	}

	// Apply negation if specified
	if cond.Negate {
		result = !result
	}

	return result
}

// evaluateLeaf evaluates a leaf condition (not a group) against a torrent.
func evaluateLeaf(cond *RuleCondition, torrent qbt.Torrent, ctx *EvalContext) bool {
	switch cond.Field {
	// String fields
	case FieldName:
		// EXISTS_IN/CONTAINS_IN are special operators for cross-category lookups
		if cond.Operator == OperatorExistsIn {
			return existsInCategory(qbt.Deref(torrent.Hash), qbt.Deref(torrent.Name), cond.Value, ctx)
		}
		if cond.Operator == OperatorContainsIn {
			return containsInCategory(qbt.Deref(torrent.Hash), qbt.Deref(torrent.Name), cond.Value, ctx)
		}
		return compareString(qbt.Deref(torrent.Name), cond)
	case FieldHash:
		return compareString(qbt.Deref(torrent.Hash), cond)
	case FieldInfohashV1:
		return compareString(qbt.Deref(torrent.InfoHashV1), cond)
	case FieldInfohashV2:
		return compareString(qbt.Deref(torrent.InfoHashV2), cond)
	case FieldMagnetURI:
		return compareString(qbt.Deref(torrent.MagnetURI), cond)
	case FieldCategory:
		return compareString(qbt.Deref(torrent.Category), cond)
	case FieldTags:
		return compareTags(qbt.Deref(torrent.Tags), cond)
	case FieldSavePath:
		return compareString(qbt.Deref(torrent.SavePath), cond)
	case FieldContentPath:
		return compareString(qbt.Deref(torrent.ContentPath), cond)
	case FieldDownloadPath:
		return compareString(qbt.Deref(torrent.DownloadPath), cond)
	case FieldCreatedBy:
		return compareString("", cond)
	case FieldContentType:
		return compareString(torrentContentType(torrent, ctx), cond)
	case FieldEffectiveName:
		return compareString(torrentEffectiveName(torrent, ctx), cond)
	case FieldRlsSource:
		return compareString(torrentRlsSource(torrent, ctx), cond)
	case FieldRlsResolution:
		return compareString(torrentRlsResolution(torrent, ctx), cond)
	case FieldRlsCodec:
		return compareString(torrentRlsCodec(torrent, ctx), cond)
	case FieldRlsHDR:
		return compareString(torrentRlsHDR(torrent, ctx), cond)
	case FieldRlsAudio:
		return compareString(torrentRlsAudio(torrent, ctx), cond)
	case FieldRlsChannels:
		return compareString(torrentRlsChannels(torrent, ctx), cond)
	case FieldRlsGroup:
		return compareString(torrentRlsGroup(torrent, ctx), cond)
	case FieldState:
		return compareState(torrent, cond, ctx)
	case FieldTracker:
		return compareTracker(qbt.Deref(torrent.Tracker), cond, ctx)
	case FieldTrackers:
		return compareTrackers(torrent, cond, ctx)
	case FieldComment:
		return compareString("", cond)

	// Bytes fields (int64)
	case FieldSize:
		return compareInt64(qbt.Deref(torrent.Size), cond)
	case FieldTotalSize:
		return compareInt64(qbt.Deref(torrent.TotalSize), cond)
	case FieldCompleted:
		return compareInt64(qbt.Deref(torrent.Completed), cond)
	case FieldDownloaded:
		return compareInt64(qbt.Deref(torrent.Downloaded), cond)
	case FieldDownloadedSession:
		return compareInt64(qbt.Deref(torrent.DownloadedSession), cond)
	case FieldUploaded:
		return compareInt64(qbt.Deref(torrent.Uploaded), cond)
	case FieldUploadedSession:
		return compareInt64(qbt.Deref(torrent.UploadedSession), cond)
	case FieldAmountLeft:
		return compareInt64(qbt.Deref(torrent.AmountLeft), cond)
	case FieldFreeSpace:
		if ctx == nil {
			return false
		}
		return compareInt64(ctx.FreeSpace+ctx.SpaceToClear, cond)

	// Time fields from qBittorrent:
	// - Unix timestamps are evaluated as age durations (seconds since event).
	// - Native seconds fields are evaluated directly.
	case FieldAddedOn:
		return compareAgeIfSet(qbt.Deref(torrent.AddedOn), cond, ctx)
	case FieldCompletionOn:
		return compareAgeIfSet(qbt.Deref(torrent.CompletionOn), cond, ctx)
	case FieldLastActivity:
		return compareAgeIfSet(qbt.Deref(torrent.LastActivity), cond, ctx)
	case FieldSeenComplete:
		return compareAgeIfSet(qbt.Deref(torrent.SeenComplete), cond, ctx)
	case FieldETA:
		return compareInt64(qbt.Deref(torrent.ETA), cond)
	case FieldReannounce:
		return compareInt64(qbt.Deref(torrent.Reannounce), cond)
	case FieldSeedingTime:
		return compareInt64(qbt.Deref(torrent.SeedingTime), cond)
	case FieldTimeActive:
		return compareInt64(qbt.Deref(torrent.TimeActive), cond)
	case FieldMaxSeedingTime:
		return compareInt64(qbt.Deref(torrent.MaxSeedingTime), cond)
	case FieldMaxInactiveSeedingTime:
		return compareInt64(qbt.Deref(torrent.InactiveSeedingTimeLimit), cond)
	case FieldSeedingTimeLimit:
		return compareInt64(qbt.Deref(torrent.SeedingTimeLimit), cond)
	case FieldInactiveSeedingTimeLimit:
		return compareInt64(qbt.Deref(torrent.InactiveSeedingTimeLimit), cond)

	// Age fields (time since timestamp). Kept as compatibility aliases.
	case FieldAddedOnAge:
		return compareAgeIfSet(qbt.Deref(torrent.AddedOn), cond, ctx)
	case FieldCompletionOnAge:
		return compareAgeIfSet(qbt.Deref(torrent.CompletionOn), cond, ctx)
	case FieldLastActivityAge:
		return compareAgeIfSet(qbt.Deref(torrent.LastActivity), cond, ctx)

	// Float64 fields
	case FieldRatio:
		return compareFloat64(qbt.Deref(torrent.Ratio), cond)
	case FieldRatioLimit:
		return compareFloat64(qbt.Deref(torrent.RatioLimit), cond)
	case FieldMaxRatio:
		return compareFloat64(qbt.Deref(torrent.MaxRatio), cond)
	case FieldProgress:
		return compareFloat64(qbt.Deref(torrent.Progress), normalizeProgressCondition(cond))
	case FieldAvailability:
		return compareFloat64(qbt.Deref(torrent.Availability), cond)
	case FieldPopularity:
		return compareFloat64(float64(0), cond)

	// Speed fields (int64)
	case FieldDlSpeed:
		return compareInt64(qbt.Deref(torrent.DlSpeed), cond)
	case FieldUpSpeed:
		return compareInt64(qbt.Deref(torrent.UpSpeed), cond)
	case FieldDlLimit:
		return compareInt64(qbt.Deref(torrent.DlLimit), cond)
	case FieldUpLimit:
		return compareInt64(qbt.Deref(torrent.UpLimit), cond)

	// Count fields (int64)
	case FieldNumSeeds:
		return compareInt64(int64(qbt.Deref(torrent.NumSeeds)), cond)
	case FieldNumLeechs:
		return compareInt64(int64(qbt.Deref(torrent.NumLeechs)), cond)
	case FieldNumComplete:
		return compareInt64(int64(qbt.Deref(torrent.NumComplete)), cond)
	case FieldNumIncomplete:
		return compareInt64(int64(qbt.Deref(torrent.NumIncomplete)), cond)
	case FieldTrackersCount:
		return compareInt64(int64(qbt.Deref(torrent.TrackersCount)), cond)
	case FieldPriority:
		return compareInt64(int64(qbt.Deref(torrent.Priority)), cond)
	case FieldGroupSize:
		size := int64(0)
		if idx := resolveConditionGroupIndex(cond, ctx); idx != nil {
			size = int64(idx.SizeForHash(qbt.Deref(torrent.Hash)))
		}
		return compareInt64(size, cond)

	// Boolean fields
	case FieldPrivate:
		return compareBool(qbt.Deref(torrent.Private), cond)
	case FieldAutoManaged:
		return compareBool(false, cond)
	case FieldFirstLastPiecePrio:
		return compareBool(qbt.Deref(torrent.FirstLastPiecePrio), cond)
	case FieldForceStart:
		return compareBool(qbt.Deref(torrent.ForceStart), cond)
	case FieldSequentialDownload:
		return compareBool(qbt.Deref(torrent.SequentialDownload), cond)
	case FieldSuperSeeding:
		return compareBool(qbt.Deref(torrent.SuperSeeding), cond)
	case FieldIsUnregistered:
		isUnregistered := false
		if ctx != nil && ctx.UnregisteredSet != nil {
			_, isUnregistered = ctx.UnregisteredSet[qbt.Deref(torrent.Hash)]
		}
		return compareBool(isUnregistered, cond)

	case FieldHardlinkScope:
		// Instances without local filesystem access cannot detect hardlink scope.
		// Return false so the condition doesn't match and rules won't trigger unintended actions.
		// Note: Automations using HARDLINK_SCOPE are validated at creation time to require local access.
		if ctx == nil || !ctx.InstanceHasLocalAccess {
			return false
		}
		// If scope couldn't be computed for this torrent (files inaccessible, stat failures, etc.),
		// treat as "unknown" and don't match any condition to prevent unintended rule triggers.
		if ctx.HardlinkScopeByHash == nil {
			return false
		}
		scope, ok := ctx.HardlinkScopeByHash[qbt.Deref(torrent.Hash)]
		if !ok {
			return false // Unknown scope - don't match
		}
		return compareHardlinkScope(scope, cond)

	case FieldHasMissingFiles:
		// Instances without local filesystem access cannot detect missing files.
		// Return false so the condition doesn't match and rules won't trigger unintended actions.
		if ctx == nil || !ctx.InstanceHasLocalAccess {
			return false
		}
		// If missing files couldn't be computed for this torrent (incomplete, etc.),
		// treat as "unknown" and don't match any condition to prevent unintended rule triggers.
		if ctx.HasMissingFilesByHash == nil {
			return false
		}
		hasMissing, ok := ctx.HasMissingFilesByHash[qbt.Deref(torrent.Hash)]
		if !ok {
			return false // Unknown state - don't match
		}
		return compareBool(hasMissing, cond)

	case FieldIsGrouped:
		grouped := false
		if idx := resolveConditionGroupIndex(cond, ctx); idx != nil {
			grouped = idx.SizeForHash(qbt.Deref(torrent.Hash)) > 1
		}
		return compareBool(grouped, cond)

	default:
		return false
	}
}

func compareState(torrent qbt.Torrent, cond *RuleCondition, ctx *EvalContext) bool {
	if cond == nil {
		return false
	}

	matches := matchesStateValue(torrent, cond.Value, ctx)
	switch cond.Operator {
	case OperatorEqual:
		return matches
	case OperatorNotEqual:
		return !matches
	default:
		// Preserve legacy behavior for non-state operators (even though the UI only offers EQUAL/NOT_EQUAL).
		return compareString(string(qbt.Deref(torrent.State)), cond)
	}
}

// matchesStateValue matches against the torrent "status" buckets used by the sidebar filters
// (e.g. "errored", "stalled_uploading") with a fallback to exact torrent.State string matching.
func matchesStateValue(torrent qbt.Torrent, value string, ctx *EvalContext) bool {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return false
	}

	switch strings.ToLower(normalized) {
	// Sidebar status buckets
	case "completed":
		return qbt.Deref(torrent.Progress) >= 1.0
	case "downloading":
		return slices.Contains([]qbt.TorrentState{
			qbt.StateDownloading,
			qbt.StateStalledDL,
			qbt.StateMetaDL,
			qbt.StateQueuedDL,
			qbt.StateAllocating,
			qbt.StateCheckingDL,
			qbt.StateForcedDL,
		}, qbt.Deref(torrent.State))
	case "uploading", "seeding":
		return slices.Contains([]qbt.TorrentState{
			qbt.StateUploading,
			qbt.StateStalledUP,
			qbt.StateQueuedUP,
			qbt.StateCheckingUP,
			qbt.StateForcedUP,
		}, qbt.Deref(torrent.State))
	case "paused", "stopped":
		return slices.Contains([]qbt.TorrentState{
			qbt.StatePausedDL,
			qbt.StatePausedUP,
			qbt.StateStoppedDL,
			qbt.StateStoppedUP,
		}, qbt.Deref(torrent.State))
	case "running", "resumed":
		return !slices.Contains([]qbt.TorrentState{
			qbt.StatePausedDL,
			qbt.StatePausedUP,
			qbt.StateStoppedDL,
			qbt.StateStoppedUP,
		}, qbt.Deref(torrent.State))
	case "active":
		return slices.Contains([]qbt.TorrentState{
			qbt.StateDownloading,
			qbt.StateUploading,
			qbt.StateForcedDL,
			qbt.StateForcedUP,
		}, qbt.Deref(torrent.State))
	case "inactive":
		return !slices.Contains([]qbt.TorrentState{
			qbt.StateDownloading,
			qbt.StateUploading,
			qbt.StateForcedDL,
			qbt.StateForcedUP,
		}, qbt.Deref(torrent.State))
	case "stalled":
		return slices.Contains([]qbt.TorrentState{
			qbt.StateStalledDL,
			qbt.StateStalledUP,
		}, qbt.Deref(torrent.State))
	case "stalled_uploading", "stalled_seeding":
		return qbt.Deref(torrent.State) == qbt.StateStalledUP
	case "stalled_downloading":
		return qbt.Deref(torrent.State) == qbt.StateStalledDL
	case "checking":
		return slices.Contains([]qbt.TorrentState{
			qbt.StateCheckingDL,
			qbt.StateCheckingUP,
			qbt.StateCheckingResumeData,
		}, qbt.Deref(torrent.State))
	case "moving":
		return qbt.Deref(torrent.State) == qbt.StateMoving
	case "errored", "error":
		return qbt.Deref(torrent.State) == qbt.StateError || qbt.Deref(torrent.State) == qbt.StateMissingFiles
	case "missingfiles":
		return qbt.Deref(torrent.State) == qbt.StateMissingFiles
	case "unregistered":
		if ctx == nil || ctx.UnregisteredSet == nil {
			return false
		}
		_, ok := ctx.UnregisteredSet[qbt.Deref(torrent.Hash)]
		return ok
	case "tracker_down":
		if ctx == nil || ctx.TrackerDownSet == nil {
			return false
		}
		_, ok := ctx.TrackerDownSet[qbt.Deref(torrent.Hash)]
		return ok
	}

	// Fallback to raw torrent state (qBittorrent Web API value, e.g. "stalledUP").
	return strings.EqualFold(string(qbt.Deref(torrent.State)), normalized)
}

// compareString compares a string value against the condition.
func compareString(value string, cond *RuleCondition) bool {
	// Regex matching
	if cond.Regex || cond.Operator == OperatorMatches {
		if cond.Compiled == nil {
			return false
		}
		matched := cond.Compiled.MatchString(value)
		if cond.Operator == OperatorNotContains || cond.Operator == OperatorNotEqual {
			return !matched
		}
		return matched
	}

	switch cond.Operator {
	case OperatorEqual:
		return strings.EqualFold(value, cond.Value)
	case OperatorNotEqual:
		return !strings.EqualFold(value, cond.Value)
	case OperatorContains:
		return strings.Contains(strings.ToLower(value), strings.ToLower(cond.Value))
	case OperatorNotContains:
		return !strings.Contains(strings.ToLower(value), strings.ToLower(cond.Value))
	case OperatorStartsWith:
		return strings.HasPrefix(strings.ToLower(value), strings.ToLower(cond.Value))
	case OperatorEndsWith:
		return strings.HasSuffix(strings.ToLower(value), strings.ToLower(cond.Value))
	default:
		return false
	}
}

func compareTracker(trackerURL string, cond *RuleCondition, ctx *EvalContext) bool {
	return compareStringCandidates(trackerCandidates(trackerURL, ctx), cond)
}

func compareTrackers(torrent qbt.Torrent, cond *RuleCondition, ctx *EvalContext) bool {
	return compareStringCandidates(trackerCandidates(qbt.Deref(torrent.Tracker), ctx), cond)
}

func trackerCandidates(trackerURL string, ctx *EvalContext) []string {
	// Candidates: raw URL, extracted domain, optional customization display name.
	raw := strings.TrimSpace(trackerURL)
	domain := extractTrackerDomain(raw)
	displayName := ""
	if ctx != nil && ctx.TrackerDisplayNameByDomain != nil && domain != "" {
		if name, ok := ctx.TrackerDisplayNameByDomain[strings.ToLower(domain)]; ok {
			displayName = strings.TrimSpace(name)
		}
	}

	candidates := make([]string, 0, 3)
	if raw != "" {
		candidates = append(candidates, raw)
	}
	if domain != "" && !strings.EqualFold(domain, raw) {
		candidates = append(candidates, domain)
	}
	if displayName != "" && !strings.EqualFold(displayName, domain) && !strings.EqualFold(displayName, raw) {
		candidates = append(candidates, displayName)
	}

	return candidates
}

func compareStringCandidates(candidates []string, cond *RuleCondition) bool {
	// Preserve existing empty-string behavior (e.g., equals "").
	if len(candidates) == 0 {
		return compareString("", cond)
	}

	// De-duplicate case-insensitively while preserving order.
	seen := make(map[string]struct{}, len(candidates))
	uniq := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, candidate)
	}

	if cond.Regex || cond.Operator == OperatorMatches {
		if cond.Compiled == nil {
			return false
		}
		anyMatch := slices.ContainsFunc(uniq, func(c string) bool {
			return cond.Compiled.MatchString(c)
		})
		if cond.Operator == OperatorNotContains || cond.Operator == OperatorNotEqual {
			// Negative operators apply to the combined candidate set: fail if any candidate matches.
			return !anyMatch
		}
		// Regex-enabled string operators: succeed if any candidate matches.
		return anyMatch
	}

	// Important: negative operators must apply to the combined candidate set.
	// Example: NOT_EQUAL "BHD" must be false if any candidate equals "BHD".
	if cond.Operator == OperatorNotEqual {
		return !slices.ContainsFunc(uniq, func(c string) bool {
			return strings.EqualFold(c, cond.Value)
		})
	}
	if cond.Operator == OperatorNotContains {
		condLower := strings.ToLower(cond.Value)
		return !slices.ContainsFunc(uniq, func(c string) bool {
			return strings.Contains(strings.ToLower(c), condLower)
		})
	}

	return slices.ContainsFunc(uniq, func(c string) bool {
		if compareString(c, cond) {
			return true
		}
		return false
	})
}

func extractTrackerDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// URL parsing with scheme (http/https/udp/etc).
	if u, err := url.Parse(raw); err == nil {
		if h := u.Hostname(); h != "" {
			return normalizeLower(h)
		}
	}

	// Scheme-less input (tracker.example.com/announce).
	if !strings.Contains(raw, "://") {
		if u, err := url.Parse("//" + raw); err == nil {
			if h := u.Hostname(); h != "" {
				return normalizeLower(h)
			}
		}
	}

	// Manual fallback: host[:port][/path]
	candidate := raw
	if idx := strings.IndexAny(candidate, "/?#"); idx != -1 {
		candidate = candidate[:idx]
	}
	candidate = strings.TrimPrefix(candidate, "//")
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	// Try to split host:port (IPv6 requires brackets for SplitHostPort).
	if host, _, err := net.SplitHostPort(candidate); err == nil {
		return normalizeLower(strings.Trim(host, "[]"))
	}

	// If it's a plain IP (including IPv6 without port), keep it.
	if ip := net.ParseIP(candidate); ip != nil && strings.Contains(candidate, ":") {
		return normalizeLower(candidate)
	}

	// Strip :port for hostnames/IPv4.
	if idx := strings.Index(candidate, ":"); idx != -1 {
		candidate = candidate[:idx]
	}
	candidate = strings.Trim(candidate, "[]")
	return normalizeLowerTrim(candidate)
}

// compareTags compares tags against the condition, treating tags as a set.
// For string operators, checks individual tags rather than the full comma-separated string.
// Regex matching still operates on the full string for flexibility.
func compareTags(tagsRaw string, cond *RuleCondition) bool {
	// Regex matching operates on full string for flexibility
	if cond.Regex || cond.Operator == OperatorMatches {
		if cond.Compiled == nil {
			return false
		}
		matched := cond.Compiled.MatchString(tagsRaw)
		if cond.Operator == OperatorNotContains || cond.Operator == OperatorNotEqual {
			return !matched
		}
		return matched
	}

	tags := splitTags(tagsRaw)
	condValue := normalizeLowerTrim(cond.Value)

	switch cond.Operator {
	case OperatorEqual:
		return anyTagMatches(tags, condValue, strings.EqualFold)
	case OperatorNotEqual:
		return !anyTagMatches(tags, condValue, strings.EqualFold)
	case OperatorContains:
		return anyTagMatches(tags, condValue, tagContains)
	case OperatorNotContains:
		return !anyTagMatches(tags, condValue, tagContains)
	case OperatorStartsWith:
		return anyTagMatches(tags, condValue, tagStartsWith)
	case OperatorEndsWith:
		return anyTagMatches(tags, condValue, tagEndsWith)
	default:
		return false
	}
}

// anyTagMatches returns true if any tag in the slice satisfies the match function.
func anyTagMatches(tags []string, condValue string, match func(string, string) bool) bool {
	for _, tag := range tags {
		if match(tag, condValue) {
			return true
		}
	}
	return false
}

// tagContains checks if tag contains condValue (case-insensitive).
func tagContains(tag, condValue string) bool {
	return strings.Contains(strings.ToLower(tag), condValue)
}

// tagStartsWith checks if tag starts with condValue (case-insensitive).
func tagStartsWith(tag, condValue string) bool {
	return strings.HasPrefix(strings.ToLower(tag), condValue)
}

// tagEndsWith checks if tag ends with condValue (case-insensitive).
func tagEndsWith(tag, condValue string) bool {
	return strings.HasSuffix(strings.ToLower(tag), condValue)
}

// compareInt64 compares an int64 value against the condition.
func compareInt64(value int64, cond *RuleCondition) bool {
	// Parse the condition value as int64
	condValue, err := strconv.ParseInt(cond.Value, 10, 64)
	if err != nil && cond.Value != "" {
		return false
	}

	switch cond.Operator {
	case OperatorEqual:
		return value == condValue
	case OperatorNotEqual:
		return value != condValue
	case OperatorGreaterThan:
		return value > condValue
	case OperatorGreaterThanOrEqual:
		return value >= condValue
	case OperatorLessThan:
		return value < condValue
	case OperatorLessThanOrEqual:
		return value <= condValue
	case OperatorBetween:
		if cond.MinValue == nil || cond.MaxValue == nil {
			return false
		}
		return float64(value) >= *cond.MinValue && float64(value) <= *cond.MaxValue
	default:
		return false
	}
}

// compareFloat64 compares a float64 value against the condition.
func compareFloat64(value float64, cond *RuleCondition) bool {
	// Parse the condition value as float64
	condValue, err := strconv.ParseFloat(cond.Value, 64)
	if err != nil && cond.Value != "" {
		return false
	}

	switch cond.Operator {
	case OperatorEqual:
		return value == condValue
	case OperatorNotEqual:
		return value != condValue
	case OperatorGreaterThan:
		return value > condValue
	case OperatorGreaterThanOrEqual:
		return value >= condValue
	case OperatorLessThan:
		return value < condValue
	case OperatorLessThanOrEqual:
		return value <= condValue
	case OperatorBetween:
		if cond.MinValue == nil || cond.MaxValue == nil {
			return false
		}
		return value >= *cond.MinValue && value <= *cond.MaxValue
	default:
		return false
	}
}

func normalizeProgressCondition(cond *RuleCondition) *RuleCondition {
	if cond == nil {
		return nil
	}

	normalized := *cond

	if normalized.Value != "" {
		if v, err := strconv.ParseFloat(normalized.Value, 64); err == nil {
			v = normalizeProgressValue(v)
			normalized.Value = strconv.FormatFloat(v, 'f', -1, 64)
		}
	}

	if normalized.MinValue != nil {
		v := normalizeProgressValue(*normalized.MinValue)
		normalized.MinValue = &v
	}

	if normalized.MaxValue != nil {
		v := normalizeProgressValue(*normalized.MaxValue)
		normalized.MaxValue = &v
	}

	return &normalized
}

func normalizeProgressValue(v float64) float64 {
	// Older workflows stored progress conditions as 0-100 percentages; normalize to 0-1.
	if v > 1 {
		v /= 100
	}

	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// compareBool compares a boolean value against the condition.
func compareBool(value bool, cond *RuleCondition) bool {
	condValue := strings.ToLower(cond.Value) == "true" || cond.Value == "1"

	switch cond.Operator {
	case OperatorEqual:
		return value == condValue
	case OperatorNotEqual:
		return value != condValue
	default:
		return false
	}
}

// compareHardlinkScope compares a hardlink scope value against the condition.
func compareHardlinkScope(value string, cond *RuleCondition) bool {
	switch cond.Operator {
	case OperatorEqual:
		return strings.EqualFold(value, cond.Value)
	case OperatorNotEqual:
		return !strings.EqualFold(value, cond.Value)
	default:
		return false
	}
}

// compareAge computes the age (time since timestamp) and compares it against the condition.
// Age is calculated as: nowUnix - timestamp, clamped to 0 to avoid clock-skew weirdness.
func compareAge(timestamp int64, cond *RuleCondition, ctx *EvalContext) bool {
	// Get current time from context (for testability) or use time.Now()
	nowUnix := time.Now().Unix()
	if ctx != nil && ctx.NowUnix > 0 {
		nowUnix = ctx.NowUnix
	}

	// Compute age in seconds, clamped to 0 to avoid negative ages from clock skew
	ageSeconds := max(nowUnix-timestamp, 0)

	return compareInt64(ageSeconds, cond)
}

// compareAgeIfSet compares age for Unix timestamp fields and treats unset values (<= 0) as unknown/no-match.
func compareAgeIfSet(timestamp int64, cond *RuleCondition, ctx *EvalContext) bool {
	if timestamp <= 0 {
		return false
	}
	return compareAge(timestamp, cond, ctx)
}

// splitTags splits a comma-separated tag string into individual tags.
// Returns nil for empty input.
func splitTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// LoadFreeSpaceSourceState loads the projection state for the given source key into evalCtx.
// If the source key differs from the currently active source, the current state is persisted
// to FreeSpaceStates before loading the new source.
// Does nothing if sourceKey is empty or FreeSpaceStates is nil.
func (ctx *EvalContext) LoadFreeSpaceSourceState(sourceKey string) {
	if ctx == nil || sourceKey == "" || ctx.FreeSpaceStates == nil {
		return
	}

	// Already loaded
	if ctx.ActiveFreeSpaceSource == sourceKey {
		return
	}

	// Persist current state before switching
	if ctx.ActiveFreeSpaceSource != "" {
		ctx.PersistFreeSpaceSourceState()
	}

	// Load new source state
	state, ok := ctx.FreeSpaceStates[sourceKey]
	if !ok || state == nil {
		// Source not found - set FreeSpace to MaxInt64 so FREE_SPACE conditions won't match.
		// Using 0 would cause "< threshold" comparisons to always trigger, which is dangerous.
		ctx.FreeSpace = math.MaxInt64
		ctx.SpaceToClear = 0
		ctx.FilesToClear = nil
		ctx.HardlinkSignaturesToClear = nil
		ctx.ActiveFreeSpaceSource = ""
		return
	}

	ctx.FreeSpace = state.FreeSpace
	ctx.SpaceToClear = state.SpaceToClear
	ctx.FilesToClear = state.FilesToClear
	ctx.HardlinkSignaturesToClear = state.HardlinkSignaturesToClear
	ctx.ActiveFreeSpaceSource = sourceKey
}

// PersistFreeSpaceSourceState persists the current projection state back to FreeSpaceStates.
// Does nothing if no source is currently active.
func (ctx *EvalContext) PersistFreeSpaceSourceState() {
	if ctx == nil || ctx.ActiveFreeSpaceSource == "" || ctx.FreeSpaceStates == nil {
		return
	}

	state := ctx.FreeSpaceStates[ctx.ActiveFreeSpaceSource]
	if state == nil {
		return
	}

	state.SpaceToClear = ctx.SpaceToClear
	state.FilesToClear = ctx.FilesToClear
	state.HardlinkSignaturesToClear = ctx.HardlinkSignaturesToClear
}
