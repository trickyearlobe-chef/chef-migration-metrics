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

// ---------------------------------------------------------------------------
// Git Repos List endpoint
//
// GET /api/v1/git-repos
//
// Returns all git repos, optionally filtered by name (substring match)
// and/or compatibility status. Each repo includes a compatibility field
// ("compatible", "incompatible", or "untested") computed from git repo
// complexity records for the specified target Chef version.
//
// Supports pagination via page/per_page query parameters.
//
// Query parameters:
//   - name: case-insensitive substring filter on repo name
//   - target_chef_version: Chef version to evaluate compatibility against
//     (defaults to the first configured target version)
//   - compatibility: filter by status — "compatible", "incompatible",
//     "untested", or "" (no filter)
//   - page: page number (default 1)
//   - per_page: items per page (default 25)
//
// Response (200):
//
//	{
//	  "data": [ { ... } ],
//	  "pagination": { "page": 1, "per_page": 25, "total_items": 42, "total_pages": 2 }
//	}
//
// ---------------------------------------------------------------------------

// handleGitRepos handles GET /api/v1/git-repos — lists all git repos.
func (r *Router) handleGitRepos(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	repos, err := r.db.ListGitRepos(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repos: %v", err)
		WriteInternalError(w, "Failed to list git repos.")
		return
	}

	// Determine target Chef version for compatibility.
	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetChefVersion = r.cfg.TargetChefVersions[0]
	}

	// Build compatibility map from git repo complexity records.
	compatByName := make(map[string]string)
	if targetChefVersion != "" {
		repoNameByID := make(map[string]string, len(repos))
		for _, gr := range repos {
			repoNameByID[gr.ID] = gr.Name
		}
		allComplexities, cxErr := r.db.ListAllGitRepoComplexities(ctx)
		if cxErr != nil {
			r.logf("WARN", "listing git repo complexities for compatibility: %v", cxErr)
		} else {
			for _, cc := range allComplexities {
				if cc.TargetChefVersion != targetChefVersion {
					continue
				}
				name := repoNameByID[cc.GitRepoID]
				if name == "" {
					continue
				}
				if _, seen := compatByName[name]; seen {
					continue
				}
				if cc.ErrorCount == 0 {
					compatByName[name] = "compatible"
				} else {
					compatByName[name] = "incompatible"
				}
			}
		}
	}

	// Apply optional name filter (case-insensitive substring).
	nameFilter := queryString(req, "name", "")
	if nameFilter != "" {
		filtered := repos[:0]
		for _, gr := range repos {
			if containsFold(gr.Name, nameFilter) {
				filtered = append(filtered, gr)
			}
		}
		repos = filtered
	}

	// Apply optional compatibility filter.
	compatFilter := queryString(req, "compatibility", "")
	if compatFilter != "" {
		filtered := repos[:0]
		for _, gr := range repos {
			c := compatByName[gr.Name]
			if c == "" {
				c = "untested"
			}
			if c == compatFilter {
				filtered = append(filtered, gr)
			}
		}
		repos = filtered
	}

	// Paginate.
	pg := ParsePagination(req)
	total := len(repos)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	type gitRepoResp struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		GitRepoURL        string `json:"git_repo_url"`
		HeadCommitSHA     string `json:"head_commit_sha,omitempty"`
		DefaultBranch     string `json:"default_branch,omitempty"`
		HasTestSuite      bool   `json:"has_test_suite"`
		LastFetchedAt     string `json:"last_fetched_at,omitempty"`
		Compatibility     string `json:"compatibility"`
		TargetChefVersion string `json:"target_chef_version,omitempty"`
	}

	result := make([]gitRepoResp, 0, end-start)
	for _, gr := range repos[start:end] {
		c := compatByName[gr.Name]
		if c == "" {
			c = "untested"
		}
		resp := gitRepoResp{
			ID:                gr.ID,
			Name:              gr.Name,
			GitRepoURL:        gr.GitRepoURL,
			HeadCommitSHA:     gr.HeadCommitSHA,
			DefaultBranch:     gr.DefaultBranch,
			HasTestSuite:      gr.HasTestSuite,
			Compatibility:     c,
			TargetChefVersion: targetChefVersion,
		}
		if !gr.LastFetchedAt.IsZero() {
			resp.LastFetchedAt = gr.LastFetchedAt.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, resp)
	}

	WritePaginated(w, result, pg, total)
}

// ---------------------------------------------------------------------------
// Git Repo Detail endpoint
//
// GET /api/v1/git-repos/:name
//
// Returns all git repo rows for the given cookbook name (there may be
// multiple if the same cookbook is tracked at different git URLs), along
// with cookstyle results, test kitchen results, and complexity records.
//
// Also dispatches to sub-path handlers:
//   - /api/v1/git-repos/:name/committers       → handleGitRepoCommitters
//   - /api/v1/git-repos/:name/committers/assign → handleGitRepoCommittersAssign
//   - /api/v1/git-repos/:name/rescan            → handleGitRepoRescan
//   - /api/v1/git-repos/:name/reset             → handleGitRepoReset
//   - /api/v1/git-repos/:name/:version/remediation → handleGitRepoRemediation
//
// Response (200):
//
//	{
//	  "name": "cron",
//	  "git_repos": [
//	    {
//	      "git_repo": { ... },
//	      "cookstyle": [ ... ],
//	      "test_kitchen": [ ... ],
//	      "complexity": [ ... ]
//	    }
//	  ]
//	}
//
// ---------------------------------------------------------------------------

// handleGitRepoDetail handles /api/v1/git-repos/:name and sub-path dispatch.
func (r *Router) handleGitRepoDetail(w http.ResponseWriter, req *http.Request) {
	segments := pathSegments(req.URL.Path, "/api/v1/git-repos/")

	// /api/v1/git-repos/:name/:version/remediation
	if len(segments) >= 3 && segments[len(segments)-1] == "remediation" {
		r.handleGitRepoRemediation(w, req)
		return
	}

	// /api/v1/git-repos/:name/rescan
	if len(segments) >= 2 && segments[len(segments)-1] == "rescan" {
		r.handleGitRepoRescan(w, req)
		return
	}

	// /api/v1/git-repos/:name/reset
	if len(segments) >= 2 && segments[len(segments)-1] == "reset" {
		r.handleGitRepoReset(w, req)
		return
	}

	// /api/v1/git-repos/:name/committers[/assign]
	if len(segments) >= 2 && segments[1] == "committers" {
		repoName := segments[0]
		if len(segments) == 3 && segments[2] == "assign" {
			r.handleCookbookCommittersAssign(w, req, repoName)
			return
		}
		if len(segments) == 2 {
			r.handleCookbookCommitters(w, req, repoName)
			return
		}
		WriteNotFound(w, fmt.Sprintf("Unknown committers endpoint: %s", req.URL.Path))
		return
	}

	// Default: detail view.
	name := pathParam(req, "/api/v1/git-repos/")
	if name == "" {
		WriteNotFound(w, "Git repo name is required.")
		return
	}

	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	gitRepos, err := r.db.ListGitReposByName(ctx, name)
	if err != nil {
		r.logf("ERROR", "listing git repos for %s: %v", name, err)
		WriteInternalError(w, "Failed to get git repo.")
		return
	}

	if len(gitRepos) == 0 {
		WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", name))
		return
	}

	type gitRepoDetailEntry struct {
		GitRepo     datastore.GitRepo                    `json:"git_repo"`
		Cookstyle   []datastore.GitRepoCookstyleResult   `json:"cookstyle,omitempty"`
		TestKitchen []datastore.GitRepoTestKitchenResult `json:"test_kitchen,omitempty"`
		Complexity  []datastore.GitRepoComplexity        `json:"complexity,omitempty"`
	}

	details := make([]gitRepoDetailEntry, 0, len(gitRepos))
	for _, gr := range gitRepos {
		detail := gitRepoDetailEntry{GitRepo: gr}

		cookstyle, csErr := r.db.ListGitRepoCookstyleResults(ctx, gr.ID)
		if csErr != nil {
			r.logf("WARN", "listing cookstyle results for git repo %s: %v", gr.ID, csErr)
		} else {
			detail.Cookstyle = cookstyle
		}

		tk, tkErr := r.db.ListGitRepoTestKitchenResults(ctx, gr.ID)
		if tkErr != nil {
			r.logf("WARN", "listing test kitchen results for git repo %s: %v", gr.ID, tkErr)
		} else {
			detail.TestKitchen = tk
		}

		complexity, cxErr := r.db.ListGitRepoComplexitiesByRepo(ctx, gr.ID)
		if cxErr != nil {
			r.logf("WARN", "listing complexity for git repo %s: %v", gr.ID, cxErr)
		} else {
			detail.Complexity = complexity
		}

		details = append(details, detail)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"name":      name,
		"git_repos": details,
	})
}

// ---------------------------------------------------------------------------
// Git Repo Rescan endpoint
//
// POST /api/v1/git-repos/:name/rescan
//
// Invalidates all cached CookStyle results, complexity scores, and
// autocorrect previews for all git repos with the given name. The next
// collection cycle will re-run analysis.
// ---------------------------------------------------------------------------

func (r *Router) handleGitRepoRescan(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/git-repos/")
	if len(segments) < 2 || segments[len(segments)-1] != "rescan" {
		WriteNotFound(w, "Expected path: /api/v1/git-repos/:name/rescan")
		return
	}
	repoName := segments[0]
	if repoName == "" {
		WriteBadRequest(w, "Git repo name is required.")
		return
	}

	ctx := req.Context()

	gitRepos, err := r.db.ListGitReposByName(ctx, repoName)
	if err != nil {
		r.logf("ERROR", "listing git repos for rescan %s: %v", repoName, err)
		WriteInternalError(w, "Failed to look up git repo.")
		return
	}

	if len(gitRepos) == 0 {
		WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", repoName))
		return
	}

	invalidated := 0
	var lastErr error

	for _, gr := range gitRepos {
		csErr := r.db.DeleteGitRepoCookstyleResultsByRepo(ctx, gr.ID)
		if csErr != nil {
			r.logf("WARN", "deleting cookstyle results for git repo %s (%s): %v", gr.ID, gr.Name, csErr)
			lastErr = csErr
		}

		cxErr := r.db.DeleteGitRepoComplexitiesByRepo(ctx, gr.ID)
		if cxErr != nil {
			r.logf("WARN", "deleting complexity records for git repo %s (%s): %v", gr.ID, gr.Name, cxErr)
			lastErr = cxErr
		}

		acErr := r.db.DeleteGitRepoAutocorrectPreviewsByRepo(ctx, gr.ID)
		if acErr != nil {
			r.logf("WARN", "deleting autocorrect previews for git repo %s (%s): %v", gr.ID, gr.Name, acErr)
			lastErr = acErr
		}

		if csErr == nil && cxErr == nil && acErr == nil {
			invalidated++
		}
	}

	if lastErr != nil && invalidated == 0 {
		WriteInternalError(w, "Failed to invalidate git repo analysis results.")
		return
	}

	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventGitRepoStatusChanged, map[string]any{
			"git_repo_name":     repoName,
			"action":            "rescan",
			"repos_invalidated": invalidated,
		}))
	}

	r.logf("INFO", "git repo rescan requested for %s — %d repo(s) invalidated", repoName, invalidated)

	WriteJSON(w, http.StatusOK, map[string]any{
		"git_repo_name":     repoName,
		"repos_invalidated": invalidated,
		"message":           "Analysis results invalidated — rescan will run on the next collection cycle.",
	})
}

// ---------------------------------------------------------------------------
// Git Repo Reset endpoint
//
// POST /api/v1/git-repos/:name/reset
//
// Removes all git repo rows for the given name from the database, along
// with associated committer data and analysis results. Also deletes the
// local git clone directory so the next collection cycle will re-clone.
//
// Requires the operator or admin role.
// ---------------------------------------------------------------------------

func (r *Router) handleGitRepoReset(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	if !requireOperatorOrAdmin(w, req) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/git-repos/")
	if len(segments) < 2 || segments[len(segments)-1] != "reset" {
		WriteNotFound(w, "Expected path: /api/v1/git-repos/:name/reset")
		return
	}
	repoName := segments[0]
	if repoName == "" {
		WriteBadRequest(w, "Git repo name is required.")
		return
	}

	ctx := req.Context()

	result, err := r.db.DeleteGitReposByName(ctx, repoName)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("No git repo %q found.", repoName))
			return
		}
		r.logf("ERROR", "deleting git repo %s: %v", repoName, err)
		WriteInternalError(w, "Failed to delete git repo data.")
		return
	}

	// Remove the local git clone directory.
	localCloneRemoved := removeLocalGitClone(r, repoName)

	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventGitRepoStatusChanged, map[string]any{
			"git_repo_name":      repoName,
			"action":             "reset",
			"repos_deleted":      result.ReposDeleted,
			"committers_deleted": result.CommittersDeleted,
		}))
	}

	repoURLs := result.RepoURLs
	if repoURLs == nil {
		repoURLs = []string{}
	}

	r.logf("INFO", "git repo reset for %s — %d repo(s), %d committer(s) deleted, %d repo URL(s) cleaned up, local clone removed: %v",
		repoName, result.ReposDeleted, result.CommittersDeleted, len(repoURLs), localCloneRemoved)

	WriteJSON(w, http.StatusOK, map[string]any{
		"git_repo_name":       repoName,
		"repos_deleted":       result.ReposDeleted,
		"committers_deleted":  result.CommittersDeleted,
		"repo_urls_removed":   repoURLs,
		"local_clone_removed": localCloneRemoved,
		"message":             "Git repo reset — will be re-cloned on the next collection cycle.",
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containsFold reports whether s contains substr using a case-insensitive
// comparison. This is a simple ASCII-safe approach suitable for cookbook names.
func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	// Use lowercase comparison (cookbook names are ASCII).
	sl := toLowerASCII(s)
	fl := toLowerASCII(substr)
	for i := 0; i <= len(sl)-len(fl); i++ {
		if sl[i:i+len(fl)] == fl {
			return true
		}
	}
	return false
}

// toLowerASCII converts ASCII letters to lowercase. Non-ASCII bytes are
// left unchanged. This avoids importing strings just for ToLower.
func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// removeLocalGitClone removes the local git clone directory for a cookbook
// name, returning true if it was successfully removed. This is shared
// between the git repo reset and cookbook reset-git handlers.
func removeLocalGitClone(r *Router, cookbookName string) bool {
	if r.cfg.Storage.GitCookbookDir == "" {
		return false
	}

	// filepath.Base strips directory components so user-controlled input
	// cannot escape the GitCookbookDir via path traversal (e.g. "../").
	clean := filepath.Base(cookbookName)
	if clean == "." || clean == ".." {
		r.logf("WARN", "rejected unsafe cookbook name for clone removal: %q", cookbookName)
		return false
	}

	repoDir := filepath.Join(r.cfg.Storage.GitCookbookDir, clean)
	if _, statErr := os.Stat(repoDir); statErr != nil {
		return false // Directory doesn't exist.
	}
	if rmErr := os.RemoveAll(repoDir); rmErr != nil {
		r.logf("WARN", "failed to remove local git clone for %s at %s: %v",
			cookbookName, repoDir, rmErr)
		return false
	}
	r.logf("INFO", "removed local git clone for %s at %s", cookbookName, repoDir)
	return true
}
