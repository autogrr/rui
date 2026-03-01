// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package static embeds the compiled CSS and JavaScript assets for the
// server-rendered UI.
package static

import "embed"

//go:embed output.css js themes
var Files embed.FS
