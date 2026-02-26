// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package jackett

import "context"

// SearchPriority returns the desired scheduler priority embedded in ctx via WithSearchPriority.
func SearchPriority(ctx context.Context) (RateLimitPriority, bool) {
	return getSearchPriorityFromContext(ctx)
}
