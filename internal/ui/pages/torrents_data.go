// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents_data.go implements TorrentsTableBody as a plain Go ComponentFunc.
// This sidesteps templ's HTML-escaping context: templ compiles <script> bodies
// as literal string constants so Go expressions inside them don't work.
// Writing via ComponentFunc lets us embed the JSON payload verbatim.

package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

// TorrentsTableBody emits a compact JSON blob consumed by the virtual scroll
// renderer (initVirtualScroll). No HTML rows are sent — JS creates <tr>
// elements on demand so the DOM stays small regardless of torrent count.
func TorrentsTableBody(p TorrentsProps) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := fmt.Fprintf(w, `<script id="vt-rows-json" type="application/json">%s</script>`, torrentsToJSON(p))
		return err
	})
}

// TorrentsColDefaults emits the column-visibility defaults as a typed JSON
// element. Go expressions cannot be interpolated inside templ <script> raw-text
// blocks, so this ComponentFunc is used to inject the value at render time.
func TorrentsColDefaults() templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := fmt.Fprintf(w, `<script id="col-defaults-json" type="application/json">%s</script>`, colDefaultsJSON())
		return err
	})
}

// TorrentsSidebarJSON emits all sidebar filter lists (categories, tags,
// trackers, save paths) as a single typed JSON element so the JS sidebar
// renderer can populate VirtualList instances without templ for-loops.
func TorrentsSidebarJSON(p TorrentsProps) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		type sidebarPayload struct {
			BaseURL    string   `json:"baseURL"`
			Categories []string `json:"categories"`
			Tags       []string `json:"tags"`
			Trackers   []string `json:"trackers"`
			SavePaths  []string `json:"savePaths"`
		}
		payload := sidebarPayload{
			BaseURL:    p.BaseURL,
			Categories: p.Categories,
			Tags:       p.Tags,
			Trackers:   p.Trackers,
			SavePaths:  p.SavePaths,
		}
		if payload.Categories == nil {
			payload.Categories = []string{}
		}
		if payload.Tags == nil {
			payload.Tags = []string{}
		}
		if payload.Trackers == nil {
			payload.Trackers = []string{}
		}
		if payload.SavePaths == nil {
			payload.SavePaths = []string{}
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, `<script id="vt-sidebar-json" type="application/json">%s</script>`, b)
		return err
	})
}
