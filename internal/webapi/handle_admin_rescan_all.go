// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
)

// ---------------------------------------------------------------------------
// Admin Rescan All CookStyle endpoint
//
// POST /api/v1/admin/rescan-all-cookstyle
//
// Invalidates ALL cached CookStyle results, complexity scores, and
// autocorrect previews across every cookbook, then triggers an immediate
// collection run so the rescan starts right away instead of waiting for
// the next scheduled cycle.
//
// This is useful after upgrading CookStyle to a version with new or changed
// cops, or after a bulk configuration change that affects analysis results.
//
// Response (200):
//
//	{
//	  "message": "All CookStyle results invalidated — collection run triggered.",
//	  "collection_triggered": true
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

	// Delete all server cookbook cookstyle results.
	if err := r.db.DeleteAllServerCookbookCookstyleResults(ctx); err != nil {
		r.logf("ERROR", "deleting all server cookbook cookstyle results: %v", err)
		WriteInternalError(w, "Failed to delete server cookbook cookstyle results.")
		return
	}

	// Delete all git repo cookstyle results.
	if err := r.db.DeleteAllGitRepoCookstyleResults(ctx); err != nil {
		r.logf("ERROR", "deleting all git repo cookstyle results: %v", err)
		WriteInternalError(w, "Failed to delete git repo cookstyle results.")
		return
	}

	// Delete all server cookbook complexity records.
	if err := r.db.DeleteAllServerCookbookComplexities(ctx); err != nil {
		r.logf("ERROR", "deleting all server cookbook complexities: %v", err)
		WriteInternalError(w, "Failed to delete server cookbook complexity records.")
		return
	}

	// Delete all git repo complexity records.
	if err := r.db.DeleteAllGitRepoComplexities(ctx); err != nil {
		r.logf("ERROR", "deleting all git repo complexities: %v", err)
		WriteInternalError(w, "Failed to delete git repo complexity records.")
		return
	}

	// Delete all server cookbook autocorrect previews.
	if err := r.db.DeleteAllServerCookbookAutocorrectPreviews(ctx); err != nil {
		r.logf("ERROR", "deleting all server cookbook autocorrect previews: %v", err)
		WriteInternalError(w, "Failed to delete server cookbook autocorrect previews.")
		return
	}

	// Delete all git repo autocorrect previews.
	if err := r.db.DeleteAllGitRepoAutocorrectPreviews(ctx); err != nil {
		r.logf("ERROR", "deleting all git repo autocorrect previews: %v", err)
		WriteInternalError(w, "Failed to delete git repo autocorrect previews.")
		return
	}

	// Reset download_status to 'pending' for all server cookbooks so the
	// streaming pipeline re-downloads and re-scans them on the next cycle.
	// When delete_server_cookbooks_after_scan is false (the default), files
	// already in the cache directory will be overwritten in place.
	resetCount, resetErr := r.db.ResetAllServerCookbookDownloadStatuses(ctx)
	if resetErr != nil {
		r.logf("ERROR", "resetting server cookbook download statuses: %v", resetErr)
		WriteInternalError(w, "Failed to reset cookbook download statuses.")
		return
	}
	if resetCount > 0 {
		r.logf("INFO", "admin rescan-all-cookstyle: reset download status for %d server cookbook version(s)", resetCount)
	}

	// Broadcast a rescan event so the UI can update.
	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventRescanStarted, map[string]any{
			"message": "Full rescan initiated for all cookbooks",
		}))
	}

	r.logf("INFO", "admin rescan-all-cookstyle: all cookstyle results, complexity records, and autocorrect previews deleted")

	// Trigger an immediate collection run in the background so the rescan
	// starts right away instead of waiting for the next scheduled cycle.
	triggered := r.triggerCollectionInBackground()

	msg := "All CookStyle results invalidated"
	if triggered {
		msg += " — collection run triggered."
	} else {
		msg += " — rescan will run on the next collection cycle."
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message":              msg,
		"collection_triggered": triggered,
	})
}

// triggerCollectionInBackground launches an immediate collection run in a
// background goroutine. Returns true if the trigger was dispatched, false
// if no trigger function is configured or if a run is already in progress.
//
// The goroutine uses a detached context so it is not cancelled when the
// HTTP request completes.
func (r *Router) triggerCollectionInBackground() bool {
	if r.triggerCollection == nil {
		return false
	}

	// Use a detached context — the collection run must not be cancelled
	// when the HTTP response is sent.
	bgCtx := context.Background()

	if err := r.triggerCollection(bgCtx); err != nil {
		r.logf("WARN", "could not trigger immediate collection run: %v", err)
		return false
	}

	r.logf("INFO", "immediate collection run triggered by rescan request")
	return true
}
