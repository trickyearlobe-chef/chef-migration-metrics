// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
)

// ---------------------------------------------------------------------------
// Cookbook Rescan endpoint
//
// POST /api/v1/cookbooks/:name/rescan
//
// Invalidates all cached CookStyle results, complexity scores, and
// autocorrect previews for every version of the named cookbook. The next
// collection cycle will re-run CookStyle (with whatever cops the currently
// installed version provides) and recompute derived data.
//
// This is useful after upgrading CookStyle to a version with new or changed
// cops, or after making changes to a git-sourced cookbook outside the normal
// collection cycle.
//
// Response (200):
//
//	{
//	  "cookbook_name": "apt",
//	  "versions_invalidated": 3,
//	  "message": "CookStyle results invalidated — rescan will run on the next collection cycle."
//	}
//
// ---------------------------------------------------------------------------

// handleCookbookRescan handles POST /api/v1/cookbooks/:name/rescan.
func (r *Router) handleCookbookRescan(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"Only POST is allowed for this endpoint.")
		return
	}

	// Extract the cookbook name from the path.
	// The route is registered as "/api/v1/cookbooks/" so the detail handler
	// dispatches here when it detects the /rescan suffix.
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")
	if len(segments) < 2 || segments[len(segments)-1] != "rescan" {
		WriteNotFound(w, "Expected path: /api/v1/cookbooks/:name/rescan")
		return
	}
	cookbookName := segments[0]
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	ctx := req.Context()

	// Look up all versions of the cookbook.
	cookbooks, err := r.db.ListCookbooksByName(ctx, cookbookName)
	if err != nil {
		r.logf("ERROR", "listing cookbooks for rescan %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook.")
		return
	}
	if len(cookbooks) == 0 {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q not found.", cookbookName))
		return
	}

	// Invalidate cookstyle results, complexity scores, and autocorrect
	// previews for every version. For server cookbooks, also reset the
	// download_status to 'pending' so the streaming pipeline will
	// re-download and re-scan them on the next collection cycle (the
	// pipeline deletes cookbook files from disk after scanning, so the
	// files are no longer available for a re-scan without re-downloading).
	invalidated := 0
	var lastErr error
	for _, cb := range cookbooks {
		csErr := r.db.DeleteCookstyleResultsForCookbook(ctx, cb.ID)
		if csErr != nil {
			r.logf("WARN", "deleting cookstyle results for cookbook %s (%s): %v", cb.ID, cb.Name, csErr)
			lastErr = csErr
		}

		cxErr := r.db.DeleteCookbookComplexitiesForCookbook(ctx, cb.ID)
		if cxErr != nil {
			r.logf("WARN", "deleting complexity records for cookbook %s (%s): %v", cb.ID, cb.Name, cxErr)
			lastErr = cxErr
		}

		acErr := r.db.DeleteAutocorrectPreviewsForCookbook(ctx, cb.ID)
		if acErr != nil {
			r.logf("WARN", "deleting autocorrect previews for cookbook %s (%s): %v", cb.ID, cb.Name, acErr)
			lastErr = acErr
		}

		// Server cookbooks: reset download_status to 'pending' so the
		// streaming pipeline re-downloads the files (they were deleted
		// after the previous scan).
		if cb.IsChefServer() {
			if _, dlErr := r.db.ResetCookbookDownloadStatus(ctx, cb.ID); dlErr != nil {
				r.logf("WARN", "resetting download status for cookbook %s (%s): %v", cb.ID, cb.Name, dlErr)
				lastErr = dlErr
			}
		}

		if csErr == nil && cxErr == nil && acErr == nil {
			invalidated++
		}
	}

	if lastErr != nil && invalidated == 0 {
		WriteInternalError(w, "Failed to invalidate CookStyle results.")
		return
	}

	// Broadcast a rescan event so the UI can update.
	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventRescanStarted, map[string]any{
			"cookbook_name":        cookbookName,
			"versions_invalidated": invalidated,
		}))
	}

	r.logf("INFO", "cookstyle rescan requested for %s — %d version(s) invalidated", cookbookName, invalidated)

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name":        cookbookName,
		"versions_invalidated": invalidated,
		"message":              "CookStyle results invalidated — rescan will run on the next collection cycle.",
	})
}
