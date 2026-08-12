// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Saved filters are a named, owned selection in a list view's own filter
// vocabulary (see journeys/named-cohorts.md). Applying one needs no
// endpoint: the client holds the params and issues the view's existing list
// request, so the read path stays unchanged and a saved filter can never
// diverge from what the view natively supports.

type savedFilterCreateRequest struct {
	Name    string              `json:"name"`
	View    string              `json:"view"`
	Filters map[string][]string `json:"filters"`
	Shared  bool                `json:"shared"`
}

// savedFilterUpdateRequest carries only the fields being changed — a rename, a
// new selection, and a share toggle are all the same call. Absent fields are
// left alone, so pointers distinguish "not supplied" from "set to empty".
type savedFilterUpdateRequest struct {
	Name    *string              `json:"name"`
	Filters *map[string][]string `json:"filters"`
	Shared  *bool                `json:"shared"`
}

// handleSavedFilters serves /api/v1/saved-filters — GET lists the filters
// visible to the caller (their own plus every shared one), POST creates one.
func (r *Router) handleSavedFilters(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listSavedFilters(w, req)
	case http.MethodPost:
		r.createSavedFilter(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
	}
}

// handleSavedFilter serves /api/v1/saved-filters/{id} — PATCH renames, replaces
// the selection, or shares/unshares; DELETE removes. Both are owner-only.
func (r *Router) handleSavedFilter(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPatch:
		r.updateSavedFilter(w, req)
	case http.MethodDelete:
		r.deleteSavedFilter(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
	}
}

func (r *Router) listSavedFilters(w http.ResponseWriter, req *http.Request) {
	username, ok := requireSessionUsername(w, req)
	if !ok {
		return
	}

	view := queryString(req, "view", "")
	if view != "" {
		if _, known := savedFilterVocabulary[view]; !known {
			WriteErrorf(w, http.StatusBadRequest, ErrCodeBadRequest, "Unknown view %q.", view)
			return
		}
	}

	filters, err := r.db.ListSavedFilters(req.Context(), datastore.SavedFilterListFilter{
		Username: username,
		View:     view,
	})
	if err != nil {
		r.logf("ERROR", "listing saved filters for %s: %v", username, err)
		WriteInternalError(w, "Failed to list saved filters.")
		return
	}
	if filters == nil {
		filters = []datastore.SavedFilter{}
	}

	WriteJSON(w, http.StatusOK, filters)
}

func (r *Router) createSavedFilter(w http.ResponseWriter, req *http.Request) {
	username, ok := requireSessionUsername(w, req)
	if !ok {
		return
	}

	var body savedFilterCreateRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}

	if !validSavedFilterName(w, body.Name) {
		return
	}
	if err := validateSavedFilterSelection(body.View, body.Filters); err != nil {
		WriteBadRequest(w, err.Error())
		return
	}

	filter, err := r.db.InsertSavedFilter(req.Context(), datastore.InsertSavedFilterParams{
		OwnerUsername: username,
		View:          body.View,
		Name:          body.Name,
		Filters:       body.Filters,
		Shared:        body.Shared,
	})
	if err != nil {
		if errors.Is(err, datastore.ErrAlreadyExists) {
			WriteErrorf(w, http.StatusConflict, ErrCodeValidationError,
				"You already have a saved filter named %q on this view.", body.Name)
			return
		}
		r.logf("ERROR", "creating saved filter for %s: %v", username, err)
		WriteInternalError(w, "Failed to create the saved filter.")
		return
	}

	WriteJSON(w, http.StatusCreated, filter)
}

func (r *Router) updateSavedFilter(w http.ResponseWriter, req *http.Request) {
	existing, username, ok := r.ownedSavedFilter(w, req)
	if !ok {
		return
	}

	var body savedFilterUpdateRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}
	if body.Name == nil && body.Filters == nil && body.Shared == nil {
		WriteBadRequest(w, "No changes supplied — set at least one of name, filters, or shared.")
		return
	}

	if body.Name != nil && !validSavedFilterName(w, *body.Name) {
		return
	}
	// The selection is validated against the filter's stored view: a saved
	// filter cannot be edited into carrying a param its view does not accept.
	if body.Filters != nil {
		if err := validateSavedFilterSelection(existing.View, *body.Filters); err != nil {
			WriteBadRequest(w, err.Error())
			return
		}
	}

	updated, err := r.db.UpdateSavedFilter(req.Context(), existing.ID, datastore.UpdateSavedFilterParams{
		Name:    body.Name,
		Filters: body.Filters,
		Shared:  body.Shared,
	})
	if err != nil {
		switch {
		case errors.Is(err, datastore.ErrNotFound):
			WriteNotFound(w, "Saved filter not found.")
		case errors.Is(err, datastore.ErrAlreadyExists):
			WriteError(w, http.StatusConflict, ErrCodeValidationError,
				"You already have a saved filter of that name on this view.")
		default:
			r.logf("ERROR", "updating saved filter %s for %s: %v", existing.ID, username, err)
			WriteInternalError(w, "Failed to update the saved filter.")
		}
		return
	}

	WriteJSON(w, http.StatusOK, updated)
}

func (r *Router) deleteSavedFilter(w http.ResponseWriter, req *http.Request) {
	existing, username, ok := r.ownedSavedFilter(w, req)
	if !ok {
		return
	}

	if err := r.db.DeleteSavedFilter(req.Context(), existing.ID); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, "Saved filter not found.")
			return
		}
		r.logf("ERROR", "deleting saved filter %s for %s: %v", existing.ID, username, err)
		WriteInternalError(w, "Failed to delete the saved filter.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ownedSavedFilter loads the saved filter named in the path and authorises the
// caller to mutate it. A shared filter is read-only to everyone but its owner;
// another user's *private* filter is invisible, so it 404s rather than 403s —
// its existence is not the caller's business.
func (r *Router) ownedSavedFilter(w http.ResponseWriter, req *http.Request) (datastore.SavedFilter, string, bool) {
	username, ok := requireSessionUsername(w, req)
	if !ok {
		return datastore.SavedFilter{}, "", false
	}

	id := pathParam(req, "/api/v1/saved-filters/")
	if id == "" {
		WriteBadRequest(w, "A saved filter id is required.")
		return datastore.SavedFilter{}, "", false
	}

	filter, err := r.db.GetSavedFilter(req.Context(), id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, "Saved filter not found.")
			return datastore.SavedFilter{}, "", false
		}
		r.logf("ERROR", "getting saved filter %s: %v", id, err)
		WriteInternalError(w, "Failed to load the saved filter.")
		return datastore.SavedFilter{}, "", false
	}

	if filter.OwnerUsername != username {
		if !filter.Shared {
			WriteNotFound(w, "Saved filter not found.")
			return datastore.SavedFilter{}, "", false
		}
		WriteForbidden(w, "This saved filter belongs to another user. Shared filters are read-only — copy it to your own to change it.")
		return datastore.SavedFilter{}, "", false
	}

	return filter, username, true
}

// requireSessionUsername returns the caller's username. A saved filter is owned,
// so it cannot be created or mutated without an identity to own it.
func requireSessionUsername(w http.ResponseWriter, req *http.Request) (string, bool) {
	info := auth.SessionFromContext(req.Context())
	if info == nil || info.Username == "" {
		WriteUnauthorized(w, "Authentication is required to use saved filters.")
		return "", false
	}
	return info.Username, true
}

func validSavedFilterName(w http.ResponseWriter, name string) bool {
	switch {
	case name == "":
		WriteBadRequest(w, "A saved filter name is required.")
		return false
	case len(name) > maxSavedFilterNameLen:
		WriteErrorf(w, http.StatusBadRequest, ErrCodeBadRequest,
			"A saved filter name may be at most %d characters.", maxSavedFilterNameLen)
		return false
	}
	return true
}
