// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// A person's own credentials, from their own record, without raising a ticket.
//
// Everything here is scoped to whoever is asking. There is no address for
// somebody else's credentials — not for an administrator either. Making one is
// an act of handing access to a tool, which only its owner can do, and an
// administrator who needs somebody's access stopped locks or deletes the
// account, which takes every credential with it.

// tokenNameMaxLen bounds the name. Long enough for "laptop editor, project X"
// and short enough that a listing stays readable.
const tokenNameMaxLen = 100

// handleMyTokens serves /api/v1/auth/me/tokens — listing and creating.
func (r *Router) handleMyTokens(w http.ResponseWriter, req *http.Request) {
	username, ok := r.credentialOwner(w, req)
	if !ok {
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.listMyTokens(w, req, username)
	case http.MethodPost:
		r.createMyToken(w, req, username)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This address answers GET and POST.")
	}
}

// handleMyTokenByID serves /api/v1/auth/me/tokens/{id} — destroying one.
func (r *Router) handleMyTokenByID(w http.ResponseWriter, req *http.Request) {
	username, ok := r.credentialOwner(w, req)
	if !ok {
		return
	}

	id := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/auth/me/tokens/"), "/")
	if id == "" || strings.Contains(id, "/") {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound,
			"Name one credential to destroy, by its id.")
		return
	}

	if req.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This address answers DELETE.")
		return
	}

	if err := r.credentials.Destroy(req.Context(), username, id); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			// Also what somebody else's credential looks like. Deliberately
			// the same answer: otherwise this address reports which ids exist.
			WriteError(w, http.StatusNotFound, ErrCodeNotFound,
				"No credential of yours has that id.")
			return
		}
		r.logf("ERROR", "destroying credential %s for %q: %v", id, username, err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to destroy the credential.")
		return
	}

	r.logf("INFO", "user %q destroyed credential %s", username, id)
	w.WriteHeader(http.StatusNoContent)
}

// listMyTokens answers what exists in somebody's name and roughly when each was
// last used. Nothing here can be used as a credential — the secret is not
// stored, so there is none to leak.
func (r *Router) listMyTokens(w http.ResponseWriter, req *http.Request, username string) {
	tokens, err := r.credentials.List(req.Context(), username)
	if err != nil {
		r.logf("ERROR", "listing credentials for %q: %v", username, err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to read your credentials.")
		return
	}
	if tokens == nil {
		tokens = []datastore.APIToken{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// createTokenRequest makes a credential for a tool.
type createTokenRequest struct {
	Name string `json:"name"`
	// CanWrite is chosen here, by whoever is about to hand this to a tool,
	// because only they know what for. Absent means read-only: a caller
	// that never heard of the field cannot get a writing credential.
	CanWrite bool `json:"can_write"`
}

// createMyToken mints one and returns the secret. This is the only response
// that will ever contain it.
func (r *Router) createMyToken(w http.ResponseWriter, req *http.Request, username string) {
	var body createTokenRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Could not read the request: "+err.Error())
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		WriteBadRequest(w, "Give the credential a name — it is how you will recognise it "+
			"later and know which one to destroy.")
		return
	}
	if len(name) > tokenNameMaxLen {
		WriteBadRequest(w, "That name is too long to show in a list. Keep it under 100 characters.")
		return
	}

	tok, secret, err := r.credentials.Issue(req.Context(), username, name, body.CanWrite)
	if err != nil {
		r.logf("ERROR", "issuing credential %q for %q: %v", name, username, err)
		WriteBadRequest(w, "Failed to create the credential. You may already have one by "+
			"that name.")
		return
	}

	r.logf("INFO", "user %q created credential %q (can_write=%t)", username, name, body.CanWrite)

	WriteJSON(w, http.StatusCreated, map[string]any{
		"token": tok,
		// Shown once. Nothing stores it, so this response is the only copy
		// that will ever exist outside whoever is reading it.
		"secret": secret,
		"notice": "Copy this now. It is not stored and cannot be shown again. " +
			"If you lose it, destroy this credential and make another.",
	})
}

// credentialOwner returns the account making the request, or writes the
// refusal and reports false.
func (r *Router) credentialOwner(w http.ResponseWriter, req *http.Request) (string, bool) {
	if r.credentials == nil {
		WriteError(w, http.StatusNotImplemented, "not_implemented",
			"Credentials are not available on this deployment.")
		return "", false
	}
	info := auth.SessionFromContext(req.Context())
	if info == nil || info.Username == "" {
		WriteUnauthorized(w, "Authentication required.")
		return "", false
	}
	return info.Username, true
}
