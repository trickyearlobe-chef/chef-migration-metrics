// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// ---------------------------------------------------------------------------
// Git Repos List endpoint
//
// GET /api/v1/git-repos
//
// Returns all git repos, optionally filtered by name (substring match),
// compatibility status, clone status, and/or TK status. Each repo includes
// a compatibility field ("compatible", "incompatible", or "untested")
// computed from git repo complexity records for the specified target Chef
// version.
//
// Supports pagination via page/per_page query parameters.
// Supports sorting via sort/order query parameters on: name, has_test_suite,
// compatibility, tk_status, last_fetched, git_url, clone_status.

// gitRepoResp is the JSON response item for the git repos list.
type gitRepoResp struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	GitRepoURL        string `json:"git_repo_url"`
	HeadCommitSHA     string `json:"head_commit_sha,omitempty"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	HasTestSuite      bool   `json:"has_test_suite"`
	CloneStatus       string `json:"clone_status"`
	CloneError        string `json:"clone_error,omitempty"`
	LastFetchedAt     string `json:"last_fetched_at,omitempty"`
	Compatibility     string `json:"compatibility"`
	TargetChefVersion string `json:"target_chef_version,omitempty"`
	TKStatus          string `json:"tk_status"`
	TKPassed          int    `json:"tk_passed"`
	TKTotal           int    `json:"tk_total"`
}
//
// Query parameters:
//   - name: case-insensitive substring filter on repo name
//   - target_chef_version: Chef version to evaluate compatibility against
//     (defaults to the first configured target version)
//   - compatibility: filter by status — "compatible", "incompatible",
//     "untested", or "" (no filter)
//   - clone_status: filter by clone/fetch status — "ok", "failed",
//     "pending", or "" (no filter)
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

	// Parse query params into filter struct.
	f := datastore.GitRepoFilter{
		Name:                queryString(req, "name", ""),
		CompatibilityStatus: queryString(req, "compatibility", ""),
		TKStatus:            queryString(req, "tk_status", ""),
		CloneStatus:         queryString(req, "clone_status", ""),
		Sort:                queryString(req, "sort", "name"),
		SortOrder:           queryString(req, "order", "asc"),
	}

	// Parse has_test_suite filter (yes, no, yes,no — comma-separated).
	if tsf := queryString(req, "has_test_suite", ""); tsf != "" {
		wantYes := false
		wantNo := false
		for _, v := range strings.Split(tsf, ",") {
			switch strings.TrimSpace(v) {
			case "yes":
				wantYes = true
			case "no":
				wantNo = true
			}
		}
		// Only filter if not both selected (both = no filter).
		if wantYes != wantNo {
			b := wantYes
			f.HasTestSuite = &b
		}
	}

	// Pagination.
	pg := ParsePagination(req)
	f.Limit = pg.PerPage
	f.Offset = (pg.Page - 1) * pg.PerPage

	repos, total, err := r.db.ListGitReposFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing git repos (filtered): %v", err)
		WriteInternalError(w, "Failed to list git repos.")
		return
	}

	// Determine target Chef version for response metadata.
	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	// Build response objects from materialised columns.
	result := make([]gitRepoResp, 0, len(repos))
	for _, gr := range repos {
		compat := gr.CompatibilityStatus
		if compat == "" {
			compat = "untested"
		}
		tkStatus := gr.TKStatus
		if tkStatus == "" {
			tkStatus = "untested"
		}
		resp := gitRepoResp{
			ID:                gr.Name,
			Name:              gr.Name,
			GitRepoURL:        gr.GitRepoURL,
			HeadCommitSHA:     gr.HeadCommitSHA,
			DefaultBranch:     gr.DefaultBranch,
			HasTestSuite:      gr.HasTestSuite,
			CloneStatus:       gr.CloneStatus,
			CloneError:        gr.CloneError,
			Compatibility:     compat,
			TargetChefVersion: targetChefVersion,
			TKStatus:          tkStatus,
			TKPassed:          gr.TKPassed,
			TKTotal:           gr.TKTotal,
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
// with cookstyle results and complexity records.
//
// Also dispatches to sub-path handlers:
//   - /api/v1/git-repos/:name/committers              → handleGitRepoCommitters
//   - /api/v1/git-repos/:name/committers/assign       → handleGitRepoCommittersAssign
//   - /api/v1/git-repos/:name/rescan                  → handleGitRepoRescan
//   - /api/v1/git-repos/:name/reset                   → handleGitRepoReset
//   - /api/v1/git-repos/:name/:version/remediation    → handleGitRepoRemediation
//
// Response (200):
//
//	{
//	  "name": "cron",
//	  "git_repos": [
//	    {
//	      "git_repo": { ... },
//	      "cookstyle": [ ... ],
//	      "complexity": [ ... ]
//	    }
//	  ]
//	}
//
// ---------------------------------------------------------------------------

// handleGitRepoDetail handles /api/v1/git-repos/:name and sub-path dispatch.
func (r *Router) handleGitRepoDetail(w http.ResponseWriter, req *http.Request) {
	segments := pathSegments(req.URL.Path, "/api/v1/git-repos/")

	// /api/v1/git-repos/excluded — list all excluded repos
	if len(segments) == 1 && segments[0] == "excluded" {
		r.handleListExcludedGitRepos(w, req)
		return
	}

	// /api/v1/git-repos/:name/exclude — POST to exclude, DELETE to clear
	if len(segments) == 2 && segments[1] == "exclude" {
		repoName := segments[0]
		switch req.Method {
		case http.MethodPost:
			r.handleGitRepoExclude(w, req, repoName)
		case http.MethodDelete:
			r.handleGitRepoClearExclusion(w, req, repoName)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"This endpoint supports POST and DELETE.")
		}
		return
	}

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

	// /api/v1/git-repos/:name/files — list directory
	// /api/v1/git-repos/:name/files/content — read file content
	if len(segments) >= 2 && segments[1] == "files" {
		repoName := segments[0]
		if len(segments) == 3 && segments[2] == "content" {
			r.handleGitRepoFileContent(w, req, repoName)
			return
		}
		if len(segments) == 2 {
			r.handleGitRepoFileTree(w, req, repoName)
			return
		}
		WriteNotFound(w, fmt.Sprintf("Unknown files endpoint: %s", req.URL.Path))
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
		Complexity  []datastore.GitRepoComplexity        `json:"complexity,omitempty"`
		TKStatus    string                               `json:"tk_status,omitempty"`
		TKPassed    int                                  `json:"tk_passed,omitempty"`
		TKTotal     int                                  `json:"tk_total,omitempty"`
	}

	details := make([]gitRepoDetailEntry, 0, len(gitRepos))
	for _, gr := range gitRepos {
		detail := gitRepoDetailEntry{GitRepo: gr}

		cookstyle, csErr := r.db.ListGitRepoCookstyleResults(ctx, gr.Name, gr.GitRepoURL)
		if csErr != nil {
			r.logf("WARN", "listing cookstyle results for git repo %s: %v", gr.Name, csErr)
		} else {
			detail.Cookstyle = cookstyle
		}

		complexity, cxErr := r.db.ListGitRepoComplexitiesByRepo(ctx, gr.Name, gr.GitRepoURL)
		if cxErr != nil {
			r.logf("WARN", "listing complexity for git repo %s: %v", gr.Name, cxErr)
		} else {
			detail.Complexity = complexity
		}

		details = append(details, detail)
	}

	// Enrich with TK summary from active kitchen results.
	activeResults, tkErr := r.db.ListActiveGitKitchenResults(ctx)
	if tkErr != nil {
		r.logf("WARN", "listing active kitchen results for git repo detail %s: %v", name, tkErr)
	} else {
		tkByRepo := buildTKSummaryMap(activeResults)
		for i := range details {
			if s := tkByRepo[details[i].GitRepo.Name]; s != nil && s.Total > 0 {
				details[i].TKStatus = tkstatus.ComputeTKStatus(s.Passed, s.Failed)
				details[i].TKPassed = s.Passed
				details[i].TKTotal = s.Total
			}
		}
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
		csErr := r.db.DeleteGitRepoCookstyleResultsByRepo(ctx, gr.Name, gr.GitRepoURL)
		if csErr != nil {
			r.logf("WARN", "deleting cookstyle results for git repo %s (%s): %v", gr.Name, gr.Name, csErr)
			lastErr = csErr
		}

		cxErr := r.db.DeleteGitRepoComplexitiesByRepo(ctx, gr.Name, gr.GitRepoURL)
		if cxErr != nil {
			r.logf("WARN", "deleting complexity records for git repo %s (%s): %v", gr.Name, gr.Name, cxErr)
			lastErr = cxErr
		}

		acErr := r.db.DeleteGitRepoAutocorrectPreviewsByRepo(ctx, gr.Name, gr.GitRepoURL)
		if acErr != nil {
			r.logf("WARN", "deleting autocorrect previews for git repo %s (%s): %v", gr.Name, gr.Name, acErr)
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

	// Trigger an immediate collection run in the background so the rescan
	// starts right away instead of waiting for the next scheduled cycle.
	triggered := r.triggerCollectionInBackground()

	msg := "Analysis results invalidated"
	if triggered {
		msg += " — collection run triggered."
	} else {
		msg += " — rescan will run on the next collection cycle."
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"git_repo_name":        repoName,
		"repos_invalidated":    invalidated,
		"collection_triggered": triggered,
		"message":              msg,
	})
}

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
// TK Summary helper
// ---------------------------------------------------------------------------

// tkRepoSummary holds aggregated TK pass/fail counts for a single repo.
type tkRepoSummary struct {
	Passed int
	Failed int
	Total  int
}

// buildTKSummaryMap groups kitchen results by repo name and counts pass/fail.
func buildTKSummaryMap(results []datastore.GitKitchenResult) map[string]*tkRepoSummary {
	m := make(map[string]*tkRepoSummary)
	for _, res := range results {
		if res.Passed == nil {
			continue
		}
		s := m[res.GitRepoName]
		if s == nil {
			s = &tkRepoSummary{}
			m[res.GitRepoName] = s
		}
		s.Total++
		if *res.Passed {
			s.Passed++
		} else {
			s.Failed++
		}
	}
	return m
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

// sortGitRepoResps sorts enriched git repo response items by the given field.
func sortGitRepoResps(items []gitRepoResp, field, order string) {
	statusRank := func(s string) int {
		switch s {
		case "failed":
			return 0
		case "partial":
			return 1
		case "incompatible":
			return 0
		case "compatible", "passed":
			return 2
		default: // untested, pending, etc.
			return 3
		}
	}

	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch field {
		case "compatibility":
			ri, rj := statusRank(items[i].Compatibility), statusRank(items[j].Compatibility)
			if ri != rj {
				less = ri < rj
			} else {
				less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
		case "tk_status":
			ri, rj := statusRank(items[i].TKStatus), statusRank(items[j].TKStatus)
			if ri != rj {
				less = ri < rj
			} else {
				less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
		case "last_fetched":
			less = items[i].LastFetchedAt < items[j].LastFetchedAt
		case "git_url":
			less = strings.ToLower(items[i].GitRepoURL) < strings.ToLower(items[j].GitRepoURL)
		case "clone_status":
			less = items[i].CloneStatus < items[j].CloneStatus
		case "has_test_suite":
			less = !items[i].HasTestSuite && items[j].HasTestSuite
		default: // "name"
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if strings.EqualFold(order, "desc") {
			return !less
		}
		return less
	})
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

// ---------------------------------------------------------------------------
// GET /api/v1/git-repos/excluded
// ---------------------------------------------------------------------------

// handleListExcludedGitRepos returns all git repos excluded from kitchen
// testing.
func (r *Router) handleListExcludedGitRepos(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	repos, err := r.db.ListExcludedGitRepos(req.Context())
	if err != nil {
		r.logf("ERROR", "git-repos: listing excluded repos: %v", err)
		WriteInternalError(w, "Failed to list excluded git repos.")
		return
	}
	if repos == nil {
		repos = []datastore.GitRepo{}
	}
	WriteJSON(w, http.StatusOK, repos)
}

// ---------------------------------------------------------------------------
// POST /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

// handleGitRepoExclude marks a git repo as excluded from kitchen testing.
func (r *Router) handleGitRepoExclude(w http.ResponseWriter, req *http.Request, name string) {
	if !requirePOST(w, req) {
		return
	}

	var body struct {
		Reason     string `json:"reason"`
		ExcludedBy string `json:"excluded_by"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	err := r.db.SetGitRepoKitchenExclusion(req.Context(), name, body.Reason, body.ExcludedBy)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", name))
			return
		}
		r.logf("ERROR", "git-repos: excluding repo %s: %v", name, err)
		WriteInternalError(w, "Failed to exclude git repo.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Git repo %q excluded from kitchen testing.", name),
	})
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

// handleGitRepoClearExclusion removes the kitchen exclusion flag from a
// git repo.
func (r *Router) handleGitRepoClearExclusion(w http.ResponseWriter, req *http.Request, name string) {
	if !requireMethod(w, req, http.MethodDelete) {
		return
	}

	err := r.db.ClearGitRepoKitchenExclusion(req.Context(), name)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", name))
			return
		}
		r.logf("ERROR", "git-repos: clearing exclusion for repo %s: %v", name, err)
		WriteInternalError(w, "Failed to clear git repo exclusion.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
