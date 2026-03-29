// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Cookbook Platform Coverage endpoint
//
// GET /api/v1/cookbooks/:name/platform-coverage
//
// Returns the platform coverage analysis for a cookbook — comparing
// kitchen-tested platforms against production node platforms. See
// test-kitchen-drivers.md § Platform Coverage Analysis.
// ---------------------------------------------------------------------------

// handleCookbookPlatformCoverage handles GET /api/v1/cookbooks/:name/platform-coverage.
func (r *Router) handleCookbookPlatformCoverage(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Extract path segments: /api/v1/cookbooks/{name}/platform-coverage
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")
	if len(segments) < 2 || segments[len(segments)-1] != "platform-coverage" {
		WriteNotFound(w, "Expected path: /api/v1/cookbooks/:name/platform-coverage")
		return
	}

	cookbookName := segments[0]
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	ctx := req.Context()

	coverage, err := r.db.GetCookbookPlatformCoverage(ctx, cookbookName)
	if err != nil {
		r.logf("ERROR", "failed to get platform coverage for %q: %v", cookbookName, err)
		WriteInternalError(w, "Failed to retrieve platform coverage.")
		return
	}

	if coverage == nil {
		WriteNotFound(w, "No platform coverage data available for cookbook "+cookbookName+".")
		return
	}

	WriteJSON(w, http.StatusOK, coverage)
}
