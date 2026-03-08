// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// Middleware provides HTTP middleware for session enforcement and role-based
// access control. It validates the session token on every request and
// attaches the SessionInfo to the request context for downstream handlers.
type Middleware struct {
	sessions *SessionManager
	logger   func(level, msg string)
}

// MiddlewareOption is a functional option for NewMiddleware.
type MiddlewareOption func(*Middleware)

// WithMiddlewareLogger sets a logging callback for middleware events.
func WithMiddlewareLogger(fn func(level, msg string)) MiddlewareOption {
	return func(m *Middleware) {
		m.logger = fn
	}
}

// NewMiddleware creates a new authentication middleware backed by the given
// session manager.
func NewMiddleware(sessions *SessionManager, opts ...MiddlewareOption) *Middleware {
	m := &Middleware{
		sessions: sessions,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// RequireAuth returns an http.Handler that enforces authentication. If the
// request carries a valid session token (via Authorization header or session
// cookie), the SessionInfo is attached to the context and the inner handler
// is called. Otherwise a 401 Unauthorized JSON response is returned.
//
// Public endpoints (health, version, login, SAML) should NOT be wrapped with
// this middleware.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			m.writeUnauthorized(w, "Authentication required. Provide a session token via the Authorization header or session cookie.")
			return
		}

		info, err := m.sessions.ValidateSession(r.Context(), token)
		if err != nil {
			m.logf("DEBUG", "session validation failed for token %.8s...: %v", truncateToken(token), err)
			m.writeUnauthorized(w, "Session is invalid or expired. Please log in again.")
			return
		}

		// Attach the session to the request context.
		ctx := ContextWithSession(r.Context(), info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthFunc is a convenience wrapper around RequireAuth that accepts
// an http.HandlerFunc instead of an http.Handler.
func (m *Middleware) RequireAuthFunc(next http.HandlerFunc) http.Handler {
	return m.RequireAuth(next)
}

// RequireAdmin returns an http.Handler that enforces the admin role. The
// request must already have a valid session (i.e. this should be chained
// after RequireAuth). If the session role is not "admin", a 403 Forbidden
// JSON response is returned.
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := SessionFromContext(r.Context())
		if info == nil {
			// Should not happen if RequireAuth is applied first, but
			// be defensive.
			m.writeUnauthorized(w, "Authentication required.")
			return
		}

		if !info.IsAdmin() {
			m.logf("WARN", "forbidden: user %q (role=%s) attempted admin action %s %s",
				info.Username, info.Role, r.Method, r.URL.Path)
			m.writeForbidden(w, "This action requires the admin role.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAdminFunc is a convenience wrapper around RequireAdmin that accepts
// an http.HandlerFunc instead of an http.Handler.
func (m *Middleware) RequireAdminFunc(next http.HandlerFunc) http.Handler {
	return m.RequireAdmin(http.HandlerFunc(next))
}

// RequireRole returns an http.Handler that enforces the request's session
// role is one of the given allowed roles. The request must already have a
// valid session.
func (m *Middleware) RequireRole(next http.Handler, allowedRoles ...string) http.Handler {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := SessionFromContext(r.Context())
		if info == nil {
			m.writeUnauthorized(w, "Authentication required.")
			return
		}

		if !allowed[info.Role] {
			m.logf("WARN", "forbidden: user %q (role=%s) requires one of [%s] for %s %s",
				info.Username, info.Role, strings.Join(allowedRoles, ", "), r.Method, r.URL.Path)
			m.writeForbidden(w, fmt.Sprintf("This action requires one of the following roles: %s.",
				strings.Join(allowedRoles, ", ")))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Authenticated is a combined middleware that enforces authentication and
// passes the request through. This is the standard middleware for
// viewer-level endpoints (any authenticated user).
func (m *Middleware) Authenticated(next http.HandlerFunc) http.Handler {
	return m.RequireAuth(http.HandlerFunc(next))
}

// AdminOnly is a combined middleware that enforces authentication AND the
// admin role. This is the standard middleware for admin-level endpoints.
func (m *Middleware) AdminOnly(next http.HandlerFunc) http.Handler {
	return m.RequireAuth(m.RequireAdmin(http.HandlerFunc(next)))
}

// ---------------------------------------------------------------------------
// JSON error writers
// ---------------------------------------------------------------------------

// writeUnauthorized writes a 401 Unauthorized JSON response.
func (m *Middleware) writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"error":"unauthorized","message":%q}`, message)
}

// writeForbidden writes a 403 Forbidden JSON response.
func (m *Middleware) writeForbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `{"error":"forbidden","message":%q}`, message)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// truncateToken returns the first 8 characters of a token for safe logging.
// If the token is shorter than 8 characters, it is returned as-is with "..."
// appended.
func truncateToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

// logf logs a formatted message if a logger is configured.
func (m *Middleware) logf(level, format string, args ...any) {
	if m.logger != nil {
		m.logger(level, fmt.Sprintf(format, args...))
	}
}
