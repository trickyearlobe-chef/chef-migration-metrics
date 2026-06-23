// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dashboard — deployment status (live) + deployment progress trend
// ---------------------------------------------------------------------------

// deploymentStatusVersionEntry holds per-version deployment state for the
// status endpoint response.
type deploymentStatusVersionEntry struct {
	Version         string `json:"version"`
	Staged          int    `json:"staged"`
	Activated       int    `json:"activated"`
	ConvergePassing int    `json:"converge_passing"`
	ConvergeFailing int    `json:"converge_failing"`
	Total           int    `json:"total"`
}

// handleDashboardDeploymentStatus handles GET /api/v1/dashboard/deployment/status.
// Returns current per-version deployment state via a live GROUP BY query on
// node_snapshots.
func (r *Router) handleDashboardDeploymentStatus(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for deployment status: %v", err)
		WriteInternalError(w, "Failed to compute deployment status.")
		return
	}

	ctx := req.Context()
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	f := datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}
	rows, totalNodes, err := r.db.CountNodesByDeploymentVersion(ctx, f)
	if err != nil {
		r.logf("ERROR", "counting deployment version distribution: %v", err)
		WriteInternalError(w, "Failed to compute deployment status.")
		return
	}

	data := make([]deploymentStatusVersionEntry, 0, len(rows))
	for _, row := range rows {
		data = append(data, deploymentStatusVersionEntry{
			Version:         row.Version,
			Staged:          row.Staged,
			Activated:       row.Activated,
			ConvergePassing: row.ConvergePassing,
			ConvergeFailing: row.ConvergeFailing,
			Total:           row.Staged + row.Activated,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"data":        data,
		"total_nodes": totalNodes,
	})
}

// deploymentTrendPoint is a single data point in the deployment progress
// trend response. Each point represents staged/activated and converge-passing
// counts from one collection run.
type deploymentTrendPoint struct {
	OrganisationName  string                              `json:"organisation_name"`
	CollectionRunOrg  string                              `json:"collection_run_org"`
	CompletedAt       string                              `json:"completed_at"`
	TotalNodes        int                                 `json:"total_nodes"`
	StagedOrActivated int                                 `json:"staged_or_activated"`
	ConvergePassing   int                                 `json:"converge_passing"`
	ByVersion         map[string]deploymentTrendVersionPt `json:"by_version,omitempty"`
}

// deploymentTrendVersionPt holds per-version data within a trend point.
type deploymentTrendVersionPt struct {
	StagedOrActivated int `json:"staged_or_activated"`
	ConvergePassing   int `json:"converge_passing"`
}

// handleDashboardDeploymentTrend handles GET /api/v1/dashboard/deployment/trend.
// Returns deployment progress (staged+activated count and converge-passing
// count) over time by reading the deployment breakdown from node_metrics
// snapshots. When multiple organisations are selected, per-day data is
// aggregated (summed) across all orgs so the chart shows fleet-wide totals.
func (r *Router) handleDashboardDeploymentTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for deployment trend: %v", err)
		WriteInternalError(w, "Failed to compute deployment trend.")
		return
	}

	ctx := req.Context()

	// Aggregate by day across all orgs. Key = date string (YYYY-MM-DD).
	type dayAgg struct {
		day               string
		timestamp         string // latest snapshot_at for ordering
		totalNodes        int
		stagedOrActivated int
		convergePassing   int
		byVersion         map[string]deploymentTrendVersionPt
	}
	byDay := make(map[string]*dayAgg)

	for _, org := range orgs {
		metrics, mErr := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "node_metrics", 365)
		if mErr != nil {
			r.logf("WARN", "listing node_metrics snapshots for org %s in deployment trend: %v", org.Name, mErr)
			continue
		}
		for _, ms := range metrics {
			var payload struct {
				TotalNodes int `json:"total_nodes"`
				Deployment struct {
					StagedOrActivated int `json:"staged_or_activated"`
					ConvergePassing   int `json:"converge_passing"`
					ByVersion         map[string]struct {
						Staged          int `json:"staged"`
						Activated       int `json:"activated"`
						ConvergePassing int `json:"converge_passing"`
					} `json:"by_version"`
				} `json:"deployment"`
			}
			if jErr := json.Unmarshal(ms.Data, &payload); jErr != nil {
				r.logf("WARN", "unmarshalling node_metrics snapshot %d for deployment trend: %v", ms.ID, jErr)
				continue
			}
			// Skip snapshots that predate the deployment field (all zeros).
			if payload.Deployment.StagedOrActivated == 0 && payload.Deployment.ConvergePassing == 0 {
				continue
			}

			day := ms.SnapshotAt.Format("2006-01-02")
			agg, exists := byDay[day]
			if !exists {
				agg = &dayAgg{
					day:       day,
					timestamp: ms.SnapshotAt.Format(trendTimestampFormat),
					byVersion: make(map[string]deploymentTrendVersionPt),
				}
				byDay[day] = agg
			}
			// Keep the latest timestamp for the day (for chart ordering).
			ts := ms.SnapshotAt.Format(trendTimestampFormat)
			if ts > agg.timestamp {
				agg.timestamp = ts
			}

			agg.totalNodes += payload.TotalNodes
			agg.stagedOrActivated += payload.Deployment.StagedOrActivated
			agg.convergePassing += payload.Deployment.ConvergePassing

			for ver, vd := range payload.Deployment.ByVersion {
				existing := agg.byVersion[ver]
				existing.StagedOrActivated += vd.Staged + vd.Activated
				existing.ConvergePassing += vd.ConvergePassing
				agg.byVersion[ver] = existing
			}
		}
	}

	// Build response points from aggregated data.
	points := make([]deploymentTrendPoint, 0, len(byDay))
	for _, agg := range byDay {
		pt := deploymentTrendPoint{
			CompletedAt:       agg.timestamp,
			TotalNodes:        agg.totalNodes,
			StagedOrActivated: agg.stagedOrActivated,
			ConvergePassing:   agg.convergePassing,
		}
		if len(agg.byVersion) > 0 {
			pt.ByVersion = agg.byVersion
		}
		points = append(points, pt)
	}

	if len(points) == 0 {
		points = []deploymentTrendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
