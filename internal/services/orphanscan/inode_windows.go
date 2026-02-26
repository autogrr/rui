// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

//go:build windows

package orphanscan

import "io/fs"

func inodeKeyFromInfo(info fs.FileInfo) (inodeKey, uint64, bool) {
	return inodeKey{}, 0, false
}
