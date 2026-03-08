// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// LocalAuthStore is the interface required by the local authenticator for
// user lookups and login bookkeeping. It is satisfied by *datastore.DB.
type LocalAuthStore interface {
	GetUserByUsername(ctx context.Context, username string) (datastore.User, error)
	IncrementFailedLoginAttempts(ctx context.Context, username string) (int, error)
	LockUser(ctx context.Context, username string) error
	RecordLoginSuccess(ctx context.Context, username string) error
}

// LoginResult holds the outcome of a local authentication attempt.
type LoginResult struct {
	// Success is true if the user was authenticated.
	Success bool

	// User is populated on success with the authenticated user record.
	User datastore.User

	// Error is the reason for failure (nil on success).
	Error error

	// Locked is true if the account was locked as a result of this
	// attempt (i.e. the lockout threshold was just crossed).
	Locked bool

	// FailedAttempts is the number of consecutive failed attempts after
	// this login attempt (0 on success).
	FailedAttempts int
}

// Sentinel errors returned by the local authenticator.
var (
	ErrInvalidCredentials = errors.New("auth: invalid username or password")
	ErrAccountLocked      = errors.New("auth: account is locked")
	ErrUserNotFound       = errors.New("auth: user not found")
)

// LocalAuthenticator handles authentication of local user accounts with
// brute-force protection (failed-attempt counting and automatic lockout).
type LocalAuthenticator struct {
	store           LocalAuthStore
	lockoutAttempts int
	logger          func(level, msg string)
}

// LocalAuthOption is a functional option for NewLocalAuthenticator.
type LocalAuthOption func(*LocalAuthenticator)

// WithLocalAuthLogger sets a logging callback for authentication events.
func WithLocalAuthLogger(fn func(level, msg string)) LocalAuthOption {
	return func(a *LocalAuthenticator) {
		a.logger = fn
	}
}

// NewLocalAuthenticator creates a new LocalAuthenticator.
//
// The lockoutAttempts parameter controls how many consecutive failed login
// attempts are allowed before the account is locked. If lockoutAttempts is
// zero or negative, a default of 5 is used.
func NewLocalAuthenticator(store LocalAuthStore, lockoutAttempts int, opts ...LocalAuthOption) *LocalAuthenticator {
	if lockoutAttempts <= 0 {
		lockoutAttempts = 5
	}
	a := &LocalAuthenticator{
		store:           store,
		lockoutAttempts: lockoutAttempts,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// LockoutAttempts returns the configured lockout threshold.
func (a *LocalAuthenticator) LockoutAttempts() int {
	return a.lockoutAttempts
}

// Authenticate verifies the username and password against the local user
// store. It implements brute-force protection by:
//
//  1. Rejecting attempts against locked accounts immediately.
//  2. Incrementing the failed-attempt counter on password mismatch.
//  3. Locking the account when the counter reaches lockoutAttempts.
//  4. Resetting the counter on successful authentication.
//
// The sourceIP parameter is used for logging only and may be empty.
func (a *LocalAuthenticator) Authenticate(ctx context.Context, username, password, sourceIP string) LoginResult {
	// Look up the user by username.
	user, err := a.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			a.logf("WARN", "login failed: user %q not found (ip=%s)", username, sourceIP)
			// Return generic error to avoid user enumeration.
			return LoginResult{Error: ErrInvalidCredentials}
		}
		a.logf("ERROR", "login failed: error looking up user %q: %v", username, err)
		return LoginResult{Error: fmt.Errorf("auth: looking up user: %w", err)}
	}

	// Only local provider users can authenticate here.
	if user.AuthProvider != "local" {
		a.logf("WARN", "login failed: user %q uses provider %q, not local (ip=%s)",
			username, user.AuthProvider, sourceIP)
		return LoginResult{Error: ErrInvalidCredentials}
	}

	// Check if the account is locked.
	if user.IsLocked {
		a.logf("WARN", "login rejected: account %q is locked (ip=%s)", username, sourceIP)
		return LoginResult{Error: ErrAccountLocked, Locked: true}
	}

	// Verify the password.
	if err := CheckPassword(user.PasswordHash, password); err != nil {
		// Password mismatch — increment the failure counter.
		return a.handleFailedAttempt(ctx, user, sourceIP)
	}

	// Password matches — record success and return the user.
	if err := a.store.RecordLoginSuccess(ctx, username); err != nil {
		a.logf("ERROR", "failed to record login success for %q: %v", username, err)
		// Non-fatal — the login itself succeeded.
	}

	a.logf("INFO", "login succeeded: user %q (role=%s, ip=%s)", username, user.Role, sourceIP)

	// Refresh the user record to get the updated last_login_at.
	refreshed, refreshErr := a.store.GetUserByUsername(ctx, username)
	if refreshErr == nil {
		user = refreshed
	}

	return LoginResult{
		Success: true,
		User:    user,
	}
}

// handleFailedAttempt increments the failed-attempt counter and locks the
// account if the threshold is reached.
func (a *LocalAuthenticator) handleFailedAttempt(ctx context.Context, user datastore.User, sourceIP string) LoginResult {
	failedCount, err := a.store.IncrementFailedLoginAttempts(ctx, user.Username)
	if err != nil {
		a.logf("ERROR", "failed to increment failed login attempts for %q: %v", user.Username, err)
		return LoginResult{Error: ErrInvalidCredentials}
	}

	a.logf("WARN", "login failed: invalid password for %q (attempt %d/%d, ip=%s)",
		user.Username, failedCount, a.lockoutAttempts, sourceIP)

	// Lock the account if the threshold has been reached.
	if failedCount >= a.lockoutAttempts {
		if lockErr := a.store.LockUser(ctx, user.Username); lockErr != nil {
			a.logf("ERROR", "failed to lock account %q after %d failed attempts: %v",
				user.Username, failedCount, lockErr)
		} else {
			a.logf("WARN", "account %q locked after %d consecutive failed login attempts (ip=%s)",
				user.Username, failedCount, sourceIP)
		}
		return LoginResult{
			Error:          ErrAccountLocked,
			Locked:         true,
			FailedAttempts: failedCount,
		}
	}

	return LoginResult{
		Error:          ErrInvalidCredentials,
		FailedAttempts: failedCount,
	}
}

// ---------------------------------------------------------------------------
// Full login flow (convenience method)
// ---------------------------------------------------------------------------

// Login performs the full local login flow: authenticate the user, create a
// session, set the session cookie, and return the login response. This is
// the method called by the HTTP handler.
func (a *LocalAuthenticator) Login(
	ctx context.Context,
	sessions *SessionManager,
	username, password string,
	r *http.Request,
	w http.ResponseWriter,
) (*LoginResponse, *LoginError) {
	sourceIP := extractSourceIP(r)

	result := a.Authenticate(ctx, username, password, sourceIP)
	if !result.Success {
		return nil, loginErrorFromResult(result)
	}

	// Create a session for the authenticated user.
	sess, err := sessions.CreateSession(ctx,
		result.User.ID,
		result.User.Username,
		result.User.AuthProvider,
		result.User.Role,
	)
	if err != nil {
		a.logf("ERROR", "failed to create session for %q: %v", username, err)
		return nil, &LoginError{
			StatusCode: http.StatusInternalServerError,
			ErrorCode:  "internal_error",
			Message:    "Failed to create session.",
		}
	}

	// Set the session cookie.
	SetSessionCookie(w, sess.ID, sess.ExpiresAt)

	return &LoginResponse{
		Token:     sess.ID,
		ExpiresAt: sess.ExpiresAt,
		User: LoginUserInfo{
			Username:    result.User.Username,
			DisplayName: result.User.DisplayName,
			Role:        result.User.Role,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// LoginResponse is the JSON body returned on successful login.
type LoginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expires_at"`
	User      LoginUserInfo `json:"user"`
}

// LoginUserInfo is the user information included in a login response.
type LoginUserInfo struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// LoginError encapsulates an authentication failure with the appropriate
// HTTP status code and error code for the API response.
type LoginError struct {
	StatusCode int    // HTTP status code (401, 423, 429, etc.)
	ErrorCode  string // Machine-readable error code
	Message    string // Human-readable message
}

func (e *LoginError) Error() string {
	return e.Message
}

// loginErrorFromResult converts a failed LoginResult into a LoginError.
func loginErrorFromResult(result LoginResult) *LoginError {
	if result.Locked || errors.Is(result.Error, ErrAccountLocked) {
		return &LoginError{
			StatusCode: http.StatusLocked,
			ErrorCode:  "account_locked",
			Message:    "Account is locked due to excessive failed login attempts. Contact an administrator.",
		}
	}

	// Default: invalid credentials.
	return &LoginError{
		StatusCode: http.StatusUnauthorized,
		ErrorCode:  "unauthorized",
		Message:    "Invalid username or password.",
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractSourceIP extracts the client IP address from the request, checking
// X-Forwarded-For and X-Real-IP headers before falling back to RemoteAddr.
func extractSourceIP(r *http.Request) string {
	// Check X-Forwarded-For (may contain multiple IPs; take the first).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := splitFirst(xff, ",")
		ip := trimSpaces(parts)
		if ip != "" {
			return ip
		}
	}

	// Check X-Real-IP.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return trimSpaces(xri)
	}

	// Fall back to RemoteAddr (host:port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// splitFirst splits s on the first occurrence of sep and returns the part
// before the separator. If sep is not found, returns s.
func splitFirst(s, sep string) string {
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i]
		}
	}
	return s
}

// trimSpaces trims leading and trailing ASCII spaces from s.
func trimSpaces(s string) string {
	start := 0
	for start < len(s) && s[start] == ' ' {
		start++
	}
	end := len(s)
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

// logf logs a formatted message if a logger is configured.
func (a *LocalAuthenticator) logf(level, format string, args ...any) {
	if a.logger != nil {
		a.logger(level, fmt.Sprintf(format, args...))
	}
}
