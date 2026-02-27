// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package ui provides the server-rendered HTTP handlers and templates for the
// qui web interface built with Go templ + templui + HTMX + Alpine.js.
package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/auth"
	"github.com/autogrr/rui/internal/backups"
	"github.com/autogrr/rui/internal/config"
	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/services/arr"
	"github.com/autogrr/rui/internal/services/automations"
	"github.com/autogrr/rui/internal/services/crossseed"
	"github.com/autogrr/rui/internal/services/externalprograms"
	"github.com/autogrr/rui/internal/services/jackett"
	"github.com/autogrr/rui/internal/services/notifications"
	"github.com/autogrr/rui/internal/ui/pages"
)

// OIDCProvider is a minimal interface abstracted to avoid importing the handlers
// package (which would create a circular dependency).
type OIDCProvider interface {
	// GetAuthURLWithState returns the provider's authorization URL and the
	// generated state string. The caller is responsible for persisting the
	// state in the session.
	GetAuthURL() (authURL string, state string)
}

// Handler holds dependencies for the server-rendered UI.
type Handler struct {
	sessionManager *scs.SessionManager
	authService    *auth.Service
	instanceStore  *models.InstanceStore
	cfg            *config.AppConfig
	version        string
	oidcProvider   OIDCProvider             // nil when OIDC is not configured
	syncManager    *qbittorrent.SyncManager // nil if not yet connected

	// Extended services for fully-implemented pages.
	jackettService          *jackett.Service
	indexerStore            *models.TorznabIndexerStore
	arrService              *arr.Service
	arrInstanceStore        *models.ArrInstanceStore
	extProgramService       *externalprograms.Service
	extProgramStore         *models.ExternalProgramStore
	notificationService     *notifications.Service
	notificationTargetStore *models.NotificationTargetStore
	crossSeedService        *crossseed.Service
	crossSeedCompStore      *models.InstanceCrossSeedCompletionStore
	automationService       *automations.Service
	automationStore         *models.AutomationStore
	automationActivityStore *models.AutomationActivityStore
	backupsService          *backups.Service
	clientAPIKeyStore       *models.ClientAPIKeyStore
}

// Dependencies are the inputs required to build a Handler.
type Dependencies struct {
	SessionManager *scs.SessionManager
	AuthService    *auth.Service
	InstanceStore  *models.InstanceStore
	Config         *config.AppConfig
	Version        string
	OIDCProvider   OIDCProvider             // optional
	SyncManager    *qbittorrent.SyncManager // optional; enables live dashboard stats

	// Extended services — all optional; nil disables the relevant page/section.
	JackettService          *jackett.Service
	IndexerStore            *models.TorznabIndexerStore
	ArrService              *arr.Service
	ArrInstanceStore        *models.ArrInstanceStore
	ExtProgramService       *externalprograms.Service
	ExtProgramStore         *models.ExternalProgramStore
	NotificationService     *notifications.Service
	NotificationTargetStore *models.NotificationTargetStore
	CrossSeedService        *crossseed.Service
	CrossSeedCompStore      *models.InstanceCrossSeedCompletionStore
	AutomationService       *automations.Service
	AutomationStore         *models.AutomationStore
	AutomationActivityStore *models.AutomationActivityStore
	BackupsService          *backups.Service
	ClientAPIKeyStore       *models.ClientAPIKeyStore
}

// NewHandler constructs a new UI Handler.
func NewHandler(deps Dependencies) *Handler {
	return &Handler{
		sessionManager: deps.SessionManager,
		authService:    deps.AuthService,
		instanceStore:  deps.InstanceStore,
		cfg:            deps.Config,
		version:        deps.Version,
		oidcProvider:   deps.OIDCProvider,
		syncManager:    deps.SyncManager,

		jackettService:          deps.JackettService,
		indexerStore:            deps.IndexerStore,
		arrService:              deps.ArrService,
		arrInstanceStore:        deps.ArrInstanceStore,
		extProgramService:       deps.ExtProgramService,
		extProgramStore:         deps.ExtProgramStore,
		notificationService:     deps.NotificationService,
		notificationTargetStore: deps.NotificationTargetStore,
		crossSeedService:        deps.CrossSeedService,
		crossSeedCompStore:      deps.CrossSeedCompStore,
		automationService:       deps.AutomationService,
		automationStore:         deps.AutomationStore,
		automationActivityStore: deps.AutomationActivityStore,
		backupsService:          deps.BackupsService,
		clientAPIKeyStore:       deps.ClientAPIKeyStore,
	}
}

// ------------------------------------------------------------------
// Render helpers
// ------------------------------------------------------------------

// render executes a templ component into the response writer with text/html.
func render(w http.ResponseWriter, r *http.Request, status int, t templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.Render(r.Context(), w); err != nil {
		log.Error().Err(err).Msg("templ render error")
	}
}

// baseURL returns the configured base URL (without trailing slash).
func (h *Handler) baseURL() string {
	if h.cfg == nil {
		return ""
	}
	return strings.TrimRight(h.cfg.Config.BaseURL, "/")
}

// ------------------------------------------------------------------
// GET /ui/login
// ------------------------------------------------------------------

func (h *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard.
	if h.sessionManager.GetBool(r.Context(), "authenticated") {
		http.Redirect(w, r, h.baseURL()+"/ui/dashboard", http.StatusSeeOther)
		return
	}

	props := pages.LoginProps{
		BaseURL: h.baseURL(),
		Error:   r.URL.Query().Get("error"),
	}

	if h.cfg != nil && h.cfg.Config.OIDCEnabled && h.oidcProvider != nil {
		authURL, state := h.oidcProvider.GetAuthURL()
		h.sessionManager.Put(r.Context(), "oidc_state", state)
		props.OIDCEnabled = true
		props.OIDCAuthURL = authURL
	}

	// Preserve redirect_to query parameter.
	if to := r.URL.Query().Get("redirect_to"); to != "" {
		props.RedirectTo = to
	}

	render(w, r, http.StatusOK, pages.Login(props))
}

// ------------------------------------------------------------------
// POST /ui/auth/login
// ------------------------------------------------------------------

func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	rememberMe := r.FormValue("remember_me") == "on"
	redirectTo := r.FormValue("redirect_to")

	user, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("ui: login failed")
		redirect := h.baseURL() + "/ui/login?error=Invalid+username+or+password"
		if redirectTo != "" {
			redirect += "&redirect_to=" + redirectTo
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	if err := h.sessionManager.RenewToken(r.Context()); err != nil {
		log.Error().Err(err).Msg("ui: failed to renew session token")
		http.Redirect(w, r, h.baseURL()+"/ui/login?error=Session+error", http.StatusSeeOther)
		return
	}

	h.sessionManager.Put(r.Context(), "authenticated", true)
	h.sessionManager.Put(r.Context(), "user_id", user.ID)
	h.sessionManager.Put(r.Context(), "username", user.Username)
	h.sessionManager.Put(r.Context(), "auth_method", "password")
	h.sessionManager.RememberMe(r.Context(), rememberMe)

	target := h.baseURL() + "/ui/dashboard"
	if redirectTo != "" && strings.HasPrefix(redirectTo, "/") {
		target = redirectTo
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// ------------------------------------------------------------------
// GET /ui/setup
// ------------------------------------------------------------------

func (h *Handler) GetSetup(w http.ResponseWriter, r *http.Request) {
	// Redirect to login if setup already complete.
	complete, err := h.authService.IsSetupComplete(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("ui: failed to check setup status")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if complete {
		http.Redirect(w, r, h.baseURL()+"/ui/login", http.StatusSeeOther)
		return
	}

	render(w, r, http.StatusOK, pages.Setup(pages.SetupProps{
		BaseURL: h.baseURL(),
		Error:   r.URL.Query().Get("error"),
	}))
}

// ------------------------------------------------------------------
// POST /ui/auth/setup
// ------------------------------------------------------------------

func (h *Handler) PostSetup(w http.ResponseWriter, r *http.Request) {
	// Guard: only allowed before setup is complete.
	complete, err := h.authService.IsSetupComplete(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if complete {
		http.Redirect(w, r, h.baseURL()+"/ui/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Basic validation.
	if len(username) < 3 {
		http.Redirect(w, r, h.baseURL()+"/ui/setup?error=Username+must+be+at+least+3+characters", http.StatusSeeOther)
		return
	}
	if len(password) < 8 {
		http.Redirect(w, r, h.baseURL()+"/ui/setup?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
		return
	}
	if password != confirmPassword {
		http.Redirect(w, r, h.baseURL()+"/ui/setup?error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}

	if _, err := h.authService.SetupUser(r.Context(), username, password); err != nil {
		log.Error().Err(err).Msg("ui: setup user failed")
		http.Redirect(w, r, h.baseURL()+"/ui/setup?error=Failed+to+create+account", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, h.baseURL()+"/ui/login", http.StatusSeeOther)
}

// ------------------------------------------------------------------
// POST /ui/auth/logout
// ------------------------------------------------------------------

func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessionManager.Destroy(r.Context()); err != nil {
		log.Error().Err(err).Msg("ui: failed to destroy session")
	}
	http.Redirect(w, r, h.baseURL()+"/ui/login", http.StatusSeeOther)
}

// ------------------------------------------------------------------
// Auth middleware for protected UI routes
// ------------------------------------------------------------------

// RequireAuth redirects unauthenticated requests to the login page.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg != nil && h.cfg.Config.IsAuthDisabled() {
			// Synthetic admin session when auth is disabled.
			ctx := context.WithValue(r.Context(), authUserKey{}, "admin")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if !h.sessionManager.GetBool(r.Context(), "authenticated") {
			http.Redirect(w, r, h.baseURL()+"/ui/login?redirect_to="+r.URL.RequestURI(), http.StatusSeeOther)
			return
		}

		username := h.sessionManager.GetString(r.Context(), "username")
		ctx := context.WithValue(r.Context(), authUserKey{}, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authUserKey is the context key for the authenticated username.
type authUserKey struct{}

// UsernameFromContext returns the authenticated username stored in ctx.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(authUserKey{}).(string)
	return v
}

// ------------------------------------------------------------------
// Setup guard middleware
// ------------------------------------------------------------------

// RequireSetupComplete ensures setup has been done before accessing UI pages.
// If not complete and the request isn't the setup page itself, redirect there.
func (h *Handler) RequireSetupComplete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg != nil && (h.cfg.Config.IsAuthDisabled() || h.cfg.Config.OIDCEnabled) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/ui/setup") ||
			strings.HasSuffix(r.URL.Path, "/ui/auth/setup") {
			next.ServeHTTP(w, r)
			return
		}

		complete, err := h.authService.IsSetupComplete(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("ui: setup check failed")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if !complete {
			http.Redirect(w, r, h.baseURL()+"/ui/setup", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}
