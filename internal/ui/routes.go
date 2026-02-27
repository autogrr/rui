// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	uistatic "github.com/autogrr/rui/internal/ui/static"
)

// RegisterRoutes mounts all server-rendered UI routes onto the provided router.
// The router must already have the SCS session middleware applied.
//
// Route layout:
//
//	GET  /ui/static/*          Static assets (CSS & JS) — public
//	GET  /ui/login             Login page — public
//	POST /ui/auth/login        Process login form — public
//	GET  /ui/setup             First-run account creation — public (guarded)
//	POST /ui/auth/setup        Process account creation — public (guarded)
//	POST /ui/auth/logout       Destroy session — private
//	GET  /ui/dashboard         Dashboard — private
//	GET  /ui/partials/dashboard HTMX live-stats fragment — private
//	GET  /ui/torrents          Torrents list — private
//	GET  /ui/partials/torrents  HTMX torrent rows fragment — private
//	GET  /ui/settings[/section] Settings — private
//	GET  /ui/                  Redirect to /ui/dashboard
//	GET  /ui                   Redirect to /ui/dashboard
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/ui", func(r chi.Router) {
		// ----------------------------------------------------------
		// Static assets — always public
		// ----------------------------------------------------------
		staticFS, err := fs.Sub(uistatic.Files, ".")
		if err != nil {
			panic("ui: failed to create static sub-FS: " + err.Error())
		}
		r.Handle("/static/*", http.StripPrefix(h.baseURL()+"/ui/static", http.FileServerFS(staticFS)))

		// ----------------------------------------------------------
		// Unauthenticated routes (still check setup where needed)
		// ----------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Use(h.RequireSetupComplete)

			r.Get("/login", h.GetLogin)
			r.Post("/auth/login", h.PostLogin)
		})

		// Setup routes — bypass RequireSetupComplete so unset installs can reach them
		r.Get("/setup", h.GetSetup)
		r.Post("/auth/setup", h.PostSetup)

		// ----------------------------------------------------------
		// Authenticated routes
		// ----------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Use(h.RequireSetupComplete)
			r.Use(h.RequireAuth)

			r.Post("/auth/logout", h.PostLogout)

			// Top-level redirect.
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, h.baseURL()+"/ui/dashboard", http.StatusSeeOther)
			})

			// Main pages.
			r.Get("/dashboard", h.GetDashboard)
			r.Get("/torrents", h.GetTorrents)
			r.Get("/search", h.GetSearch)
			r.Get("/cross-seed", h.GetCrossSeed)
			r.Get("/automations", h.GetAutomations)
			r.Get("/backups", h.GetBackups)
			r.Get("/rss", h.GetRSS)
			r.Get("/settings", h.GetSettings)
			r.Get("/settings/{section}", h.GetSettings)
			r.Get("/instances", h.GetInstances)
			r.Get("/dir-scan", h.GetDirScan)

			// Instance CRUD (HTMX-driven).
			r.Post("/instances", h.PostInstance)
			r.Put("/instances/{id}", h.PutInstance)
			r.Delete("/instances/{id}", h.DeleteInstance)
			r.Post("/instances/{id}/toggle", h.PostInstanceToggle)

			// HTMX partial fragments — return HTML snippets, not full pages.
			r.Get("/partials/dashboard", h.GetDashboardPartial)
			r.Post("/partials/dashboard/{id}/alt-speed", h.PostAltSpeedToggle)
			r.Get("/partials/torrents", h.GetTorrentsPartial)
			r.Get("/partials/torrents/{hash}", h.GetTorrentDetailPartial)
			r.Post("/partials/torrents/action", h.PostTorrentsAction)
			r.Post("/partials/torrents/add", h.PostAddTorrent)
			r.Get("/partials/instances/form", h.GetInstanceForm)
			r.Get("/partials/instances/form/{id}", h.GetInstanceForm)

			// Settings partials (API keys, change password).
			r.Get("/partials/settings/api-keys", h.GetAPIKeysListPartial)
			r.Get("/partials/settings/api-keys/form", h.GetAPIKeyFormPartial)
			r.Post("/partials/settings/api-keys", h.PostAPIKey)
			r.Delete("/partials/settings/api-keys/{id}", h.DeleteAPIKey)
			r.Post("/partials/settings/security", h.PostChangePassword)

			// Indexers.
			r.Get("/partials/settings/indexers", h.GetIndexersListPartial)
			r.Get("/partials/settings/indexers/form", h.GetIndexerForm)
			r.Get("/partials/settings/indexers/form/{id}", h.GetIndexerForm)
			r.Post("/partials/settings/indexers", h.PostIndexer)
			r.Put("/partials/settings/indexers/{id}", h.PutIndexer)
			r.Delete("/partials/settings/indexers/{id}", h.DeleteIndexer)
			r.Post("/partials/settings/indexers/{id}/test", h.PostIndexerTest)

			// Search Cache.
			r.Post("/partials/settings/search-cache", h.PostSearchCacheTTL)

			// *arr Integrations.
			r.Get("/partials/settings/integrations", h.GetIntegrationsListPartial)
			r.Get("/partials/settings/integrations/form", h.GetIntegrationForm)
			r.Get("/partials/settings/integrations/form/{id}", h.GetIntegrationForm)
			r.Post("/partials/settings/integrations", h.PostIntegration)
			r.Put("/partials/settings/integrations/{id}", h.PutIntegration)
			r.Delete("/partials/settings/integrations/{id}", h.DeleteIntegration)
			r.Post("/partials/settings/integrations/{id}/test", h.PostIntegrationTest)

			// Client API Keys.
			r.Get("/partials/settings/client-api/form", h.GetClientAPIKeyForm)
			r.Post("/partials/settings/client-api", h.PostClientAPIKey)
			r.Delete("/partials/settings/client-api/{id}", h.DeleteClientAPIKey)

			// External Programs.
			r.Get("/partials/settings/external-programs", h.GetExtProgramsListPartial)
			r.Get("/partials/settings/external-programs/form", h.GetExtProgramForm)
			r.Get("/partials/settings/external-programs/form/{id}", h.GetExtProgramForm)
			r.Post("/partials/settings/external-programs", h.PostExtProgram)
			r.Put("/partials/settings/external-programs/{id}", h.PutExtProgram)
			r.Delete("/partials/settings/external-programs/{id}", h.DeleteExtProgram)

			// Notifications.
			r.Get("/partials/settings/notifications", h.GetNotificationsListPartial)
			r.Get("/partials/settings/notifications/form", h.GetNotificationForm)
			r.Get("/partials/settings/notifications/form/{id}", h.GetNotificationForm)
			r.Post("/partials/settings/notifications", h.PostNotification)
			r.Put("/partials/settings/notifications/{id}", h.PutNotification)
			r.Delete("/partials/settings/notifications/{id}", h.DeleteNotification)
			r.Post("/partials/settings/notifications/{id}/test", h.PostNotificationTest)

			// Logs.
			r.Post("/partials/settings/logs", h.PostLogSettings)
			r.Get("/partials/settings/logs/stream", h.GetLogsStream)

			// Search.
			r.Get("/partials/search", h.GetSearchPartial)
			r.Post("/partials/search/download", h.PostSearchDownload)

			// RSS.
			r.Get("/partials/rss/articles", h.GetRSSArticlesPartial)
			r.Post("/partials/rss/feeds", h.PostRSSFeed)
			r.Post("/partials/rss/folders", h.PostRSSFolder)
			r.Delete("/partials/rss/feeds", h.DeleteRSSItem)
			r.Post("/partials/rss/feeds/refresh", h.PostRSSRefresh)
			r.Post("/partials/rss/articles/mark-read", h.PostRSSMarkRead)
			r.Post("/partials/rss/articles/mark-all-read", h.PostRSSMarkAllRead)
			r.Post("/partials/rss/articles/download", h.PostRSSArticleDownload)
			r.Get("/partials/rss/rules", h.GetRSSRulesPartial)
			r.Post("/partials/rss/rules", h.PostRSSRule)
			r.Put("/partials/rss/rules", h.PutRSSRule)
			r.Delete("/partials/rss/rules", h.DeleteRSSRule)
			r.Get("/partials/rss/rules/form", h.GetRSSRuleForm)
			r.Get("/partials/rss/feeds/add-form", h.GetRSSAddFeedForm)
			r.Get("/partials/rss/feeds/folder-form", h.GetRSSAddFolderForm)

			// Cross-seed.
			r.Get("/partials/cross-seed/runs", h.GetCrossSeedRunsPartial)
			r.Get("/partials/cross-seed/blocklist", h.GetCrossSeedBlocklistPartial)
			r.Post("/partials/cross-seed/automation-settings", h.PostCrossSeedAutomationSettings)
			r.Post("/partials/cross-seed/search-settings", h.PostCrossSeedSearchSettings)
			r.Post("/partials/cross-seed/run", h.PostCrossSeedRun)
			r.Delete("/partials/cross-seed/blocklist", h.DeleteCrossSeedBlocklistEntry)

			// Automations.
			r.Get("/partials/automations", h.GetAutomationsPartial)
					r.Get("/partials/automations/rules/new", h.GetAutomationRuleFormNew)
					r.Post("/partials/automations/{instanceId}/rules", h.PostAutomationRule)
					r.Get("/partials/automations/{instanceId}/rules/{id}/edit", h.GetAutomationRuleFormEdit)
					r.Put("/partials/automations/{instanceId}/rules/{id}", h.PutAutomationRule)
					r.Delete("/partials/automations/{instanceId}/rules/{id}", h.DeleteAutomationRule)
			// Reannounce (per-instance monitoring on Automations page).
			r.Get("/partials/automations/reannounce/settings/{instanceId}", h.GetReannounceSettingsPartial)
			r.Post("/partials/automations/reannounce/settings/{instanceId}", h.PostReannounceSettings)
			r.Get("/partials/automations/reannounce/activity/{instanceId}", h.GetReannounceActivityPartial)

			// Orphan Scan (per-instance on Automations page).
			r.Get("/partials/automations/orphan-scan/settings/{instanceId}", h.GetOrphanScanSettingsPartial)
			r.Post("/partials/automations/orphan-scan/settings/{instanceId}", h.PostOrphanScanSettings)
			r.Get("/partials/automations/orphan-scan/runs/{instanceId}", h.GetOrphanScanRunsPartial)
			r.Post("/partials/automations/orphan-scan/trigger/{instanceId}", h.PostTriggerOrphanScan)

			// Dir Scan page and partials.
			r.Get("/partials/dir-scan/settings", h.GetDirScanSettingsPartial)
			r.Post("/partials/dir-scan/settings", h.PostDirScanSettings)
			r.Get("/partials/dir-scan/directories", h.GetDirScanDirectoriesPartial)
			r.Get("/partials/dir-scan/directories/form", h.GetDirScanDirectoryForm)
			r.Get("/partials/dir-scan/directories/form/{id}", h.GetDirScanDirectoryForm)
			r.Post("/partials/dir-scan/directories", h.PostDirScanDirectory)
			r.Put("/partials/dir-scan/directories/{id}", h.PutDirScanDirectory)
			r.Delete("/partials/dir-scan/directories/{id}", h.DeleteDirScanDirectory)
			r.Post("/partials/dir-scan/directories/{id}/scan", h.PostDirScanTrigger)
			r.Post("/partials/dir-scan/directories/{id}/cancel", h.PostDirScanCancel)
			r.Get("/partials/dir-scan/runs/{id}", h.GetDirScanRunsPartial)

			// Backups.
			r.Get("/partials/backups/settings/{instanceId}", h.GetBackupSettingsPartial)
			r.Post("/partials/backups/settings/{instanceId}", h.PostBackupSettings)
			r.Get("/partials/backups/runs/{instanceId}", h.GetBackupRunsPartial)
			r.Post("/partials/backups/runs/{instanceId}/trigger", h.PostTriggerBackup)
			r.Delete("/partials/backups/runs/{runId}", h.DeleteBackupRun)
		})
	})

	// Convenience redirect: /ui → /ui/dashboard
	r.Get("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, h.baseURL()+"/ui/dashboard", http.StatusFound)
	})
}
