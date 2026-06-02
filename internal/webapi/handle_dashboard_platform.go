// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
)

type dashboardPlatformCount struct {
	Platform         string  `json:"platform"`
	DisplayName      *string `json:"display_name"`
	GroupKey         string  `json:"group_key"`
	GroupDisplayName string  `json:"group_display_name"`
	Count            int     `json:"count"`
	Percent          float64 `json:"percent"`
	caption          string  // unexported: used for resolution only
	family           string  // unexported: used for resolution only
}

type dashboardPlatformGroup struct {
	GroupKey         string                   `json:"group_key"`
	GroupDisplayName string                   `json:"group_display_name"`
	TotalCount       int                      `json:"total_count"`
	TotalPercent     float64                  `json:"total_percent"`
	Versions         []dashboardPlatformCount `json:"versions"`
}

func resolveDashboardPlatformDisplayNames(result []dashboardPlatformCount, mappings []platform.DisplayNameMapping) {
	for i := range result {
		idx := strings.IndexByte(result[i].Platform, ' ')
		var plat, ver string
		if idx > 0 {
			plat = result[i].Platform[:idx]
			ver = result[i].Platform[idx+1:]
		} else {
			plat = result[i].Platform
		}
		family := result[i].family
		if family == "" {
			family = platform.DetectOSFamilyFromPlatform(plat)
		}
		info := platform.ResolveInfo(plat, ver, family, result[i].caption, mappings)
		result[i].DisplayName = &info.DisplayName
		result[i].GroupKey = info.GroupKey
		result[i].GroupDisplayName = info.GroupDisplayName
	}
}

func buildPlatformGroups(items []dashboardPlatformCount, totalNodes int) []dashboardPlatformGroup {
	groupMap := make(map[string]*dashboardPlatformGroup)
	var groupOrder []string

	for _, item := range items {
		g, exists := groupMap[item.GroupKey]
		if !exists {
			g = &dashboardPlatformGroup{
				GroupKey:         item.GroupKey,
				GroupDisplayName: item.GroupDisplayName,
			}
			groupMap[item.GroupKey] = g
			groupOrder = append(groupOrder, item.GroupKey)
		}
		g.TotalCount += item.Count
		g.Versions = append(g.Versions, item)
	}

	// Calculate percentages and build result.
	groups := make([]dashboardPlatformGroup, 0, len(groupOrder))
	for _, key := range groupOrder {
		g := groupMap[key]
		if totalNodes > 0 {
			g.TotalPercent = float64(g.TotalCount) / float64(totalNodes) * 100
		}
		groups = append(groups, *g)
	}

	// Sort groups by total count descending.
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].TotalCount != groups[j].TotalCount {
			return groups[i].TotalCount > groups[j].TotalCount
		}
		return groups[i].GroupKey < groups[j].GroupKey
	})

	return groups
}

// ---------------------------------------------------------------------------
// Dashboard — platform distribution endpoints
// ---------------------------------------------------------------------------

// handleDashboardPlatformDistribution handles
// GET /api/v1/dashboard/platform-distribution.
// Returns a count of nodes grouped by their OS platform (combining platform
// and platform_version) across all organisations.
//
// Uses SQL aggregate push-down via CountNodePlatformDistribution when no
// ownership filtering is active.
func (r *Router) handleDashboardPlatformDistribution(w http.ResponseWriter, req *http.Request) {
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
		r.logf("ERROR", "listing organisations for platform distribution: %v", err)
		WriteInternalError(w, "Failed to compute platform distribution.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	// When ownership filtering is active, fall back to in-memory path.
	ownerFilterActive := of.Active
	if ownerFilterActive {
		r.handleDashboardPlatformDistributionWithOwnerFilter(w, req, orgs, of)
		return
	}

	// --- SQL aggregate push-down path (with caption for accurate resolution) ---
	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	distRows, totalNodes, err := r.db.CountNodePlatformDistributionDetailed(ctx, f)
	if err != nil {
		r.logf("ERROR", "counting platform distribution: %v", err)
		WriteInternalError(w, "Failed to compute platform distribution.")
		return
	}

	result := make([]dashboardPlatformCount, 0, len(distRows))
	for _, row := range distRows {
		label := row.Platform
		if row.PlatformVersion != "" {
			label = row.Platform + " " + row.PlatformVersion
		}
		pct := 0.0
		if totalNodes > 0 {
			pct = float64(row.Count) / float64(totalNodes) * 100
		}
		result = append(result, dashboardPlatformCount{
			Platform: label,
			Count:    row.Count,
			Percent:  pct,
			caption:  row.PlatformCaption,
			family:   row.PlatformFamily,
		})
	}

	// Sort by count descending, then platform ascending for stability.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Platform < result[j].Platform
	})

	// Resolve display names.
	mappings, _ := r.loadPlatformDisplayNames(ctx)
	resolveDashboardPlatformDisplayNames(result, mappings)

	// Build grouped response.
	groups := buildPlatformGroups(result, totalNodes)

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
		"groups":       groups,
	})
}

// handleDashboardPlatformDistributionWithOwnerFilter is the fallback path
// when ownership filtering is active.
func (r *Router) handleDashboardPlatformDistributionWithOwnerFilter(
	w http.ResponseWriter,
	req *http.Request,
	orgs []datastore.Organisation,
	of ownerFilter,
) {
	ctx := req.Context()

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
	if err != nil {
		r.logf("ERROR", "resolving node ownership filter for platform distribution: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	nodes, _, err := r.db.ListNodeSnapshotsFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing nodes for platform distribution owner filter: %v", err)
		WriteInternalError(w, "Failed to compute platform distribution.")
		return
	}

	type platformKey struct {
		label   string
		caption string
		family  string
	}
	countMap := make(map[platformKey]int)
	totalNodes := 0
	for _, n := range nodes {
		if !ownershipInclude(n.NodeName, ownedKeys, of) {
			continue
		}
		p := n.Platform
		if p == "" {
			p = "unknown"
		}
		label := p
		if n.PlatformVersion != "" {
			label = p + " " + n.PlatformVersion
		}
		key := platformKey{label: label, caption: n.PlatformCaption, family: n.PlatformFamily}
		countMap[key]++
		totalNodes++
	}

	result := make([]dashboardPlatformCount, 0, len(countMap))
	for key, count := range countMap {
		pct := 0.0
		if totalNodes > 0 {
			pct = float64(count) / float64(totalNodes) * 100
		}
		result = append(result, dashboardPlatformCount{
			Platform: key.label,
			Count:    count,
			Percent:  pct,
			caption:  key.caption,
			family:   key.family,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Platform < result[j].Platform
	})

	// Resolve display names.
	mappings, _ := r.loadPlatformDisplayNames(ctx)
	resolveDashboardPlatformDisplayNames(result, mappings)

	// Build grouped response.
	groups := buildPlatformGroups(result, totalNodes)

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
		"groups":       groups,
	})
}

// buildDistributionResponse is a generic helper that converts a map of
// label → count into a slice of response structs with percentage calculated.
func buildDistributionResponse[T any](counts map[string]int, total int, makeFn func(label string, count int, pct float64) T) []T {
	result := make([]T, 0, len(counts))
	for label, count := range counts {
		pct := 0.0
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		result = append(result, makeFn(label, count, pct))
	}
	return result
}
