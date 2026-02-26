// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import "github.com/autogrr/rui/pkg/releases"

// ReleaseCache is preserved for backwards compatibility within the crossseed package.
// It is an alias to the shared releases.Parser.
type ReleaseCache = releases.Parser

// NewReleaseCache creates a cached parser for release metadata.
func NewReleaseCache() *ReleaseCache {
	return releases.NewDefaultParser()
}
