// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleKitchenExclusions handles GET and POST on /api/v1/kitchen/git/exclusions.
func (r *Router) handleKitchenExclusions(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handleListKitchenExclusions(w, req)
	case http.MethodPost:
		r.handleCreateKitchenExclusion(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required.")
	}
}

// handleListKitchenExclusions handles GET /api/v1/kitchen/git/exclusions[?repo=<name>].
func (r *Router) handleListKitchenExclusions(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	repoName := req.URL.Query().Get("repo")

	exclusions, err := r.db.ListKitchenExclusions(ctx, repoName)
	if err != nil {
		r.logf("ERROR", "list kitchen exclusions: %v", err)
		WriteInternalError(w, "Failed to list kitchen exclusions.")
		return
	}
	if exclusions == nil {
		exclusions = []datastore.KitchenInstanceExclusion{}
	}
	WriteJSON(w, http.StatusOK, exclusions)
}

// createKitchenExclusionRequest is the request body for creating an exclusion.
type createKitchenExclusionRequest struct {
	GitRepoName  string `json:"git_repo_name"`
	GitRepoURL   string `json:"git_repo_url"`
	SuiteName    string `json:"suite_name"`
	PlatformName string `json:"platform_name"`
	Reason       string `json:"reason"`
}

// handleCreateKitchenExclusion handles POST /api/v1/kitchen/git/exclusions.
func (r *Router) handleCreateKitchenExclusion(w http.ResponseWriter, req *http.Request) {
	if !requireAdminRole(w, req) {
		return
	}

	var body createKitchenExclusionRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}

	if body.GitRepoName == "" {
		WriteBadRequest(w, "git_repo_name is required")
		return
	}
	if body.GitRepoURL == "" {
		WriteBadRequest(w, "git_repo_url is required")
		return
	}
	if body.SuiteName == "" {
		WriteBadRequest(w, "suite_name is required")
		return
	}
	if body.PlatformName == "" {
		WriteBadRequest(w, "platform_name is required")
		return
	}
	if len(strings.TrimSpace(body.Reason)) < 10 {
		WriteBadRequest(w, "reason must be at least 10 characters")
		return
	}

	ctx := req.Context()
	exclusion, err := r.db.CreateKitchenExclusion(ctx, datastore.CreateKitchenExclusionParams{
		GitRepoName:  body.GitRepoName,
		GitRepoURL:   body.GitRepoURL,
		SuiteName:    body.SuiteName,
		PlatformName: body.PlatformName,
		Reason:       strings.TrimSpace(body.Reason),
		ExcludedBy:   adminUsername(req),
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "UNIQUE constraint") {
			WriteError(w, http.StatusConflict, "conflict",
				"An exclusion already exists for this suite/platform combination.")
			return
		}
		r.logf("ERROR", "create kitchen exclusion: %v", err)
		WriteInternalError(w, "Failed to create kitchen exclusion.")
		return
	}

	WriteJSON(w, http.StatusCreated, exclusion)
}

// handleDeleteKitchenExclusion handles DELETE /api/v1/kitchen/git/exclusions/<id>.
func (r *Router) handleDeleteKitchenExclusion(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodDelete) {
		return
	}
	if !requireAdminRole(w, req) {
		return
	}

	id := pathParam(req, "/api/v1/kitchen/git/exclusions/")
	if id == "" {
		WriteBadRequest(w, "Exclusion ID is required in the URL path.")
		return
	}

	ctx := req.Context()
	found, err := r.db.DeleteKitchenExclusion(ctx, id)
	if err != nil {
		r.logf("ERROR", "delete kitchen exclusion %q: %v", id, err)
		WriteInternalError(w, "Failed to delete kitchen exclusion.")
		return
	}
	if !found {
		WriteNotFound(w, "Exclusion not found.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
