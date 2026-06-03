// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
)

// ---------------------------------------------------------------------------
// Dashboard — deployment progress trend
// ---------------------------------------------------------------------------

// deploymentTrendPoint is a single data point in the deployment progress
// trend response. Each point represents staged/activated and converge-passing
// counts from one collection run.
type deploymentTrendPoint struct {
	OrganisationName  string `json:"organisation_name"`
	CollectionRunOrg  string `json:"collection_run_org"`
	CompletedAt       string `json:"completed_at"`
	TotalNodes        int    `json:"total_nodes"`
	StagedOrActivated int    `json:"staged_or_activated"`
	ConvergePassing   int    `json:"converge_passing"`
}

// handleDashboardDeploymentTrend handles GET /api/v1/dashboard/deployment/trend.
// Returns deployment progress (staged+activated count and converge-passing
// count) over time by reading the deployment breakdown from node_metrics
// snapshots.
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
	var points []deploymentTrendPoint

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
			points = append(points, deploymentTrendPoint{
				OrganisationName:  org.Name,
				CollectionRunOrg:  ms.CollectionRunOrg,
				CompletedAt:       ms.SnapshotAt.Format(trendTimestampFormat),
				TotalNodes:        payload.TotalNodes,
				StagedOrActivated: payload.Deployment.StagedOrActivated,
				ConvergePassing:   payload.Deployment.ConvergePassing,
			})
		}
	}

	if points == nil {
		points = []deploymentTrendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
