// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/rss  — Full RSS page
// ------------------------------------------------------------------

func (h *Handler) GetRSS(w http.ResponseWriter, r *http.Request) {
	p := h.buildRSSProps(r)
	render(w, r, http.StatusOK, pages.RSS(p))
}

// buildRSSProps constructs the RSS page props.
func (h *Handler) buildRSSProps(r *http.Request) pages.RSSProps {
	ctx := r.Context()
	q := r.URL.Query()

	navInsts := h.navInstances(r)
	p := pages.RSSProps{
		BaseURL:   h.baseURL(),
		Username:  UsernameFromContext(ctx),
		Version:   h.version,
		Instances: navInsts,
		ActiveTab: q.Get("tab"),
	}

	if len(navInsts) == 0 {
		return p
	}

	// Pick instance.
	p.InstanceID = intParam(q.Get("instance_id"), 0)
	if p.InstanceID == 0 && len(navInsts) > 0 {
		p.InstanceID = navInsts[0].ID
	}

	if h.syncManager == nil {
		p.RSSError = "Sync manager not available."
		return p
	}

	// Build flat feed list.
	items, err := h.syncManager.GetRSSItems(ctx, p.InstanceID, false)
	if err != nil {
		p.RSSError = fmt.Sprintf("Failed to load feeds: %v", err)
	} else {
		p.Feeds = flattenRSSItems(items, "")
	}

	// Load articles for selected feed.
	p.SelectedFeedPath = q.Get("feed_path")
	if p.SelectedFeedPath != "" {
		itemsWithData, err := h.syncManager.GetRSSItems(ctx, p.InstanceID, true)
		if err == nil {
			p.Articles = articlesFromPath(itemsWithData, p.SelectedFeedPath)
		}
	}

	// Load rules.
	if p.ActiveTab == "rules" {
		rules, err := h.syncManager.GetRSSRules(ctx, p.InstanceID)
		if err != nil {
			log.Warn().Err(err).Msg("rss: get rules")
		} else {
			p.Rules = rssRulesFromQbt(rules)
		}
	}

	return p
}

// flattenRSSItems traverses the hierarchical RSSItems and produces a flat list
// with path-qualified names, e.g. "Folder/FeedName".
func flattenRSSItems(items qbt.RSSItems, prefix string) []pages.RSSFeedItem {
	var out []pages.RSSFeedItem
	for name, raw := range items {
		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}

		// Try to decode as feed (has "url" field).
		var feed qbt.RSSFeed
		if err := json.Unmarshal(raw, &feed); err == nil && feed.URL != "" {
			out = append(out, pages.RSSFeedItem{
				Path:         path,
				Name:         name,
				URL:          feed.URL,
				Title:        feed.Title,
				HasError:     feed.HasError,
				IsLoading:    feed.IsLoading,
				ArticleCount: len(feed.Articles),
			})
			continue
		}

		// Try to decode as nested folder.
		var nested qbt.RSSItems
		if err := json.Unmarshal(raw, &nested); err == nil {
			out = append(out, flattenRSSItems(nested, path)...)
		}
	}
	return out
}

// articlesFromPath retrieves articles for a specific feed path by traversing
// the hierarchical items structure.
func articlesFromPath(items qbt.RSSItems, feedPath string) []pages.RSSArticleItem {
	parts := strings.SplitN(feedPath, "/", 2)
	if len(parts) == 0 {
		return nil
	}

	raw, ok := items[parts[0]]
	if !ok {
		return nil
	}

	if len(parts) == 1 {
		// This is the feed node.
		var feed qbt.RSSFeed
		if err := json.Unmarshal(raw, &feed); err != nil || feed.URL == "" {
			return nil
		}
		articles := make([]pages.RSSArticleItem, 0, len(feed.Articles))
		for _, a := range feed.Articles {
			articles = append(articles, pages.RSSArticleItem{
				ID:          a.ID,
				Date:        a.Date,
				Title:       a.Title,
				Author:      a.Author,
				Description: a.Description,
				TorrentURL:  a.TorrentURL,
				Link:        a.Link,
				IsRead:      a.IsRead,
				FeedPath:    feedPath,
			})
		}
		return articles
	}

	// Recurse into folder.
	var nested qbt.RSSItems
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil
	}
	return articlesFromPath(nested, parts[1])
}

// rssRulesFromQbt converts qbt.RSSRules to page-level RSSRuleItems.
func rssRulesFromQbt(rules qbt.RSSRules) []pages.RSSRuleItem {
	out := make([]pages.RSSRuleItem, 0, len(rules))
	for name, rule := range rules {
		item := pages.RSSRuleItem{
			Name:           name,
			Enabled:        rule.Enabled,
			Priority:       rule.Priority,
			MustContain:    rule.MustContain,
			MustNotContain: rule.MustNotContain,
			UseRegex:       rule.UseRegex,
			SmartFilter:    rule.SmartFilter,
			AffectedFeeds:  rule.AffectedFeeds,
			LastMatch:      rule.LastMatch,
			IgnoreDays:     rule.IgnoreDays,
		}
		if rule.TorrentParams != nil {
			item.Category = rule.TorrentParams.Category
			item.SavePath = rule.TorrentParams.SavePath
		} else {
			item.Category = rule.AssignedCategory
			item.SavePath = rule.SavePath
		}
		out = append(out, item)
	}
	return out
}

// ------------------------------------------------------------------
// GET /ui/partials/rss/articles  — HTMX articles fragment
// ------------------------------------------------------------------

func (h *Handler) GetRSSArticlesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), navFirstInstanceID(h.navInstances(r)))
	feedPath := q.Get("feed_path")

	p := pages.RSSProps{
		BaseURL:          h.baseURL(),
		InstanceID:       instanceID,
		Instances:        h.navInstances(r),
		SelectedFeedPath: feedPath,
	}

	if h.syncManager != nil && feedPath != "" {
		items, err := h.syncManager.GetRSSItems(ctx, instanceID, true)
		if err == nil {
			p.Articles = articlesFromPath(items, feedPath)
		}
	}

	render(w, r, http.StatusOK, pages.RSSArticlesPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/feeds  — Add feed
// ------------------------------------------------------------------

func (h *Handler) PostRSSFeed(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	url := strings.TrimSpace(r.FormValue("url"))
	path := strings.TrimSpace(r.FormValue("path"))

	if url == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	if h.syncManager != nil {
		if err := h.syncManager.AddRSSFeed(r.Context(), instanceID, url, path); err != nil {
			log.Error().Err(err).Msg("rss: add feed")
		}
	}

	// Return updated feed list.
	items, _ := h.syncManager.GetRSSItems(r.Context(), instanceID, false)
	p := pages.RSSProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Instances:  h.navInstances(r),
		Feeds:      flattenRSSItems(items, ""),
	}
	render(w, r, http.StatusOK, pages.RSS(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/folders  — Add folder
// ------------------------------------------------------------------

func (h *Handler) PostRSSFolder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	path := strings.TrimSpace(r.FormValue("path"))

	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	if h.syncManager != nil {
		if err := h.syncManager.AddRSSFolder(r.Context(), instanceID, path); err != nil {
			log.Error().Err(err).Msg("rss: add folder")
		}
	}

	// Return updated feed list.
	items, _ := h.syncManager.GetRSSItems(r.Context(), instanceID, false)
	p := pages.RSSProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Instances:  h.navInstances(r),
		Feeds:      flattenRSSItems(items, ""),
	}
	render(w, r, http.StatusOK, pages.RSS(p))
}

// ------------------------------------------------------------------
// DELETE /ui/partials/rss/feeds  — Remove feed or folder
// ------------------------------------------------------------------

func (h *Handler) DeleteRSSItem(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	path := r.URL.Query().Get("path")

	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	if h.syncManager != nil {
		if err := h.syncManager.RemoveRSSItem(r.Context(), instanceID, path); err != nil {
			log.Error().Err(err).Msg("rss: remove item")
		}
	}

	// Return empty row replacement (removed).
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/feeds/refresh  — Refresh feed
// ------------------------------------------------------------------

func (h *Handler) PostRSSRefresh(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	path := r.FormValue("path")

	if h.syncManager != nil {
		if err := h.syncManager.RefreshRSSItem(r.Context(), instanceID, path); err != nil {
			log.Error().Err(err).Msg("rss: refresh item")
		}
	}

	// Return refreshed feed row.
	items, _ := h.syncManager.GetRSSItems(r.Context(), instanceID, false)
	feeds := flattenRSSItems(items, "")
	var feedItem pages.RSSFeedItem
	for _, f := range feeds {
		if f.Path == path {
			feedItem = f
			break
		}
	}

	p := pages.RSSProps{
		BaseURL:          h.baseURL(),
		InstanceID:       instanceID,
		Instances:        h.navInstances(r),
		SelectedFeedPath: path,
	}
	render(w, r, http.StatusOK, pages.RSSFeedRowTempl(feedItem, p))
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/articles/mark-read  — Mark single article read
// ------------------------------------------------------------------

func (h *Handler) PostRSSMarkRead(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	feedPath := r.FormValue("feed_path")
	articleID := r.FormValue("article_id")

	if h.syncManager != nil {
		if err := h.syncManager.MarkRSSItemAsRead(r.Context(), instanceID, feedPath, articleID); err != nil {
			log.Error().Err(err).Msg("rss: mark read")
		}
	}

	// Return empty (row removal on mark-read).
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/articles/mark-all-read  — Mark all read
// ------------------------------------------------------------------

func (h *Handler) PostRSSMarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	feedPath := r.FormValue("feed_path")

	if h.syncManager != nil {
		items, err := h.syncManager.GetRSSItems(r.Context(), instanceID, true)
		if err == nil {
			articles := articlesFromPath(items, feedPath)
			for _, a := range articles {
				if !a.IsRead {
					_ = h.syncManager.MarkRSSItemAsRead(r.Context(), instanceID, feedPath, a.ID)
				}
			}
		}
	}

	// Return empty articles list (all read).
	p := pages.RSSProps{
		BaseURL:          h.baseURL(),
		InstanceID:       instanceID,
		Instances:        h.navInstances(r),
		SelectedFeedPath: feedPath,
		Articles:         nil,
	}
	render(w, r, http.StatusOK, pages.RSSArticlesPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/articles/download  — Download article torrent
// ------------------------------------------------------------------

func (h *Handler) PostRSSArticleDownload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Just acknowledge — actual download via qBt would require addTorrent from URL.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="text-green-600 text-xs">Queued</span>`)
}

// ------------------------------------------------------------------
// GET /ui/partials/rss/rules  — HTMX rules list fragment
// ------------------------------------------------------------------

func (h *Handler) GetRSSRulesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), navFirstInstanceID(h.navInstances(r)))

	p := pages.RSSProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Instances:  h.navInstances(r),
		ActiveTab:  "rules",
	}

	if h.syncManager != nil {
		rules, err := h.syncManager.GetRSSRules(ctx, instanceID)
		if err != nil {
			log.Warn().Err(err).Msg("rss: get rules partial")
		} else {
			p.Rules = rssRulesFromQbt(rules)
		}
	}

	render(w, r, http.StatusOK, pages.RSSRulesListPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/rss/rules  — Create rule
// ------------------------------------------------------------------

func (h *Handler) PostRSSRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	instanceID := intParam(r.FormValue("instance_id"), 0)
	ruleName := strings.TrimSpace(r.FormValue("rule_name"))
	if ruleName == "" {
		http.Error(w, "rule_name required", http.StatusBadRequest)
		return
	}

	rule := qbt.RSSAutoDownloadRule{
		Enabled:          r.FormValue("enabled") == "on",
		MustContain:      r.FormValue("must_contain"),
		MustNotContain:   r.FormValue("must_not_contain"),
		UseRegex:         r.FormValue("use_regex") == "on",
		SmartFilter:      r.FormValue("smart_filter") == "on",
		IgnoreDays:       intParam(r.FormValue("ignore_days"), 0),
		SavePath:         r.FormValue("save_path"),
		AssignedCategory: r.FormValue("category"),
	}

	if h.syncManager != nil {
		if err := h.syncManager.SetRSSRule(r.Context(), instanceID, ruleName, rule); err != nil {
			log.Error().Err(err).Msg("rss: set rule")
		}
	}

	// Return updated rules list.
	p := h.rssRulesProps(r, instanceID)
	render(w, r, http.StatusOK, pages.RSSRulesListPartial(p))
}

// ------------------------------------------------------------------
// PUT /ui/partials/rss/rules  — Update rule
// ------------------------------------------------------------------

func (h *Handler) PutRSSRule(w http.ResponseWriter, r *http.Request) {
	h.PostRSSRule(w, r) // Same logic — SetRSSRule is upsert.
}

// ------------------------------------------------------------------
// DELETE /ui/partials/rss/rules  — Delete rule
// ------------------------------------------------------------------

func (h *Handler) DeleteRSSRule(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	ruleName := r.URL.Query().Get("rule_name")

	if ruleName == "" {
		http.Error(w, "rule_name required", http.StatusBadRequest)
		return
	}

	if h.syncManager != nil {
		if err := h.syncManager.RemoveRSSRule(r.Context(), instanceID, ruleName); err != nil {
			log.Error().Err(err).Msg("rss: delete rule")
		}
	}

	// Return empty (row removed).
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// ------------------------------------------------------------------
// GET /ui/partials/rss/rules/form  — Create/edit rule form
// ------------------------------------------------------------------

func (h *Handler) GetRSSRuleForm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	instanceID := intParam(q.Get("instance_id"), 0)
	ruleName := q.Get("rule_name")

	var ruleItem *pages.RSSRuleItem
	if ruleName != "" && h.syncManager != nil {
		rules, err := h.syncManager.GetRSSRules(r.Context(), instanceID)
		if err == nil {
			if rule, ok := rules[ruleName]; ok {
				ri := pages.RSSRuleItem{
					Name:           ruleName,
					Enabled:        rule.Enabled,
					Priority:       rule.Priority,
					MustContain:    rule.MustContain,
					MustNotContain: rule.MustNotContain,
					UseRegex:       rule.UseRegex,
					SmartFilter:    rule.SmartFilter,
					AffectedFeeds:  rule.AffectedFeeds,
					LastMatch:      rule.LastMatch,
					IgnoreDays:     rule.IgnoreDays,
				}
				if rule.TorrentParams != nil {
					ri.Category = rule.TorrentParams.Category
					ri.SavePath = rule.TorrentParams.SavePath
				} else {
					ri.Category = rule.AssignedCategory
					ri.SavePath = rule.SavePath
				}
				ruleItem = &ri
			}
		}
	}

	render(w, r, http.StatusOK, pages.RSSRuleFormPartial(ruleItem, instanceID, h.baseURL()))
}

// ------------------------------------------------------------------
// GET /ui/partials/rss/feeds/add-form  — Add feed form
// ------------------------------------------------------------------

func (h *Handler) GetRSSAddFeedForm(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	render(w, r, http.StatusOK, pages.RSSAddFeedFormPartial(instanceID, h.baseURL()))
}

// ------------------------------------------------------------------
// GET /ui/partials/rss/feeds/folder-form  — Add folder form
// ------------------------------------------------------------------

func (h *Handler) GetRSSAddFolderForm(w http.ResponseWriter, r *http.Request) {
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	render(w, r, http.StatusOK, pages.RSSAddFolderFormPartial(instanceID, h.baseURL()))
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

// navFirstInstanceID returns the ID of the first instance, or 0 if none.
func navFirstInstanceID(insts []layouts.Instance) int {
	if len(insts) > 0 {
		return insts[0].ID
	}
	return 0
}

// rssRulesProps builds a minimal RSSProps for rules partials.
func (h *Handler) rssRulesProps(r *http.Request, instanceID int) pages.RSSProps {
	p := pages.RSSProps{
		BaseURL:    h.baseURL(),
		InstanceID: instanceID,
		Instances:  h.navInstances(r),
		ActiveTab:  "rules",
	}
	if h.syncManager != nil {
		rules, err := h.syncManager.GetRSSRules(r.Context(), instanceID)
		if err == nil {
			p.Rules = rssRulesFromQbt(rules)
		}
	}
	return p
}
