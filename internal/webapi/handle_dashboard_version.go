// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dashboard — version distribution endpoints
// ---------------------------------------------------------------------------

// handleDashboardVersionDistribution handles GET /api/v1/dashboard/version-distribution.
// Returns a count of nodes grouped by their current Chef client version
// across all organisations.
func (r *Router) handleDashboardVersionDistribution(w http.ResponseWriter, req *http.Request) {
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
		r.logf("ERROR", "listing organisations for version distribution: %v", err)
		WriteInternalError(w, "Failed to compute version distribution.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	// When ownership filtering is active, fall back to in-memory path
	// because ownership assignments can't be joined in the aggregate SQL.
	ownerFilterActive := of.Active
	if ownerFilterActive {
		r.handleDashboardVersionDistributionWithOwnerFilter(w, req, orgs, of)
		return
	}

	// Mid-collection guard: if any org has a running collection, serve
	// from the latest completed metric_snapshots to avoid partial counts.
	if r.anyOrgCollectionRunning(ctx, orgs) {
		if counts, totalNodes, ok := r.versionDistFromMetricSnapshots(ctx, orgs); ok {
			result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) versionDistEntry {
				return versionDistEntry{Version: label, Count: count, Percent: pct}
			})
			sort.Slice(result, func(i, j int) bool {
				if result[i].Count != result[j].Count {
					return result[i].Count > result[j].Count
				}
				return result[i].Version < result[j].Version
			})
			WriteJSON(w, http.StatusOK, map[string]any{
				"total_nodes":  totalNodes,
				"distribution": result,
			})
			return
		}
		// No metric snapshots available — fall through to live data.
	}

	// --- SQL aggregate push-down path ---
	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	counts, totalNodes, err := r.db.CountNodeVersionDistribution(ctx, f)
	if err != nil {
		r.logf("ERROR", "counting version distribution: %v", err)
		WriteInternalError(w, "Failed to compute version distribution.")
		return
	}

	result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) versionDistEntry {
		return versionDistEntry{Version: label, Count: count, Percent: pct}
	})

	// Sort by count descending, then version ascending for stability.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Version < result[j].Version
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
	})
}

// versionDistEntry is a single row in the version distribution response.
type versionDistEntry struct {
	Version string  `json:"version"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// handleDashboardVersionDistributionWithOwnerFilter is the fallback path
// when ownership filtering is active. It uses SQL push-down for node-level
// filters but applies ownership filtering in memory.
func (r *Router) handleDashboardVersionDistributionWithOwnerFilter(
	w http.ResponseWriter,
	req *http.Request,
	orgs []datastore.Organisation,
	of ownerFilter,
) {
	ctx := req.Context()

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
	if err != nil {
		r.logf("ERROR", "resolving node ownership filter for version distribution: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}

	// Mid-collection guard: if any org has a running collection, serve
	// from the latest completed metric_snapshots with ownership filtering
	// applied to the per-node data in the JSONB payload.
	if r.anyOrgCollectionRunning(ctx, orgs) {
		if counts, totalNodes, ok := r.versionDistFromMetricSnapshotsOwnerFiltered(ctx, orgs, ownedKeys, of); ok {
			result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) versionDistEntry {
				return versionDistEntry{Version: label, Count: count, Percent: pct}
			})
			sort.Slice(result, func(i, j int) bool {
				if result[i].Count != result[j].Count {
					return result[i].Count > result[j].Count
				}
				return result[i].Version < result[j].Version
			})
			WriteJSON(w, http.StatusOK, map[string]any{
				"total_nodes":  totalNodes,
				"distribution": result,
			})
			return
		}
		// No usable metric snapshots — fall through to live data.
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	// Use SQL push-down for node-level filters, no pagination.
	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	nodes, _, err2 := r.db.ListNodeSnapshotsFiltered(ctx, f)
	if err2 != nil {
		r.logf("ERROR", "listing nodes for version distribution owner filter: %v", err2)
		WriteInternalError(w, "Failed to compute version distribution.")
		return
	}

	counts := make(map[string]int)
	totalNodes := 0
	for _, n := range nodes {
		if !ownershipInclude(n.NodeName, ownedKeys, of) {
			continue
		}
		v := n.ChefVersion
		if v == "" {
			v = "unknown"
		}
		counts[v]++
		totalNodes++
	}

	result := buildDistributionResponse(counts, totalNodes, func(label string, count int, pct float64) versionDistEntry {
		return versionDistEntry{Version: label, Count: count, Percent: pct}
	})

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Version < result[j].Version
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
	})
}

// ---------------------------------------------------------------------------
// Mid-collection guard helpers
// ---------------------------------------------------------------------------

// anyOrgCollectionRunning returns true if any of the given organisations
// has a collection run currently in "running" status.
func (r *Router) anyOrgCollectionRunning(ctx context.Context, orgs []datastore.Organisation) bool {
	for _, org := range orgs {
		run, err := r.db.GetLatestCollectionRun(ctx, org.Name)
		if err != nil {
			continue // no run or error — treat as not running
		}
		if run.Status == "running" {
			return true
		}
	}
	return false
}

// versionDistFromMetricSnapshots aggregates version distribution data from
// the latest metric_snapshots for each org. Returns (counts, total, true)
// on success. Returns (nil, 0, false) if no snapshots are available.
func (r *Router) versionDistFromMetricSnapshots(
	ctx context.Context,
	orgs []datastore.Organisation,
) (map[string]int, int, bool) {
	counts := make(map[string]int)
	totalNodes := 0
	found := false

	for _, org := range orgs {
		metrics, err := r.db.ListMetricSnapshotsByOrganisation(ctx, org.Name, "chef_version_distribution", 1)
		if err != nil || len(metrics) == 0 {
			continue
		}
		var payload struct {
			Distribution map[string]int `json:"distribution"`
			TotalNodes   int            `json:"total_nodes"`
		}
		if err := json.Unmarshal(metrics[0].Data, &payload); err != nil {
			r.logf("WARN", "unmarshalling metric snapshot %d for mid-collection guard: %v", metrics[0].ID, err)
			continue
		}
		for v, cnt := range payload.Distribution {
			counts[v] += cnt
		}
		totalNodes += payload.TotalNodes
		found = true
	}

	if !found {
		return nil, 0, false
	}
	return counts, totalNodes, true
}

// versionDistFromMetricSnapshotsOwnerFiltered aggregates version distribution
// from the latest metric_snapshots, applying ownership filtering against the
// per-node data in the JSONB payload. Returns (nil, 0, false) if no usable
// snapshots are available (e.g. nodes_omitted or old format without nodes).
func (r *Router) versionDistFromMetricSnapshotsOwnerFiltered(
	ctx context.Context,
	orgs []datastore.Organisation,
	ownedKeys map[string]bool,
	of ownerFilter,
) (map[string]int, int, bool) {
	counts := make(map[string]int)
	totalNodes := 0
	found := false

	for _, org := range orgs {
		metrics, err := r.db.ListMetricSnapshotsByOrganisation(ctx, org.Name, "chef_version_distribution", 1)
		if err != nil || len(metrics) == 0 {
			continue
		}
		var payload struct {
			Nodes []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"nodes"`
			NodesOmitted bool `json:"nodes_omitted"`
		}
		if err := json.Unmarshal(metrics[0].Data, &payload); err != nil {
			r.logf("WARN", "unmarshalling metric snapshot %d for mid-collection ownership guard: %v", metrics[0].ID, err)
			continue
		}
		// Skip snapshots where per-node data is unavailable.
		if payload.NodesOmitted || payload.Nodes == nil {
			continue
		}
		for _, n := range payload.Nodes {
			if !ownershipInclude(n.Name, ownedKeys, of) {
				continue
			}
			counts[n.Version]++
			totalNodes++
		}
		found = true
	}

	if !found {
		return nil, 0, false
	}
	return counts, totalNodes, true
}

// handleDashboardVersionDistributionTrend handles
// GET /api/v1/dashboard/version-distribution/trend.
// Returns version distribution snapshots over time by examining completed
// collection runs and their associated node snapshots. Each data point
// represents one completed collection run.
func (r *Router) handleDashboardVersionDistributionTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
	if err != nil {
		r.logf("ERROR", "resolving node ownership filter for version trend: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	orgs, err2 := r.resolveOrganisationFilter(req)
	if err2 != nil {
		r.logf("ERROR", "listing organisations for version trend: %v", err2)
		WriteInternalError(w, "Failed to compute version distribution trend.")
		return
	}

	var points []versionDistTrendPoint

	// When no ownership filter is active, read from pre-aggregated
	// metric_snapshots. This avoids scanning the (now current-state-only)
	// node_snapshots table and supports historical trends even after old
	// raw snapshots have been deduplicated.
	if !ownerFilterActive {
		for _, org := range orgs {
			metrics, err := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "chef_version_distribution", 365)
			if err != nil {
				r.logf("WARN", "listing metric snapshots for org %s in version trend: %v", org.Name, err)
				continue
			}
			for _, ms := range metrics {
				var payload struct {
					Distribution map[string]int `json:"distribution"`
					TotalNodes   int            `json:"total_nodes"`
				}
				if err := json.Unmarshal(ms.Data, &payload); err != nil {
					r.logf("WARN", "unmarshalling metric snapshot %d: %v", ms.ID, err)
					continue
				}
				points = append(points, versionDistTrendPoint{
					OrganisationName: org.Name,
					CollectionRunOrg: ms.CollectionRunOrg,
					CompletedAt:      ms.SnapshotAt.Format(trendTimestampFormat),
					TotalNodes:       payload.TotalNodes,
					Distribution:     payload.Distribution,
				})
			}
		}

		// When multiple orgs are in scope, merge per-org snapshots
		// from the same collection cycle into a single data point to
		// avoid sawtooth patterns in the chart.
		if len(orgs) > 1 {
			points = mergeVersionDistributionSnapshots(points)
		}

		if points == nil {
			points = []versionDistTrendPoint{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"data": points})
		return
	}

	// Ownership-filtered path: read from metric_snapshots and apply
	// ownership filtering against the per-node data in the JSONB payload.
	// This avoids querying live node_snapshots (which suffers from the
	// sawtooth problem during mid-collection updates).
	for _, org := range orgs {
		metrics, err := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "chef_version_distribution", 365)
		if err != nil {
			r.logf("WARN", "listing metric snapshots for org %s in ownership-filtered version trend: %v", org.Name, err)
			continue
		}
		for _, ms := range metrics {
			var payload struct {
				Distribution map[string]int `json:"distribution"`
				TotalNodes   int            `json:"total_nodes"`
				Nodes        []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"nodes"`
				NodesOmitted bool `json:"nodes_omitted"`
			}
			if err := json.Unmarshal(ms.Data, &payload); err != nil {
				r.logf("WARN", "unmarshalling metric snapshot %d for ownership trend: %v", ms.ID, err)
				continue
			}

			// Skip snapshots where per-node data is unavailable
			// (large orgs with nodes_omitted, or old-format snapshots).
			if payload.NodesOmitted || payload.Nodes == nil {
				continue
			}

			// Apply ownership filtering to per-node data and
			// re-aggregate into a version distribution.
			dist := make(map[string]int)
			total := 0
			for _, n := range payload.Nodes {
				if !ownershipInclude(n.Name, ownedKeys, of) {
					continue
				}
				dist[n.Version]++
				total++
			}

			points = append(points, versionDistTrendPoint{
				OrganisationName: org.Name,
				CollectionRunOrg: ms.CollectionRunOrg,
				CompletedAt:      ms.SnapshotAt.Format(trendTimestampFormat),
				TotalNodes:       total,
				Distribution:     dist,
			})
		}
	}

	if points == nil {
		points = []versionDistTrendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
