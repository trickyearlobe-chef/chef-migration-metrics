// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Remediation endpoints — prioritised remediation planning data.
// ---------------------------------------------------------------------------

// handleRemediationPriority handles GET /api/v1/remediation/priority.
// Returns cookbooks sorted by complexity × blast radius (affected node count),
// with complexity scores, auto-correctable counts, and top deprecations.
//
// Query parameters:
//   - organisation: filter by organisation name (optional, repeatable)
//   - target_chef_version: filter by target Chef version (optional; defaults
//     to first configured target version)
//   - complexity_label: filter by complexity label — "low", "medium",
//     "high", "critical" (optional; omit to include all)
//   - sort: field to sort by — "priority_score", "complexity_score",
//     "affected_nodes", "name" (default: "priority_score")
//   - order: "asc" or "desc" (default: "desc")
func (r *Router) handleRemediationPriority(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Optional complexity label filter.
	complexityLabel := queryString(req, "complexity_label", "")

	// Resolve target Chef version — default to the first configured one.
	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetVersion = r.cfg.TargetChefVersions[0]
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	// Resolve organisations to query.
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "resolving organisation filter for remediation priority: %v", err)
		WriteInternalError(w, "Failed to resolve organisations.")
		return
	}

	type priorityItem struct {
		CookbookName         string `json:"cookbook_name"`
		CookbookVersion      string `json:"cookbook_version,omitempty"`
		CookbookID           string `json:"cookbook_id"`
		OrganisationID       string `json:"organisation_id,omitempty"`
		ComplexityScore      int    `json:"complexity_score"`
		ComplexityLabel      string `json:"complexity_label"`
		AffectedNodeCount    int    `json:"affected_node_count"`
		AffectedRoleCount    int    `json:"affected_role_count"`
		PriorityScore        int    `json:"priority_score"`
		AutoCorrectableCount int    `json:"auto_correctable_count"`
		ManualFixCount       int    `json:"manual_fix_count"`
		DeprecationCount     int    `json:"deprecation_count"`
		ErrorCount           int    `json:"error_count"`
		TargetChefVersion    string `json:"target_chef_version"`
	}

	var items []priorityItem

	for _, org := range orgs {
		complexities, err := r.db.ListCookbookComplexitiesForOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing complexities for org %s in remediation priority: %v", org.Name, err)
			continue
		}

		// Build a map from cookbook ID to cookbook metadata.
		cookbooks, err := r.db.ListCookbooksByOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing cookbooks for org %s in remediation priority: %v", org.Name, err)
			continue
		}
		cbMap := make(map[string]datastore.Cookbook, len(cookbooks))
		for _, cb := range cookbooks {
			cbMap[cb.ID] = cb
		}

		for _, cc := range complexities {
			if cc.TargetChefVersion != targetVersion {
				continue
			}

			// Priority score = complexity × blast radius (affected nodes).
			// If no affected nodes are recorded, use 1 so that even
			// unused cookbooks appear in the list.
			blastRadius := cc.AffectedNodeCount
			if blastRadius == 0 {
				blastRadius = 1
			}
			priorityScore := cc.ComplexityScore * blastRadius

			cb := cbMap[cc.CookbookID]

			items = append(items, priorityItem{
				CookbookName:         cb.Name,
				CookbookVersion:      cb.Version,
				CookbookID:           cc.CookbookID,
				OrganisationID:       org.ID,
				ComplexityScore:      cc.ComplexityScore,
				ComplexityLabel:      cc.ComplexityLabel,
				AffectedNodeCount:    cc.AffectedNodeCount,
				AffectedRoleCount:    cc.AffectedRoleCount,
				PriorityScore:        priorityScore,
				AutoCorrectableCount: cc.AutoCorrectableCount,
				ManualFixCount:       cc.ManualFixCount,
				DeprecationCount:     cc.DeprecationCount,
				ErrorCount:           cc.ErrorCount,
				TargetChefVersion:    cc.TargetChefVersion,
			})
		}
	}

	// Filter by complexity label if specified.
	if complexityLabel != "" {
		filtered := items[:0]
		for _, item := range items {
			if item.ComplexityLabel == complexityLabel {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Sort by the requested field. Default to descending for numeric fields
	// so the highest-priority items appear first.
	sp := ParseSort(req, "priority_score", []string{
		"priority_score", "complexity_score", "affected_nodes", "name",
	})
	if req.URL.Query().Get("order") == "" && sp.Field != "name" {
		sp.Order = "desc"
	}

	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch sp.Field {
		case "complexity_score":
			less = items[i].ComplexityScore < items[j].ComplexityScore
		case "affected_nodes":
			less = items[i].AffectedNodeCount < items[j].AffectedNodeCount
		case "name":
			less = items[i].CookbookName < items[j].CookbookName
		default: // priority_score
			less = items[i].PriorityScore < items[j].PriorityScore
		}
		if sp.Order == "desc" {
			return !less
		}
		return less
	})

	// Paginate.
	pg := ParsePagination(req)
	total := len(items)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	// Compute summary stats across *all* items (before pagination).
	totalAutoCorrectable := 0
	totalManualFix := 0
	totalDeprecations := 0
	totalErrors := 0
	for _, item := range items {
		totalAutoCorrectable += item.AutoCorrectableCount
		totalManualFix += item.ManualFixCount
		totalDeprecations += item.DeprecationCount
		totalErrors += item.ErrorCount
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"target_chef_version":    targetVersion,
		"total_cookbooks":        total,
		"total_auto_correctable": totalAutoCorrectable,
		"total_manual_fix":       totalManualFix,
		"total_deprecations":     totalDeprecations,
		"total_errors":           totalErrors,
		"data":                   items[start:end],
		"pagination":             NewPaginationResponse(pg, total),
	})
}

// handleRemediationSummary handles GET /api/v1/remediation/summary.
// Returns an aggregate remediation summary: total cookbooks needing
// remediation, quick wins (auto-correctable only), manual fixes, and
// blocked node counts.
//
// Query parameters:
//   - organisation: filter by organisation name (optional, repeatable)
//   - target_chef_version: filter by target Chef version (optional; defaults
//     to first configured target version)
func (r *Router) handleRemediationSummary(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Resolve target Chef version.
	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetVersion = r.cfg.TargetChefVersions[0]
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "resolving organisation filter for remediation summary: %v", err)
		WriteInternalError(w, "Failed to resolve organisations.")
		return
	}

	var (
		totalNeeding     int // cookbooks with complexity_score > 0
		quickWins        int // cookbooks where manual_fix_count == 0 and auto_correctable_count > 0
		manualFixes      int // cookbooks where manual_fix_count > 0
		blockedNodes     int // sum of affected_node_count for non-zero complexity
		totalCookbooks   int // all cookbooks evaluated for this target version
		totalAutoCorrect int
		totalManualFix   int
	)

	for _, org := range orgs {
		complexities, err := r.db.ListCookbookComplexitiesForOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing complexities for org %s in remediation summary: %v", org.Name, err)
			continue
		}

		for _, cc := range complexities {
			if cc.TargetChefVersion != targetVersion {
				continue
			}

			totalCookbooks++

			if cc.ComplexityScore > 0 {
				totalNeeding++
				blockedNodes += cc.AffectedNodeCount

				if cc.ManualFixCount == 0 && cc.AutoCorrectableCount > 0 {
					quickWins++
				}
				if cc.ManualFixCount > 0 {
					manualFixes++
				}
			}

			totalAutoCorrect += cc.AutoCorrectableCount
			totalManualFix += cc.ManualFixCount
		}
	}

	// Also compute blocked node readiness across all orgs.
	var totalReadinessBlocked int
	for _, org := range orgs {
		_, _, blocked, err := r.db.CountNodeReadiness(ctx, org.ID, targetVersion)
		if err != nil {
			r.logf("WARN", "counting node readiness for org %s version %s: %v", org.Name, targetVersion, err)
			continue
		}
		totalReadinessBlocked += blocked
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"target_chef_version":         targetVersion,
		"total_cookbooks_evaluated":   totalCookbooks,
		"total_needing_remediation":   totalNeeding,
		"quick_wins":                  quickWins,
		"manual_fixes":                manualFixes,
		"blocked_nodes_by_complexity": blockedNodes,
		"blocked_nodes_by_readiness":  totalReadinessBlocked,
		"total_auto_correctable":      totalAutoCorrect,
		"total_manual_fix":            totalManualFix,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveOrganisationFilter returns the organisations matching the optional
// "organisation" query parameter. If no filter is given, all organisations
// are returned.
func (r *Router) resolveOrganisationFilter(req *http.Request) ([]datastore.Organisation, error) {
	orgNames := queryStringSlice(req, "organisation")

	allOrgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		return nil, err
	}

	if len(orgNames) == 0 {
		return allOrgs, nil
	}

	// Build a set for fast lookup.
	wanted := make(map[string]struct{}, len(orgNames))
	for _, n := range orgNames {
		wanted[n] = struct{}{}
	}

	filtered := make([]datastore.Organisation, 0, len(orgNames))
	for _, org := range allOrgs {
		if _, ok := wanted[org.Name]; ok {
			filtered = append(filtered, org)
		}
	}
	return filtered, nil
}
