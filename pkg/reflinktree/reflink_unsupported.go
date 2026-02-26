// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

//go:build !linux && !darwin

package reflinktree

// SupportsReflink returns false on unsupported platforms.
// Windows and FreeBSD do not have a standard reflink mechanism that we support.
func SupportsReflink(_ string) (supported bool, reason string) {
	return false, "reflink is not supported on this operating system"
}

// cloneFile is not implemented on unsupported platforms.
func cloneFile(_, _ string) error {
	return ErrReflinkUnsupported
}
