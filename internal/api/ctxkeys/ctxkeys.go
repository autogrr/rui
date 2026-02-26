// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ctxkeys

// Key is a typed context key to avoid collisions across packages.
type Key int

const (
	Username Key = iota
)
