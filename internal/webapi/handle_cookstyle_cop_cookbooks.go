// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
)

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops/:cop_name/cookbooks
// ---------------------------------------------------------------------------

// copCookbookItem is one row in the cop drill-down response.
type copCookbookItem struct {
	Source           string `json:"source"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	Organisation     string `json:"organisation,omitempty"`
	OffenceCount     int    `json:"offence_count"`
	AutoCorrectable  int    `json:"auto_correctable"`
	WouldPassWithout bool   `json:"would_pass_without"`
}

// copCookbookResponse wraps the flat cop drill-down (git repos, or the legacy
// all-sources list). Each item is the finest grain: one {name, version, org} row.
type copCookbookResponse struct {
	CopName    string             `json:"cop_name"`
	Data       []copCookbookItem  `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// copCookbookGroup is one grouped row in the server drill-down: a cookbook name
// with its per-version/org detail nested under it. Server cookbooks have real
// multiplicity (many immutable versions across orgs), so grouping by name gives
// the drill-down the same grain as the header "cookbooks affected" count — the
// two must agree (shared record selection). OffenceCount and AutoCorrectable are
// summed across versions; WouldPassWithout is true only if every version would
// pass once this cop is resolved (i.e. resolving it unblocks the whole cookbook).
type copCookbookGroup struct {
	Source           string            `json:"source"`
	Name             string            `json:"name"`
	VersionCount     int               `json:"version_count"`
	OffenceCount     int               `json:"offence_count"`
	AutoCorrectable  int               `json:"auto_correctable"`
	WouldPassWithout bool              `json:"would_pass_without"`
	Versions         []copCookbookItem `json:"versions"`
}

// copCookbookGroupResponse wraps the grouped (server) cop drill-down. Grouped is
// always true so the frontend can distinguish it from the flat response.
type copCookbookGroupResponse struct {
	CopName    string             `json:"cop_name"`
	Grouped    bool               `json:"grouped"`
	Data       []copCookbookGroup `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// handleCookstyleCopCookbooks handles GET /api/v1/cookstyle/cops/<cop_name>/cookbooks.
func (r *Router) handleCookstyleCopCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Extract cop name from URL path: /api/v1/cookstyle/cops/<cop_name>/cookbooks
	copName := extractCopNameFromPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	source := queryString(req, "source", "")
	pg := ParsePagination(req)

	// Load operator overrides for "would pass without" calculation.
	overrides, err := r.db.ListCopClassifications(ctx)
	if err != nil {
		r.logf("ERROR", "listing cop classifications for cop drill-down: %v", err)
		WriteInternalError(w, "Failed to load cop classifications.")
		return
	}
	overrideMap := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideMap[o.CopName] = o.Classification
	}
	resolver := &analysis.CopClassificationResolver{
		OperatorOverrides: overrideMap,
		TargetChefVersion: targetVersion,
	}

	var items []copCookbookItem

	if source == "" || source == "server" {
		results, err := r.db.ListAllServerCookbookCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing server results for cop drill-down: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			offenses := parseFullOffenses(res.Offences)
			item := buildCopCookbookItem(copName, "server", res.CookbookName, res.CookbookVersion, res.OrganisationName, offenses, resolver)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	if source == "" || source == "git" {
		results, err := r.db.ListGitRepoCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing git results for cop drill-down: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			offenses := parseFullOffenses(res.Offences)
			item := buildCopCookbookItem(copName, "git", res.GitRepoName, res.CommitSHA, "", offenses, resolver)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	// Server cookbooks have real multiplicity (many versions across orgs), so the
	// server drill-down groups by cookbook name and paginates by name — keeping the
	// drill-down total equal to the header "cookbooks affected" count. Git is 1:1
	// (repo == cookbook) and the legacy all-sources list stays flat.
	if source == "server" {
		groups := groupCopCookbooksByName(items)
		total := len(groups)
		page, _ := PaginateSlice(groups, pg)
		WriteJSON(w, http.StatusOK, copCookbookGroupResponse{
			CopName:    copName,
			Grouped:    true,
			Data:       page,
			Pagination: NewPaginationResponse(pg, total),
		})
		return
	}

	// Sort by offence count descending.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OffenceCount > items[j].OffenceCount
	})

	total := len(items)
	page, _ := PaginateSlice(items, pg)

	WriteJSON(w, http.StatusOK, copCookbookResponse{
		CopName:    copName,
		Data:       page,
		Pagination: NewPaginationResponse(pg, total),
	})
}

// groupCopCookbooksByName collapses finest-grain {name, version, org} drill-down
// items into one group per cookbook name, nesting the per-version detail. Groups
// are ordered by total offence count descending; versions within a group likewise.
// A group's WouldPassWithout is true only when every one of its versions would
// pass once this cop is resolved.
func groupCopCookbooksByName(items []copCookbookItem) []copCookbookGroup {
	order := make([]string, 0)
	byName := make(map[string]*copCookbookGroup)
	for _, it := range items {
		g, ok := byName[it.Name]
		if !ok {
			g = &copCookbookGroup{Source: it.Source, Name: it.Name, WouldPassWithout: true}
			byName[it.Name] = g
			order = append(order, it.Name)
		}
		g.Versions = append(g.Versions, it)
		g.VersionCount++
		g.OffenceCount += it.OffenceCount
		g.AutoCorrectable += it.AutoCorrectable
		if !it.WouldPassWithout {
			g.WouldPassWithout = false
		}
	}

	groups := make([]copCookbookGroup, 0, len(order))
	for _, name := range order {
		g := byName[name]
		sort.SliceStable(g.Versions, func(i, j int) bool {
			return g.Versions[i].OffenceCount > g.Versions[j].OffenceCount
		})
		groups = append(groups, *g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].OffenceCount > groups[j].OffenceCount
	})
	return groups
}

// buildCopCookbookItem creates a drill-down item if the given cookbook has
// offenses for the specified cop. Returns nil if the cop is not present.
func buildCopCookbookItem(copName, source, name, version, org string, offenses []fullOffense, resolver *analysis.CopClassificationResolver) *copCookbookItem {
	var count, correctable int
	for _, o := range offenses {
		if o.CopName == copName {
			count++
			if o.Correctable {
				correctable++
			}
		}
	}
	if count == 0 {
		return nil
	}

	// "Would pass without": remove offenses for this cop, re-evaluate.
	var remaining []analysis.CookstyleOffense
	for _, o := range offenses {
		if o.CopName != copName {
			remaining = append(remaining, analysis.CookstyleOffense{
				Severity: o.Severity,
				CopName:  o.CopName,
			})
		}
	}
	wouldPass := analysis.EvaluatePassFailWithClassification(remaining, resolver)

	return &copCookbookItem{
		Source:           source,
		Name:             name,
		Version:          version,
		Organisation:     org,
		OffenceCount:     count,
		AutoCorrectable:  correctable,
		WouldPassWithout: wouldPass,
	}
}
