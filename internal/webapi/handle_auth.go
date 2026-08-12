// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

// loginRequest is what a caller sends to sign in.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin handles POST /api/v1/auth/login. It authenticates a local user
// and returns a session token with user information.
func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	// The local authenticator and session manager must be wired in.
	if r.localAuth == nil || r.sessions == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Local authentication is not configured.")
		return
	}

	// Parse request body.
	var body loginRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}
	if body.Username == "" || body.Password == "" {
		WriteBadRequest(w, "Both username and password are required.")
		return
	}

	// Perform the login.
	resp, loginErr := r.localAuth.Login(
		req.Context(),
		r.sessions,
		body.Username,
		body.Password,
		req,
		w,
	)
	if loginErr != nil {
		WriteError(w, loginErr.StatusCode, loginErr.ErrorCode, loginErr.Message)
		return
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handleLogout handles POST /api/v1/auth/logout. It invalidates the current
// session and clears the session cookie.
func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	if r.sessions == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Session management is not configured.")
		return
	}

	token := auth.ExtractToken(req)
	if token == "" {
		// No session to invalidate — still return 204 for idempotency.
		auth.ClearSessionCookie(w, req, r.liveConfig().Server.TrustedProxy)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Invalidate the session in the database.
	if err := r.sessions.InvalidateSession(req.Context(), token); err != nil {
		r.logf("ERROR", "logout: failed to invalidate session: %v", err)
		// Still clear the cookie and return success — the session will
		// expire naturally even if the DB delete fails.
	}

	auth.ClearSessionCookie(w, req, r.liveConfig().Server.TrustedProxy)
	w.WriteHeader(http.StatusNoContent)
}

// handleMe handles GET /api/v1/auth/me. It returns the authenticated user's
// profile and role information extracted from the session context.
func (r *Router) handleMe(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	info := auth.SessionFromContext(req.Context())
	if info == nil {
		WriteUnauthorized(w, "Authentication required.")
		return
	}

	// Look up the full user record for display_name and email. For
	// externally authenticated users (SAML) who may not have a
	// local user row, fall back to the session information.
	type meResponse struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email,omitempty"`
		Role        string `json:"role"`
		Provider    string `json:"provider"`
	}

	resp := meResponse{
		Username: info.Username,
		Role:     info.Role,
		Provider: info.AuthProvider,
	}

	if r.authStore != nil && info.Username != "" {
		user, err := r.authStore.GetUserByUsername(req.Context(), info.Username)
		if err == nil {
			resp.DisplayName = user.DisplayName
			resp.Email = user.Email
		}
	}

	// If we couldn't get the full user record, use the username as the
	// display name.
	if resp.DisplayName == "" {
		resp.DisplayName = info.Username
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handleAuthInfo handles GET /api/v1/auth/info. It returns which
// authentication providers are enabled so the login page can render
// the appropriate UI (local form, SSO button, or both). This endpoint
// is public — no session required.
func (r *Router) handleAuthInfo(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	type authInfoResponse struct {
		LocalEnabled bool `json:"local_enabled"`
		SAMLEnabled  bool `json:"saml_enabled"`
	}

	resp := authInfoResponse{
		LocalEnabled: r.localAuth != nil,
		SAMLEnabled:  r.samlHandler != nil,
	}

	WriteJSON(w, http.StatusOK, resp)
}
