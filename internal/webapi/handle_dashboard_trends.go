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

	ctx := req.Context()
	targetVersions := r.liveConfig().TargetChefVersionList()

	var points []complexityTrendPoint

	// Try snapshot-based path first (preferred).
	snapshotFound := false
	for _, org := range orgs {
		for _, tv := range targetVersions {
			metrics, mErr := r.db.ListDailyMetricSnapshotsByOrganisationAndVersion(ctx, org.Name, "complexity_summary", tv, 365)
			if mErr != nil {
				r.logf("WARN", "listing complexity snapshots for org %s version %s: %v", org.Name, tv, mErr)
				continue
			}
			if len(metrics) > 0 {
				snapshotFound = true
			}
			for _, ms := range metrics {
				var payload struct {
					TotalCookbooks int     `json:"total_cookbooks"`
					TotalScore     int     `json:"total_score"`
					AverageScore   float64 `json:"average_score"`
					LowCount       int     `json:"low_count"`
					MediumCount    int     `json:"medium_count"`
					HighCount      int     `json:"high_count"`
					CriticalCount  int     `json:"critical_count"`
				}
				if jErr := json.Unmarshal(ms.Data, &payload); jErr != nil {
					r.logf("WARN", "unmarshalling complexity snapshot %d: %v", ms.ID, jErr)
					continue
				}

				points = append(points, complexityTrendPoint{
					OrganisationName:  org.Name,
					CollectionRunOrg:  ms.CollectionRunOrg,
					CompletedAt:       ms.SnapshotAt.Format(trendTimestampFormat),
					TargetChefVersion: tv,
					TotalCookbooks:    payload.TotalCookbooks,
					TotalScore:        payload.TotalScore,
					AverageScore:      payload.AverageScore,
					LowCount:          payload.LowCount,
					MediumCount:       payload.MediumCount,
					HighCount:         payload.HighCount,
					CriticalCount:     payload.CriticalCount,
				})
			}
		}
	}

	// Fallback: if no snapshots were found (pre-upgrade data), query live
	// ServerCookbookComplexity for a single current-state point.
	if !snapshotFound {
		for _, org := range orgs {
			complexities, cErr := r.db.ListServerCookbookComplexitiesByOrganisation(ctx, org.Name)
			if cErr != nil {
				r.logf("WARN", "listing complexities for org %s in trend fallback: %v", org.Name, cErr)
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
				pt := complexityTrendPoint{
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
	}

	// When multiple orgs are in scope, merge per-org snapshots
	// from the same collection cycle into a single data point to
	// avoid sawtooth patterns in the chart.
	if len(orgs) > 1 && snapshotFound {
		points = mergeComplexityTrendSnapshots(points)
	}

	if points == nil {
		points = []complexityTrendPoint{}
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

	ctx := req.Context()
	var points []staleTrendPoint

	// Collect dates covered by node_metrics so we can skip legacy dupes.
	nodeMetricsDates := make(map[string]bool)

	// Try node_metrics snapshots (preferred — includes staleness breakdown).
	for _, org := range orgs {
		metrics, mErr := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "node_metrics", 365)
		if mErr != nil {
			r.logf("WARN", "listing node_metrics snapshots for org %s in stale trend: %v", org.Name, mErr)
			continue
		}
		for _, ms := range metrics {
			var payload struct {
				TotalNodes  int `json:"total_nodes"`
				ByStaleness struct {
					Fresh    int `json:"fresh"`
					Warning  int `json:"warning"`
					Critical int `json:"critical"`
				} `json:"by_staleness"`
			}
			if jErr := json.Unmarshal(ms.Data, &payload); jErr != nil {
				r.logf("WARN", "unmarshalling node_metrics snapshot %d for stale trend: %v", ms.ID, jErr)
				continue
			}
			day := ms.SnapshotAt.Format("2006-01-02")
			nodeMetricsDates[org.Name+"/"+day] = true
			points = append(points, staleTrendPoint{
				OrganisationName: org.Name,
				CollectionRunOrg: ms.CollectionRunOrg,
				CompletedAt:      ms.SnapshotAt.Format(trendTimestampFormat),
				TotalNodes:       payload.TotalNodes,
				StaleNodes:       payload.ByStaleness.Warning + payload.ByStaleness.Critical,
				FreshNodes:       payload.ByStaleness.Fresh,
				WarningNodes:     payload.ByStaleness.Warning,
				CriticalNodes:    payload.ByStaleness.Critical,
			})
		}
	}

	// Backfill from legacy chef_version_distribution for dates not covered.
	for _, org := range orgs {
		metrics, err := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "chef_version_distribution", 365)
		if err != nil {
			r.logf("WARN", "listing metric snapshots for org %s in stale trend: %v", org.Name, err)
			continue
		}
		for _, ms := range metrics {
			day := ms.SnapshotAt.Format("2006-01-02")
			if nodeMetricsDates[org.Name+"/"+day] {
				continue
			}
			var payload struct {
				TotalNodes    int `json:"total_nodes"`
				StaleNodes    int `json:"stale_nodes"`
				FreshNodes    int `json:"fresh_nodes"`
				WarningNodes  int `json:"warning_nodes"`
				CriticalNodes int `json:"critical_nodes"`
			}
			if err := json.Unmarshal(ms.Data, &payload); err != nil {
				r.logf("WARN", "unmarshalling metric snapshot %d for stale trend: %v", ms.ID, err)
				continue
			}
			points = append(points, staleTrendPoint{
				OrganisationName: org.Name,
				CollectionRunOrg: ms.CollectionRunOrg,
				CompletedAt:      ms.SnapshotAt.Format(trendTimestampFormat),
				TotalNodes:       payload.TotalNodes,
				StaleNodes:       payload.StaleNodes,
				FreshNodes:       payload.FreshNodes,
				WarningNodes:     payload.WarningNodes,
				CriticalNodes:    payload.CriticalNodes,
			})
		}
	}

	// When multiple orgs are in scope, merge per-org snapshots
	// from the same collection cycle into a single data point to
	// avoid sawtooth patterns in the chart.
	if len(orgs) > 1 {
		points = mergeStaleTrendSnapshots(points)
	}

	if points == nil {
		points = []staleTrendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
