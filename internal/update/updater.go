// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package update

import (
	"errors"
)

// errSelfUpdateDisabled is returned when the self-update command is invoked.
// Outbound requests to GitHub have been removed from rui.
var errSelfUpdateDisabled = errors.New("self-update is disabled — outbound requests have been removed from rui")

// Config holds updater configuration (kept for API compatibility).
type Config struct {
	Repository string
	Version    string
}

// Updater is a no-op stub. Self-update functionality has been removed.
type Updater struct {
	config Config
}

// NewUpdater returns a no-op Updater.
func NewUpdater(config Config) *Updater {
	return &Updater{
		config: config,
	}
}

// Run always returns an error — self-update is disabled.
func (u *Updater) Run() error {
	return errSelfUpdateDisabled
}
