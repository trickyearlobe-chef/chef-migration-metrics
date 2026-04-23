// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Git Kitchen Results — list and filter
// ---------------------------------------------------------------------------

// handleGitKitchenResults dispatches GET for /api/v1/git-kitchen-results.
// Supports query params: repo (filter by git_repo_name), batch_id (filter
// by batch).
func (r *Router) handleGitKitchenResults(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()
	q := req.URL.Query()

	repo := q.Get("repo")
	batchID := q.Get("batch_id")

	switch {
	case repo != "":
		results, err := r.db.ListGitKitchenResultsByRepo(ctx, repo)
		if err != nil {
			r.logf("ERROR", "git-kitchen-results: listing by repo %s: %v", repo, err)
			WriteInternalError(w, "Failed to list git kitchen results.")
			return
		}
		if results == nil {
			results = []datastore.GitKitchenResult{}
		}
		WriteJSON(w, http.StatusOK, results)

	case batchID != "":
		results, err := r.db.ListGitKitchenResultsByBatch(ctx, batchID)
		if err != nil {
			r.logf("ERROR", "git-kitchen-results: listing by batch %s: %v", batchID, err)
			WriteInternalError(w, "Failed to list git kitchen results.")
			return
		}
		if results == nil {
			results = []datastore.GitKitchenResult{}
		}
		WriteJSON(w, http.StatusOK, results)

	default:
		results, err := r.db.ListGitKitchenResults(ctx)
		if err != nil {
			r.logf("ERROR", "git-kitchen-results: listing all: %v", err)
			WriteInternalError(w, "Failed to list git kitchen results.")
			return
		}
		if results == nil {
			results = []datastore.GitKitchenResult{}
		}
		WriteJSON(w, http.StatusOK, results)
	}
}

// ---------------------------------------------------------------------------
// Git Kitchen Result detail — GET /api/v1/git-kitchen-results/:id
// ---------------------------------------------------------------------------

// handleGitKitchenResultDetail dispatches GET for
// /api/v1/git-kitchen-results/:id.
func (r *Router) handleGitKitchenResultDetail(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/git-kitchen-results/")
	if len(segments) == 0 {
		WriteNotFound(w, "Result ID is required in the path.")
		return
	}

	id := segments[0]
	result, err := r.db.GetGitKitchenResult(req.Context(), id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Git kitchen result %q not found.", id))
			return
		}
		r.logf("ERROR", "git-kitchen-results: getting result %s: %v", id, err)
		WriteInternalError(w, "Failed to get git kitchen result.")
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Batch sub-resources
// ---------------------------------------------------------------------------

// handleBatchResults handles GET /api/v1/kitchen/batches/:id/results.
func (r *Router) handleBatchResults(w http.ResponseWriter, req *http.Request, batchID string) {
	if !requireGET(w, req) {
		return
	}

	results, err := r.db.ListGitKitchenResultsByBatch(req.Context(), batchID)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: listing results for batch %s: %v", batchID, err)
		WriteInternalError(w, "Failed to list batch results.")
		return
	}
	if results == nil {
		results = []datastore.GitKitchenResult{}
	}
	WriteJSON(w, http.StatusOK, results)
}

// handleBatchProgress handles GET /api/v1/kitchen/batches/:id/progress.
func (r *Router) handleBatchProgress(w http.ResponseWriter, req *http.Request, batchID string) {
	if !requireGET(w, req) {
		return
	}

	passed, failed, pending, timedOut, errored, err := r.db.CountGitKitchenResultsByBatch(req.Context(), batchID)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: counting progress for batch %s: %v", batchID, err)
		WriteInternalError(w, "Failed to get batch progress.")
		return
	}

	total := passed + failed + pending + timedOut + errored
	WriteJSON(w, http.StatusOK, map[string]int{
		"passed":    passed,
		"failed":    failed,
		"pending":   pending,
		"timed_out": timedOut,
		"errored":   errored,
		"total":     total,
	})
}
