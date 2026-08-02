// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// mockSessionStore is a minimal auth.SessionStore implementation for
// WebSocket auth tests. It always returns ErrNotFound on GetValidSession
// so any token presented is treated as invalid.
type mockSessionStore struct{}

func (mockSessionStore) InsertSession(_ context.Context, _ datastore.InsertSessionParams) (datastore.Session, error) {
	return datastore.Session{}, nil
}
func (mockSessionStore) GetValidSession(_ context.Context, _ string) (datastore.Session, error) {
	return datastore.Session{}, datastore.ErrNotFound
}
func (mockSessionStore) DeleteSession(_ context.Context, _ string) error { return nil }
func (mockSessionStore) DeleteSessionsByUsername(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (mockSessionStore) DeleteExpiredSessions(_ context.Context) (int, error) { return 0, nil }

// mockLocalAuthStore is a minimal auth.LocalAuthStore for constructing a
// LocalAuthenticator without a real database.
type mockLocalAuthStore struct{}

func (mockLocalAuthStore) GetUserByUsername(_ context.Context, _ string) (datastore.User, error) {
	return datastore.User{}, errors.New("not found")
}
func (mockLocalAuthStore) IncrementFailedLoginAttempts(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (mockLocalAuthStore) LockUser(_ context.Context, _ string) error           { return nil }
func (mockLocalAuthStore) RecordLoginSuccess(_ context.Context, _ string) error { return nil }

// testRouterWithAuth returns a Router wired with real auth middleware. The
// session store always rejects every token so any request without a session
// cookie/header receives 401.
func testRouterWithAuth() *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()

	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions)
	localAuth := auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5)

	r := NewRouter(nil, cfg, hub,
		WithAuth(localAuth, sessions, mw, nil),
	)
	return r
}

// ---------------------------------------------------------------------------
// WebSocket auth enforcement tests
// ---------------------------------------------------------------------------

// TestWebSocketAuth_UnauthenticatedReturns401 verifies that GET /api/v1/ws
// returns 401 when auth is configured and no session token is present.
func TestWebSocketAuth_UnauthenticatedReturns401(t *testing.T) {
	r := testRouterWithAuth()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/ws without auth: status = %d, want 401", w.Code)
	}
}

// TestWebSocketAuth_InvalidTokenReturns401 verifies that GET /api/v1/ws
// returns 401 when an invalid/expired Bearer token is presented.
func TestWebSocketAuth_InvalidTokenReturns401(t *testing.T) {
	r := testRouterWithAuth()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/ws with invalid token: status = %d, want 401", w.Code)
	}
}

// TestWebSocketAuth_NoAuthConfigured_AllowsUpgrade verifies that when auth is
// NOT configured (dev mode), GET /api/v1/ws does not return 401 (it may fail
// the WebSocket upgrade because the test request is not a real WS handshake,
// but it must not be blocked by auth).
func TestWebSocketAuth_NoAuthConfigured_AllowsUpgrade(t *testing.T) {
	r := testRouter() // no auth

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Without a real WebSocket handshake the upgrade will fail (400), but
	// auth must not be the cause (not 401).
	if w.Code == http.StatusUnauthorized {
		t.Errorf("GET /api/v1/ws without auth configured: got 401, want non-401 (upgrade attempt should pass auth layer)")
	}
}
