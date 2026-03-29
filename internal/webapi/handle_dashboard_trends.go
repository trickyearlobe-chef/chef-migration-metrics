// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dashboard — trend endpoints (complexity, stale)
// ---------------------------------------------------------------------------

// handleDashboardComplexityTrend handles
// GET /api/v1/dashboard/complexity/trend.
// Returns aggregate cookbook complexity scores over time by examining
// complexity records per organisation and target Chef version. Each data
// point represents the current aggregate state for one (organisation,
// target_chef_version) pair.
func (r *Router) handleDashboardComplexityTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for complexity trend: %v", err)
		WriteInternalError(w, "Failed to compute complexity trend.")
		return
	}

	targetVersions := r.cfg.TargetChefVersions

	type trendPoint struct {
		OrganisationName  string  `json:"organisation_name"`
		TargetChefVersion string  `json:"target_chef_version"`
		TotalCookbooks    int     `json:"total_cookbooks"`
		TotalScore        int     `json:"total_score"`
		AverageScore      float64 `json:"average_score"`
		LowCount          int     `json:"low_count"`
		MediumCount       int     `json:"medium_count"`
		HighCount         int     `json:"high_count"`
		CriticalCount     int     `json:"critical_count"`
	}

	var points []trendPoint
	for _, org := range orgs {
		complexities, err := r.db.ListServerCookbookComplexitiesByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing complexities for org %s in trend: %v", org.Name, err)
			continue
		}

		// Group by target chef version.
		byVersion := make(map[string][]datastore.ServerCookbookComplexity)
		for _, cc := range complexities {
			byVersion[cc.TargetChefVersion] = append(byVersion[cc.TargetChefVersion], cc)
		}

		for _, tv := range targetVersions {
			ccs := byVersion[tv]
			if len(ccs) == 0 {
				continue
			}
			pt := trendPoint{
				OrganisationName:  org.Name,
				TargetChefVersion: tv,
				TotalCookbooks:    len(ccs),
			}
			for _, cc := range ccs {
				pt.TotalScore += cc.ComplexityScore
				switch cc.ComplexityLabel {
				case "low":
					pt.LowCount++
				case "medium":
					pt.MediumCount++
				case "high":
					pt.HighCount++
				case "critical":
					pt.CriticalCount++
				}
			}
			if pt.TotalCookbooks > 0 {
				pt.AverageScore = float64(pt.TotalScore) / float64(pt.TotalCookbooks)
			}
			points = append(points, pt)
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// handleDashboardStaleTrend handles GET /api/v1/dashboard/stale/trend.
// Returns stale vs. fresh node counts over time by examining completed
// collection runs and the is_stale flag on their associated node snapshots.
// Each data point represents one completed collection run.
func (r *Router) handleDashboardStaleTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for stale trend: %v", err)
		WriteInternalError(w, "Failed to compute stale node trend.")
		return
	}

	type trendPoint struct {
		OrganisationName string `json:"organisation_name"`
		CollectionRunID  string `json:"collection_run_id"`
		CompletedAt      string `json:"completed_at"`
		TotalNodes       int    `json:"total_nodes"`
		StaleNodes       int    `json:"stale_nodes"`
		FreshNodes       int    `json:"fresh_nodes"`
	}

	ctx := req.Context()
	var points []trendPoint

	// Read from pre-aggregated metric_snapshots. The
	// chef_version_distribution snapshots contain stale/fresh counts
	// alongside the version distribution data.
	for _, org := range orgs {
		metrics, err := r.db.ListMetricSnapshotsByOrganisation(ctx, org.ID, "chef_version_distribution", 10)
		if err != nil {
			r.logf("WARN", "listing metric snapshots for org %s in stale trend: %v", org.Name, err)
			continue
		}
		for _, ms := range metrics {
			var payload struct {
				TotalNodes int `json:"total_nodes"`
				StaleNodes int `json:"stale_nodes"`
				FreshNodes int `json:"fresh_nodes"`
			}
			if err := json.Unmarshal(ms.Data, &payload); err != nil {
				r.logf("WARN", "unmarshalling metric snapshot %s for stale trend: %v", ms.ID, err)
				continue
			}
			points = append(points, trendPoint{
				OrganisationName: org.Name,
				CollectionRunID:  ms.CollectionRunID,
				CompletedAt:      ms.SnapshotAt.Format("2006-01-02T15:04:05Z"),
				TotalNodes:       payload.TotalNodes,
				StaleNodes:       payload.StaleNodes,
				FreshNodes:       payload.FreshNodes,
			})
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
