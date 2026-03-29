// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Dashboard — readiness endpoints
// ---------------------------------------------------------------------------

// handleDashboardReadiness handles GET /api/v1/dashboard/readiness.
// Returns an aggregate readiness summary across all organisations and
// target Chef versions.
func (r *Router) handleDashboardReadiness(w http.ResponseWriter, req *http.Request) {
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
		r.logf("ERROR", "resolving node ownership filter for readiness: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}

	orgs, err2 := r.resolveOrganisationFilter(req)
	if err2 != nil {
		r.logf("ERROR", "listing organisations for readiness: %v", err2)
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

	// When owner filtering is active, collect allowed node names and count
	// readiness by inspecting per-node readiness records. Otherwise, use
	// the fast aggregate CountNodeReadiness path.
	if ownedKeys != nil {
		// Build the set of allowed node names across all orgs.
		allowedNodes := make(map[string]string) // node_name -> snapshot_id
		for _, org := range orgs {
			nodes, err := r.db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
			if err != nil {
				r.logf("WARN", "listing nodes for org %s in readiness owner filter: %v", org.Name, err)
				continue
			}
			for _, n := range nodes {
				if ownershipInclude(n.NodeName, ownedKeys, of) {
					allowedNodes[n.NodeName] = n.ID
				}
			}
		}

		var summaries []readinessSummary
		for _, tv := range targetVersions {
			var totalAll, readyAll, blockedAll int
			for _, snapshotID := range allowedNodes {
				readiness, err := r.db.ListNodeReadinessForSnapshot(ctx, snapshotID)
				if err != nil {
					continue
				}
				for _, nr := range readiness {
					if nr.TargetChefVersion != tv {
						continue
					}
					totalAll++
					if nr.IsReady {
						readyAll++
					} else {
						blockedAll++
					}
				}
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
		return
	}

	// Fast path: no owner filtering — use aggregate counts.
	var summaries []readinessSummary
	for _, tv := range targetVersions {
		var totalAll, readyAll, blockedAll int
		for _, org := range orgs {
			total, ready, blocked, err := r.db.CountNodeReadiness(ctx, org.ID, tv)
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

	orgs, err := r.resolveOrganisationFilter(req)
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
