// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

//go:build !windows

package orphanscan

import (
	"errors"
	"syscall"
)

func isReadOnlyFSError(err error) bool {
	return errors.Is(err, syscall.EROFS)
}
