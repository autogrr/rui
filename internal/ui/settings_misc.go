// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// settings_misc.go contains HTMX partial handlers for:
//   - Logs Settings section (save log settings, stream log lines via SSE)

package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/config"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ──────────────────────────────────────────────────────────────────
// Logs section
//
//	POST /ui/partials/settings/logs         → save log settings
//	GET  /ui/partials/settings/logs/stream  → SSE log stream
// ──────────────────────────────────────────────────────────────────

// PostLogSettings saves log level, path, max-size, max-backups.
func (h *Handler) PostLogSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Invalid form data"))
		return
	}

	var update config.LogSettingsUpdate

	if level := strings.TrimSpace(r.FormValue("level")); level != "" {
		valid := map[string]bool{"trace": true, "debug": true, "info": true, "warn": true, "error": true,
			"TRACE": true, "DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
		if !valid[level] {
			render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Invalid log level: "+level))
			return
		}
		update.Level = &level
	}

	if maxSizeStr := r.FormValue("max_size"); maxSizeStr != "" {
		v, err := strconv.Atoi(maxSizeStr)
		if err != nil || v < 1 {
			render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Max size must be at least 1 MB"))
			return
		}
		update.MaxSize = &v
	}

	if maxBackupsStr := r.FormValue("max_backups"); maxBackupsStr != "" {
		v, err := strconv.Atoi(maxBackupsStr)
		if err != nil || v < 0 {
			render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Max backups cannot be negative"))
			return
		}
		update.MaxBackups = &v
	}

	if path := strings.TrimSpace(r.FormValue("path")); path != "" {
		update.Path = &path
	}

	if h.cfg == nil {
		render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Config not available"))
		return
	}

	if _, err := h.cfg.UpdateLogSettings(update); err != nil {
		log.Error().Err(err).Msg("ui: failed to update log settings")
		render(w, r, http.StatusUnprocessableEntity, pages.LogSettingsResultPartial(false, "Failed to update log settings: "+err.Error()))
		return
	}

	render(w, r, http.StatusOK, pages.LogSettingsResultPartial(true, "Log settings saved"))
}

// GetLogsStream streams log lines to the browser via SSE.
func (h *Handler) GetLogsStream(w http.ResponseWriter, r *http.Request) {
	if h.cfg == nil {
		http.Error(w, "Config not available", http.StatusServiceUnavailable)
		return
	}

	mgr := h.cfg.GetLogManager()
	if mgr == nil {
		http.Error(w, "Log manager not available", http.StatusServiceUnavailable)
		return
	}

	hub := mgr.GetHub()
	if hub == nil {
		http.Error(w, "Log stream not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send history first.
	limit := 1000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	for _, line := range hub.History(limit) {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
			return
		}
	}
	flusher.Flush()

	// Stream live lines.
	sub := hub.Subscribe(r.Context())
	defer hub.Unsubscribe(sub)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-sub.Channel():
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			// Heartbeat to keep the connection alive.
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
