// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// cookbookRow represents a single server cookbook version in the list.
// Each row is a specific version from a specific organisation — no collapsing.
type cookbookRow struct {
	OrganisationName string
	Name             string
	Version          string
	IsActive         bool
	IsStaleCookbook  bool
	DownloadStatus   string
	DownloadError    string
	Compatibility    string // "compatible", "incompatible", "untested"
}

// key returns the composite natural key for this cookbook row.
func (c cookbookRow) key() string {
	return c.OrganisationName + "/" + c.Name + "/" + c.Version
}

// handleCookbooks handles GET /api/v1/cookbooks — lists all server cookbooks
// across all organisations. Each row is a specific cookbook version in a
// specific organisation. Git repos have their own dedicated list page.
//
// Supported query-parameter filters:
//
//	active           — "true" or "false"
//	name             — case-insensitive substring match
//	compatibility    — "compatible", "incompatible", or "untested"
//	download_status  — "ok", "pending", or "failed"
//	organisation     — filter by organisation name(s), comma-separated
//	target_chef_version — which target version to evaluate compatibility against
//	sort             — "name" (default), "version", "compatibility", "active", "download_status"
//	order            — "asc" (default) or "desc"
//	page / per_page  — pagination
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

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbooks: %v", err)
		WriteInternalError(w, "Failed to list cookbooks.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	q := req.URL.Query()
	pg := ParsePagination(req)

	// Build SQL filter from query parameters.
	f := datastore.CookbookFilter{
		OrganisationNames: orgIDs,
		Name:              q.Get("name"),
		DownloadStatus:    q.Get("download_status"),
		Compatibility:     q.Get("compatibility"),
		TKStatus:          q.Get("tk_status"),
		TargetChefVersion: targetChefVersion,
		Sort:              queryString(req, "sort", "name"),
		SortOrder:         queryString(req, "order", "asc"),
		Limit:             pg.Limit(),
		Offset:            pg.Offset(),
	}

	// Parse the active filter (bool pointer — nil means no filter).
	switch q.Get("active") {
	case "true":
		v := true
		f.Active = &v
	case "false":
		v := false
		f.Active = &v
	}

	// When ownership filtering is active, we disable SQL pagination and
	// apply ownership + pagination in memory (same pattern as nodes).
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		f.Limit = 0
		f.Offset = 0
	}

	rows, total, err := r.db.ListCookbooksFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing filtered cookbooks: %v", err)
		WriteInternalError(w, "Failed to list cookbooks.")
		return
	}

	// Apply ownership filter in memory if active.
	if ownerFilterActive {
		ownedKeys, oErr := r.resolveOwnershipFilter(ctx, of, "cookbook")
		if oErr != nil {
			r.logf("ERROR", "resolving cookbook ownership filter: %v", oErr)
			WriteInternalError(w, "Failed to resolve ownership filter.")
			return
		}
		rows = filterByOwnershipKey(rows, ownedKeys, of, func(cb datastore.CookbookFilterRow) string { return cb.Name })
		total = len(rows)
		pageRows, _ := PaginateSlice(rows, pg)
		rows = pageRows
	}

	type cookbookResp struct {
		ID                string `json:"id"`
		OrganisationID    string `json:"organisation_id,omitempty"`
		OrganisationName  string `json:"organisation_name,omitempty"`
		Name              string `json:"name"`
		Version           string `json:"version"`
		IsActive          bool   `json:"is_active"`
		IsStaleCookbook   bool   `json:"is_stale_cookbook"`
		DownloadStatus    string `json:"download_status"`
		DownloadError     string `json:"download_error,omitempty"`
		Compatibility     string `json:"compatibility"`
		TargetChefVersion string `json:"target_chef_version,omitempty"`
		TKStatus          string `json:"tk_status,omitempty"`
	}

	result := make([]cookbookResp, 0, len(rows))
	for _, cb := range rows {
		tkDisplay := cb.TKStatus
		if tkDisplay == "no_repo" {
			tkDisplay = ""
		}
		resp := cookbookResp{
			ID:                cb.OrganisationName + "/" + cb.Name + "/" + cb.Version,
			OrganisationID:    cb.OrganisationName,
			OrganisationName:  cb.OrganisationName,
			Name:              cb.Name,
			Version:           cb.Version,
			IsActive:          cb.IsActive,
			IsStaleCookbook:   cb.IsStaleCookbook,
			DownloadStatus:    cb.DownloadStatus,
			DownloadError:     cb.DownloadError,
			Compatibility:     cb.Compatibility,
			TargetChefVersion: targetChefVersion,
			TKStatus:          tkDisplay,
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

	// /api/v1/cookbooks/:name/platform-coverage
	if len(segments) >= 2 && segments[len(segments)-1] == "platform-coverage" {
		r.handleCookbookPlatformCoverage(w, req)
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
		GitRepo   datastore.GitRepo                  `json:"git_repo"`
		Cookstyle []datastore.GitRepoCookstyleResult `json:"cookstyle,omitempty"`
	}

	serverDetails := make([]serverVersionDetail, 0, len(serverCookbooks))
	for _, sc := range serverCookbooks {
		detail := serverVersionDetail{Cookbook: sc}

		cookstyle, csErr := r.db.ListServerCookbookCookstyleResults(ctx, sc.OrganisationName, sc.Name, sc.Version)
		if csErr != nil {
			r.logf("WARN", "listing cookstyle results for server cookbook %s/%s@%s: %v", sc.OrganisationName, sc.Name, sc.Version, csErr)
		} else {
			detail.Cookstyle = cookstyle
		}

		serverDetails = append(serverDetails, detail)
	}

	gitDetails := make([]gitRepoDetail, 0, len(gitRepos))
	for _, gr := range gitRepos {
		detail := gitRepoDetail{GitRepo: gr}

		cookstyle, csErr := r.db.ListGitRepoCookstyleResults(ctx, gr.Name, gr.GitRepoURL)
		if csErr != nil {
			r.logf("WARN", "listing cookstyle results for git repo %s: %v", gr.Name, csErr)
		} else {
			detail.Cookstyle = cookstyle
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

// filterCookbookRows applies optional query-parameter filters (active, name,
// download_status) to the given slice, returning only matching entries. The
// name filter uses case-insensitive partial (substring) matching.
//
// The compatibility and download_status filters are applied separately in the
// handler after compatibility has been computed and assigned.
func filterCookbookRows(req *http.Request, rows []cookbookRow) []cookbookRow {
	q := req.URL.Query()
	active := q.Get("active")
	nameFilter := q.Get("name")

	if active == "" && nameFilter == "" {
		return rows
	}

	filtered := make([]cookbookRow, 0, len(rows))
	for _, cb := range rows {
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

// sortCookbookRows sorts the cookbook list in-place by the given field and
// order ("asc" or "desc"). Supported fields: "name" (default), "version",
// "compatibility", "active", "download_status".
func sortCookbookRows(items []cookbookRow, field, order string) {
	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch field {
		case "version":
			if items[i].Name == items[j].Name {
				less = items[i].Version < items[j].Version
			} else {
				less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
		case "compatibility":
			less = items[i].Compatibility < items[j].Compatibility
		case "active":
			// false < true so inactive sorts first in ascending
			less = !items[i].IsActive && items[j].IsActive
		case "download_status":
			less = items[i].DownloadStatus < items[j].DownloadStatus
		default: // "name"
			if strings.EqualFold(items[i].Name, items[j].Name) {
				less = items[i].Version < items[j].Version
			} else {
				less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
		}
		if strings.EqualFold(order, "desc") {
			return !less
		}
		return less
	})
}
