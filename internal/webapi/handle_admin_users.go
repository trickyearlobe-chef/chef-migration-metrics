// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/admin/users — list all local users
// ---------------------------------------------------------------------------

func (r *Router) handleAdminListUsers(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	if r.authStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"User management is not configured.")
		return
	}

	users, err := r.authStore.ListUsers(req.Context())
	if err != nil {
		r.logf("ERROR", "admin/users: listing users: %v", err)
		WriteInternalError(w, "Failed to list users.")
		return
	}

	// Convert to the API response format (never expose password_hash).
	type userResponse struct {
		Username    string     `json:"username"`
		DisplayName string     `json:"display_name"`
		Email       string     `json:"email,omitempty"`
		Role        string     `json:"role"`
		Provider    string     `json:"provider"`
		Locked      bool       `json:"locked"`
		CreatedAt   time.Time  `json:"created_at"`
		LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	}

	data := make([]userResponse, 0, len(users))
	for _, u := range users {
		ur := userResponse{
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Role:        u.Role,
			Provider:    u.AuthProvider,
			Locked:      u.IsLocked,
			CreatedAt:   u.CreatedAt,
		}
		if !u.LastLoginAt.IsZero() {
			t := u.LastLoginAt
			ur.LastLoginAt = &t
		}
		data = append(data, ur)
	}

	// Use simple pagination over the full list. The user table is expected
	// to be small (tens to low hundreds) so in-memory pagination is fine.
	pg := ParsePagination(req)
	total := len(data)

	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	WritePaginated(w, data[start:end], pg, total)
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/users — create a new local user
// ---------------------------------------------------------------------------

func (r *Router) handleAdminCreateUser(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	if r.authStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"User management is not configured.")
		return
	}

	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// Validate required fields.
	if body.Username == "" {
		WriteBadRequest(w, "username is required.")
		return
	}
	if body.Password == "" {
		WriteBadRequest(w, "password is required.")
		return
	}
	if body.Role == "" {
		body.Role = "viewer"
	}
	if body.Role != "admin" && body.Role != "viewer" {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("Invalid role %q. Must be \"admin\" or \"viewer\".", body.Role))
		return
	}

	// Validate password complexity.
	minLen := r.cfg.Auth.MinPasswordLength
	if minLen <= 0 {
		minLen = 8
	}
	if err := auth.ValidatePassword(body.Password, minLen); err != nil {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, err.Error())
		return
	}

	// Hash the password.
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		r.logf("ERROR", "admin/users: hashing password: %v", err)
		WriteInternalError(w, "Failed to create user.")
		return
	}

	user, err := r.authStore.InsertUser(req.Context(), datastore.InsertUserParams{
		Username:     body.Username,
		DisplayName:  body.DisplayName,
		Email:        body.Email,
		PasswordHash: hash,
		Role:         body.Role,
		AuthProvider: "local",
	})
	if err != nil {
		if errors.Is(err, datastore.ErrAlreadyExists) {
			WriteError(w, http.StatusConflict, "conflict",
				fmt.Sprintf("User %q already exists.", body.Username))
			return
		}
		r.logf("ERROR", "admin/users: creating user: %v", err)
		WriteInternalError(w, "Failed to create user.")
		return
	}

	r.logf("INFO", "admin/users: created user %q (role=%s) by %s",
		user.Username, user.Role, adminUsername(req))

	WriteJSON(w, http.StatusCreated, map[string]any{
		"username":     user.Username,
		"display_name": user.DisplayName,
		"email":        user.Email,
		"role":         user.Role,
		"provider":     user.AuthProvider,
		"locked":       user.IsLocked,
		"created_at":   user.CreatedAt,
	})
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/users/:username — update an existing user
// ---------------------------------------------------------------------------

func (r *Router) handleAdminUpdateUser(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPut) {
		return
	}

	if r.authStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"User management is not configured.")
		return
	}

	username := pathParam(req, "/api/v1/admin/users/")

	// Strip a trailing "/password" if present — that's a different endpoint.
	if hasSuffix(username, "/password") {
		r.handleAdminResetPassword(w, req)
		return
	}

	if username == "" {
		WriteBadRequest(w, "Username is required in the URL path.")
		return
	}

	var body struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Role        *string `json:"role"`
		Locked      *bool   `json:"locked"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// Validate role if provided.
	if body.Role != nil && *body.Role != "admin" && *body.Role != "viewer" {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("Invalid role %q. Must be \"admin\" or \"viewer\".", *body.Role))
		return
	}

	updated, err := r.authStore.UpdateUser(req.Context(), username, datastore.UpdateUserParams{
		DisplayName: body.DisplayName,
		Email:       body.Email,
		Role:        body.Role,
		IsLocked:    body.Locked,
	})
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("User %q not found.", username))
			return
		}
		r.logf("ERROR", "admin/users: updating user %q: %v", username, err)
		WriteInternalError(w, "Failed to update user.")
		return
	}

	// If the account was locked, invalidate all their sessions.
	if body.Locked != nil && *body.Locked && r.sessions != nil {
		if n, sessErr := r.sessions.InvalidateUserSessionsByUsername(req.Context(), username); sessErr != nil {
			r.logf("WARN", "admin/users: failed to invalidate sessions for locked user %q: %v", username, sessErr)
		} else if n > 0 {
			r.logf("INFO", "admin/users: invalidated %d session(s) for locked user %q", n, username)
		}
	}

	r.logf("INFO", "admin/users: updated user %q by %s", username, adminUsername(req))

	var lastLogin *time.Time
	if !updated.LastLoginAt.IsZero() {
		t := updated.LastLoginAt
		lastLogin = &t
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"username":      updated.Username,
		"display_name":  updated.DisplayName,
		"email":         updated.Email,
		"role":          updated.Role,
		"provider":      updated.AuthProvider,
		"locked":        updated.IsLocked,
		"created_at":    updated.CreatedAt,
		"last_login_at": lastLogin,
	})
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/users/:username/password — reset a user's password
// ---------------------------------------------------------------------------

func (r *Router) handleAdminResetPassword(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPut) {
		return
	}

	if r.authStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"User management is not configured.")
		return
	}

	// Extract the username from the path. The path looks like
	// /api/v1/admin/users/bob/password — we need to strip both the
	// prefix and the "/password" suffix.
	raw := pathParam(req, "/api/v1/admin/users/")
	username := trimSuffix(raw, "/password")

	if username == "" {
		WriteBadRequest(w, "Username is required in the URL path.")
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}
	if body.Password == "" {
		WriteBadRequest(w, "password is required.")
		return
	}

	// Validate password complexity.
	minLen := r.cfg.Auth.MinPasswordLength
	if minLen <= 0 {
		minLen = 8
	}
	if err := auth.ValidatePassword(body.Password, minLen); err != nil {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, err.Error())
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		r.logf("ERROR", "admin/users: hashing password for %q: %v", username, err)
		WriteInternalError(w, "Failed to reset password.")
		return
	}

	if err := r.authStore.UpdateUserPassword(req.Context(), username, hash); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("User %q not found.", username))
			return
		}
		r.logf("ERROR", "admin/users: resetting password for %q: %v", username, err)
		WriteInternalError(w, "Failed to reset password.")
		return
	}

	// Invalidate all sessions for this user so they must log in with the
	// new password.
	if r.sessions != nil {
		if n, sessErr := r.sessions.InvalidateUserSessionsByUsername(req.Context(), username); sessErr != nil {
			r.logf("WARN", "admin/users: failed to invalidate sessions after password reset for %q: %v", username, sessErr)
		} else if n > 0 {
			r.logf("INFO", "admin/users: invalidated %d session(s) after password reset for %q", n, username)
		}
	}

	r.logf("INFO", "admin/users: password reset for %q by %s", username, adminUsername(req))

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/users/:username — delete a local user
// ---------------------------------------------------------------------------

func (r *Router) handleAdminDeleteUser(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodDelete) {
		return
	}

	if r.authStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"User management is not configured.")
		return
	}

	username := pathParam(req, "/api/v1/admin/users/")
	if username == "" {
		WriteBadRequest(w, "Username is required in the URL path.")
		return
	}

	// Prevent self-deletion.
	info := auth.SessionFromContext(req.Context())
	if info != nil && info.Username == username {
		WriteError(w, http.StatusConflict, "conflict",
			"You cannot delete your own account.")
		return
	}

	if err := r.authStore.DeleteUser(req.Context(), username); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("User %q not found.", username))
			return
		}
		r.logf("ERROR", "admin/users: deleting user %q: %v", username, err)
		WriteInternalError(w, "Failed to delete user.")
		return
	}

	r.logf("INFO", "admin/users: deleted user %q by %s", username, adminUsername(req))

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Router dispatch for /api/v1/admin/users[/...]
// ---------------------------------------------------------------------------

// handleAdminUsers dispatches admin user requests based on method and path
// depth. This is called by the router for both the collection endpoint
// (/api/v1/admin/users) and per-user endpoints (/api/v1/admin/users/:username).
func (r *Router) handleAdminUsers(w http.ResponseWriter, req *http.Request) {
	// Exact match: /api/v1/admin/users (collection)
	if req.URL.Path == "/api/v1/admin/users" {
		switch req.Method {
		case http.MethodGet:
			r.handleAdminListUsers(w, req)
		case http.MethodPost:
			r.handleAdminCreateUser(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"This endpoint supports GET and POST.")
		}
		return
	}

	// Sub-path: /api/v1/admin/users/:username[/password]
	remainder := pathParam(req, "/api/v1/admin/users/")
	if remainder == "" {
		WriteNotFound(w, "User endpoint requires a username.")
		return
	}

	// Check if this is a password reset request.
	if hasSuffix(remainder, "/password") {
		switch req.Method {
		case http.MethodPut:
			r.handleAdminResetPassword(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"Password reset endpoint supports PUT.")
		}
		return
	}

	// Per-user operations.
	switch req.Method {
	case http.MethodPut:
		r.handleAdminUpdateUser(w, req)
	case http.MethodDelete:
		r.handleAdminDeleteUser(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports PUT and DELETE.")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// adminUsername extracts the acting admin's username from the request context
// for audit logging.
func adminUsername(req *http.Request) string {
	info := auth.SessionFromContext(req.Context())
	if info != nil {
		return info.Username
	}
	return "unknown"
}

// hasSuffix checks whether s ends with suffix.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// trimSuffix removes the suffix from s if present.
func trimSuffix(s, suffix string) string {
	if hasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}
