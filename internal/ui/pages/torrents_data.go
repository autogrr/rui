// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// torrents_data.go implements TorrentsTableBody as a plain Go ComponentFunc.
// This sidesteps templ's HTML-escaping context: templ compiles <script> bodies
// as literal string constants so Go expressions inside them don't work.
// Writing via ComponentFunc lets us embed the JSON payload verbatim.

package pages

import (
	"context"
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


