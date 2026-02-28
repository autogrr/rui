// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package update

import (
	"context"

	"github.com/autogrr/rui/pkg/version"

	"github.com/rs/zerolog"
)

// Service is a no-op stub. Outbound update checks have been removed from rui.
// The struct is kept so that callers (API handlers, etc.) can still reference it
// without code changes, but it never makes network requests.
type Service struct {
	log            zerolog.Logger
	currentVersion string
}

// NewService creates a new (no-op) update Service instance.
// The enabled and userAgent parameters are accepted for API compatibility but ignored.
func NewService(log zerolog.Logger, _ bool, currentVersion, _ string) *Service {
	return &Service{
		log:            log.With().Str("component", "update").Logger(),
		currentVersion: currentVersion,
	}
}

// Start is a no-op. Outbound update checks have been removed from rui.
func (s *Service) Start(_ context.Context) {}

// GetLatestRelease always returns nil. Outbound update checks have been removed.
func (s *Service) GetLatestRelease(_ context.Context) *version.Release {
	return nil
}

// CheckUpdates is a no-op. Outbound update checks have been removed.
func (s *Service) CheckUpdates(_ context.Context) {}

// CheckUpdateAvailable is a no-op. Always returns nil.
func (s *Service) CheckUpdateAvailable(_ context.Context) (*version.Release, error) {
	return nil, nil
}

// SetEnabled is a no-op. Kept for API compatibility.
func (s *Service) SetEnabled(_ bool) {}
