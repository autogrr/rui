// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package domain

const RedactedStr = "<redacted>"

// RedactString replaces a string with redacted placeholder
func RedactString(s string) string {
	if len(s) == 0 {
		return ""
	}

	return RedactedStr
}

// IsRedactedString checks if a value is the redacted placeholder
func IsRedactedString(s string) bool {
	if s == "" {
		return false
	}
	return s == RedactedStr
}
