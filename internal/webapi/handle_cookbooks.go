// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// cookbookSummary is a unified view used for the cookbook list endpoint.
// It can represent either a ServerCookbook or a GitRepo so the API can
// present a single collapsed-by-name list.
type cookbookSummary struct {
	ID              string
	OrganisationID  string
	Name            string
	Version         string
	Source          string // "chef_server" or "git"
	HasTestSuite    bool
	IsActive        bool
	IsStaleCookbook bool
	DownloadStatus  string
	DownloadError   string
}

func serverCookbookToSummary(sc datastore.ServerCookbook) cookbookSummary {
	return cookbookSummary{
		ID:              sc.ID,
		OrganisationID:  sc.OrganisationID,
		Name:            sc.Name,
		Version:         sc.Version,
		Source:          "chef_server",
		HasTestSuite:    false,
		IsActive:        sc.IsActive,
		IsStaleCookbook: sc.IsStaleCookbook,
		DownloadStatus:  sc.DownloadStatus,
		DownloadError:   sc.DownloadError,
	}
}

func gitRepoToSummary(gr datastore.GitRepo) cookbookSummary {
	return cookbookSummary{
		ID:             gr.ID,
		Name:           gr.Name,
		Source:         "git",
		HasTestSuite:   gr.HasTestSuite,
		IsActive:       true,
		DownloadStatus: "ok",
	}
}

// handleCookbooks handles GET /api/v1/cookbooks — lists all cookbooks across
// all organisations, optionally filtered by query parameters. Cookbooks are
// collapsed by name so each unique cookbook name appears once with a total
// version count across all sources (git and chef_server).
func (r *Router) handleCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned cookbook keys when ownership filtering is active.
	var ownedKeys map[string]bool
	if of.Active && r.cfg.Ownership.Enabled {
		ctx := req.Context()
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving all owned cookbook keys: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving owned cookbook keys: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbooks: %v", err)
		WriteInternalError(w, "Failed to list cookbooks.")
		return
	}

	// Collect git-sourced cookbooks first so they become the preferred
	// representative entry when collapsing by name.
	var allCookbooks []cookbookSummary
	gitRepos, err := r.db.ListGitRepos(req.Context())
	if err != nil {
		r.logf("WARN", "listing git repos: %v", err)
	} else {
		for _, gr := range gitRepos {
			allCookbooks = append(allCookbooks, gitRepoToSummary(gr))
		}
	}

	for _, org := range orgs {
		cbs, err := r.db.ListServerCookbooksByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing server cookbooks for org %s: %v", org.Name, err)
			continue
		}
		for _, sc := range cbs {
			allCookbooks = append(allCookbooks, serverCookbookToSummary(sc))
		}
	}

	// Apply optional query-parameter filters.
	allCookbooks = filterCookbookSummaries(req, allCookbooks)

	// Collapse all cookbooks by name so the summary page shows one row per
	// cookbook with a total version count across all sources.
	allCookbooks, versionCounts := collapseCookbookSummaries(allCookbooks)

	// Apply owner filter if active and ownership is enabled.
	if of.Active && r.cfg.Ownership.Enabled && ownedKeys != nil {
		if of.Unowned {
			filtered := allCookbooks[:0]
			for _, cb := range allCookbooks {
				if !ownedKeys[cb.Name] {
					filtered = append(filtered, cb)
				}
			}
			allCookbooks = filtered
		} else {
			filtered := allCookbooks[:0]
			for _, cb := range allCookbooks {
				if ownedKeys[cb.Name] {
					filtered = append(filtered, cb)
				}
			}
			allCookbooks = filtered
		}
	}

	// Paginate the results.
	pg := ParsePagination(req)
	total := len(allCookbooks)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	type cookbookResp struct {
		ID              string `json:"id"`
		OrganisationID  string `json:"organisation_id,omitempty"`
		Name            string `json:"name"`
		VersionCount    int    `json:"version_count"`
		HasTestSuite    bool   `json:"has_test_suite"`
		IsActive        bool   `json:"is_active"`
		IsStaleCookbook bool   `json:"is_stale_cookbook"`
		DownloadStatus  string `json:"download_status"`
	}

	result := make([]cookbookResp, 0, end-start)
	for _, cb := range allCookbooks[start:end] {
		resp := cookbookResp{
			ID:              cb.ID,
			OrganisationID:  cb.OrganisationID,
			Name:            cb.Name,
			HasTestSuite:    cb.HasTestSuite,
			IsActive:        cb.IsActive,
			IsStaleCookbook: cb.IsStaleCookbook,
			DownloadStatus:  cb.DownloadStatus,
			VersionCount:    versionCounts[cb.Name],
		}
		result = append(result, resp)
	}

	WritePaginated(w, result, pg, total)
}

// handleCookbookDetail handles GET /api/v1/cookbooks/:name — returns all
// versions of a cookbook by name, along with complexity and compatibility
// information.
func (r *Router) handleCookbookDetail(w http.ResponseWriter, req *http.Request) {
	// Check for sub-path dispatching.
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")

	// /api/v1/cookbooks/:name/:version/remediation
	if len(segments) >= 3 && segments[len(segments)-1] == "remediation" {
		r.handleCookbookRemediation(w, req)
		return
	}

	// /api/v1/cookbooks/:name/rescan
	if len(segments) >= 2 && segments[len(segments)-1] == "rescan" {
		r.handleCookbookRescan(w, req)
		return
	}

	// /api/v1/cookbooks/:name/reset-git
	if len(segments) >= 2 && segments[len(segments)-1] == "reset-git" {
		r.handleCookbookResetGit(w, req)
		return
	}

	// /api/v1/cookbooks/:name/committers[/assign]
	if len(segments) >= 2 && segments[1] == "committers" {
		cookbookName := segments[0]
		if len(segments) == 3 && segments[2] == "assign" {
			r.handleCookbookCommittersAssign(w, req, cookbookName)
			return
		}
		if len(segments) == 2 {
			r.handleCookbookCommitters(w, req, cookbookName)
			return
		}
		WriteNotFound(w, fmt.Sprintf("Unknown committers endpoint: %s", req.URL.Path))
		return
	}

	name := pathParam(req, "/api/v1/cookbooks/")
	if name == "" {
		WriteNotFound(w, "Cookbook name is required.")
		return
	}

	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Gather server cookbook versions.
	serverCookbooks, err := r.db.ListServerCookbooksByName(ctx, name)
	if err != nil {
		r.logf("ERROR", "listing server cookbook versions for %s: %v", name, err)
		WriteInternalError(w, "Failed to get cookbook.")
		return
	}

	// Gather git repo entries.
	gitRepos, err := r.db.ListGitReposByName(ctx, name)
	if err != nil {
		r.logf("ERROR", "listing git repos for %s: %v", name, err)
		WriteInternalError(w, "Failed to get cookbook.")
		return
	}

	if len(serverCookbooks) == 0 && len(gitRepos) == 0 {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q not found.", name))
		return
	}

	// Build version details for server cookbooks.
	type serverVersionDetail struct {
		Cookbook  datastore.ServerCookbook                  `json:"cookbook"`
		Cookstyle []datastore.ServerCookbookCookstyleResult `json:"cookstyle,omitempty"`
	}

	type gitRepoDetail struct {
		GitRepo     datastore.GitRepo                    `json:"git_repo"`
		Cookstyle   []datastore.GitRepoCookstyleResult   `json:"cookstyle,omitempty"`
		TestKitchen []datastore.GitRepoTestKitchenResult `json:"test_kitchen,omitempty"`
	}

	serverDetails := make([]serverVersionDetail, 0, len(serverCookbooks))
	for _, sc := range serverCookbooks {
		detail := serverVersionDetail{Cookbook: sc}

		cookstyle, csErr := r.db.ListServerCookbookCookstyleResults(ctx, sc.ID)
		if csErr != nil {
			r.logf("WARN", "listing cookstyle results for server cookbook %s: %v", sc.ID, csErr)
		} else {
			detail.Cookstyle = cookstyle
		}

		serverDetails = append(serverDetails, detail)
	}

	gitDetails := make([]gitRepoDetail, 0, len(gitRepos))
	for _, gr := range gitRepos {
		detail := gitRepoDetail{GitRepo: gr}

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

		gitDetails = append(gitDetails, detail)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"name":             name,
		"server_cookbooks": serverDetails,
		"git_repos":        gitDetails,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collapseCookbookSummaries groups all cookbook summaries by name regardless of
// source, keeping only the first occurrence of each name as the representative
// entry while counting every version. Because git-sourced entries are
// appended to the slice before chef_server ones, the git entry is
// naturally preferred as the representative when both exist.
func collapseCookbookSummaries(cookbooks []cookbookSummary) ([]cookbookSummary, map[string]int) {
	versionCounts := make(map[string]int)
	seen := make(map[string]bool)
	collapsed := make([]cookbookSummary, 0, len(cookbooks))

	// First pass: count all versions per cookbook name.
	for _, cb := range cookbooks {
		versionCounts[cb.Name]++
	}

	// Second pass: keep only the first occurrence of each name.
	for _, cb := range cookbooks {
		if seen[cb.Name] {
			continue
		}
		seen[cb.Name] = true
		collapsed = append(collapsed, cb)
	}

	return collapsed, versionCounts
}

// filterCookbookSummaries applies optional query-parameter filters (active,
// name) to the given slice, returning only matching entries. The name filter
// uses case-insensitive partial (substring) matching.
func filterCookbookSummaries(req *http.Request, cookbooks []cookbookSummary) []cookbookSummary {
	q := req.URL.Query()
	active := q.Get("active")
	nameFilter := q.Get("name")

	if active == "" && nameFilter == "" {
		return cookbooks
	}

	filtered := make([]cookbookSummary, 0, len(cookbooks))
	for _, cb := range cookbooks {
		if active == "true" && !cb.IsActive {
			continue
		}
		if active == "false" && cb.IsActive {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(cb.Name), strings.ToLower(nameFilter)) {
			continue
		}
		filtered = append(filtered, cb)
	}
	return filtered
}

// Ensure datastore.ErrNotFound is used (compile-time check).
var _ = errors.Is(nil, datastore.ErrNotFound)

// Ensure sort is used.
var _ = sort.Slice
