// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package crossseed

import (
	"testing"

	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/pkg/stringutils"
)

func TestClassifyTorrentLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		files  []qbt.TorrentFile
		expect TorrentLayout
	}{
		{
			name: "single mkv with sidecar nfo",
			files: []qbt.TorrentFile{
				{Name: qbt.Ptr("Show.S01E01.1080p.WEB-DL.mkv"), Size: qbt.Ptr(int64(4 << 30))},
				{Name: qbt.Ptr("Show.S01E01.nfo"), Size: qbt.Ptr(int64(1024))},
			},
			expect: LayoutFiles,
		},
		{
			name: "rar multi-part release",
			files: []qbt.TorrentFile{
				{Name: qbt.Ptr("Release.part01.rar"), Size: qbt.Ptr(int64(2 << 30))},
				{Name: qbt.Ptr("Release.part02.r00"), Size: qbt.Ptr(int64(2 << 30))},
				{Name: qbt.Ptr("Release.sfv"), Size: qbt.Ptr(int64(2048))},
			},
			expect: LayoutArchives,
		},
		{
			name: "gz archive",
			files: []qbt.TorrentFile{
				{Name: qbt.Ptr("Archive.tar.gz"), Size: qbt.Ptr(int64(1 << 30))},
			},
			expect: LayoutArchives,
		},
		{
			name: "all ignored files (hardcoded patterns)",
			files: []qbt.TorrentFile{
				{Name: qbt.Ptr("readme.txt"), Size: qbt.Ptr(int64(512))},
				{Name: qbt.Ptr("info.nfo"), Size: qbt.Ptr(int64(1024))},
			},
			expect: LayoutUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := classifyTorrentLayout(tt.files, stringutils.NewDefaultNormalizer())
			require.Equal(t, tt.expect, layout)
		})
	}
}
