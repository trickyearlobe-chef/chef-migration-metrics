// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Admin Rescan All CookStyle endpoint
//
// POST /api/v1/admin/rescan-all-cookstyle
//
// Invalidates ALL cached CookStyle results, complexity scores, and
// autocorrect previews across every cookbook. The next collection cycle will
// re-run CookStyle (with whatever cops the currently installed version
// provides) and recompute all derived data.
//
// This is useful after upgrading CookStyle to a version with new or changed
// cops, or after a bulk configuration change that affects analysis results.
//
// Response (200):
//
//	{
//	  "message": "All CookStyle results invalidated — rescan will run on the next collection cycle."
//	}
//
// ---------------------------------------------------------------------------

// handleAdminRescanAllCookstyle handles POST /api/v1/admin/rescan-all-cookstyle.
func (r *Router) handleAdminRescanAllCookstyle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"Only POST is allowed for this endpoint.")
		return
	}

	ctx := req.Context()

	// Delete all cookstyle results.
	if err := r.db.DeleteAllCookstyleResults(ctx); err != nil {
		r.logf("ERROR", "deleting all cookstyle results: %v", err)
		WriteInternalError(w, "Failed to delete cookstyle results.")
		return
	}

	// Delete all cookbook complexity records.
	if err := r.db.DeleteAllCookbookComplexities(ctx); err != nil {
		r.logf("ERROR", "deleting all cookbook complexities: %v", err)
		WriteInternalError(w, "Failed to delete cookbook complexity records.")
		return
	}

	// Delete all autocorrect previews.
	if err := r.db.DeleteAllAutocorrectPreviews(ctx); err != nil {
		r.logf("ERROR", "deleting all autocorrect previews: %v", err)
		WriteInternalError(w, "Failed to delete autocorrect previews.")
		return
	}

	// Broadcast a rescan event so the UI can update.
	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventRescanStarted, map[string]any{
			"message": "Full rescan initiated for all cookbooks",
		}))
	}

	r.logf("INFO", "admin rescan-all-cookstyle: all cookstyle results, complexity records, and autocorrect previews deleted")

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "All CookStyle results invalidated — rescan will run on the next collection cycle.",
	})
}
