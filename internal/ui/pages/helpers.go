// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package pages

import "fmt"

// itoa converts n to its decimal string representation.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// formatBytesPerSec returns a human-readable speed string (e.g. "12.3 MB/s").
func formatBytesPerSec(bytesPerSec uint64) string {
	return formatBytes(bytesPerSec) + "/s"
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
