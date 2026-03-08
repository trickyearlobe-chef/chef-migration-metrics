// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Helper: build a Middleware backed by a mockSessionStore
// ---------------------------------------------------------------------------

func newTestMiddleware(store SessionStore) *Middleware {
	sm := NewSessionManager(store, time.Hour)
	return NewMiddleware(sm)
}

func newTestMiddlewareWithLogger(store SessionStore) (*Middleware, *[]string) {
	var logs []string
	sm := NewSessionManager(store, time.Hour)
	mw := NewMiddleware(sm, WithMiddlewareLogger(func(level, msg string) {
		logs = append(logs, level+": "+msg)
	}))
	return mw, &logs
}

// okHandler is a trivial handler that writes 200 OK with a JSON body. It is
// used as the "inner" handler when testing middleware.
func okHandler(w http.ResponseWriter, r *http.Request) {
	info := SessionFromContext(r.Context())
	if info != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"username": info.Username,
			"role":     info.Role,
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// ---------------------------------------------------------------------------
// RequireAuth tests
// ---------------------------------------------------------------------------

func TestRequireAuthNoToken(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error code 'unauthorized', got %q", body["error"])
	}
}

func TestRequireAuthInvalidToken(t *testing.T) {
	// The default mock returns ErrNotFound for any session lookup.
	mw := newTestMiddleware(&mockSessionStore{})

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-abc")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuthValidBearerToken(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			if id == "good-token" {
				return datastore.Session{
					ID:           "good-token",
					UserID:       "user-1",
					Username:     "alice",
					AuthProvider: "local",
					Role:         "admin",
					ExpiresAt:    time.Now().Add(time.Hour),
					CreatedAt:    time.Now().Add(-time.Minute),
				}, nil
			}
			return datastore.Session{}, datastore.ErrNotFound
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["username"] != "alice" {
		t.Errorf("expected username 'alice', got %q", body["username"])
	}
	if body["role"] != "admin" {
		t.Errorf("expected role 'admin', got %q", body["role"])
	}
}

func TestRequireAuthValidCookie(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			if id == "cookie-token" {
				return datastore.Session{
					ID:           "cookie-token",
					UserID:       "user-2",
					Username:     "bob",
					AuthProvider: "local",
					Role:         "viewer",
					ExpiresAt:    time.Now().Add(time.Hour),
					CreatedAt:    time.Now().Add(-time.Minute),
				}, nil
			}
			return datastore.Session{}, datastore.ErrNotFound
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "cookie-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["username"] != "bob" {
		t.Errorf("expected username 'bob', got %q", body["username"])
	}
}

func TestRequireAuthExpiredSession(t *testing.T) {
	// GetValidSession returns ErrNotFound for expired sessions.
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{}, datastore.ErrNotFound
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired session, got %d", w.Code)
	}
}

func TestRequireAuthLogsFailedValidation(t *testing.T) {
	store := &mockSessionStore{} // returns ErrNotFound by default
	mw, logs := newTestMiddlewareWithLogger(store)

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-bad-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	if len(*logs) == 0 {
		t.Error("expected at least one log message for failed validation")
	}
	found := false
	for _, l := range *logs {
		if containsStr(l, "session validation failed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log about session validation failure, got: %v", *logs)
	}
}

// ---------------------------------------------------------------------------
// RequireAuthFunc tests
// ---------------------------------------------------------------------------

func TestRequireAuthFuncWrapsHandlerFunc(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				Username: "charlie",
				Role:     "viewer",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.RequireAuthFunc(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RequireAdmin tests
// ---------------------------------------------------------------------------

func TestRequireAdminWithAdminRole(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	adminHandler := mw.RequireAdmin(inner)

	// Manually set session context (simulates RequireAuth having run).
	info := &SessionInfo{
		SessionID: "s1",
		Username:  "admin-user",
		Role:      "admin",
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/something", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	adminHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d", w.Code)
	}
}

func TestRequireAdminWithViewerRole(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	adminHandler := mw.RequireAdmin(inner)

	info := &SessionInfo{
		SessionID: "s2",
		Username:  "viewer-user",
		Role:      "viewer",
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/something", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	adminHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer user, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "forbidden" {
		t.Errorf("expected error code 'forbidden', got %q", body["error"])
	}
}

func TestRequireAdminNoSession(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	adminHandler := mw.RequireAdmin(inner)

	// No session context — should get 401, not a panic.
	req := httptest.NewRequest(http.MethodGet, "/admin/something", nil)
	w := httptest.NewRecorder()

	adminHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no session, got %d", w.Code)
	}
}

func TestRequireAdminLogsForbidden(t *testing.T) {
	mw, logs := newTestMiddlewareWithLogger(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	adminHandler := mw.RequireAdmin(inner)

	info := &SessionInfo{
		SessionID: "s3",
		Username:  "sneaky-viewer",
		Role:      "viewer",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	adminHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	found := false
	for _, l := range *logs {
		if containsStr(l, "forbidden") && containsStr(l, "sneaky-viewer") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log about forbidden access, got: %v", *logs)
	}
}

// ---------------------------------------------------------------------------
// RequireAdminFunc tests
// ---------------------------------------------------------------------------

func TestRequireAdminFuncWrapsHandlerFunc(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	adminHandler := mw.RequireAdminFunc(okHandler)

	info := &SessionInfo{
		SessionID: "s4",
		Username:  "admin2",
		Role:      "admin",
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	adminHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RequireRole tests
// ---------------------------------------------------------------------------

func TestRequireRoleAllowed(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	roleHandler := mw.RequireRole(inner, "admin", "viewer")

	info := &SessionInfo{SessionID: "s5", Username: "viewer1", Role: "viewer"}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	roleHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed role, got %d", w.Code)
	}
}

func TestRequireRoleDenied(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	roleHandler := mw.RequireRole(inner, "admin") // only admin allowed

	info := &SessionInfo{SessionID: "s6", Username: "viewer2", Role: "viewer"}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	roleHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for disallowed role, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "forbidden" {
		t.Errorf("expected error 'forbidden', got %q", body["error"])
	}
	if !containsStr(body["message"], "admin") {
		t.Errorf("expected message to mention allowed roles, got %q", body["message"])
	}
}

func TestRequireRoleNoSession(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	roleHandler := mw.RequireRole(inner, "admin")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	roleHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no session, got %d", w.Code)
	}
}

func TestRequireRoleUnknownRole(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	inner := http.HandlerFunc(okHandler)
	roleHandler := mw.RequireRole(inner, "admin", "viewer")

	info := &SessionInfo{SessionID: "s7", Username: "hacker", Role: "superuser"}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(ContextWithSession(req.Context(), info))
	w := httptest.NewRecorder()

	roleHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unknown role, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Authenticated (combined) tests
// ---------------------------------------------------------------------------

func TestAuthenticatedCombinedValid(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				UserID:   "u1",
				Username: "combined-user",
				Role:     "viewer",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.Authenticated(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer combined-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["username"] != "combined-user" {
		t.Errorf("expected username 'combined-user', got %q", body["username"])
	}
}

func TestAuthenticatedCombinedNoToken(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	handler := mw.Authenticated(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// AdminOnly (combined) tests
// ---------------------------------------------------------------------------

func TestAdminOnlyCombinedAdminSuccess(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				UserID:   "u-admin",
				Username: "superadmin",
				Role:     "admin",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.AdminOnly(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", w.Code)
	}
}

func TestAdminOnlyCombinedViewerForbidden(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				UserID:   "u-viewer",
				Username: "regular-user",
				Role:     "viewer",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.AdminOnly(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer viewer-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer trying admin endpoint, got %d", w.Code)
	}
}

func TestAdminOnlyCombinedNoToken(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	handler := mw.AdminOnly(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminOnlyCombinedExpiredToken(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{}, datastore.ErrNotFound
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.AdminOnly(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer expired")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Content-Type checks
// ---------------------------------------------------------------------------

func TestMiddlewareResponsesAreJSON(t *testing.T) {
	mw := newTestMiddleware(&mockSessionStore{})

	// 401 response
	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !containsStr(ct, "application/json") {
		t.Errorf("expected JSON Content-Type for 401, got %q", ct)
	}

	// 403 response
	adminHandler := mw.RequireAdmin(http.HandlerFunc(okHandler))
	info := &SessionInfo{SessionID: "s", Username: "v", Role: "viewer"}
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2 = req2.WithContext(ContextWithSession(req2.Context(), info))
	w2 := httptest.NewRecorder()
	adminHandler.ServeHTTP(w2, req2)

	ct2 := w2.Header().Get("Content-Type")
	if !containsStr(ct2, "application/json") {
		t.Errorf("expected JSON Content-Type for 403, got %q", ct2)
	}
}

// ---------------------------------------------------------------------------
// truncateToken tests
// ---------------------------------------------------------------------------

func TestTruncateToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdefghijklmnop", "abcdefgh"},
		{"abcdefgh", "abcdefgh"},
		{"short", "short"},
		{"", ""},
		{"12345678extra", "12345678"},
	}
	for _, tc := range tests {
		got := truncateToken(tc.input)
		if got != tc.expected {
			t.Errorf("truncateToken(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge case: middleware created without logger
// ---------------------------------------------------------------------------

func TestMiddlewareWithoutLoggerDoesNotPanic(t *testing.T) {
	store := &mockSessionStore{} // returns ErrNotFound
	sm := NewSessionManager(store, time.Hour)
	mw := NewMiddleware(sm) // no logger

	handler := mw.RequireAuth(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()

	// Should not panic even without a logger.
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Edge case: chaining RequireAuth → RequireAdmin correctly
// ---------------------------------------------------------------------------

func TestChainedRequireAuthThenRequireAdmin(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				UserID:   "u",
				Username: "chained-admin",
				Role:     "admin",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	// This is effectively what AdminOnly does: RequireAuth wrapping RequireAdmin.
	handler := mw.RequireAuth(mw.RequireAdmin(http.HandlerFunc(okHandler)))
	req := httptest.NewRequest(http.MethodGet, "/chained", nil)
	req.Header.Set("Authorization", "Bearer chain-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for chained admin, got %d", w.Code)
	}

	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["username"] != "chained-admin" {
		t.Errorf("expected username 'chained-admin', got %q", body["username"])
	}
}

func TestChainedRequireAuthThenRequireAdminViewerBlocked(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{
				ID:       id,
				UserID:   "u",
				Username: "chained-viewer",
				Role:     "viewer",
			}, nil
		},
	}
	mw := newTestMiddleware(store)

	handler := mw.RequireAuth(mw.RequireAdmin(http.HandlerFunc(okHandler)))
	req := httptest.NewRequest(http.MethodGet, "/chained", nil)
	req.Header.Set("Authorization", "Bearer chain-viewer-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for chained viewer, got %d", w.Code)
	}
}
