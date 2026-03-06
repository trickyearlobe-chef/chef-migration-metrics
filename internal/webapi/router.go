// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
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

	// logger is an optional callback for logging request-level events.
	// If nil, events are silently discarded. The webapi package does not
	// import the logging package to avoid circular dependencies — the
	// caller provides a logging function at construction time.
	logger func(level, msg string)
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
	// Authentication (placeholder — will be implemented by auth package)
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/auth/login", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/logout", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/me", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/saml/acs", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/saml/metadata", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/auth/saml/login", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Dashboard endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/dashboard/version-distribution", r.handleDashboardVersionDistribution)
	r.mux.HandleFunc("/api/v1/dashboard/version-distribution/trend", r.handleDashboardVersionDistributionTrend)
	r.mux.HandleFunc("/api/v1/dashboard/readiness", r.handleDashboardReadiness)
	r.mux.HandleFunc("/api/v1/dashboard/readiness/trend", r.handleDashboardReadinessTrend)
	r.mux.HandleFunc("/api/v1/dashboard/cookbook-compatibility", r.handleDashboardCookbookCompatibility)

	// -----------------------------------------------------------------
	// Node endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/nodes", r.handleNodes)
	r.mux.HandleFunc("/api/v1/nodes/by-version/", r.handleNodesByVersion)
	r.mux.HandleFunc("/api/v1/nodes/by-cookbook/", r.handleNodesByCookbook)
	// Node detail: /api/v1/nodes/:organisation/:name — uses a prefix
	// pattern and the handler extracts path segments.
	r.mux.HandleFunc("/api/v1/nodes/", r.handleNodeDetail)

	// -----------------------------------------------------------------
	// Cookbook endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/cookbooks", r.handleCookbooks)
	r.mux.HandleFunc("/api/v1/cookbooks/", r.handleCookbookDetail)

	// -----------------------------------------------------------------
	// Remediation endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/remediation/priority", r.handleRemediationPriority)
	r.mux.HandleFunc("/api/v1/remediation/summary", r.handleRemediationSummary)

	// -----------------------------------------------------------------
	// Dependency graph endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/dependency-graph/table", r.handleDependencyGraphTable)
	r.mux.HandleFunc("/api/v1/dependency-graph", r.handleDependencyGraph)

	// -----------------------------------------------------------------
	// Export endpoints (placeholder)
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/exports", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/exports/", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Notification endpoints (placeholder)
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/notifications", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/notifications/", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Organisation endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/organisations", r.handleOrganisations)
	r.mux.HandleFunc("/api/v1/organisations/", r.handleOrganisationDetail)

	// -----------------------------------------------------------------
	// Filter option endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/filters/environments", r.handleFilterEnvironments)
	r.mux.HandleFunc("/api/v1/filters/roles", r.handleFilterRoles)
	r.mux.HandleFunc("/api/v1/filters/policy-names", r.handleFilterPolicyNames)
	r.mux.HandleFunc("/api/v1/filters/policy-groups", r.handleFilterPolicyGroups)
	r.mux.HandleFunc("/api/v1/filters/platforms", r.handleFilterPlatforms)
	r.mux.HandleFunc("/api/v1/filters/target-chef-versions", r.handleFilterTargetChefVersions)
	r.mux.HandleFunc("/api/v1/filters/complexity-labels", r.handleFilterComplexityLabels)

	// -----------------------------------------------------------------
	// Log endpoints
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/logs", r.handleLogs)
	r.mux.HandleFunc("/api/v1/logs/collection-runs", r.handleCollectionRuns)
	r.mux.HandleFunc("/api/v1/logs/", r.handleLogDetail)

	// -----------------------------------------------------------------
	// Admin endpoints (placeholder)
	// -----------------------------------------------------------------
	r.mux.HandleFunc("/api/v1/admin/credentials", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/admin/credentials/", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/admin/users", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/admin/users/", r.handleNotImplemented)
	r.mux.HandleFunc("/api/v1/admin/status", r.handleNotImplemented)

	// -----------------------------------------------------------------
	// Frontend SPA fallback — serves index.html for client-side routing.
	// For now, return a simple text response; the embedded frontend
	// assets will be wired in when the React app is built.
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

	// For now, return a simple text response. Once the React frontend is
	// built and embedded via go:embed, this will serve index.html and
	// static assets.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Chef Migration Metrics %s\n\nFrontend not yet built. API available at /api/v1/\n", r.version)
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
