// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
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

	targetVersions := r.liveConfig().TargetChefVersions

	type readinessSummary struct {
		TargetChefVersion string  `json:"target_chef_version"`
		TotalNodes        int     `json:"total_nodes"`
		ReadyNodes        int     `json:"ready_nodes"`
		NeedsReviewNodes  int     `json:"needs_review_nodes"`
		BlockedNodes      int     `json:"blocked_nodes"`
		ReadyPercent      float64 `json:"ready_percent"`
	}

	// When owner filtering is active, collect allowed node names and count
	// readiness by inspecting per-node readiness records. Otherwise, use
	// the fast aggregate CountNodeReadiness path.
	if ownedKeys != nil {
		// Build the set of allowed node names across all orgs.
		type nodeKey struct {
			orgName  string
			nodeName string
		}
		var allowedNodes []nodeKey
		for _, org := range orgs {
			nodes, err := r.db.ListNodeSnapshotsByOrganisation(ctx, org.Name)
			if err != nil {
				r.logf("WARN", "listing nodes for org %s in readiness owner filter: %v", org.Name, err)
				continue
			}
			for _, n := range nodes {
				if ownershipInclude(n.NodeName, ownedKeys, of) {
					allowedNodes = append(allowedNodes, nodeKey{orgName: n.OrganisationName, nodeName: n.NodeName})
				}
			}
		}

		var summaries []readinessSummary
		for _, tv := range targetVersions {
			var totalAll, readyAll, needsReviewAll, blockedAll int
			for _, nk := range allowedNodes {
				readiness, err := r.db.ListNodeReadinessByNodeName(ctx, nk.orgName, nk.nodeName)
				if err != nil {
					continue
				}
				for _, nr := range readiness {
					if nr.TargetChefVersion != tv {
						continue
					}
					totalAll++
					switch nr.Status {
					case "ready":
						readyAll++
					case "needs_review":
						needsReviewAll++
					default:
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
				NeedsReviewNodes:  needsReviewAll,
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
		var totalAll, readyAll, needsReviewAll, blockedAll int
		for _, org := range orgs {
			total, ready, needsReview, blocked, err := r.db.CountNodeReadinessByStatus(ctx, org.Name, tv)
			if err != nil {
				r.logf("WARN", "counting readiness for org %s version %s: %v", org.Name, tv, err)
				continue
			}
			totalAll += total
			readyAll += ready
			needsReviewAll += needsReview
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
			NeedsReviewNodes:  needsReviewAll,
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
// Returns readiness over time by reading from pre-aggregated
// readiness_summary metric snapshots recorded at the end of each collection
// run. Falls back to live CountNodeReadiness when no snapshots exist (e.g.
// before the first post-upgrade collection completes).
func (r *Router) handleDashboardReadinessTrend(w http.ResponseWriter, req *http.Request) {
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
		r.logf("ERROR", "resolving node ownership filter for readiness trend: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	// Parse staleness filter: ?stale=fresh,warning,critical (default: fresh).
	staleFilter := parseStalenessFilter(req)

	orgs, err2 := r.resolveOrganisationFilter(req)
	if err2 != nil {
		r.logf("ERROR", "listing organisations for readiness trend: %v", err2)
		WriteInternalError(w, "Failed to compute readiness trend.")
		return
	}

	targetVersions := r.liveConfig().TargetChefVersions

	var points []readinessTrendPoint

	// Collect dates covered by node_metrics so we can skip legacy dupes.
	nodeMetricsDates := make(map[string]bool)

	// Use node_metrics ONLY when the user explicitly set ?stale=fresh.
	// node_metrics only has readiness data for fresh nodes. Without an
	// explicit filter, we use legacy readiness_summary (all nodes) to
	// maintain continuity with historical data.
	useNodeMetrics := !ownerFilterActive && staleFilter.isFreshOnly()
	if useNodeMetrics {
		for _, org := range orgs {
			metrics, mErr := r.db.ListDailyMetricSnapshotsByOrganisation(ctx, org.Name, "node_metrics", 365)
			if mErr != nil {
				r.logf("WARN", "listing node_metrics snapshots for org %s in readiness trend: %v", org.Name, mErr)
				continue
			}
			for _, ms := range metrics {
				var payload struct {
					TotalNodes    int    `json:"total_nodes"`
					TargetChefVer string `json:"target_chef_version"`
					Fresh         struct {
						Total        int `json:"total"`
						Ready        int `json:"ready"`
						BlockedTotal int `json:"blocked_total"`
						BlockedBy    struct {
							Cookstyle   int `json:"cookstyle"`
							TestKitchen int `json:"test_kitchen"`
							Disk        int `json:"disk"`
							FoodCritic  int `json:"foodcritic"`
							ChefSpec    int `json:"chefspec"`
						} `json:"blocked_by"`
					} `json:"fresh"`
				}
				if jErr := json.Unmarshal(ms.Data, &payload); jErr != nil {
					r.logf("WARN", "unmarshalling node_metrics snapshot %d for readiness trend: %v", ms.ID, jErr)
					continue
				}

				// We only reach here when filter is explicitly fresh-only,
				// so serve the fresh breakdown directly.
				total := payload.Fresh.Total
				ready := payload.Fresh.Ready
				blocked := payload.Fresh.BlockedTotal
				if total == 0 {
					continue
				}
				pct := float64(ready) / float64(total) * 100

				day := ms.SnapshotAt.Format("2006-01-02")
				nodeMetricsDates[org.Name+"/"+day] = true

				points = append(points, readinessTrendPoint{
					OrganisationName:  org.Name,
					CollectionRunOrg:  ms.CollectionRunOrg,
					CompletedAt:       ms.SnapshotAt.Format(trendTimestampFormat),
					TargetChefVersion: payload.TargetChefVer,
					TotalNodes:        total,
					ReadyNodes:        ready,
					BlockedNodes:      blocked,
					ReadyPercent:      pct,
					BlockedBy: &blockedByResponse{
						Cookstyle:   payload.Fresh.BlockedBy.Cookstyle,
						TestKitchen: payload.Fresh.BlockedBy.TestKitchen,
						Disk:        payload.Fresh.BlockedBy.Disk,
						FoodCritic:  payload.Fresh.BlockedBy.FoodCritic,
						ChefSpec:    payload.Fresh.BlockedBy.ChefSpec,
					},
				})
			}
		}
	}

	// Backfill from legacy readiness_summary for dates not covered.
	snapshotFound := len(nodeMetricsDates) > 0
	for _, org := range orgs {
		for _, tv := range targetVersions {
			metrics, mErr := r.db.ListDailyMetricSnapshotsByOrganisationAndVersion(ctx, org.Name, "readiness_summary", tv, 365)
			if mErr != nil {
				r.logf("WARN", "listing readiness snapshots for org %s version %s: %v", org.Name, tv, mErr)
				continue
			}
			if len(metrics) > 0 {
				snapshotFound = true
			}
			for _, ms := range metrics {
				day := ms.SnapshotAt.Format("2006-01-02")
				if nodeMetricsDates[org.Name+"/"+day] {
					continue
				}
				var payload struct {
					TotalNodes int `json:"total_nodes"`
					Ready      int `json:"ready"`
					Blocked    int `json:"blocked"`
					Nodes      []struct {
						Name    string `json:"name"`
						IsReady bool   `json:"is_ready"`
					} `json:"nodes"`
					NodesOmitted bool `json:"nodes_omitted"`
				}
				if jErr := json.Unmarshal(ms.Data, &payload); jErr != nil {
					r.logf("WARN", "unmarshalling readiness snapshot %d: %v", ms.ID, jErr)
					continue
				}

				total := payload.TotalNodes
				ready := payload.Ready
				blocked := payload.Blocked

				if ownerFilterActive {
					if payload.NodesOmitted || payload.Nodes == nil {
						continue
					}
					total = 0
					ready = 0
					blocked = 0
					for _, n := range payload.Nodes {
						if !ownershipInclude(n.Name, ownedKeys, of) {
							continue
						}
						total++
						if n.IsReady {
							ready++
						} else {
							blocked++
						}
					}
				}

				if total == 0 {
					continue
				}
				pct := float64(ready) / float64(total) * 100

				points = append(points, readinessTrendPoint{
					OrganisationName:  org.Name,
					CollectionRunOrg:  ms.CollectionRunOrg,
					CompletedAt:       ms.SnapshotAt.Format(trendTimestampFormat),
					TargetChefVersion: tv,
					TotalNodes:        total,
					ReadyNodes:        ready,
					BlockedNodes:      blocked,
					ReadyPercent:      pct,
					FilterLimited:     !staleFilter.isDefault(),
				})
			}
		}
	}

	// Fallback: if no snapshots were found (pre-upgrade data), query live
	// CountNodeReadiness for a single current-state point.
	if !snapshotFound && !ownerFilterActive {
		for _, org := range orgs {
			for _, tv := range targetVersions {
				total, ready, needsReview, blocked, cErr := r.db.CountNodeReadinessByStatus(ctx, org.Name, tv)
				if cErr != nil {
					r.logf("WARN", "counting readiness for org %s version %s in trend fallback: %v", org.Name, tv, cErr)
					continue
				}
				if total == 0 {
					continue
				}
				pct := float64(ready) / float64(total) * 100
				points = append(points, readinessTrendPoint{
					OrganisationName:  org.Name,
					TargetChefVersion: tv,
					TotalNodes:        total,
					ReadyNodes:        ready,
					NeedsReviewNodes:  needsReview,
					BlockedNodes:      blocked,
					ReadyPercent:      pct,
					FilterLimited:     !staleFilter.isDefault(),
				})
			}
		}
	}

	// When multiple orgs are in scope, merge per-org snapshots
	// from the same collection cycle into a single data point to
	// avoid sawtooth patterns in the chart.
	if len(orgs) > 1 {
		points = mergeReadinessTrendSnapshots(points)
	}

	if points == nil {
		points = []readinessTrendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}
