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
// all organisations, optionally filtered by query parameters.
func (r *Router) handleCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbooks: %v", err)
		WriteInternalError(w, "Failed to list cookbooks.")
		return
	}

	var allCookbooks []datastore.Cookbook
	for _, org := range orgs {
		cbs, err := r.db.ListCookbooksByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing cookbooks for org %s: %v", org.Name, err)
			continue
		}
		allCookbooks = append(allCookbooks, cbs...)
	}

	// Also include git-sourced cookbooks.
	gitCookbooks, err := r.db.ListGitCookbooks(req.Context())
	if err != nil {
		r.logf("WARN", "listing git cookbooks: %v", err)
	} else {
		allCookbooks = append(allCookbooks, gitCookbooks...)
	}

	// Apply optional query-parameter filters.
	allCookbooks = filterCookbooks(req, allCookbooks)

	// Collapse chef_server cookbooks by name so the summary page shows one
	// row per cookbook with a version count instead of one row per version.
	allCookbooks, versionCounts := collapseChefServerCookbooks(allCookbooks)

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
		Version         string `json:"version,omitempty"`
		VersionCount    int    `json:"version_count,omitempty"`
		Source          string `json:"source"`
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
			Version:         cb.Version,
			Source:          cb.Source,
			HasTestSuite:    cb.HasTestSuite,
			IsActive:        cb.IsActive,
			IsStaleCookbook: cb.IsStaleCookbook,
			DownloadStatus:  cb.DownloadStatus,
		}
		if count, ok := versionCounts[cb.Name]; ok && cb.Source == "chef_server" {
			resp.VersionCount = count
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

	// Gather complexity and cookstyle results for each cookbook version.
	type versionDetail struct {
		Cookbook   datastore.Cookbook             `json:"cookbook"`
		Complexity []datastore.CookbookComplexity `json:"complexity,omitempty"`
		Cookstyle  []datastore.CookstyleResult    `json:"cookstyle,omitempty"`
	}

	details := make([]versionDetail, 0, len(cookbooks))
	for _, cb := range cookbooks {
		detail := versionDetail{Cookbook: cb}

		complexity, err := r.db.ListCookbookComplexitiesForCookbook(req.Context(), cb.ID)
		if err != nil {
			r.logf("WARN", "listing complexity for cookbook %s: %v", cb.ID, err)
		} else {
			detail.Complexity = complexity
		}

		cookstyle, err := r.db.ListCookstyleResultsForCookbook(req.Context(), cb.ID)
		if err != nil {
			r.logf("WARN", "listing cookstyle results for cookbook %s: %v", cb.ID, err)
		} else {
			detail.Cookstyle = cookstyle
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

// collapseChefServerCookbooks groups chef_server cookbooks by name, keeping
// only the first occurrence of each name while recording the total version
// count. Git-sourced cookbooks are passed through unchanged. The returned
// map holds the version count keyed by cookbook name (only for chef_server
// entries with more than one version).
func collapseChefServerCookbooks(cookbooks []datastore.Cookbook) ([]datastore.Cookbook, map[string]int) {
	versionCounts := make(map[string]int)
	seen := make(map[string]bool)
	collapsed := make([]datastore.Cookbook, 0, len(cookbooks))

	// First pass: count versions per chef_server cookbook name.
	for _, cb := range cookbooks {
		if cb.Source == "chef_server" {
			versionCounts[cb.Name]++
		}
	}

	// Second pass: keep git cookbooks as-is; for chef_server, keep only
	// the first occurrence of each name.
	for _, cb := range cookbooks {
		if cb.Source != "chef_server" {
			collapsed = append(collapsed, cb)
			continue
		}
		if seen[cb.Name] {
			continue
		}
		seen[cb.Name] = true
		collapsed = append(collapsed, cb)
	}

	return collapsed, versionCounts
}

// filterCookbooks applies optional query-parameter filters (source, active,
// name) to the given slice, returning only matching cookbooks. The name
// filter uses case-insensitive partial (substring) matching.
func filterCookbooks(req *http.Request, cookbooks []datastore.Cookbook) []datastore.Cookbook {
	q := req.URL.Query()
	source := q.Get("source")
	active := q.Get("active")
	nameFilter := q.Get("name")

	if source == "" && active == "" && nameFilter == "" {
		return cookbooks
	}

	filtered := make([]datastore.Cookbook, 0, len(cookbooks))
	for _, cb := range cookbooks {
		if source != "" && cb.Source != source {
			continue
		}
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
