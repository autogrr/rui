// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"
)

func TestIsWindowsDriveAbs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		// Valid Windows absolute paths
		{"C:/", true},
		{"C:/Downloads", true},
		{"D:/foo/bar.mkv", true},
		{"z:/lower", true},

		// Invalid: drive-relative (no leading slash after colon)
		{"C:folder", false},
		{"C:", false},

		// Invalid: URLs should not match
		{"http://example.com", false},
		{"https://foo", false},

		// Invalid: too short
		{"C", false},
		{"", false},

		// Invalid: POSIX paths
		{"/downloads", false},
		{"/", false},

		// Invalid: relative paths
		{"foo/bar", false},
		{".", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, isWindowsDriveAbs(tt.path))
		})
	}
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Empty/edge cases
		{"empty", "", ""},

		// POSIX paths
		{"posix root", "/", "/"},
		{"posix simple", "/downloads/show", "/downloads/show"},
		{"posix trailing slash", "/downloads/show/", "/downloads/show"},
		{"posix dot segments", "/downloads/./show/../movies", "/downloads/movies"},

		// Windows paths - drive root preservation
		{"windows drive root", "C:/", "C:/"},
		{"windows drive root backslash", "C:\\", "C:/"},
		{"windows drive relative bare", "C:", "C:"}, // drive-relative, not absolute
		{"windows path", "C:/Downloads/Movie.mkv", "C:/Downloads/Movie.mkv"},
		{"windows path backslashes", "C:\\Downloads\\Movie.mkv", "C:/Downloads/Movie.mkv"},
		{"windows path trailing slash", "C:/Downloads/", "C:/Downloads"},
		{"windows path dot segments", "C:/Downloads/./foo/../bar", "C:/Downloads/bar"},
		{"windows lowercase drive", "c:/Downloads", "c:/Downloads"},

		// Mixed slashes
		{"mixed slashes", "/downloads\\tv/show", "/downloads/tv/show"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, normalizePath(tt.input))
		})
	}
}

func TestResolveRootlessContentDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		torrent        *qbt.Torrent
		candidateFiles []qbt.TorrentFile
		expected       string
	}{
		{
			name:           "nil torrent",
			torrent:        nil,
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f.mkv")}},
			expected:       "",
		},
		{
			name:           "empty content path",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f.mkv")}},
			expected:       "",
		},
		{
			name:           "no candidate files",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("/downloads/show/f.mkv")},
			candidateFiles: nil,
			expected:       "",
		},
		{
			name:           "dot content path",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr(".")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f.mkv")}},
			expected:       "",
		},
		{
			name:           "single file extracts dir",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("/downloads/show/f.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f.mkv")}},
			expected:       "/downloads/show",
		},
		{
			name:           "single file relative path returns empty",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("file.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("file.mkv")}},
			expected:       "",
		},
		{
			name:           "single file normalizes backslashes",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("/downloads\\tv\\Show\\file.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("file.mkv")}},
			expected:       "/downloads/tv/Show",
		},
		{
			name:           "multi-file uses content path",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("/downloads/show")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f1.mkv")}, {Name: qbt.Ptr("f2.mkv")}},
			expected:       "/downloads/show",
		},
		{
			name:           "multi-file cleans trailing slash",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("/downloads/show/")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("f1.mkv")}, {Name: qbt.Ptr("f2.mkv")}},
			expected:       "/downloads/show",
		},
		// Windows path tests
		{
			name:           "windows single file extracts dir",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("C:/Downloads/Movie.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("Movie.mkv")}},
			expected:       "C:/Downloads",
		},
		{
			name:           "windows drive root single file",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("C:/Movie.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("Movie.mkv")}},
			expected:       "C:/",
		},
		{
			name:           "windows multi-file",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("D:/Shows/MyShow")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("e01.mkv")}, {Name: qbt.Ptr("e02.mkv")}},
			expected:       "D:/Shows/MyShow",
		},
		{
			name:           "windows backslash path",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("C:\\Downloads\\Movie.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("Movie.mkv")}},
			expected:       "C:/Downloads",
		},
		// URL rejection (should not be treated as Windows absolute)
		{
			name:           "http url rejected",
			torrent:        &qbt.Torrent{ContentPath: qbt.Ptr("http://example.com/file.mkv")},
			candidateFiles: []qbt.TorrentFile{{Name: qbt.Ptr("file.mkv")}},
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, resolveRootlessContentDir(tt.torrent, tt.candidateFiles))
		})
	}
}
