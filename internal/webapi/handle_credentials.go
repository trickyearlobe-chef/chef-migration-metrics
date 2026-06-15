// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// credentialResponse is the API representation of a credential's non-sensitive
// metadata. It is used by list, create, and update responses. The encrypted
// value and plaintext are NEVER included.
type credentialResponse struct {
	Name           string         `json:"name"`
	CredentialType string         `json:"credential_type"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	LastRotatedAt  *time.Time     `json:"last_rotated_at,omitempty"`
	CreatedBy      string         `json:"created_by"`
	UpdatedBy      string         `json:"updated_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Router dispatch for /api/v1/admin/credentials[/...]
// ---------------------------------------------------------------------------

// handleCredentials dispatches admin credential requests based on method and
// path depth. This is called by the router for both the collection endpoint
// (/api/v1/admin/credentials) and per-credential endpoints
// (/api/v1/admin/credentials/:name[/test]).
func (r *Router) handleCredentials(w http.ResponseWriter, req *http.Request) {
	if r.credentialStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Credential storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	// Exact match: /api/v1/admin/credentials (collection)
	if req.URL.Path == "/api/v1/admin/credentials" {
		switch req.Method {
		case http.MethodGet:
			r.handleListCredentials(w, req)
		case http.MethodPost:
			r.handleCreateCredential(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"This endpoint supports GET and POST.")
		}
		return
	}

	// Sub-path: /api/v1/admin/credentials/:name[/test]
	remainder := pathParam(req, "/api/v1/admin/credentials/")
	if remainder == "" {
		WriteNotFound(w, "Credential endpoint requires a name.")
		return
	}

	// Check if this is a test request.
	if hasSuffix(remainder, "/test") {
		switch req.Method {
		case http.MethodPost:
			r.handleTestCredential(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"Test endpoint supports POST.")
		}
		return
	}

	// Per-credential operations.
	switch req.Method {
	case http.MethodPut:
		r.handleUpdateCredential(w, req)
	case http.MethodDelete:
		r.handleDeleteCredential(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports PUT and DELETE.")
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/credentials — list all credentials
// ---------------------------------------------------------------------------

func (r *Router) handleListCredentials(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var (
		creds []secrets.CredentialMetadata
		err   error
	)

	// Optional type filter.
	if typeFilter := req.URL.Query().Get("type"); typeFilter != "" {
		creds, err = r.credentialStore.ListByType(ctx, typeFilter)
		if err != nil {
			if errors.Is(err, secrets.ErrInvalidCredentialType) {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("Invalid credential type %q.", typeFilter))
				return
			}
			r.logf("ERROR", "credentials: listing by type %q: %v", typeFilter, err)
			WriteInternalError(w, "Failed to list credentials.")
			return
		}
	} else {
		creds, err = r.credentialStore.List(ctx)
		if err != nil {
			r.logf("ERROR", "credentials: listing: %v", err)
			WriteInternalError(w, "Failed to list credentials.")
			return
		}
	}

	data := make([]credentialResponse, 0, len(creds))
	for i := range creds {
		data = append(data, credentialToResponse(&creds[i]))
	}

	// In-memory pagination — the credential table is expected to be small.
	pg := ParsePagination(req)
	page, total := PaginateSlice(data, pg)

	WritePaginated(w, page, pg, total)
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/credentials — create a new credential
// ---------------------------------------------------------------------------

func (r *Router) handleCreateCredential(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Name           string `json:"name"`
		CredentialType string `json:"credential_type"`
		Value          string `json:"value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.Name == "" {
		WriteBadRequest(w, "name is required.")
		return
	}
	if body.CredentialType == "" {
		WriteBadRequest(w, "credential_type is required.")
		return
	}
	if !secrets.IsValidCredentialType(body.CredentialType) {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("Invalid credential type %q.", body.CredentialType))
		return
	}
	if body.Value == "" {
		WriteBadRequest(w, "value is required.")
		return
	}

	input := secrets.CreateCredentialInput{
		Name:           body.Name,
		CredentialType: body.CredentialType,
		Plaintext:      []byte(body.Value),
		CreatedBy:      adminUsername(req),
	}
	meta, err := r.credentialStore.Create(req.Context(), input)
	secrets.ZeroBytes(input.Plaintext)

	if err != nil {
		if errors.Is(err, secrets.ErrCredentialAlreadyExists) {
			WriteError(w, http.StatusConflict, "conflict",
				fmt.Sprintf("Credential %q already exists.", body.Name))
			return
		}
		if isValidationError(err) {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, err.Error())
			return
		}
		r.logf("ERROR", "credentials: creating %q: %v", body.Name, err)
		WriteInternalError(w, "Failed to create credential.")
		return
	}

	r.logf("INFO", "credentials: created %q (type=%s) by %s",
		meta.Name, meta.CredentialType, adminUsername(req))

	WriteJSON(w, http.StatusCreated, credentialToResponse(meta))
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/credentials/:name — rotate a credential's value
// ---------------------------------------------------------------------------

func (r *Router) handleUpdateCredential(w http.ResponseWriter, req *http.Request) {
	name := pathParam(req, "/api/v1/admin/credentials/")
	if name == "" {
		WriteBadRequest(w, "Credential name is required in the URL path.")
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}
	if body.Value == "" {
		WriteBadRequest(w, "value is required.")
		return
	}

	input := secrets.UpdateCredentialInput{
		Name:      name,
		Plaintext: []byte(body.Value),
		UpdatedBy: adminUsername(req),
	}
	meta, err := r.credentialStore.Update(req.Context(), input)
	secrets.ZeroBytes(input.Plaintext)

	if err != nil {
		if errors.Is(err, secrets.ErrCredentialNotFound) {
			WriteNotFound(w, fmt.Sprintf("Credential %q not found.", name))
			return
		}
		if isValidationError(err) {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, err.Error())
			return
		}
		r.logf("ERROR", "credentials: updating %q: %v", name, err)
		WriteInternalError(w, "Failed to update credential.")
		return
	}

	r.logf("INFO", "credentials: rotated %q by %s", name, adminUsername(req))

	WriteJSON(w, http.StatusOK, credentialToResponse(meta))
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/credentials/:name — delete a credential
// ---------------------------------------------------------------------------

func (r *Router) handleDeleteCredential(w http.ResponseWriter, req *http.Request) {
	name := pathParam(req, "/api/v1/admin/credentials/")
	if name == "" {
		WriteBadRequest(w, "Credential name is required in the URL path.")
		return
	}

	if req.URL.Query().Get("confirm") != "true" {
		WriteBadRequest(w, "Delete requires confirm=true query parameter.")
		return
	}

	ctx := req.Context()

	// Check for references before attempting deletion so we can return
	// a helpful error message listing what still depends on the credential.
	refs, err := r.credentialStore.ReferencedBy(ctx, name)
	if err != nil {
		if errors.Is(err, secrets.ErrCredentialNotFound) {
			WriteNotFound(w, fmt.Sprintf("Credential %q not found.", name))
			return
		}
		r.logf("ERROR", "credentials: checking references for %q: %v", name, err)
		WriteInternalError(w, "Failed to check credential references.")
		return
	}

	if err := r.credentialStore.Delete(ctx, name); err != nil {
		if errors.Is(err, secrets.ErrCredentialNotFound) {
			WriteNotFound(w, fmt.Sprintf("Credential %q not found.", name))
			return
		}
		if errors.Is(err, secrets.ErrCredentialInUse) {
			parts := make([]string, 0, len(refs))
			for _, ref := range refs {
				parts = append(parts, fmt.Sprintf("%s %q", ref.EntityType, ref.EntityName))
			}
			msg := fmt.Sprintf("Credential %q is still referenced by: %s.",
				name, strings.Join(parts, ", "))
			WriteError(w, http.StatusConflict, "conflict", msg)
			return
		}
		r.logf("ERROR", "credentials: deleting %q: %v", name, err)
		WriteInternalError(w, "Failed to delete credential.")
		return
	}

	r.logf("INFO", "credentials: deleted %q by %s", name, adminUsername(req))

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/credentials/:name/test — test a credential
// ---------------------------------------------------------------------------

func (r *Router) handleTestCredential(w http.ResponseWriter, req *http.Request) {
	raw := pathParam(req, "/api/v1/admin/credentials/")
	name := trimSuffix(raw, "/test")
	if name == "" {
		WriteBadRequest(w, "Credential name is required in the URL path.")
		return
	}

	result, err := r.credentialStore.Test(req.Context(), name)
	if err != nil {
		if errors.Is(err, secrets.ErrCredentialNotFound) {
			WriteNotFound(w, fmt.Sprintf("Credential %q not found.", name))
			return
		}
		r.logf("ERROR", "credentials: testing %q: %v", name, err)
		WriteInternalError(w, "Failed to test credential.")
		return
	}

	r.logf("INFO", "credentials: tested %q by %s (valid=%v)",
		name, adminUsername(req), result.Valid)

	resp := map[string]any{
		"valid":    result.Valid,
		"error":    nil,
		"metadata": nil,
	}
	if result.Error != nil {
		resp["error"] = result.Error.Error()
	}
	if result.Metadata != nil {
		resp["metadata"] = result.Metadata
	}

	WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// credentialToResponse converts a CredentialMetadata to the API response
// representation. The encrypted value and plaintext are never included.
func credentialToResponse(m *secrets.CredentialMetadata) credentialResponse {
	return credentialResponse{
		Name:           m.Name,
		CredentialType: m.CredentialType,
		Metadata:       m.Metadata,
		LastRotatedAt:  m.LastRotatedAt,
		CreatedBy:      m.CreatedBy,
		UpdatedBy:      m.UpdatedBy,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// isValidationError returns true if the error is a credential validation
// error that should result in a 422 response. It checks the known validation
// sentinel errors from the secrets package.
func isValidationError(err error) bool {
	return errors.Is(err, secrets.ErrInvalidCredentialType) ||
		errors.Is(err, secrets.ErrEmptyValue) ||
		errors.Is(err, secrets.ErrInvalidPEMKey)
}
