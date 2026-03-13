// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock session store
// ---------------------------------------------------------------------------

type mockSessionStore struct {
	insertSessionFn            func(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error)
	getValidSessionFn          func(ctx context.Context, id string) (datastore.Session, error)
	deleteSessionFn            func(ctx context.Context, id string) error
	deleteSessionsByUserIDFn   func(ctx context.Context, userID string) (int, error)
	deleteSessionsByUsernameFn func(ctx context.Context, username string) (int, error)
	deleteExpiredSessionsFn    func(ctx context.Context) (int, error)
}

func (m *mockSessionStore) InsertSession(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error) {
	if m.insertSessionFn != nil {
		return m.insertSessionFn(ctx, p)
	}
	return datastore.Session{
		ID:           "sess-uuid-1234",
		UserID:       p.UserID,
		Username:     p.Username,
		AuthProvider: p.AuthProvider,
		Role:         p.Role,
		ExpiresAt:    p.ExpiresAt,
		CreatedAt:    time.Now(),
	}, nil
}

func (m *mockSessionStore) GetValidSession(ctx context.Context, id string) (datastore.Session, error) {
	if m.getValidSessionFn != nil {
		return m.getValidSessionFn(ctx, id)
	}
	return datastore.Session{}, datastore.ErrNotFound
}

func (m *mockSessionStore) DeleteSession(ctx context.Context, id string) error {
	if m.deleteSessionFn != nil {
		return m.deleteSessionFn(ctx, id)
	}
	return nil
}

func (m *mockSessionStore) DeleteSessionsByUserID(ctx context.Context, userID string) (int, error) {
	if m.deleteSessionsByUserIDFn != nil {
		return m.deleteSessionsByUserIDFn(ctx, userID)
	}
	return 0, nil
}

func (m *mockSessionStore) DeleteSessionsByUsername(ctx context.Context, username string) (int, error) {
	if m.deleteSessionsByUsernameFn != nil {
		return m.deleteSessionsByUsernameFn(ctx, username)
	}
	return 0, nil
}

func (m *mockSessionStore) DeleteExpiredSessions(ctx context.Context) (int, error) {
	if m.deleteExpiredSessionsFn != nil {
		return m.deleteExpiredSessionsFn(ctx)
	}
	return 0, nil
}

// compile-time check
var _ SessionStore = (*mockSessionStore)(nil)

// ---------------------------------------------------------------------------
// SessionManager constructor tests
// ---------------------------------------------------------------------------

func TestNewSessionManagerDefaults(t *testing.T) {
	store := &mockSessionStore{}
	sm := NewSessionManager(store, 0)
	if sm.Lifetime() != 8*time.Hour {
		t.Errorf("expected default lifetime 8h, got %s", sm.Lifetime())
	}
}

func TestNewSessionManagerNegativeLifetime(t *testing.T) {
	store := &mockSessionStore{}
	sm := NewSessionManager(store, -5*time.Minute)
	if sm.Lifetime() != 8*time.Hour {
		t.Errorf("expected default lifetime 8h for negative input, got %s", sm.Lifetime())
	}
}

func TestNewSessionManagerCustomLifetime(t *testing.T) {
	store := &mockSessionStore{}
	sm := NewSessionManager(store, 24*time.Hour)
	if sm.Lifetime() != 24*time.Hour {
		t.Errorf("expected lifetime 24h, got %s", sm.Lifetime())
	}
}

func TestNewSessionManagerWithLogger(t *testing.T) {
	var logged bool
	store := &mockSessionStore{}
	sm := NewSessionManager(store, time.Hour, WithSessionLogger(func(level, msg string) {
		logged = true
	}))

	// Creating a session should trigger a log message.
	_, _ = sm.CreateSession(context.Background(), "uid", "alice", "local", "admin")
	if !logged {
		t.Error("expected logger to be called during CreateSession")
	}
}

// ---------------------------------------------------------------------------
// CreateSession tests
// ---------------------------------------------------------------------------

func TestCreateSessionSuccess(t *testing.T) {
	var insertedParams datastore.InsertSessionParams
	store := &mockSessionStore{
		insertSessionFn: func(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error) {
			insertedParams = p
			return datastore.Session{
				ID:           "new-session-id",
				UserID:       p.UserID,
				Username:     p.Username,
				AuthProvider: p.AuthProvider,
				Role:         p.Role,
				ExpiresAt:    p.ExpiresAt,
				CreatedAt:    time.Now(),
			}, nil
		},
	}

	sm := NewSessionManager(store, 2*time.Hour)
	before := time.Now()

	sess, err := sm.CreateSession(context.Background(), "user-123", "alice", "local", "admin")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if sess.ID != "new-session-id" {
		t.Errorf("session ID = %q, want %q", sess.ID, "new-session-id")
	}
	if sess.Username != "alice" {
		t.Errorf("session Username = %q, want %q", sess.Username, "alice")
	}
	if sess.Role != "admin" {
		t.Errorf("session Role = %q, want %q", sess.Role, "admin")
	}

	// The expiry should be approximately 2 hours from now.
	expectedExpiry := before.Add(2 * time.Hour)
	if insertedParams.ExpiresAt.Before(expectedExpiry.Add(-5 * time.Second)) {
		t.Errorf("session expiry is too early: %v", insertedParams.ExpiresAt)
	}
	if insertedParams.ExpiresAt.After(expectedExpiry.Add(5 * time.Second)) {
		t.Errorf("session expiry is too late: %v", insertedParams.ExpiresAt)
	}
}

func TestCreateSessionStoreError(t *testing.T) {
	store := &mockSessionStore{
		insertSessionFn: func(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error) {
			return datastore.Session{}, errors.New("db connection lost")
		},
	}

	sm := NewSessionManager(store, time.Hour)
	_, err := sm.CreateSession(context.Background(), "uid", "alice", "local", "admin")
	if err == nil {
		t.Fatal("expected error when store returns error")
	}
}

// ---------------------------------------------------------------------------
// ValidateSession tests
// ---------------------------------------------------------------------------

func TestValidateSessionSuccess(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			if id != "valid-token" {
				return datastore.Session{}, datastore.ErrNotFound
			}
			return datastore.Session{
				ID:           "valid-token",
				UserID:       "user-1",
				Username:     "bob",
				AuthProvider: "local",
				Role:         "viewer",
				ExpiresAt:    expires,
				CreatedAt:    time.Now().Add(-time.Minute),
			}, nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	info, err := sm.ValidateSession(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if info.SessionID != "valid-token" {
		t.Errorf("SessionID = %q, want %q", info.SessionID, "valid-token")
	}
	if info.Username != "bob" {
		t.Errorf("Username = %q, want %q", info.Username, "bob")
	}
	if info.Role != "viewer" {
		t.Errorf("Role = %q, want %q", info.Role, "viewer")
	}
	if info.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", info.UserID, "user-1")
	}
}

func TestValidateSessionNotFound(t *testing.T) {
	store := &mockSessionStore{} // default returns ErrNotFound

	sm := NewSessionManager(store, time.Hour)
	info, err := sm.ValidateSession(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if info != nil {
		t.Error("expected nil SessionInfo for nonexistent session")
	}
}

func TestValidateSessionEmptyToken(t *testing.T) {
	store := &mockSessionStore{}
	sm := NewSessionManager(store, time.Hour)

	info, err := sm.ValidateSession(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if info != nil {
		t.Error("expected nil SessionInfo for empty token")
	}
}

func TestValidateSessionStoreError(t *testing.T) {
	store := &mockSessionStore{
		getValidSessionFn: func(ctx context.Context, id string) (datastore.Session, error) {
			return datastore.Session{}, errors.New("database is down")
		},
	}

	sm := NewSessionManager(store, time.Hour)
	info, err := sm.ValidateSession(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected error when store returns a non-ErrNotFound error")
	}
	if info != nil {
		t.Error("expected nil SessionInfo on error")
	}
}

// ---------------------------------------------------------------------------
// InvalidateSession tests
// ---------------------------------------------------------------------------

func TestInvalidateSessionSuccess(t *testing.T) {
	deletedID := ""
	store := &mockSessionStore{
		deleteSessionFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	err := sm.InvalidateSession(context.Background(), "token-abc")
	if err != nil {
		t.Fatalf("InvalidateSession returned error: %v", err)
	}
	if deletedID != "token-abc" {
		t.Errorf("deleted session ID = %q, want %q", deletedID, "token-abc")
	}
}

func TestInvalidateSessionAlreadyGone(t *testing.T) {
	store := &mockSessionStore{
		deleteSessionFn: func(ctx context.Context, id string) error {
			return datastore.ErrNotFound
		},
	}

	sm := NewSessionManager(store, time.Hour)
	err := sm.InvalidateSession(context.Background(), "expired-token")
	if err != nil {
		t.Errorf("InvalidateSession should not error for already-gone session, got: %v", err)
	}
}

func TestInvalidateSessionEmptyToken(t *testing.T) {
	store := &mockSessionStore{}
	sm := NewSessionManager(store, time.Hour)

	err := sm.InvalidateSession(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestInvalidateSessionStoreError(t *testing.T) {
	store := &mockSessionStore{
		deleteSessionFn: func(ctx context.Context, id string) error {
			return errors.New("db error")
		},
	}

	sm := NewSessionManager(store, time.Hour)
	err := sm.InvalidateSession(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error when store returns error")
	}
}

// ---------------------------------------------------------------------------
// InvalidateUserSessions tests
// ---------------------------------------------------------------------------

func TestInvalidateUserSessions(t *testing.T) {
	store := &mockSessionStore{
		deleteSessionsByUserIDFn: func(ctx context.Context, userID string) (int, error) {
			if userID == "user-1" {
				return 3, nil
			}
			return 0, nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	n, err := sm.InvalidateUserSessions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("InvalidateUserSessions returned error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 sessions invalidated, got %d", n)
	}
}

func TestInvalidateUserSessionsByUsername(t *testing.T) {
	store := &mockSessionStore{
		deleteSessionsByUsernameFn: func(ctx context.Context, username string) (int, error) {
			if username == "alice" {
				return 2, nil
			}
			return 0, nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	n, err := sm.InvalidateUserSessionsByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("InvalidateUserSessionsByUsername returned error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 sessions invalidated, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// CleanupExpired tests
// ---------------------------------------------------------------------------

func TestCleanupExpired(t *testing.T) {
	store := &mockSessionStore{
		deleteExpiredSessionsFn: func(ctx context.Context) (int, error) {
			return 5, nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	n, err := sm.CleanupExpired(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpired returned error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 expired sessions cleaned up, got %d", n)
	}
}

func TestCleanupExpiredNoSessions(t *testing.T) {
	store := &mockSessionStore{
		deleteExpiredSessionsFn: func(ctx context.Context) (int, error) {
			return 0, nil
		},
	}

	sm := NewSessionManager(store, time.Hour)
	n, err := sm.CleanupExpired(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpired returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestCleanupExpiredStoreError(t *testing.T) {
	store := &mockSessionStore{
		deleteExpiredSessionsFn: func(ctx context.Context) (int, error) {
			return 0, errors.New("cleanup failed")
		},
	}

	sm := NewSessionManager(store, time.Hour)
	_, err := sm.CleanupExpired(context.Background())
	if err == nil {
		t.Fatal("expected error when store returns error")
	}
}

// ---------------------------------------------------------------------------
// SessionInfo tests
// ---------------------------------------------------------------------------

func TestSessionInfoIsAdmin(t *testing.T) {
	tests := []struct {
		role     string
		isAdmin  bool
		isViewer bool
	}{
		{"admin", true, true},
		{"viewer", false, true},
		{"unknown", false, false},
	}
	for _, tc := range tests {
		info := &SessionInfo{Role: tc.role}
		if info.IsAdmin() != tc.isAdmin {
			t.Errorf("SessionInfo{Role: %q}.IsAdmin() = %v, want %v", tc.role, info.IsAdmin(), tc.isAdmin)
		}
		if info.IsViewer() != tc.isViewer {
			t.Errorf("SessionInfo{Role: %q}.IsViewer() = %v, want %v", tc.role, info.IsViewer(), tc.isViewer)
		}
	}
}

// ---------------------------------------------------------------------------
// Context helpers tests
// ---------------------------------------------------------------------------

func TestContextWithSessionAndSessionFromContext(t *testing.T) {
	info := &SessionInfo{
		SessionID: "s1",
		Username:  "alice",
		Role:      "admin",
	}
	ctx := ContextWithSession(context.Background(), info)

	got := SessionFromContext(ctx)
	if got == nil {
		t.Fatal("SessionFromContext returned nil")
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "s1")
	}
	if got.Username != "alice" {
		t.Errorf("Username = %q, want %q", got.Username, "alice")
	}
}

func TestSessionFromContextNil(t *testing.T) {
	got := SessionFromContext(context.Background())
	if got != nil {
		t.Error("expected nil from context with no session")
	}
}

// ---------------------------------------------------------------------------
// ExtractToken tests
// ---------------------------------------------------------------------------

func TestExtractTokenFromBearerHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-session-token")

	token := ExtractToken(req)
	if token != "my-session-token" {
		t.Errorf("ExtractToken = %q, want %q", token, "my-session-token")
	}
}

func TestExtractTokenFromBearerHeaderCaseInsensitive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer my-token")

	token := ExtractToken(req)
	if token != "my-token" {
		t.Errorf("ExtractToken = %q, want %q", token, "my-token")
	}
}

func TestExtractTokenFromBearerHeaderWithExtraSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer   spaced-token  ")

	token := ExtractToken(req)
	if token != "spaced-token" {
		t.Errorf("ExtractToken = %q, want %q", token, "spaced-token")
	}
}

func TestExtractTokenFromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "cookie-token-value",
	})

	token := ExtractToken(req)
	if token != "cookie-token-value" {
		t.Errorf("ExtractToken = %q, want %q", token, "cookie-token-value")
	}
}

func TestExtractTokenBearerTakesPrecedenceOverCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "cookie-token",
	})

	token := ExtractToken(req)
	if token != "bearer-token" {
		t.Errorf("ExtractToken should prefer Bearer header, got %q", token)
	}
}

func TestExtractTokenEmptyWhenNonePresent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	token := ExtractToken(req)
	if token != "" {
		t.Errorf("ExtractToken should return empty string when no token present, got %q", token)
	}
}

func TestExtractTokenIgnoresNonBearerAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	token := ExtractToken(req)
	if token != "" {
		t.Errorf("ExtractToken should return empty for non-Bearer auth, got %q", token)
	}
}

func TestExtractTokenIgnoresEmptyCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "",
	})

	token := ExtractToken(req)
	if token != "" {
		t.Errorf("ExtractToken should return empty for empty cookie, got %q", token)
	}
}

func TestExtractTokenIgnoresWrongCookieName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "other-cookie",
		Value: "some-value",
	})

	token := ExtractToken(req)
	if token != "" {
		t.Errorf("ExtractToken should return empty for wrong cookie name, got %q", token)
	}
}

// ---------------------------------------------------------------------------
// SetSessionCookie / ClearSessionCookie tests
// ---------------------------------------------------------------------------

func TestSetSessionCookie(t *testing.T) {
	t.Run("plain HTTP sets Secure=false", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil) // plain HTTP
		expires := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		SetSessionCookie(w, r, "my-token-value", expires)

		found := findCookie(t, w.Result().Cookies(), "session")
		if found.Value != "my-token-value" {
			t.Errorf("cookie value = %q, want %q", found.Value, "my-token-value")
		}
		if !found.HttpOnly {
			t.Error("expected HttpOnly flag to be set")
		}
		if found.SameSite != http.SameSiteLaxMode {
			t.Errorf("expected SameSite=Lax, got %v", found.SameSite)
		}
		if found.Secure {
			t.Error("expected Secure=false for plain HTTP request")
		}
		if found.Path != "/" {
			t.Errorf("expected Path=/,  got %q", found.Path)
		}
		if !found.Expires.Equal(expires) {
			t.Errorf("cookie Expires = %v, want %v", found.Expires, expires)
		}
	})

	t.Run("TLS request sets Secure=true", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		r.TLS = &tls.ConnectionState{} // simulate TLS connection
		expires := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		SetSessionCookie(w, r, "tls-token", expires)

		found := findCookie(t, w.Result().Cookies(), "session")
		if !found.Secure {
			t.Error("expected Secure=true for TLS request")
		}
	})

	t.Run("X-Forwarded-Proto https sets Secure=true", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-Proto", "https")
		expires := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		SetSessionCookie(w, r, "proxy-token", expires)

		found := findCookie(t, w.Result().Cookies(), "session")
		if !found.Secure {
			t.Error("expected Secure=true when X-Forwarded-Proto is https")
		}
	})
}

func TestClearSessionCookie(t *testing.T) {
	t.Run("plain HTTP sets Secure=false", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		ClearSessionCookie(w, r)

		found := findCookie(t, w.Result().Cookies(), "session")
		if found.Value != "" {
			t.Errorf("expected empty cookie value, got %q", found.Value)
		}
		if found.MaxAge != -1 {
			t.Errorf("expected MaxAge=-1 to expire cookie, got %d", found.MaxAge)
		}
		if !found.HttpOnly {
			t.Error("expected HttpOnly flag to be set on clearing cookie")
		}
		if found.Secure {
			t.Error("expected Secure=false for plain HTTP request")
		}
	})

	t.Run("TLS request sets Secure=true", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		r.TLS = &tls.ConnectionState{}
		ClearSessionCookie(w, r)

		found := findCookie(t, w.Result().Cookies(), "session")
		if !found.Secure {
			t.Error("expected Secure=true for TLS request")
		}
	})
}

// findCookie is a test helper that locates a cookie by name or fails the test.
func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("expected %q cookie to be set", name)
	return nil
}

// ---------------------------------------------------------------------------
// ParseDuration tests
// ---------------------------------------------------------------------------

func TestParseDurationValid(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"8h", 8 * time.Hour},
		{"24h", 24 * time.Hour},
		{"30m", 30 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"2h0m0s", 2 * time.Hour},
	}
	for _, tc := range tests {
		got := ParseDuration(tc.input, time.Hour)
		if got != tc.expected {
			t.Errorf("ParseDuration(%q, 1h) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

func TestParseDurationEmpty(t *testing.T) {
	got := ParseDuration("", 4*time.Hour)
	if got != 4*time.Hour {
		t.Errorf("ParseDuration(\"\", 4h) = %s, want 4h", got)
	}
}

func TestParseDurationInvalid(t *testing.T) {
	got := ParseDuration("not-a-duration", 6*time.Hour)
	if got != 6*time.Hour {
		t.Errorf("ParseDuration(\"not-a-duration\", 6h) = %s, want 6h", got)
	}
}

func TestParseDurationNegative(t *testing.T) {
	got := ParseDuration("-1h", 5*time.Hour)
	if got != 5*time.Hour {
		t.Errorf("ParseDuration(\"-1h\", 5h) = %s, want 5h (negative should fall back to default)", got)
	}
}

func TestParseDurationZero(t *testing.T) {
	got := ParseDuration("0s", 3*time.Hour)
	if got != 3*time.Hour {
		t.Errorf("ParseDuration(\"0s\", 3h) = %s, want 3h (zero should fall back to default)", got)
	}
}
