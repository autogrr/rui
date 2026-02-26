// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package automations

import (
	"github.com/autogrr/rui/internal/models"
)

func shouldResetTagActionInClient(action *models.TagAction) bool {
	if action == nil || !action.Enabled || action.UseTrackerAsTag {
		return false
	}

	if len(models.SanitizeCommaSeparatedStringSlice(action.Tags)) == 0 {
		return false
	}

	return action.DeleteFromClient
}
