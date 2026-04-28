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
	Platform    string  `json:"platform"`
	DisplayName *string `json:"display_name"`
	Count       int     `json:"count"`
	Percent     float64 `json:"percent"`
}

func resolveDashboardPlatformDisplayNames(result []dashboardPlatformCount, mappings []platform.DisplayNameMapping) {
	for i := range result {
		idx := strings.IndexByte(result[i].Platform, ' ')
		if idx < 0 {
			continue
		}
		plat := result[i].Platform[:idx]
		ver := result[i].Platform[idx+1:]
		if name, ok := platform.ResolveName(plat, ver, mappings); ok {
			result[i].DisplayName = &name
		}
	}
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
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		r.handleDashboardPlatformDistributionWithOwnerFilter(w, req, orgs, of)
		return
	}

	// --- SQL aggregate push-down path ---
	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	counts, totalNodes, err := r.db.CountNodePlatformDistribution(ctx, f)
	if err != nil {
		r.logf("ERROR", "counting platform distribution: %v", err)
		WriteInternalError(w, "Failed to compute platform distribution.")
		return
	}

	result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) dashboardPlatformCount {
		return dashboardPlatformCount{Platform: label, Count: count, Percent: pct}
	})

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

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
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

	counts := make(map[string]int)
	totalNodes := 0
	for _, n := range nodes {
		if !ownershipInclude(n.NodeName, ownedKeys, of) {
			continue
		}
		p := n.Platform
		if p == "" {
			p = "unknown"
		}
		if n.PlatformVersion != "" {
			p = p + " " + n.PlatformVersion
		}
		counts[p]++
		totalNodes++
	}

	result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) dashboardPlatformCount {
		return dashboardPlatformCount{Platform: label, Count: count, Percent: pct}
	})

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Platform < result[j].Platform
	})

	// Resolve display names.
	mappings, _ := r.loadPlatformDisplayNames(ctx)
	resolveDashboardPlatformDisplayNames(result, mappings)

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
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
