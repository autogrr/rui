// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
)

func TestCancelAutomationRun_NoActiveRun(t *testing.T) {
	s := &Service{}

	// When no run is active, cancel should return false
	if got := s.CancelAutomationRun(); got {
		t.Errorf("CancelAutomationRun() = %v, want false when no run is active", got)
	}
}

func TestCancelAutomationRun_ActiveRun(t *testing.T) {
	s := &Service{}

	// Simulate an active run
	s.runActive.Store(true)
	canceled := false
	s.runCancel = func() { canceled = true }

	// When a run is active, cancel should return true and call the cancel func
	if got := s.CancelAutomationRun(); !got {
		t.Errorf("CancelAutomationRun() = %v, want true when run is active", got)
	}
	if !canceled {
		t.Errorf("CancelAutomationRun() did not call the cancel function")
	}
}

func TestCancelAutomationRun_ActiveRunNilCancel(t *testing.T) {
	s := &Service{}

	// Simulate an active run but with nil cancel (shouldn't happen in practice)
	s.runActive.Store(true)
	s.runCancel = nil

	// Should return false since cancel is nil
	if got := s.CancelAutomationRun(); got {
		t.Errorf("CancelAutomationRun() = %v, want false when cancel is nil", got)
	}
}

// TestShouldSkipErroredTorrent tests the actual Service.shouldSkipErroredTorrent
// method used by findCandidates and refreshSearchQueue to filter errored torrents.
func TestShouldSkipErroredTorrent(t *testing.T) {
	tests := []struct {
		name           string
		state          qbt.TorrentState
		recoverEnabled bool
		shouldSkip     bool
	}{
		{"error state, recovery disabled", qbt.StateError, false, true},
		{"missingFiles state, recovery disabled", qbt.StateMissingFiles, false, true},
		{"completed state, recovery disabled", qbt.StatePausedUP, false, false},
		{"seeding state, recovery disabled", qbt.StateUploading, false, false},
		{"downloading state, recovery disabled", qbt.StateDownloading, false, false},
		{"error state, recovery enabled", qbt.StateError, true, false},
		{"missingFiles state, recovery enabled", qbt.StateMissingFiles, true, false},
		{"completed state, recovery enabled", qbt.StatePausedUP, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				recoverErroredTorrentsEnabled: tt.recoverEnabled,
			}

			got := s.shouldSkipErroredTorrent(tt.state)
			if got != tt.shouldSkip {
				t.Errorf("shouldSkipErroredTorrent(%v) with recoverEnabled=%v: got %v, want %v",
					tt.state, tt.recoverEnabled, got, tt.shouldSkip)
			}
		})
	}
}

// TestRecoverErroredTorrentsEnabled_DefaultDisabled verifies that the default
// (zero value) for recoverErroredTorrentsEnabled is false, meaning errored
// torrents are filtered out by default.
func TestRecoverErroredTorrentsEnabled_DefaultDisabled(t *testing.T) {
	s := &Service{} // Zero value - recoverErroredTorrentsEnabled defaults to false

	if s.recoverErroredTorrentsEnabled {
		t.Error("expected default recoverErroredTorrentsEnabled to be false")
	}

	// With default (false), errored torrents should be skipped
	if !s.shouldSkipErroredTorrent(qbt.StateError) {
		t.Error("expected errored torrents to be skipped when recovery is disabled")
	}
	if !s.shouldSkipErroredTorrent(qbt.StateMissingFiles) {
		t.Error("expected missingFiles torrents to be skipped when recovery is disabled")
	}
}
