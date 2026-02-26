// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import "errors"

var (
	// ErrBasicAuthPasswordRequired is returned when a basic auth username is provided but the password is missing.
	ErrBasicAuthPasswordRequired = errors.New("basic_password is required when basic auth is enabled")
)
