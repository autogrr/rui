// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import "github.com/autogrr/rui/pkg/stringutils"

var lowerTrimNormalizer = stringutils.NewDefaultNormalizer()

func normalizeLowerTrim(value string) string {
	return lowerTrimNormalizer.Normalize(value)
}
