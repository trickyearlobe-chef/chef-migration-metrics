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
	var allCookbooks []datastore.Cookbook
	gitCookbooks, err := r.db.ListGitCookbooks(req.Context())
	if err != nil {
		r.logf("WARN", "listing git cookbooks: %v", err)
	} else {
		allCookbooks = append(allCookbooks, gitCookbooks...)
	}

	for _, org := range orgs {
		cbs, err := r.db.ListCookbooksByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing cookbooks for org %s: %v", org.Name, err)
			continue
		}
		allCookbooks = append(allCookbooks, cbs...)
	}

	// Apply optional query-parameter filters.
	allCookbooks = filterCookbooks(req, allCookbooks)

	// Collapse all cookbooks by name so the summary page shows one row per
	// cookbook with a total version count across all sources.
	allCookbooks, versionCounts := collapseCookbooks(allCookbooks)

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

	cookbooks, err := r.db.ListCookbooksByName(req.Context(), name)
	if err != nil {
		r.logf("ERROR", "listing cookbook versions for %s: %v", name, err)
		WriteInternalError(w, "Failed to get cookbook.")
		return
	}
	if len(cookbooks) == 0 {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q not found.", name))
		return
	}

	// Gather cookstyle and test kitchen results for each cookbook version.
	type versionDetail struct {
		Cookbook    datastore.Cookbook            `json:"cookbook"`
		Cookstyle   []datastore.CookstyleResult   `json:"cookstyle,omitempty"`
		TestKitchen []datastore.TestKitchenResult `json:"test_kitchen,omitempty"`
	}

	details := make([]versionDetail, 0, len(cookbooks))
	for _, cb := range cookbooks {
		detail := versionDetail{Cookbook: cb}

		cookstyle, err := r.db.ListCookstyleResultsForCookbook(req.Context(), cb.ID)
		if err != nil {
			r.logf("WARN", "listing cookstyle results for cookbook %s: %v", cb.ID, err)
		} else {
			detail.Cookstyle = cookstyle
		}

		if cb.Source == "git" {
			tk, err := r.db.ListTestKitchenResultsForCookbook(req.Context(), cb.ID)
			if err != nil {
				r.logf("WARN", "listing test kitchen results for cookbook %s: %v", cb.ID, err)
			} else {
				detail.TestKitchen = tk
			}
		}

		details = append(details, detail)
	}

	// Sort so that git-sourced cookbooks appear before chef_server ones.
	sort.SliceStable(details, func(i, j int) bool {
		if details[i].Cookbook.Source == details[j].Cookbook.Source {
			return false
		}
		return details[i].Cookbook.Source == "git"
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"name": name,
		"data": details,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collapseCookbooks groups all cookbooks by name regardless of source,
// keeping only the first occurrence of each name as the representative
// entry while counting every version. Because git-sourced cookbooks are
// appended to the slice before chef_server ones, the git entry is
// naturally preferred as the representative when both exist.
func collapseCookbooks(cookbooks []datastore.Cookbook) ([]datastore.Cookbook, map[string]int) {
	versionCounts := make(map[string]int)
	seen := make(map[string]bool)
	collapsed := make([]datastore.Cookbook, 0, len(cookbooks))

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

// filterCookbooks applies optional query-parameter filters (active, name)
// to the given slice, returning only matching cookbooks. The name filter
// uses case-insensitive partial (substring) matching.
func filterCookbooks(req *http.Request, cookbooks []datastore.Cookbook) []datastore.Cookbook {
	q := req.URL.Query()
	active := q.Get("active")
	nameFilter := q.Get("name")

	if active == "" && nameFilter == "" {
		return cookbooks
	}

	filtered := make([]datastore.Cookbook, 0, len(cookbooks))
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
