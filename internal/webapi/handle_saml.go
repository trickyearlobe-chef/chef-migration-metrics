// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/jit"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/samlsp"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// SAMLHandler holds the SAML-related HTTP handlers and their dependencies.
type SAMLHandler struct {
	provider    *samlsp.Provider
	provisioner *jit.Provisioner
	sessions    *auth.SessionManager
	userStore   SAMLUserStore
	logger      func(level, msg string)
	trustedProxy bool
}

// SAMLUserStore defines the user store methods needed by the SAML handler.
type SAMLUserStore interface {
	jit.UserStore
	GetUserBySAMLSubject(ctx context.Context, samlSubject string) (datastore.User, error)
	DeleteSAMLSessionsByUsername(ctx context.Context, username string) (int, error)
}

// NewSAMLHandler creates a new SAMLHandler.
func NewSAMLHandler(
	provider *samlsp.Provider,
	provisioner *jit.Provisioner,
	sessions *auth.SessionManager,
	userStore SAMLUserStore,
	trustedProxy bool,
	logger func(level, msg string),
) *SAMLHandler {
	if logger == nil {
		logger = func(string, string) {}
	}
	return &SAMLHandler{
		provider:     provider,
		provisioner:  provisioner,
		sessions:     sessions,
		userStore:    userStore,
		logger:       logger,
		trustedProxy: trustedProxy,
	}
}

// HandleMetadata serves the SP metadata XML at GET /saml/metadata.
func (h *SAMLHandler) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "GET only")
		return
	}

	metaBytes, err := h.provider.Metadata()
	if err != nil {
		h.logger("ERROR", fmt.Sprintf("SAML metadata generation failed: %v", err))
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to generate SP metadata.")
		return
	}

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	// Prepend XML declaration.
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(metaBytes)
}

// HandleLogin initiates SP-initiated SSO at GET /saml/login.
func (h *SAMLHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "GET only")
		return
	}

	// RelayState is the URL to redirect to after SSO completes.
	// Only allow relative paths to prevent open redirect.
	relayState := r.URL.Query().Get("returnTo")
	if relayState != "" && !isRelativePath(relayState) {
		WriteBadRequest(w, "returnTo must be a relative path (starting with '/').")
		return
	}
	if relayState == "" {
		relayState = "/"
	}

	redirectURL, err := h.provider.MakeAuthnRequest(relayState)
	if err != nil {
		h.logger("ERROR", fmt.Sprintf("SAML AuthnRequest creation failed: %v", err))
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to initiate SSO.")
		return
	}

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// HandleACS processes the IdP response at POST /saml/acs.
func (h *SAMLHandler) HandleACS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "POST only")
		return
	}

	// Parse and validate the SAML response.
	userInfo, err := h.provider.ParseACSResponse(r)
	if err != nil {
		h.logger("ERROR", fmt.Sprintf("SAML ACS validation failed: %v", err))
		WriteError(w, http.StatusForbidden, ErrCodeForbidden,
			"SAML assertion validation failed.")
		return
	}

	// JIT provision the user (create or update).
	user, _, provErr := h.provisioner.Provision(r.Context(), userInfo)
	if provErr != nil {
		h.logger("ERROR", fmt.Sprintf("JIT provisioning failed: %v", provErr))
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"User provisioning failed.")
		return
	}

	// Check if user is locked.
	if user.IsLocked {
		h.logger("WARN", fmt.Sprintf("SAML login denied for locked user: %s", user.Username))
		WriteError(w, http.StatusForbidden, ErrCodeForbidden,
			"Account is locked. Contact an administrator.")
		return
	}

	// Create a session.
	session, sessErr := h.sessions.CreateSession(r.Context(), user.Username, "saml", user.Role)
	if sessErr != nil {
		h.logger("ERROR", fmt.Sprintf("Session creation failed after SAML login: %v", sessErr))
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Session creation failed.")
		return
	}

	// Set the session cookie.
	auth.SetSessionCookie(w, r, session.ID, session.ExpiresAt, h.trustedProxy)

	// Redirect to RelayState (validated earlier by the provider as a stored value).
	relayState := r.FormValue("RelayState")
	if relayState == "" || !isRelativePath(relayState) {
		relayState = "/"
	}

	http.Redirect(w, r, relayState, http.StatusFound)
}

// HandleSLO processes inbound Single Logout at POST /saml/slo.
func (h *SAMLHandler) HandleSLO(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "POST only")
		return
	}

	// Parse and validate the LogoutRequest.
	samlSubject, err := h.provider.ParseSLORequest(r)
	if err != nil {
		h.logger("ERROR", fmt.Sprintf("SAML SLO request validation failed: %v", err))
		WriteError(w, http.StatusBadRequest, ErrCodeBadRequest,
			"Invalid LogoutRequest.")
		return
	}

	// Look up the user by SAML subject.
	user, lookupErr := h.userStore.GetUserBySAMLSubject(r.Context(), samlSubject)
	if lookupErr != nil {
		// User not found — return success (idempotent per spec).
		h.logger("INFO", fmt.Sprintf("SAML SLO: no user found for subject %s (idempotent success)", samlSubject))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Delete only SAML sessions for this user (local sessions are preserved).
	n, delErr := h.userStore.DeleteSAMLSessionsByUsername(r.Context(), user.Username)
	if delErr != nil {
		h.logger("ERROR", fmt.Sprintf("SAML SLO: failed to delete sessions for %s: %v", user.Username, delErr))
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to process logout.")
		return
	}

	h.logger("INFO", fmt.Sprintf("SAML SLO completed: user=%s sessions_invalidated=%d", user.Username, n))
	w.WriteHeader(http.StatusOK)
}

// isRelativePath checks if a URL path is relative (starts with /) and does
// not contain a protocol/authority to prevent open redirect.
func isRelativePath(s string) bool {
	if !strings.HasPrefix(s, "/") {
		return false
	}
	// Reject protocol-relative URLs like "//evil.com".
	if strings.HasPrefix(s, "//") {
		return false
	}
	return true
}
