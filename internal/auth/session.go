// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// SessionStore is the interface required by the session manager for
// persistence. It is satisfied by *datastore.DB.
type SessionStore interface {
	InsertSession(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error)
	GetValidSession(ctx context.Context, id string) (datastore.Session, error)
	DeleteSession(ctx context.Context, id string) error
	DeleteSessionsByUserID(ctx context.Context, userID string) (int, error)
	DeleteSessionsByUsername(ctx context.Context, username string) (int, error)
	DeleteExpiredSessions(ctx context.Context) (int, error)
}

// SessionInfo holds the identity and authorisation information extracted from
// a validated session. It is attached to the request context by the auth
// middleware so that downstream handlers can inspect the caller's identity.
type SessionInfo struct {
	SessionID    string
	UserID       string
	Username     string
	AuthProvider string
	Role         string
	ExpiresAt    time.Time
}

// IsAdmin returns true if the session has the admin role.
func (s *SessionInfo) IsAdmin() bool {
	return s.Role == "admin"
}

// IsViewer returns true if the session has the viewer role (any
// authenticated user has at least viewer-level access).
func (s *SessionInfo) IsViewer() bool {
	return s.Role == "viewer" || s.Role == "admin"
}

// contextKey is an unexported type used for context keys to prevent
// collisions with other packages.
type contextKey int

const sessionContextKey contextKey = 0

// ContextWithSession attaches a SessionInfo to the given context.
func ContextWithSession(ctx context.Context, info *SessionInfo) context.Context {
	return context.WithValue(ctx, sessionContextKey, info)
}

// SessionFromContext extracts the SessionInfo from the context. Returns nil
// if no session is present (i.e. the request is unauthenticated).
func SessionFromContext(ctx context.Context) *SessionInfo {
	v, _ := ctx.Value(sessionContextKey).(*SessionInfo)
	return v
}

// SessionManager handles session creation, validation, and invalidation. It
// wraps the SessionStore and applies the configured session lifetime.
type SessionManager struct {
	store    SessionStore
	lifetime time.Duration
	logger   func(level, msg string)
}

// SessionManagerOption is a functional option for NewSessionManager.
type SessionManagerOption func(*SessionManager)

// WithSessionLogger sets a logging callback for session lifecycle events.
func WithSessionLogger(fn func(level, msg string)) SessionManagerOption {
	return func(m *SessionManager) {
		m.logger = fn
	}
}

// NewSessionManager creates a new SessionManager.
//
// The lifetime parameter sets the duration from creation to expiry for new
// sessions. If lifetime is zero or negative, a default of 8 hours is used.
func NewSessionManager(store SessionStore, lifetime time.Duration, opts ...SessionManagerOption) *SessionManager {
	if lifetime <= 0 {
		lifetime = 8 * time.Hour
	}
	m := &SessionManager{
		store:    store,
		lifetime: lifetime,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Lifetime returns the configured session lifetime.
func (m *SessionManager) Lifetime() time.Duration {
	return m.lifetime
}

// CreateSession creates a new session for the given user and returns it. The
// session expiry is set to now + lifetime.
func (m *SessionManager) CreateSession(ctx context.Context, userID, username, authProvider, role string) (datastore.Session, error) {
	expiresAt := time.Now().Add(m.lifetime)

	sess, err := m.store.InsertSession(ctx, datastore.InsertSessionParams{
		UserID:       userID,
		Username:     username,
		AuthProvider: authProvider,
		Role:         role,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return datastore.Session{}, fmt.Errorf("auth: creating session: %w", err)
	}

	m.logf("INFO", "session created for user %q (provider=%s, role=%s, expires=%s)",
		username, authProvider, role, expiresAt.Format(time.RFC3339))
	return sess, nil
}

// ValidateSession looks up a session by its token (UUID) and verifies it has
// not expired. Returns a SessionInfo on success, or an error if the session
// is missing, expired, or invalid.
func (m *SessionManager) ValidateSession(ctx context.Context, token string) (*SessionInfo, error) {
	if token == "" {
		return nil, errors.New("auth: empty session token")
	}

	sess, err := m.store.GetValidSession(ctx, token)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			return nil, errors.New("auth: session not found or expired")
		}
		return nil, fmt.Errorf("auth: validating session: %w", err)
	}

	return &SessionInfo{
		SessionID:    sess.ID,
		UserID:       sess.UserID,
		Username:     sess.Username,
		AuthProvider: sess.AuthProvider,
		Role:         sess.Role,
		ExpiresAt:    sess.ExpiresAt,
	}, nil
}

// InvalidateSession removes a session by its token (explicit logout).
func (m *SessionManager) InvalidateSession(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("auth: empty session token")
	}

	err := m.store.DeleteSession(ctx, token)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			// Session already gone (expired and cleaned up, or double-logout).
			// Not an error from the caller's perspective.
			m.logf("DEBUG", "session %s already invalidated or expired", token)
			return nil
		}
		return fmt.Errorf("auth: invalidating session: %w", err)
	}

	m.logf("INFO", "session %s invalidated", token)
	return nil
}

// InvalidateUserSessions removes all sessions for the given user ID. This is
// used when locking an account, changing a password, or deleting a user.
// Returns the number of sessions removed.
func (m *SessionManager) InvalidateUserSessions(ctx context.Context, userID string) (int, error) {
	n, err := m.store.DeleteSessionsByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("auth: invalidating user sessions: %w", err)
	}
	if n > 0 {
		m.logf("INFO", "invalidated %d session(s) for user_id %s", n, userID)
	}
	return n, nil
}

// InvalidateUserSessionsByUsername removes all sessions for the given
// username. This covers both local and externally authenticated sessions.
// Returns the number of sessions removed.
func (m *SessionManager) InvalidateUserSessionsByUsername(ctx context.Context, username string) (int, error) {
	n, err := m.store.DeleteSessionsByUsername(ctx, username)
	if err != nil {
		return 0, fmt.Errorf("auth: invalidating user sessions by username: %w", err)
	}
	if n > 0 {
		m.logf("INFO", "invalidated %d session(s) for username %q", n, username)
	}
	return n, nil
}

// CleanupExpired removes all expired sessions from the database. This should
// be called periodically (e.g. once per hour or on each collection run).
// Returns the number of sessions removed.
func (m *SessionManager) CleanupExpired(ctx context.Context) (int, error) {
	n, err := m.store.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("auth: cleaning up expired sessions: %w", err)
	}
	if n > 0 {
		m.logf("DEBUG", "cleaned up %d expired session(s)", n)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Token extraction helpers
// ---------------------------------------------------------------------------

// cookieName is the name of the HTTP-only session cookie.
const cookieName = "session"

// ExtractToken extracts the session token from an HTTP request. It checks,
// in order:
//  1. The Authorization header (Bearer token)
//  2. The "session" cookie
//
// Returns an empty string if no token is found.
func ExtractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); auth != "" {
		const prefix = "Bearer "
		if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			return strings.TrimSpace(auth[len(prefix):])
		}
	}

	// 2. Session cookie
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// SetSessionCookie writes a secure, HTTP-only session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

// ClearSessionCookie writes an expired session cookie to the response,
// effectively removing it from the browser.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// logf logs a formatted message if a logger is configured.
func (m *SessionManager) logf(level, format string, args ...any) {
	if m.logger != nil {
		m.logger(level, fmt.Sprintf(format, args...))
	}
}

// ParseDuration parses a session expiry duration string like "8h", "24h",
// "30m", etc. Falls back to the given default if the string is empty or
// invalid.
func ParseDuration(s string, defaultDuration time.Duration) time.Duration {
	if s == "" {
		return defaultDuration
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return defaultDuration
	}
	return d
}
