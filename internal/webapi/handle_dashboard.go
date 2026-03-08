// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dashboard endpoints — aggregate queries over existing datastore data.
// ---------------------------------------------------------------------------

// handleDashboardVersionDistribution handles GET /api/v1/dashboard/version-distribution.
// Returns a count of nodes grouped by their current Chef client version
// across all organisations.
func (r *Router) handleDashboardVersionDistribution(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for version distribution: %v", err)
		WriteInternalError(w, "Failed to compute version distribution.")
		return
	}

	counts := make(map[string]int)
	totalNodes := 0
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s in version distribution: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			v := n.ChefVersion
			if v == "" {
				v = "unknown"
			}
			counts[v]++
			totalNodes++
		}
	}

	type versionCount struct {
		Version string  `json:"version"`
		Count   int     `json:"count"`
		Percent float64 `json:"percent"`
	}

	result := make([]versionCount, 0, len(counts))
	for v, c := range counts {
		pct := 0.0
		if totalNodes > 0 {
			pct = float64(c) / float64(totalNodes) * 100
		}
		result = append(result, versionCount{
			Version: v,
			Count:   c,
			Percent: pct,
		})
	}

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

// handleDashboardVersionDistributionTrend handles
// GET /api/v1/dashboard/version-distribution/trend.
// Returns version distribution snapshots over time by examining completed
// collection runs and their associated node snapshots. Each data point
// represents one completed collection run.
func (r *Router) handleDashboardVersionDistributionTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for version trend: %v", err)
		WriteInternalError(w, "Failed to compute version distribution trend.")
		return
	}

	type trendPoint struct {
		OrganisationName string         `json:"organisation_name"`
		CollectionRunID  string         `json:"collection_run_id"`
		CompletedAt      string         `json:"completed_at"`
		TotalNodes       int            `json:"total_nodes"`
		Distribution     map[string]int `json:"distribution"`
	}

	var points []trendPoint
	for _, org := range orgs {
		// Get recent completed runs (limit to 10 per org for performance).
		runs, err := r.db.ListCollectionRuns(req.Context(), org.ID, 10)
		if err != nil {
			r.logf("WARN", "listing collection runs for org %s in trend: %v", org.Name, err)
			continue
		}
		for _, run := range runs {
			if run.Status != "completed" {
				continue
			}
			nodes, err := r.db.ListNodeSnapshotsByCollectionRun(req.Context(), run.ID)
			if err != nil {
				r.logf("WARN", "listing nodes for run %s in trend: %v", run.ID, err)
				continue
			}
			dist := make(map[string]int)
			for _, n := range nodes {
				v := n.ChefVersion
				if v == "" {
					v = "unknown"
				}
				dist[v]++
			}
			completedAt := ""
			if !run.CompletedAt.IsZero() {
				completedAt = run.CompletedAt.Format("2006-01-02T15:04:05Z")
			}
			points = append(points, trendPoint{
				OrganisationName: org.Name,
				CollectionRunID:  run.ID,
				CompletedAt:      completedAt,
				TotalNodes:       len(nodes),
				Distribution:     dist,
			})
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// handleDashboardReadiness handles GET /api/v1/dashboard/readiness.
// Returns an aggregate readiness summary across all organisations and
// target Chef versions.
func (r *Router) handleDashboardReadiness(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for readiness: %v", err)
		WriteInternalError(w, "Failed to compute readiness summary.")
		return
	}

	targetVersions := r.cfg.TargetChefVersions

	type readinessSummary struct {
		TargetChefVersion string  `json:"target_chef_version"`
		TotalNodes        int     `json:"total_nodes"`
		ReadyNodes        int     `json:"ready_nodes"`
		BlockedNodes      int     `json:"blocked_nodes"`
		ReadyPercent      float64 `json:"ready_percent"`
	}

	var summaries []readinessSummary
	for _, tv := range targetVersions {
		var totalAll, readyAll, blockedAll int
		for _, org := range orgs {
			total, ready, blocked, err := r.db.CountNodeReadiness(req.Context(), org.ID, tv)
			if err != nil {
				r.logf("WARN", "counting readiness for org %s version %s: %v", org.Name, tv, err)
				continue
			}
			totalAll += total
			readyAll += ready
			blockedAll += blocked
		}
		pct := 0.0
		if totalAll > 0 {
			pct = float64(readyAll) / float64(totalAll) * 100
		}
		summaries = append(summaries, readinessSummary{
			TargetChefVersion: tv,
			TotalNodes:        totalAll,
			ReadyNodes:        readyAll,
			BlockedNodes:      blockedAll,
			ReadyPercent:      pct,
		})
	}

	if summaries == nil {
		summaries = []readinessSummary{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

// handleDashboardReadinessTrend handles GET /api/v1/dashboard/readiness/trend.
// Returns readiness over time by examining each organisation's readiness
// records associated with completed collection runs.
func (r *Router) handleDashboardReadinessTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for readiness trend: %v", err)
		WriteInternalError(w, "Failed to compute readiness trend.")
		return
	}

	targetVersions := r.cfg.TargetChefVersions

	type trendPoint struct {
		OrganisationName  string  `json:"organisation_name"`
		TargetChefVersion string  `json:"target_chef_version"`
		TotalNodes        int     `json:"total_nodes"`
		ReadyNodes        int     `json:"ready_nodes"`
		BlockedNodes      int     `json:"blocked_nodes"`
		ReadyPercent      float64 `json:"ready_percent"`
	}

	var points []trendPoint
	for _, org := range orgs {
		for _, tv := range targetVersions {
			total, ready, blocked, err := r.db.CountNodeReadiness(req.Context(), org.ID, tv)
			if err != nil {
				r.logf("WARN", "counting readiness for org %s version %s in trend: %v", org.Name, tv, err)
				continue
			}
			if total == 0 {
				continue
			}
			pct := 0.0
			if total > 0 {
				pct = float64(ready) / float64(total) * 100
			}
			points = append(points, trendPoint{
				OrganisationName:  org.Name,
				TargetChefVersion: tv,
				TotalNodes:        total,
				ReadyNodes:        ready,
				BlockedNodes:      blocked,
				ReadyPercent:      pct,
			})
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// handleDashboardCookbookCompatibility handles
// GET /api/v1/dashboard/cookbook-compatibility.
// Returns a summary of cookbook compatibility across all organisations and
// target Chef versions, based on test kitchen results.
func (r *Router) handleDashboardCookbookCompatibility(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbook compatibility: %v", err)
		WriteInternalError(w, "Failed to compute cookbook compatibility.")
		return
	}

	targetVersions := r.cfg.TargetChefVersions

	type compatSummary struct {
		TargetChefVersion     string  `json:"target_chef_version"`
		TotalCookbooks        int     `json:"total_cookbooks"`
		CompatibleCookbooks   int     `json:"compatible_cookbooks"`
		IncompatibleCookbooks int     `json:"incompatible_cookbooks"`
		UntestedCookbooks     int     `json:"untested_cookbooks"`
		CompatiblePercent     float64 `json:"compatible_percent"`
	}

	var summaries []compatSummary
	for _, tv := range targetVersions {
		var totalAll, compatAll, incompatAll int

		for _, org := range orgs {
			cookbooks, err := r.db.ListCookbooksByOrganisation(req.Context(), org.ID)
			if err != nil {
				r.logf("WARN", "listing cookbooks for org %s: %v", org.Name, err)
				continue
			}
			for _, cb := range cookbooks {
				totalAll++
				result, err := r.db.GetLatestTestKitchenResult(req.Context(), cb.ID, tv)
				if err != nil {
					r.logf("WARN", "getting test kitchen result for cookbook %s version %s: %v", cb.ID, tv, err)
					continue
				}
				if result == nil {
					// No test result — counted as untested (neither compat nor incompat).
					continue
				}
				if result.Compatible {
					compatAll++
				} else {
					incompatAll++
				}
			}
		}

		untestedAll := totalAll - compatAll - incompatAll
		pct := 0.0
		if totalAll > 0 {
			pct = float64(compatAll) / float64(totalAll) * 100
		}

		summaries = append(summaries, compatSummary{
			TargetChefVersion:     tv,
			TotalCookbooks:        totalAll,
			CompatibleCookbooks:   compatAll,
			IncompatibleCookbooks: incompatAll,
			UntestedCookbooks:     untestedAll,
			CompatiblePercent:     pct,
		})
	}

	if summaries == nil {
		summaries = []compatSummary{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

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

	orgs, err := r.db.ListOrganisations(req.Context())
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
		complexities, err := r.db.ListCookbookComplexitiesForOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing complexities for org %s in trend: %v", org.Name, err)
			continue
		}

		// Group by target chef version.
		byVersion := make(map[string][]datastore.CookbookComplexity)
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

	orgs, err := r.db.ListOrganisations(req.Context())
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

	var points []trendPoint
	for _, org := range orgs {
		runs, err := r.db.ListCollectionRuns(req.Context(), org.ID, 10)
		if err != nil {
			r.logf("WARN", "listing collection runs for org %s in stale trend: %v", org.Name, err)
			continue
		}
		for _, run := range runs {
			if run.Status != "completed" {
				continue
			}
			nodes, err := r.db.ListNodeSnapshotsByCollectionRun(req.Context(), run.ID)
			if err != nil {
				r.logf("WARN", "listing nodes for run %s in stale trend: %v", run.ID, err)
				continue
			}
			stale := 0
			for _, n := range nodes {
				if n.IsStale {
					stale++
				}
			}
			completedAt := ""
			if !run.CompletedAt.IsZero() {
				completedAt = run.CompletedAt.Format("2006-01-02T15:04:05Z")
			}
			points = append(points, trendPoint{
				OrganisationName: org.Name,
				CollectionRunID:  run.ID,
				CompletedAt:      completedAt,
				TotalNodes:       len(nodes),
				StaleNodes:       stale,
				FreshNodes:       len(nodes) - stale,
			})
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// handleDashboardCookbookDownloadStatus handles
// GET /api/v1/dashboard/cookbook-download-status.
// Returns a summary of cookbook download statuses across all organisations,
// including aggregate counts by status and a list of failed cookbook versions
// with their error details so operators can investigate download failures.
func (r *Router) handleDashboardCookbookDownloadStatus(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbook download status: %v", err)
		WriteInternalError(w, "Failed to compute cookbook download status.")
		return
	}

	// Aggregate counts by download status.
	statusCounts := map[string]int{
		"ok":      0,
		"failed":  0,
		"pending": 0,
	}
	totalCookbooks := 0

	type failedCookbook struct {
		ID             string `json:"id"`
		OrganisationID string `json:"organisation_id"`
		OrgName        string `json:"organisation_name,omitempty"`
		Name           string `json:"name"`
		Version        string `json:"version"`
		DownloadError  string `json:"download_error"`
		IsActive       bool   `json:"is_active"`
	}

	var failedList []failedCookbook

	// Build an org ID → name lookup for annotating failures.
	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
	}

	for _, org := range orgs {
		cookbooks, cbErr := r.db.ListCookbooksByOrganisation(req.Context(), org.ID)
		if cbErr != nil {
			r.logf("WARN", "listing cookbooks for org %s in download status: %v", org.Name, cbErr)
			continue
		}
		for _, cb := range cookbooks {
			// Only count Chef server-sourced cookbooks — git cookbooks don't
			// have a download_status lifecycle.
			if cb.Source != "chef_server" {
				continue
			}
			totalCookbooks++
			status := cb.DownloadStatus
			if status == "" {
				status = "pending"
			}
			statusCounts[status]++

			if status == "failed" {
				failedList = append(failedList, failedCookbook{
					ID:             cb.ID,
					OrganisationID: cb.OrganisationID,
					OrgName:        orgNameByID[cb.OrganisationID],
					Name:           cb.Name,
					Version:        cb.Version,
					DownloadError:  cb.DownloadError,
					IsActive:       cb.IsActive,
				})
			}
		}
	}

	// Sort failures: active cookbooks first (higher priority), then by
	// name and version for stable ordering.
	sort.Slice(failedList, func(i, j int) bool {
		if failedList[i].IsActive != failedList[j].IsActive {
			return failedList[i].IsActive // active first
		}
		if failedList[i].Name != failedList[j].Name {
			return failedList[i].Name < failedList[j].Name
		}
		return failedList[i].Version < failedList[j].Version
	})

	// Compute percentages.
	okPercent := 0.0
	failedPercent := 0.0
	pendingPercent := 0.0
	if totalCookbooks > 0 {
		okPercent = float64(statusCounts["ok"]) / float64(totalCookbooks) * 100
		failedPercent = float64(statusCounts["failed"]) / float64(totalCookbooks) * 100
		pendingPercent = float64(statusCounts["pending"]) / float64(totalCookbooks) * 100
	}

	if failedList == nil {
		failedList = []failedCookbook{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_cookbooks": totalCookbooks,
		"status_counts": map[string]any{
			"ok":      statusCounts["ok"],
			"failed":  statusCounts["failed"],
			"pending": statusCounts["pending"],
		},
		"status_percentages": map[string]any{
			"ok_percent":      okPercent,
			"failed_percent":  failedPercent,
			"pending_percent": pendingPercent,
		},
		"failed_cookbooks":      failedList,
		"failed_cookbook_count": len(failedList),
		"has_failures":          len(failedList) > 0,
		"failure_message":       cookbookDownloadFailureMessage(len(failedList)),
	})
}

// cookbookDownloadFailureMessage returns a human-readable summary message
// for the dashboard.
func cookbookDownloadFailureMessage(failedCount int) string {
	if failedCount == 0 {
		return "All cookbook versions downloaded successfully."
	}
	return fmt.Sprintf(
		"%d cookbook version(s) failed to download. These versions are excluded from compatibility analysis. "+
			"They will be retried on the next collection run.",
		failedCount,
	)
}

// Ensure datastore import is used.
var _ datastore.NodeSnapshot
