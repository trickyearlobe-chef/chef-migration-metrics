// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Ensure unused imports don't cause issues — os and filepath are still used
// by the clone removal logic below.

// ---------------------------------------------------------------------------
// Git Cookbook Reset endpoint
//
// POST /api/v1/cookbooks/:name/reset-git
//
// Removes all git-sourced cookbook rows for the named cookbook from the
// database, along with associated committer data and analysis results
// (complexity, cookstyle, autocorrect previews — handled by cascading
// foreign-key deletes). It also deletes the local git clone directory so
// that the next collection cycle will perform a fresh clone from whatever
// git base URLs are currently configured.
//
// This is useful when a repository has moved (e.g. from Stash to GitLab)
// and the stored git_repo_url is stale. Resetting allows the collector to
// re-discover the cookbook at its new URL.
//
// Requires the operator or admin role.
//
// Response (200):
//
//	{
//	  "cookbook_name":       "cron",
//	  "cookbooks_deleted":  1,
//	  "committers_deleted": 5,
//	  "repo_urls_removed":  ["git@github.com:old-org/cron"],
//	  "local_clone_removed": true,
//	  "message":            "Git cookbook reset — will be re-cloned on the next collection cycle."
//	}
//
// ---------------------------------------------------------------------------

// handleCookbookResetGit handles POST /api/v1/cookbooks/:name/reset-git.
func (r *Router) handleCookbookResetGit(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	if !requireOperatorOrAdmin(w, req) {
		return
	}

	// Extract the cookbook name from the path.
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")
	if len(segments) < 2 || segments[len(segments)-1] != "reset-git" {
		WriteNotFound(w, "Expected path: /api/v1/cookbooks/:name/reset-git")
		return
	}
	cookbookName := segments[0]
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	ctx := req.Context()

	// Delete all git repo rows and associated committer data.
	result, err := r.db.DeleteGitReposByName(ctx, cookbookName)
	if err != nil {
		// Check for not-found (no git-sourced rows for this name).
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("No git-sourced cookbook %q found.", cookbookName))
			return
		}
		r.logf("ERROR", "deleting git cookbook %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to delete git cookbook data.")
		return
	}

	// Remove the local git clone directory.
	localCloneRemoved := false
	repoDir := filepath.Join(r.cfg.Storage.GitCookbookDir, cookbookName)
	if _, statErr := os.Stat(repoDir); statErr == nil {
		if rmErr := os.RemoveAll(repoDir); rmErr != nil {
			r.logf("WARN", "failed to remove local git clone for %s at %s: %v",
				cookbookName, repoDir, rmErr)
		} else {
			localCloneRemoved = true
			r.logf("INFO", "removed local git clone for %s at %s", cookbookName, repoDir)
		}
	}

	// Broadcast a WebSocket event so the UI can react.
	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventCookbookStatusChanged, map[string]any{
			"cookbook_name":      cookbookName,
			"action":             "reset-git",
			"repos_deleted":      result.ReposDeleted,
			"committers_deleted": result.CommittersDeleted,
		}))
	}

	// Ensure repo_urls_removed is always a JSON array, not null.
	repoURLs := result.RepoURLs
	if repoURLs == nil {
		repoURLs = []string{}
	}

	r.logf("INFO", "git repo reset for %s — %d repo(s), %d committer(s) deleted, %d repo URL(s) cleaned up, local clone removed: %v",
		cookbookName, result.ReposDeleted, result.CommittersDeleted, len(repoURLs), localCloneRemoved)

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name":       cookbookName,
		"repos_deleted":       result.ReposDeleted,
		"committers_deleted":  result.CommittersDeleted,
		"repo_urls_removed":   repoURLs,
		"local_clone_removed": localCloneRemoved,
		"message":             "Git repo reset — will be re-cloned on the next collection cycle.",
	})
}
