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

// cookbookSummary represents a server cookbook entry on the cookbook list page.
// Git repos have their own dedicated page and are not included here.
type cookbookSummary struct {
	ID              string
	OrganisationID  string
	Name            string
	Version         string
	IsActive        bool
	IsStaleCookbook bool
	DownloadStatus  string
	DownloadError   string
	Compatibility   string // "compatible", "incompatible", "untested"
}

func serverCookbookToSummary(sc datastore.ServerCookbook) cookbookSummary {
	return cookbookSummary{
		ID:              sc.ID,
		OrganisationID:  sc.OrganisationID,
		Name:            sc.Name,
		Version:         sc.Version,
		IsActive:        sc.IsActive,
		IsStaleCookbook: sc.IsStaleCookbook,
		DownloadStatus:  sc.DownloadStatus,
		DownloadError:   sc.DownloadError,
	}
}

// handleCookbooks handles GET /api/v1/cookbooks — lists all server cookbooks
// across all organisations, optionally filtered by query parameters. Cookbooks
// are collapsed by name so each unique cookbook name appears once with a total
// version count. Git repos have their own dedicated list page.
func (r *Router) handleCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned cookbook keys when ownership filtering is active.
	var ownedKeys map[string]bool
	if of.Active && r.cfg.Ownership.Enabled {
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

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbooks: %v", err)
		WriteInternalError(w, "Failed to list cookbooks.")
		return
	}

	var allCookbooks []cookbookSummary
	for _, org := range orgs {
		cbs, err := r.db.ListServerCookbooksByOrganisation(ctx, org.ID)
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

	// Compute compatibility per cookbook name from cookstyle results.
	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	compatByName := make(map[string]string)
	if targetChefVersion != "" {
		for _, org := range orgs {
			csResults, cErr := r.db.ListServerCookbookCookstyleResultsByOrganisation(ctx, org.ID)
			if cErr != nil {
				r.logf("WARN", "listing cookstyle results for org %s: %v", org.Name, cErr)
				continue
			}
			serverCBs, scErr := r.db.ListServerCookbooksByOrganisation(ctx, org.ID)
			if scErr != nil {
				r.logf("WARN", "listing server cookbooks for org %s: %v", org.Name, scErr)
				continue
			}
			nameByID := make(map[string]string, len(serverCBs))
			for _, sc := range serverCBs {
				nameByID[sc.ID] = sc.Name
			}
			for _, cs := range csResults {
				if cs.TargetChefVersion != targetChefVersion {
					continue
				}
				cbName := nameByID[cs.ServerCookbookID]
				if cbName == "" {
					continue
				}
				if _, seen := compatByName[cbName]; seen {
					continue // first version wins
				}
				if cs.Passed {
					compatByName[cbName] = "compatible"
				} else {
					compatByName[cbName] = "incompatible"
				}
			}
		}
	}

	// Assign compatibility to each collapsed cookbook.
	for i := range allCookbooks {
		if c, ok := compatByName[allCookbooks[i].Name]; ok {
			allCookbooks[i].Compatibility = c
		} else {
			allCookbooks[i].Compatibility = "untested"
		}
	}

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

	// Apply compatibility filter if specified.
	compatFilter := req.URL.Query().Get("compatibility")
	if compatFilter != "" {
		filtered := allCookbooks[:0]
		for _, cb := range allCookbooks {
			if cb.Compatibility == compatFilter {
				filtered = append(filtered, cb)
			}
		}
		allCookbooks = filtered
	}

	// Sort the results.
	sortField := queryString(req, "sort", "name")
	sortOrder := queryString(req, "order", "asc")
	sortCookbookSummaries(allCookbooks, sortField, sortOrder)

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
		ID                string `json:"id"`
		OrganisationID    string `json:"organisation_id,omitempty"`
		Name              string `json:"name"`
		VersionCount      int    `json:"version_count"`
		IsActive          bool   `json:"is_active"`
		IsStaleCookbook   bool   `json:"is_stale_cookbook"`
		DownloadStatus    string `json:"download_status"`
		DownloadError     string `json:"download_error,omitempty"`
		Compatibility     string `json:"compatibility"`
		TargetChefVersion string `json:"target_chef_version,omitempty"`
	}

	result := make([]cookbookResp, 0, end-start)
	for _, cb := range allCookbooks[start:end] {
		resp := cookbookResp{
			ID:                cb.ID,
			OrganisationID:    cb.OrganisationID,
			Name:              cb.Name,
			IsActive:          cb.IsActive,
			IsStaleCookbook:   cb.IsStaleCookbook,
			DownloadStatus:    cb.DownloadStatus,
			DownloadError:     cb.DownloadError,
			VersionCount:      versionCounts[cb.Name],
			Compatibility:     cb.Compatibility,
			TargetChefVersion: targetChefVersion,
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

// collapseCookbookSummaries groups server cookbook summaries by name, keeping
// only the first occurrence of each name as the representative entry while
// counting every version.
func collapseCookbookSummaries(cookbooks []cookbookSummary) ([]cookbookSummary, map[string]int) {
	versionCounts := make(map[string]int)
	seen := make(map[string]bool)
	collapsed := make([]cookbookSummary, 0, len(cookbooks))

	for _, cb := range cookbooks {
		versionCounts[cb.Name]++
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

// sortCookbookSummaries sorts the collapsed cookbook list in-place by the
// given field and order ("asc" or "desc"). Supported fields: "name"
// (default), "compatibility", "active".
func sortCookbookSummaries(items []cookbookSummary, field, order string) {
	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch field {
		case "compatibility":
			less = items[i].Compatibility < items[j].Compatibility
		case "active":
			// false < true so inactive sorts first in ascending
			less = !items[i].IsActive && items[j].IsActive
		default: // "name"
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if strings.EqualFold(order, "desc") {
			return !less
		}
		return less
	})
}
