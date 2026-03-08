// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock local auth store
// ---------------------------------------------------------------------------

type mockLocalAuthStore struct {
	getUserByUsernameFn          func(ctx context.Context, username string) (datastore.User, error)
	incrementFailedLoginFn       func(ctx context.Context, username string) (int, error)
	lockUserFn                   func(ctx context.Context, username string) error
	recordLoginSuccessFn         func(ctx context.Context, username string) error
	incrementCalledWith          string
	incrementCallCount           int
	lockCalledWith               string
	lockCallCount                int
	recordLoginSuccessCalledWith string
	recordLoginSuccessCallCount  int
}

func (m *mockLocalAuthStore) GetUserByUsername(ctx context.Context, username string) (datastore.User, error) {
	if m.getUserByUsernameFn != nil {
		return m.getUserByUsernameFn(ctx, username)
	}
	return datastore.User{}, datastore.ErrNotFound
}

func (m *mockLocalAuthStore) IncrementFailedLoginAttempts(ctx context.Context, username string) (int, error) {
	m.incrementCalledWith = username
	m.incrementCallCount++
	if m.incrementFailedLoginFn != nil {
		return m.incrementFailedLoginFn(ctx, username)
	}
	return 1, nil
}

func (m *mockLocalAuthStore) LockUser(ctx context.Context, username string) error {
	m.lockCalledWith = username
	m.lockCallCount++
	if m.lockUserFn != nil {
		return m.lockUserFn(ctx, username)
	}
	return nil
}

func (m *mockLocalAuthStore) RecordLoginSuccess(ctx context.Context, username string) error {
	m.recordLoginSuccessCalledWith = username
	m.recordLoginSuccessCallCount++
	if m.recordLoginSuccessFn != nil {
		return m.recordLoginSuccessFn(ctx, username)
	}
	return nil
}

// compile-time check
var _ LocalAuthStore = (*mockLocalAuthStore)(nil)

// ---------------------------------------------------------------------------
// Helper to create a valid local user with a known password
// ---------------------------------------------------------------------------

func makeLocalUser(username, password, role string, locked bool, failedAttempts int) datastore.User {
	hash, err := HashPassword(password)
	if err != nil {
		panic("test setup: HashPassword failed: " + err.Error())
	}
	return datastore.User{
		ID:                  "user-id-" + username,
		Username:            username,
		DisplayName:         username + " Display",
		Email:               username + "@example.com",
		PasswordHash:        hash,
		Role:                role,
		AuthProvider:        "local",
		IsLocked:            locked,
		FailedLoginAttempts: failedAttempts,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNewLocalAuthenticatorDefaults(t *testing.T) {
	store := &mockLocalAuthStore{}
	auth := NewLocalAuthenticator(store, 0)
	if auth.LockoutAttempts() != 5 {
		t.Errorf("expected default lockout attempts 5, got %d", auth.LockoutAttempts())
	}
}

func TestNewLocalAuthenticatorNegativeLockout(t *testing.T) {
	store := &mockLocalAuthStore{}
	auth := NewLocalAuthenticator(store, -3)
	if auth.LockoutAttempts() != 5 {
		t.Errorf("expected default lockout attempts 5 for negative input, got %d", auth.LockoutAttempts())
	}
}

func TestNewLocalAuthenticatorCustomLockout(t *testing.T) {
	store := &mockLocalAuthStore{}
	auth := NewLocalAuthenticator(store, 10)
	if auth.LockoutAttempts() != 10 {
		t.Errorf("expected lockout attempts 10, got %d", auth.LockoutAttempts())
	}
}

func TestNewLocalAuthenticatorWithLogger(t *testing.T) {
	var logged bool
	store := &mockLocalAuthStore{}
	_ = NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logged = true
	}))
	// Logger is set but not called during construction.
	if logged {
		t.Error("logger should not be called during construction")
	}
}

// ---------------------------------------------------------------------------
// Authenticate — successful login
// ---------------------------------------------------------------------------

func TestAuthenticateSuccess(t *testing.T) {
	user := makeLocalUser("alice", "S3cretPass!", "admin", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			if username == "alice" {
				return user, nil
			}
			return datastore.User{}, datastore.ErrNotFound
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "alice", "S3cretPass!", "10.0.0.1")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.User.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", result.User.Username)
	}
	if result.User.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", result.User.Role)
	}
	if result.Locked {
		t.Error("expected Locked=false on success")
	}
	if result.FailedAttempts != 0 {
		t.Errorf("expected FailedAttempts=0 on success, got %d", result.FailedAttempts)
	}
	if store.recordLoginSuccessCallCount != 1 {
		t.Errorf("expected RecordLoginSuccess to be called once, called %d times", store.recordLoginSuccessCallCount)
	}
	if store.recordLoginSuccessCalledWith != "alice" {
		t.Errorf("expected RecordLoginSuccess called with 'alice', got %q", store.recordLoginSuccessCalledWith)
	}
}

func TestAuthenticateSuccessResetsFailedAttempts(t *testing.T) {
	// User has 3 prior failed attempts but is not locked.
	user := makeLocalUser("bob", "GoodPass1", "viewer", false, 3)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "bob", "GoodPass1", "10.0.0.2")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if store.recordLoginSuccessCallCount != 1 {
		t.Error("expected RecordLoginSuccess to be called to reset counter")
	}
}

func TestAuthenticateSuccessLogsMessage(t *testing.T) {
	user := makeLocalUser("charlie", "LogTest1!", "viewer", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	result := auth.Authenticate(context.Background(), "charlie", "LogTest1!", "192.168.1.1")
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	found := false
	for _, l := range logMessages {
		if containsStr(l, "login succeeded") && containsStr(l, "charlie") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log about successful login, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — user not found
// ---------------------------------------------------------------------------

func TestAuthenticateUserNotFound(t *testing.T) {
	store := &mockLocalAuthStore{} // default returns ErrNotFound

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "nonexistent", "anything", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure for nonexistent user")
	}
	if !errors.Is(result.Error, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", result.Error)
	}
	// Should NOT reveal that the user doesn't exist.
	if errors.Is(result.Error, ErrUserNotFound) {
		t.Error("error should not be ErrUserNotFound — that would leak user existence")
	}
}

func TestAuthenticateUserNotFoundLogs(t *testing.T) {
	store := &mockLocalAuthStore{}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	_ = auth.Authenticate(context.Background(), "ghost", "pass", "10.0.0.1")

	found := false
	for _, l := range logMessages {
		if containsStr(l, "not found") && containsStr(l, "ghost") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log about user not found, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — store error (non-ErrNotFound)
// ---------------------------------------------------------------------------

func TestAuthenticateStoreError(t *testing.T) {
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return datastore.User{}, errors.New("database connection failed")
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "alice", "pass", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure when store returns error")
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if errors.Is(result.Error, ErrInvalidCredentials) {
		t.Error("store errors should not be wrapped as ErrInvalidCredentials")
	}
}

// ---------------------------------------------------------------------------
// Authenticate — wrong provider
// ---------------------------------------------------------------------------

func TestAuthenticateNonLocalProvider(t *testing.T) {
	ldapUser := datastore.User{
		ID:           "ldap-user-1",
		Username:     "ldap-alice",
		PasswordHash: "irrelevant",
		Role:         "viewer",
		AuthProvider: "ldap",
	}
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return ldapUser, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "ldap-alice", "anything", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure for non-local provider user")
	}
	if !errors.Is(result.Error, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — account locked
// ---------------------------------------------------------------------------

func TestAuthenticateAccountLocked(t *testing.T) {
	user := makeLocalUser("locked-user", "Pass1234", "viewer", true, 5)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "locked-user", "Pass1234", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure for locked account")
	}
	if !errors.Is(result.Error, ErrAccountLocked) {
		t.Errorf("expected ErrAccountLocked, got: %v", result.Error)
	}
	if !result.Locked {
		t.Error("expected Locked=true")
	}
	// Password should not even be checked for locked accounts.
	if store.recordLoginSuccessCallCount != 0 {
		t.Error("RecordLoginSuccess should not be called for locked accounts")
	}
}

func TestAuthenticateLockedAccountWrongPassword(t *testing.T) {
	user := makeLocalUser("locked2", "Pass1234", "viewer", true, 5)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "locked2", "WrongPass1", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure for locked account with wrong password")
	}
	if !errors.Is(result.Error, ErrAccountLocked) {
		t.Errorf("expected ErrAccountLocked (not ErrInvalidCredentials), got: %v", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — wrong password / brute-force
// ---------------------------------------------------------------------------

func TestAuthenticateWrongPassword(t *testing.T) {
	user := makeLocalUser("dave", "Correct1Pass", "viewer", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 1, nil // first failure
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "dave", "WrongPassword1", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure for wrong password")
	}
	if !errors.Is(result.Error, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", result.Error)
	}
	if result.Locked {
		t.Error("account should not be locked after one failure")
	}
	if result.FailedAttempts != 1 {
		t.Errorf("expected FailedAttempts=1, got %d", result.FailedAttempts)
	}
	if store.incrementCallCount != 1 {
		t.Errorf("expected IncrementFailedLoginAttempts called once, called %d times", store.incrementCallCount)
	}
	if store.incrementCalledWith != "dave" {
		t.Errorf("expected IncrementFailedLoginAttempts called with 'dave', got %q", store.incrementCalledWith)
	}
}

func TestAuthenticateWrongPasswordMultipleAttempts(t *testing.T) {
	user := makeLocalUser("eve", "RealPass1", "viewer", false, 0)
	attemptCounter := 0

	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			attemptCounter++
			return attemptCounter, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)

	// Attempts 1-4: should fail with ErrInvalidCredentials, not locked.
	for i := 1; i <= 4; i++ {
		result := auth.Authenticate(context.Background(), "eve", "BadPass1", "10.0.0.1")
		if result.Success {
			t.Fatalf("attempt %d: expected failure", i)
		}
		if !errors.Is(result.Error, ErrInvalidCredentials) {
			t.Errorf("attempt %d: expected ErrInvalidCredentials, got: %v", i, result.Error)
		}
		if result.Locked {
			t.Errorf("attempt %d: should not be locked yet", i)
		}
		if result.FailedAttempts != i {
			t.Errorf("attempt %d: expected FailedAttempts=%d, got %d", i, i, result.FailedAttempts)
		}
	}
}

// ---------------------------------------------------------------------------
// Authenticate — lockout on threshold
// ---------------------------------------------------------------------------

func TestAuthenticateTriggersLockout(t *testing.T) {
	user := makeLocalUser("frank", "MyPass123", "viewer", false, 4) // 4 prior failures
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 5, nil // this is the 5th failure — reaches threshold
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "frank", "WrongPass1", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure")
	}
	if !errors.Is(result.Error, ErrAccountLocked) {
		t.Errorf("expected ErrAccountLocked when threshold is reached, got: %v", result.Error)
	}
	if !result.Locked {
		t.Error("expected Locked=true when threshold is reached")
	}
	if result.FailedAttempts != 5 {
		t.Errorf("expected FailedAttempts=5, got %d", result.FailedAttempts)
	}
	if store.lockCallCount != 1 {
		t.Errorf("expected LockUser to be called once, called %d times", store.lockCallCount)
	}
	if store.lockCalledWith != "frank" {
		t.Errorf("expected LockUser called with 'frank', got %q", store.lockCalledWith)
	}
}

func TestAuthenticateLockoutExceedsThreshold(t *testing.T) {
	// Even if counter returns a number > threshold, lockout should still apply.
	user := makeLocalUser("grace", "MyPass123", "viewer", false, 10)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 11, nil
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "grace", "Wrong1Pass", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure")
	}
	if !errors.Is(result.Error, ErrAccountLocked) {
		t.Errorf("expected ErrAccountLocked, got: %v", result.Error)
	}
	if !result.Locked {
		t.Error("expected Locked=true")
	}
}

func TestAuthenticateLockoutLockUserFails(t *testing.T) {
	// Even if LockUser fails, the result should still indicate lockout.
	user := makeLocalUser("hank", "MyPass123", "viewer", false, 4)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 5, nil
		},
		lockUserFn: func(ctx context.Context, username string) error {
			return errors.New("db write failed")
		},
	}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	result := auth.Authenticate(context.Background(), "hank", "Wrong1Pass", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure")
	}
	if !result.Locked {
		t.Error("result should indicate Locked=true even if LockUser fails")
	}

	// Should have logged the lock failure.
	found := false
	for _, l := range logMessages {
		if containsStr(l, "ERROR") && containsStr(l, "failed to lock") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error log about LockUser failure, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — increment failure errors
// ---------------------------------------------------------------------------

func TestAuthenticateIncrementFails(t *testing.T) {
	user := makeLocalUser("ivan", "MyPass123", "viewer", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 0, errors.New("counter update failed")
		},
	}

	auth := NewLocalAuthenticator(store, 5)
	result := auth.Authenticate(context.Background(), "ivan", "Wrong1Pass", "10.0.0.1")

	if result.Success {
		t.Fatal("expected failure")
	}
	// Should still return invalid credentials even though increment failed.
	if !errors.Is(result.Error, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — RecordLoginSuccess failure (non-fatal)
// ---------------------------------------------------------------------------

func TestAuthenticateRecordLoginSuccessFails(t *testing.T) {
	user := makeLocalUser("judy", "GoodPass1", "viewer", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		recordLoginSuccessFn: func(ctx context.Context, username string) error {
			return errors.New("db write failed")
		},
	}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	result := auth.Authenticate(context.Background(), "judy", "GoodPass1", "10.0.0.1")

	// Login should still succeed even if recording the success fails.
	if !result.Success {
		t.Fatalf("expected success even when RecordLoginSuccess fails, got: %v", result.Error)
	}

	// Should have logged the error.
	found := false
	for _, l := range logMessages {
		if containsStr(l, "ERROR") && containsStr(l, "failed to record login success") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error log about RecordLoginSuccess failure, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — logging for wrong password
// ---------------------------------------------------------------------------

func TestAuthenticateWrongPasswordLogs(t *testing.T) {
	user := makeLocalUser("karl", "Correct1P", "viewer", false, 0)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 2, nil
		},
	}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	_ = auth.Authenticate(context.Background(), "karl", "Wrong1Pass", "192.168.1.100")

	// Should log the failed attempt with IP.
	found := false
	for _, l := range logMessages {
		if containsStr(l, "WARN") && containsStr(l, "invalid password") && containsStr(l, "karl") && containsStr(l, "192.168.1.100") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WARN log about invalid password with IP, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — locked account logs
// ---------------------------------------------------------------------------

func TestAuthenticateLockedAccountLogs(t *testing.T) {
	user := makeLocalUser("lena", "Pass1234", "viewer", true, 5)
	store := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	var logMessages []string
	auth := NewLocalAuthenticator(store, 5, WithLocalAuthLogger(func(level, msg string) {
		logMessages = append(logMessages, level+": "+msg)
	}))

	_ = auth.Authenticate(context.Background(), "lena", "Pass1234", "10.1.1.1")

	found := false
	for _, l := range logMessages {
		if containsStr(l, "WARN") && containsStr(l, "locked") && containsStr(l, "lena") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WARN log about locked account, got: %v", logMessages)
	}
}

// ---------------------------------------------------------------------------
// Login (full flow) tests
// ---------------------------------------------------------------------------

func TestLoginSuccess(t *testing.T) {
	user := makeLocalUser("alice", "Secret1Pass!", "admin", false, 0)
	localStore := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			if username == "alice" {
				return user, nil
			}
			return datastore.User{}, datastore.ErrNotFound
		},
	}

	sessStore := &mockSessionStore{
		insertSessionFn: func(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error) {
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

	localAuth := NewLocalAuthenticator(localStore, 5)
	sessions := NewSessionManager(sessStore, 8*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	w := httptest.NewRecorder()

	resp, loginErr := localAuth.Login(context.Background(), sessions, "alice", "Secret1Pass!", req, w)
	if loginErr != nil {
		t.Fatalf("Login returned error: %v", loginErr)
	}
	if resp == nil {
		t.Fatal("Login returned nil response")
	}
	if resp.Token != "new-session-id" {
		t.Errorf("expected token 'new-session-id', got %q", resp.Token)
	}
	if resp.User.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", resp.User.Username)
	}
	if resp.User.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", resp.User.Role)
	}
	if resp.ExpiresAt.IsZero() {
		t.Error("expected non-zero ExpiresAt")
	}

	// Check that a session cookie was set.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if sessionCookie.Value != "new-session-id" {
		t.Errorf("expected cookie value 'new-session-id', got %q", sessionCookie.Value)
	}
	if !sessionCookie.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	user := makeLocalUser("bob", "Correct1P", "viewer", false, 0)
	localStore := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
		incrementFailedLoginFn: func(ctx context.Context, username string) (int, error) {
			return 1, nil
		},
	}

	sessStore := &mockSessionStore{}
	localAuth := NewLocalAuthenticator(localStore, 5)
	sessions := NewSessionManager(sessStore, 8*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	resp, loginErr := localAuth.Login(context.Background(), sessions, "bob", "WrongPass1!", req, w)
	if resp != nil {
		t.Fatal("expected nil response for invalid credentials")
	}
	if loginErr == nil {
		t.Fatal("expected login error")
	}
	if loginErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", loginErr.StatusCode)
	}
	if loginErr.ErrorCode != "unauthorized" {
		t.Errorf("expected error code 'unauthorized', got %q", loginErr.ErrorCode)
	}
}

func TestLoginAccountLocked(t *testing.T) {
	user := makeLocalUser("locked", "Pass1234", "viewer", true, 5)
	localStore := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	sessStore := &mockSessionStore{}
	localAuth := NewLocalAuthenticator(localStore, 5)
	sessions := NewSessionManager(sessStore, 8*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	resp, loginErr := localAuth.Login(context.Background(), sessions, "locked", "Pass1234", req, w)
	if resp != nil {
		t.Fatal("expected nil response for locked account")
	}
	if loginErr == nil {
		t.Fatal("expected login error")
	}
	if loginErr.StatusCode != http.StatusLocked {
		t.Errorf("expected status 423, got %d", loginErr.StatusCode)
	}
	if loginErr.ErrorCode != "account_locked" {
		t.Errorf("expected error code 'account_locked', got %q", loginErr.ErrorCode)
	}
}

func TestLoginSessionCreationFailure(t *testing.T) {
	user := makeLocalUser("alice", "Good1Pass!", "admin", false, 0)
	localStore := &mockLocalAuthStore{
		getUserByUsernameFn: func(ctx context.Context, username string) (datastore.User, error) {
			return user, nil
		},
	}

	sessStore := &mockSessionStore{
		insertSessionFn: func(ctx context.Context, p datastore.InsertSessionParams) (datastore.Session, error) {
			return datastore.Session{}, errors.New("database full")
		},
	}

	localAuth := NewLocalAuthenticator(localStore, 5)
	sessions := NewSessionManager(sessStore, 8*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	resp, loginErr := localAuth.Login(context.Background(), sessions, "alice", "Good1Pass!", req, w)
	if resp != nil {
		t.Fatal("expected nil response when session creation fails")
	}
	if loginErr == nil {
		t.Fatal("expected login error")
	}
	if loginErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", loginErr.StatusCode)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	localStore := &mockLocalAuthStore{} // default returns ErrNotFound

	sessStore := &mockSessionStore{}
	localAuth := NewLocalAuthenticator(localStore, 5)
	sessions := NewSessionManager(sessStore, 8*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	resp, loginErr := localAuth.Login(context.Background(), sessions, "ghost", "AnyPass1!", req, w)
	if resp != nil {
		t.Fatal("expected nil response for nonexistent user")
	}
	if loginErr == nil {
		t.Fatal("expected login error")
	}
	if loginErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", loginErr.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// LoginError.Error() tests
// ---------------------------------------------------------------------------

func TestLoginErrorMessage(t *testing.T) {
	le := &LoginError{
		StatusCode: 401,
		ErrorCode:  "unauthorized",
		Message:    "Invalid credentials.",
	}
	if le.Error() != "Invalid credentials." {
		t.Errorf("expected 'Invalid credentials.', got %q", le.Error())
	}
}

// ---------------------------------------------------------------------------
// extractSourceIP tests
// ---------------------------------------------------------------------------

func TestExtractSourceIPFromXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")

	ip := extractSourceIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("expected first IP from X-Forwarded-For, got %q", ip)
	}
}

func TestExtractSourceIPFromXForwardedForSingle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	ip := extractSourceIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got %q", ip)
	}
}

func TestExtractSourceIPFromXForwardedForWithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  10.0.0.2 , 10.0.0.3")

	ip := extractSourceIP(req)
	if ip != "10.0.0.2" {
		t.Errorf("expected '10.0.0.2', got %q", ip)
	}
}

func TestExtractSourceIPFromXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "172.16.0.1")

	ip := extractSourceIP(req)
	if ip != "172.16.0.1" {
		t.Errorf("expected '172.16.0.1', got %q", ip)
	}
}

func TestExtractSourceIPFromRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:54321"

	ip := extractSourceIP(req)
	if ip != "192.168.1.50" {
		t.Errorf("expected '192.168.1.50', got %q", ip)
	}
}

func TestExtractSourceIPXForwardedForTakesPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")
	req.RemoteAddr = "9.10.11.12:80"

	ip := extractSourceIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected X-Forwarded-For IP '1.2.3.4', got %q", ip)
	}
}

func TestExtractSourceIPXRealIPOverRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "5.6.7.8")
	req.RemoteAddr = "9.10.11.12:80"

	ip := extractSourceIP(req)
	if ip != "5.6.7.8" {
		t.Errorf("expected X-Real-IP '5.6.7.8', got %q", ip)
	}
}

func TestExtractSourceIPRemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5"

	ip := extractSourceIP(req)
	// net.SplitHostPort fails for no-port addresses, falls back to RemoteAddr.
	if ip != "10.0.0.5" {
		t.Errorf("expected '10.0.0.5', got %q", ip)
	}
}

// ---------------------------------------------------------------------------
// splitFirst tests
// ---------------------------------------------------------------------------

func TestSplitFirst(t *testing.T) {
	tests := []struct {
		s, sep, want string
	}{
		{"a,b,c", ",", "a"},
		{"abc", ",", "abc"},
		{"", ",", ""},
		{",leading", ",", ""},
		{"no-sep-here", "XX", "no-sep-here"},
		{"aXXbXXc", "XX", "a"},
	}
	for _, tc := range tests {
		got := splitFirst(tc.s, tc.sep)
		if got != tc.want {
			t.Errorf("splitFirst(%q, %q) = %q, want %q", tc.s, tc.sep, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// trimSpaces tests
// ---------------------------------------------------------------------------

func TestTrimSpaces(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"  hello  ", "hello"},
		{"  ", ""},
		{"", ""},
		{" a ", "a"},
		{"no-spaces", "no-spaces"},
		{"   leading", "leading"},
		{"trailing   ", "trailing"},
	}
	for _, tc := range tests {
		got := trimSpaces(tc.input)
		if got != tc.want {
			t.Errorf("trimSpaces(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	if ErrInvalidCredentials == nil {
		t.Error("ErrInvalidCredentials should not be nil")
	}
	if ErrAccountLocked == nil {
		t.Error("ErrAccountLocked should not be nil")
	}
	if ErrUserNotFound == nil {
		t.Error("ErrUserNotFound should not be nil")
	}

	// They should be distinct.
	if errors.Is(ErrInvalidCredentials, ErrAccountLocked) {
		t.Error("ErrInvalidCredentials and ErrAccountLocked should be distinct")
	}
	if errors.Is(ErrInvalidCredentials, ErrUserNotFound) {
		t.Error("ErrInvalidCredentials and ErrUserNotFound should be distinct")
	}
}

// ---------------------------------------------------------------------------
// loginErrorFromResult tests
// ---------------------------------------------------------------------------

func TestLoginErrorFromResultLocked(t *testing.T) {
	result := LoginResult{Error: ErrAccountLocked, Locked: true, FailedAttempts: 5}
	le := loginErrorFromResult(result)
	if le.StatusCode != http.StatusLocked {
		t.Errorf("expected status 423, got %d", le.StatusCode)
	}
	if le.ErrorCode != "account_locked" {
		t.Errorf("expected error code 'account_locked', got %q", le.ErrorCode)
	}
}

func TestLoginErrorFromResultLockedViaFlag(t *testing.T) {
	// Even if Error is ErrInvalidCredentials, the Locked flag should win.
	result := LoginResult{Error: ErrInvalidCredentials, Locked: true, FailedAttempts: 5}
	le := loginErrorFromResult(result)
	if le.StatusCode != http.StatusLocked {
		t.Errorf("expected status 423 when Locked=true, got %d", le.StatusCode)
	}
}

func TestLoginErrorFromResultInvalidCredentials(t *testing.T) {
	result := LoginResult{Error: ErrInvalidCredentials, FailedAttempts: 2}
	le := loginErrorFromResult(result)
	if le.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", le.StatusCode)
	}
	if le.ErrorCode != "unauthorized" {
		t.Errorf("expected error code 'unauthorized', got %q", le.ErrorCode)
	}
}

func TestLoginErrorFromResultGenericError(t *testing.T) {
	result := LoginResult{Error: errors.New("something unexpected")}
	le := loginErrorFromResult(result)
	if le.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected default status 401, got %d", le.StatusCode)
	}
}
