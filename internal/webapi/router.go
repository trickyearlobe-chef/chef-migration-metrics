// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Router is the top-level HTTP handler for the Chef Migration Metrics API.
// It assembles the ServeMux with all API routes, the WebSocket endpoint,
// health/version endpoints, and the frontend static asset fallback.
type Router struct {
	mux     *http.ServeMux
	hub     *EventHub
	db      DataStore
	cfg     *config.Config
	version string

	// frontendFS holds the built React SPA assets (index.html, JS, CSS).
	// When non-nil, the frontend fallback handler serves files from this
	// filesystem instead of returning a plain-text placeholder. Set via
	// WithFrontendFS at construction time.
	frontendFS fs.FS

	// logger is an optional callback for logging request-level events.
	// If nil, events are silently discarded. The webapi package does not
	// import the logging package to avoid circular dependencies — the
	// caller provides a logging function at construction time.
	logger func(level, msg string)

	// --- Authentication components (set via WithAuth) ---

	// localAuth handles local username/password authentication with
	// brute-force protection. Nil when no local provider is configured.
	localAuth *auth.LocalAuthenticator

	// sessions manages session creation, validation, and invalidation.
	// Nil when authentication is not configured.
	sessions *auth.SessionManager

	// authMiddleware provides RequireAuth and RequireAdmin HTTP middleware.
	// Nil when authentication is not configured.
	authMiddleware *auth.Middleware

	// authStore provides direct user CRUD for the admin user-management
	// endpoints. Nil when authentication is not configured.
	authStore AuthStore
}

// AuthStore is the interface consumed by admin user-management handlers. It
// abstracts the concrete *datastore.DB so that handlers can be tested with
// stubs. The signatures match the corresponding methods on *datastore.DB.
type AuthStore interface {
	InsertUser(ctx context.Context, p datastore.InsertUserParams) (datastore.User, error)
	GetUserByUsername(ctx context.Context, username string) (datastore.User, error)
	GetUserByID(ctx context.Context, id string) (datastore.User, error)
	ListUsers(ctx context.Context) ([]datastore.User, error)
	CountUsers(ctx context.Context) (int, error)
	UpdateUser(ctx context.Context, username string, p datastore.UpdateUserParams) (datastore.User, error)
	UpdateUserPassword(ctx context.Context, username, passwordHash string) error
	DeleteUser(ctx context.Context, username string) error
	IncrementFailedLoginAttempts(ctx context.Context, username string) (int, error)
	LockUser(ctx context.Context, username string) error
	RecordLoginSuccess(ctx context.Context, username string) error
}

// RouterOption is a functional option for NewRouter.
type RouterOption func(*Router)

// WithVersion sets the application version reported by /api/v1/version and
// /api/v1/health.
func WithVersion(v string) RouterOption {
	return func(r *Router) {
		r.version = v
	}
}

// WithLogger sets a logging callback for API lifecycle events.
// The level parameter is one of "DEBUG", "INFO", "WARN", "ERROR".
func WithLogger(fn func(level, msg string)) RouterOption {
	return func(r *Router) {
		r.logger = fn
	}
}

// WithFrontendFS sets the filesystem containing the built React SPA
// assets (typically the Vite output directory). When set, all non-API
// requests are served from this filesystem, with a fallback to
// index.html for client-side routing. When nil, a plain-text
// placeholder is returned instead.
func WithFrontendFS(fsys fs.FS) RouterOption {
	return func(r *Router) {
		r.frontendFS = fsys
	}
}

// WithAuth wires in the authentication components — local authenticator,
// session manager, auth middleware, and user store. When set, the auth
// placeholder routes are replaced with real handlers and protected endpoints
// are wrapped with session enforcement middleware.
func WithAuth(
	localAuth *auth.LocalAuthenticator,
	sessions *auth.SessionManager,
	mw *auth.Middleware,
	store AuthStore,
) RouterOption {
	return func(r *Router) {
		r.localAuth = localAuth
		r.sessions = sessions
		r.authMiddleware = mw
		r.authStore = store
	}
}

// NewRouter creates a new Router with all routes registered. The EventHub
// must already be running (via go hub.Run()) before requests are served.
//
// If WebSocket is disabled in the configuration, the /api/v1/ws endpoint
// returns 404.
func NewRouter(db DataStore, cfg *config.Config, hub *EventHub, opts ...RouterOption) *Router {
	r := &Router{
		mux:     http.NewServeMux(),
		hub:     hub,
		db:      db,
		cfg:     cfg,
		version: "dev",
	}
	for _, opt := range opts {
		opt(r)
	}

	r.registerRoutes()
	return r
}

// ServeHTTP implements http.Handler, delegating to the internal ServeMux.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// Hub returns the EventHub so callers (main, collector, etc.) can broadcast
// events.
func (r *Router) Hub() *EventHub {
	return r.hub
}

// registerRoutes wires all API endpoints into the ServeMux. Routes are
// grouped by concern matching the Web API specification sections.
// protect registers a route that requires authentication (any valid session).
// When authMiddleware is nil (auth not configured), the handler is registered
// without session enforcement so the API remains usable in development.
func (r *Router) protect(pattern string, handler http.HandlerFunc) {
	if r.authMiddleware != nil {
		r.mux.Handle(pattern, r.authMiddleware.Authenticated(handler))
	} else {
		r.mux.HandleFunc(pattern, handler)
	}
}

// adminOnly registers a route that requires authentication AND the admin role.
// When authMiddleware is nil, the handler is registered without enforcement.
func (r *Router) adminOnly(pattern string, handler http.HandlerFunc) {
	if r.authMiddleware != nil {
		r.mux.Handle(pattern, r.authMiddleware.AdminOnly(handler))
	} else {
		r.mux.HandleFunc(pattern, handler)
	}
}

func (r *Router) registerRoutes() {
	// -----------------------------------------------------------------
	// Health & version (public — no auth required)
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/health", r.handleHealth)
	r.mux.HandleFunc("/api/v1/version", r.handleVersion)

	// -----------------------------------------------------------------
	// WebSocket real-time events
	// -----------------------------------------------------------------
	if r.cfg.Server.WebSocket.IsEnabled() {
		wsHandler := NewWebSocketHandler(r.hub, r.webSocketOpts()...)
		r.mux.Handle("/api/v1/ws", wsHandler)
		r.logf("INFO", "WebSocket endpoint enabled at /api/v1/ws (max_connections=%d)",
			r.cfg.Server.WebSocket.MaxConnections)
	} else {
		r.mux.HandleFunc("/api/v1/ws", func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound,
				"WebSocket endpoint is disabled in server configuration.")
		})
		r.logf("INFO", "WebSocket endpoint disabled by configuration")
	}

	// -----------------------------------------------------------------
	// Authentication endpoints (public — no session required for login)
	// -----------------------------------------------------------------
	if r.localAuth != nil && r.sessions != nil {
		r.mux.HandleFunc("/api/v1/auth/login", r.handleLogin)
		r.mux.HandleFunc("/api/v1/auth/logout", r.handleLogout)
		r.protect("/api/v1/auth/me", r.handleMe)
	} else {
		r.mux.HandleFunc("/api/v1/auth/login", r.handleNotImplemented)
		r.mux.HandleFunc("/api/v1/auth/logout", r.handleNotImplemented)
		r.mux.HandleFunc("/api/v1/auth/me", r.handleNotImplemented)
	}
	// SAML endpoints remain placeholders until SAML provider is implemented.
	r.mux.HandleFunc("/api/v1/auth/saml/acs", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/saml/metadata", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/saml/login", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Dashboard endpoints (viewer — any authenticated user)
	// -----------------------------------------------------------------
	r.protect("/api/v1/dashboard/version-distribution", r.handleDashboardVersionDistribution)
	r.protect("/api/v1/dashboard/version-distribution/trend", r.handleDashboardVersionDistributionTrend)
	r.protect("/api/v1/dashboard/readiness", r.handleDashboardReadiness)
	r.protect("/api/v1/dashboard/readiness/trend", r.handleDashboardReadinessTrend)
	r.protect("/api/v1/dashboard/complexity/trend", r.handleDashboardComplexityTrend)
	r.protect("/api/v1/dashboard/stale/trend", r.handleDashboardStaleTrend)
	r.protect("/api/v1/dashboard/cookbook-compatibility", r.handleDashboardCookbookCompatibility)
	r.protect("/api/v1/dashboard/cookbook-download-status", r.handleDashboardCookbookDownloadStatus)

	// -----------------------------------------------------------------
	// Node endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/nodes", r.handleNodes)
	r.protect("/api/v1/nodes/by-version/", r.handleNodesByVersion)
	r.protect("/api/v1/nodes/by-cookbook/", r.handleNodesByCookbook)
	// Node detail: /api/v1/nodes/:organisation/:name — uses a prefix
	// pattern and the handler extracts path segments.
	r.protect("/api/v1/nodes/", r.handleNodeDetail)

	// -----------------------------------------------------------------
	// Cookbook endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/cookbooks", r.handleCookbooks)
	r.protect("/api/v1/cookbooks/", r.handleCookbookDetail)

	// -----------------------------------------------------------------
	// Git repo endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/git-repos", r.handleGitRepos)
	r.protect("/api/v1/git-repos/", r.handleGitRepoDetail)

	// -----------------------------------------------------------------
	// Remediation endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/remediation/priority", r.handleRemediationPriority)
	r.protect("/api/v1/remediation/summary", r.handleRemediationSummary)

	// -----------------------------------------------------------------
	// Dependency graph endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/dependency-graph/table", r.handleDependencyGraphTable)
	r.protect("/api/v1/dependency-graph", r.handleDependencyGraph)

	// -----------------------------------------------------------------
	// Export endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/exports", r.handleExports)
	r.protect("/api/v1/exports/", r.handleExportStatus)

	// -----------------------------------------------------------------
	// Notification endpoints (placeholder — protected for when implemented)
	// -----------------------------------------------------------------
	r.protect("/api/v1/notifications", r.handleNotImplemented)
	r.protect("/api/v1/notifications/", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Organisation endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/organisations", r.handleOrganisations)
	r.protect("/api/v1/organisations/", r.handleOrganisationDetail)

	// -----------------------------------------------------------------
	// Filter option endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/filters/environments", r.handleFilterEnvironments)
	r.protect("/api/v1/filters/roles", r.handleFilterRoles)
	r.protect("/api/v1/filters/policy-names", r.handleFilterPolicyNames)
	r.protect("/api/v1/filters/policy-groups", r.handleFilterPolicyGroups)
	r.protect("/api/v1/filters/platforms", r.handleFilterPlatforms)
	r.protect("/api/v1/filters/target-chef-versions", r.handleFilterTargetChefVersions)
	r.protect("/api/v1/filters/complexity-labels", r.handleFilterComplexityLabels)

	// -----------------------------------------------------------------
	// Log endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/logs", r.handleLogs)
	r.protect("/api/v1/logs/collection-runs", r.handleCollectionRuns)
	r.protect("/api/v1/logs/", r.handleLogDetail)

	// -----------------------------------------------------------------
	// Admin endpoints (admin role required)
	// -----------------------------------------------------------------
	// -----------------------------------------------------------------
	// Ownership endpoints (viewer for reads, operator/admin for writes)
	// -----------------------------------------------------------------
	r.protect("/api/v1/owners", r.handleOwners)
	r.protect("/api/v1/owners/", r.handleOwners)
	r.protect("/api/v1/ownership/reassign", r.handleOwnershipEndpoints)
	r.protect("/api/v1/ownership/lookup", r.handleOwnershipEndpoints)
	r.protect("/api/v1/ownership/audit-log", r.handleOwnershipEndpoints)
	r.protect("/api/v1/ownership/import", r.handleOwnershipEndpoints)

	// -----------------------------------------------------------------
	// Admin endpoints (admin role required)
	// -----------------------------------------------------------------
	r.adminOnly("/api/v1/admin/credentials", r.handleNotImplemented)
	r.adminOnly("/api/v1/admin/credentials/", r.handleNotImplemented)
	if r.authStore != nil {
		r.adminOnly("/api/v1/admin/users", r.handleAdminUsers)
		r.adminOnly("/api/v1/admin/users/", r.handleAdminUsers)
	} else {
		r.adminOnly("/api/v1/admin/users", r.handleNotImplemented)
		r.adminOnly("/api/v1/admin/users/", r.handleNotImplemented)
	}
	r.adminOnly("/api/v1/admin/status", r.handleNotImplemented)
	r.adminOnly("/api/v1/admin/rescan-all-cookstyle", r.handleAdminRescanAllCookstyle)

	// -----------------------------------------------------------------
	// Frontend SPA fallback — serves index.html for client-side routing.
	// Public so the login page can be served without a session.
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/", r.handleFrontendFallback)
}

// webSocketOpts builds the WebSocketHandler options from the loaded config.
func (r *Router) webSocketOpts() []WebSocketHandlerOption {
	wsCfg := r.cfg.Server.WebSocket
	opts := []WebSocketHandlerOption{
		WithWebSocketConfig(WebSocketConfig{
			WriteTimeout: secondsToDuration(wsCfg.WriteTimeoutSeconds),
			PingInterval: secondsToDuration(wsCfg.PingIntervalSeconds),
			PongTimeout:  secondsToDuration(wsCfg.PongTimeoutSeconds),
		}),
	}
	if r.logger != nil {
		opts = append(opts, WithWebSocketLogger(r.logger))
	}
	return opts
}

// -----------------------------------------------------------------
// Health & version handlers
// -----------------------------------------------------------------

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Health endpoint requires GET.")
		return
	}
	if err := r.db.Ping(req.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"unhealthy","error":%q}`, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":            "healthy",
		"version":           r.version,
		"websocket_enabled": r.cfg.Server.WebSocket.IsEnabled(),
		"websocket_clients": r.hub.ClientCount(),
	})
}

func (r *Router) handleVersion(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Version endpoint requires GET.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"version": r.version,
	})
}

// -----------------------------------------------------------------
// Placeholder handler for unimplemented endpoints
// -----------------------------------------------------------------

func (r *Router) handleNotImplemented(w http.ResponseWriter, req *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented",
		fmt.Sprintf("Endpoint %s %s is not yet implemented.", req.Method, req.URL.Path))
}

// -----------------------------------------------------------------
// Frontend SPA fallback
// -----------------------------------------------------------------

func (r *Router) handleFrontendFallback(w http.ResponseWriter, req *http.Request) {
	// Reject non-GET/HEAD for the frontend fallback.
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
		return
	}

	// If the path starts with /api/ but didn't match any registered
	// route, return a proper 404 rather than falling through to the SPA.
	if len(req.URL.Path) >= 5 && req.URL.Path[:5] == "/api/" {
		WriteNotFound(w, fmt.Sprintf("API endpoint %s not found.", req.URL.Path))
		return
	}

	// If the frontend FS is available, serve static assets from it.
	// For paths that don't match a real file, serve index.html so that
	// React Router can handle client-side routes.
	if r.frontendFS != nil {
		r.serveFrontendAsset(w, req)
		return
	}

	// No frontend built — return a plain-text placeholder.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Chef Migration Metrics %s\n\nFrontend not yet built. API available at /api/v1/\n", r.version)
}

// serveFrontendAsset serves a file from the frontend FS if it exists,
// otherwise falls back to index.html for SPA client-side routing.
func (r *Router) serveFrontendAsset(w http.ResponseWriter, req *http.Request) {
	// Clean and strip leading slash to get the FS-relative path.
	p := strings.TrimPrefix(path.Clean(req.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}

	// Try to open the requested path as a real file.
	f, err := r.frontendFS.Open(p)
	if err == nil {
		defer f.Close()
		// Check it's not a directory — if it is, fall through to index.html.
		if stat, statErr := f.Stat(); statErr == nil && !stat.IsDir() {
			http.ServeFileFS(w, req, r.frontendFS, p)
			return
		}
	}

	// Path didn't match a static asset — serve index.html for SPA routing.
	http.ServeFileFS(w, req, r.frontendFS, "index.html")
}

// -----------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------

// secondsToDuration converts an integer number of seconds to a time.Duration.
// If n is zero or negative, it returns a sensible default of 10 seconds.
func secondsToDuration(n int) time.Duration {
	if n <= 0 {
		return 10 * time.Second
	}
	return time.Duration(n) * time.Second
}

// logf logs a formatted message if a logger is configured.
func (r *Router) logf(level, format string, args ...any) {
	if r.logger != nil {
		r.logger(level, fmt.Sprintf(format, args...))
	}
}
