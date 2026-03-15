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

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned node keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
			if err != nil {
				r.logf("ERROR", "resolving all owned node keys for version distribution: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
			if err != nil {
				r.logf("ERROR", "resolving owned node keys for version distribution: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(ctx)
	if err != nil {
		r.logf("ERROR", "listing organisations for version distribution: %v", err)
		WriteInternalError(w, "Failed to compute version distribution.")
		return
	}

	counts := make(map[string]int)
	totalNodes := 0
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s in version distribution: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			if ownerFilterActive && ownedKeys != nil {
				if of.Unowned {
					if ownedKeys[n.NodeName] {
						continue
					}
				} else {
					if !ownedKeys[n.NodeName] {
						continue
					}
				}
			}
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

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned node keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
			if err != nil {
				r.logf("ERROR", "resolving all owned node keys for version trend: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
			if err != nil {
				r.logf("ERROR", "resolving owned node keys for version trend: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(ctx)
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
		runs, err := r.db.ListCollectionRuns(ctx, org.ID, 10)
		if err != nil {
			r.logf("WARN", "listing collection runs for org %s in trend: %v", org.Name, err)
			continue
		}
		for _, run := range runs {
			if run.Status != "completed" {
				continue
			}

			var dist map[string]int
			var total int

			if ownerFilterActive && ownedKeys != nil {
				// Build an allowed-node list from the owned keys.
				var allowed []string
				for k := range ownedKeys {
					if of.Unowned {
						// ownedKeys contains ALL owned nodes; we want the complement.
						// We can't filter by exclusion easily here, so fall back to
						// the filtered query with all non-owned nodes.
						// Instead, just pass nil and filter below.
						break
					}
					allowed = append(allowed, k)
				}

				if of.Unowned {
					// For "unowned" we need to exclude owned nodes. The filtered
					// query supports inclusion only, so we fall back to the full
					// result and subtract owned node counts.
					allDist, err := r.db.CountChefVersionsByCollectionRun(ctx, run.ID)
					if err != nil {
						r.logf("WARN", "counting versions for run %s in trend: %v", run.ID, err)
						continue
					}
					// Get owned-only counts so we can subtract.
					ownedNodeNames := make([]string, 0, len(ownedKeys))
					for k := range ownedKeys {
						ownedNodeNames = append(ownedNodeNames, k)
					}
					ownedDist, err := r.db.CountChefVersionsByCollectionRunFiltered(ctx, run.ID, ownedNodeNames)
					if err != nil {
						r.logf("WARN", "counting owned versions for run %s in trend: %v", run.ID, err)
						continue
					}
					dist = make(map[string]int)
					for v, cnt := range allDist {
						remaining := cnt - ownedDist[v]
						if remaining > 0 {
							dist[v] = remaining
							total += remaining
						}
					}
				} else {
					dist, err = r.db.CountChefVersionsByCollectionRunFiltered(ctx, run.ID, allowed)
					if err != nil {
						r.logf("WARN", "counting filtered versions for run %s in trend: %v", run.ID, err)
						continue
					}
					for _, cnt := range dist {
						total += cnt
					}
				}
			} else {
				dist, err = r.db.CountChefVersionsByCollectionRun(ctx, run.ID)
				if err != nil {
					r.logf("WARN", "counting versions for run %s in trend: %v", run.ID, err)
					continue
				}
				for _, cnt := range dist {
					total += cnt
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
				TotalNodes:       total,
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

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned node keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
			if err != nil {
				r.logf("ERROR", "resolving all owned node keys for readiness: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
			if err != nil {
				r.logf("ERROR", "resolving owned node keys for readiness: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(ctx)
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

	// When owner filtering is active, collect allowed node names and count
	// readiness by inspecting per-node readiness records. Otherwise, use
	// the fast aggregate CountNodeReadiness path.
	if ownerFilterActive && ownedKeys != nil {
		// Build the set of allowed node names across all orgs.
		allowedNodes := make(map[string]string) // node_name -> snapshot_id
		for _, org := range orgs {
			nodes, err := r.db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
			if err != nil {
				r.logf("WARN", "listing nodes for org %s in readiness owner filter: %v", org.Name, err)
				continue
			}
			for _, n := range nodes {
				include := false
				if of.Unowned {
					include = !ownedKeys[n.NodeName]
				} else {
					include = ownedKeys[n.NodeName]
				}
				if include {
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

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned cookbook keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving all owned cookbook keys for compatibility: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving owned cookbook keys for compatibility: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(ctx)
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

	// Build an allowed-names set for ownership filtering (nil = no filter).
	var allowedNames map[string]bool
	if ownerFilterActive && ownedKeys != nil {
		if of.Unowned {
			allCookbooks := make(map[string]bool)
			for _, org := range orgs {
				cbs, err := r.db.ListServerCookbooksByOrganisation(ctx, org.ID)
				if err != nil {
					r.logf("WARN", "listing server cookbooks for org %s: %v", org.Name, err)
					continue
				}
				for _, cb := range cbs {
					allCookbooks[cb.Name] = true
				}
			}
			allowedNames = make(map[string]bool)
			for name := range allCookbooks {
				if !ownedKeys[name] {
					allowedNames[name] = true
				}
			}
		} else {
			allowedNames = ownedKeys
		}
	}

	// Compute compatibility from server cookbook cookstyle results, aggregated
	// per target Chef version. A cookbook is "compatible" when cookstyle passed,
	// "incompatible" when it did not, and "untested" when no result exists.
	// We deduplicate by cookbook name so each name counts once per target version.
	type perVersion struct {
		total        int
		compatible   int
		incompatible int
		untested     int
	}
	byTV := make(map[string]*perVersion)
	for _, tv := range targetVersions {
		byTV[tv] = &perVersion{}
	}

	// Track which cookbook names we have already counted per target version
	// so we only count each name once (the first version encountered).
	type tvName struct {
		tv   string
		name string
	}
	seen := make(map[tvName]bool)

	for _, org := range orgs {
		cookstyleResults, err := r.db.ListServerCookbookComplexitiesByOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing server cookbook complexities for org %s: %v", org.Name, err)
			continue
		}

		// Also need the cookbook metadata to get the name.
		serverCookbooks, scErr := r.db.ListServerCookbooksByOrganisation(ctx, org.ID)
		if scErr != nil {
			r.logf("WARN", "listing server cookbooks for org %s: %v", org.Name, scErr)
			continue
		}
		cookbookNameByID := make(map[string]string, len(serverCookbooks))
		for _, sc := range serverCookbooks {
			cookbookNameByID[sc.ID] = sc.Name
		}

		// Derive compatibility from complexity records: a cookbook with no
		// errors and no deprecations is considered compatible.
		for _, cc := range cookstyleResults {
			cbName := cookbookNameByID[cc.ServerCookbookID]
			if cbName == "" {
				continue
			}
			if allowedNames != nil && !allowedNames[cbName] {
				continue
			}
			pv, ok := byTV[cc.TargetChefVersion]
			if !ok {
				continue
			}
			key := tvName{tv: cc.TargetChefVersion, name: cbName}
			if seen[key] {
				continue
			}
			seen[key] = true
			pv.total++
			// A cookbook with complexity score 0 and no errors is compatible.
			if cc.ErrorCount == 0 && cc.DeprecationCount == 0 {
				pv.compatible++
			} else {
				pv.incompatible++
			}
		}

		// Count untested: server cookbooks with no complexity record for a
		// given target version.
		for _, sc := range serverCookbooks {
			if allowedNames != nil && !allowedNames[sc.Name] {
				continue
			}
			for _, tv := range targetVersions {
				key := tvName{tv: tv, name: sc.Name}
				if seen[key] {
					continue
				}
				seen[key] = true
				pv := byTV[tv]
				pv.total++
				pv.untested++
			}
		}
	}

	var summaries []compatSummary
	for _, tv := range targetVersions {
		pv := byTV[tv]
		pct := 0.0
		if pv.total > 0 {
			pct = float64(pv.compatible) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion:     tv,
			TotalCookbooks:        pv.total,
			CompatibleCookbooks:   pv.compatible,
			IncompatibleCookbooks: pv.incompatible,
			UntestedCookbooks:     pv.untested,
			CompatiblePercent:     pct,
		})
	}

	if summaries == nil {
		summaries = []compatSummary{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

// handleDashboardGitRepoCompatibility handles
// GET /api/v1/dashboard/git-repo-compatibility — CookStyle compatibility
// breakdown for git repos, aggregated per target Chef version.
func (r *Router) handleDashboardGitRepoCompatibility(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned cookbook keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving all owned cookbook keys for git repo compatibility: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "cookbook")
			if err != nil {
				r.logf("ERROR", "resolving owned cookbook keys for git repo compatibility: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	targetVersions := r.cfg.TargetChefVersions

	type compatSummary struct {
		TargetChefVersion string  `json:"target_chef_version"`
		TotalRepos        int     `json:"total_repos"`
		CompatibleRepos   int     `json:"compatible_repos"`
		IncompatibleRepos int     `json:"incompatible_repos"`
		UntestedRepos     int     `json:"untested_repos"`
		CompatiblePercent float64 `json:"compatible_percent"`
	}

	// Build an allowed-names set for ownership filtering (nil = no filter).
	var allowedNames map[string]bool
	if ownerFilterActive && ownedKeys != nil {
		gitRepos, err := r.db.ListGitRepos(ctx)
		if err != nil {
			r.logf("ERROR", "listing git repos for compatibility ownership filter: %v", err)
			WriteInternalError(w, "Failed to compute git repo compatibility.")
			return
		}
		if of.Unowned {
			allNames := make(map[string]bool, len(gitRepos))
			for _, gr := range gitRepos {
				allNames[gr.Name] = true
			}
			allowedNames = make(map[string]bool)
			for name := range allNames {
				if !ownedKeys[name] {
					allowedNames[name] = true
				}
			}
		} else {
			allowedNames = ownedKeys
		}
	}

	// Compute compatibility from git repo complexity records.
	type perVersion struct {
		total        int
		compatible   int
		incompatible int
		untested     int
	}
	byTV := make(map[string]*perVersion)
	for _, tv := range targetVersions {
		byTV[tv] = &perVersion{}
	}

	type tvName struct {
		tv   string
		name string
	}
	seen := make(map[tvName]bool)

	// Load all git repos and build a name-by-ID lookup.
	gitRepos, err := r.db.ListGitRepos(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repos for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute git repo compatibility.")
		return
	}
	repoNameByID := make(map[string]string, len(gitRepos))
	for _, gr := range gitRepos {
		repoNameByID[gr.ID] = gr.Name
	}

	// Load all git repo complexities.
	complexities, err := r.db.ListAllGitRepoComplexities(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repo complexities for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute git repo compatibility.")
		return
	}

	for _, cc := range complexities {
		repoName := repoNameByID[cc.GitRepoID]
		if repoName == "" {
			continue
		}
		if allowedNames != nil && !allowedNames[repoName] {
			continue
		}
		pv, ok := byTV[cc.TargetChefVersion]
		if !ok {
			continue
		}
		key := tvName{tv: cc.TargetChefVersion, name: repoName}
		if seen[key] {
			continue
		}
		seen[key] = true
		pv.total++
		if cc.ErrorCount == 0 && cc.DeprecationCount == 0 {
			pv.compatible++
		} else {
			pv.incompatible++
		}
	}

	// Count untested: git repos with no complexity record for a given
	// target version.
	for _, gr := range gitRepos {
		if allowedNames != nil && !allowedNames[gr.Name] {
			continue
		}
		for _, tv := range targetVersions {
			key := tvName{tv: tv, name: gr.Name}
			if seen[key] {
				continue
			}
			seen[key] = true
			pv := byTV[tv]
			pv.total++
			pv.untested++
		}
	}

	var summaries []compatSummary
	for _, tv := range targetVersions {
		pv := byTV[tv]
		pct := 0.0
		if pv.total > 0 {
			pct = float64(pv.compatible) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion: tv,
			TotalRepos:        pv.total,
			CompatibleRepos:   pv.compatible,
			IncompatibleRepos: pv.incompatible,
			UntestedRepos:     pv.untested,
			CompatiblePercent: pct,
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
			total, stale, fresh, err := r.db.CountStaleFreshByCollectionRun(req.Context(), run.ID)
			if err != nil {
				r.logf("WARN", "counting stale/fresh for run %s in stale trend: %v", run.ID, err)
				continue
			}
			completedAt := ""
			if !run.CompletedAt.IsZero() {
				completedAt = run.CompletedAt.Format("2006-01-02T15:04:05Z")
			}
			points = append(points, trendPoint{
				OrganisationName: org.Name,
				CollectionRunID:  run.ID,
				CompletedAt:      completedAt,
				TotalNodes:       total,
				StaleNodes:       stale,
				FreshNodes:       fresh,
			})
		}
	}

	if points == nil {
		points = []trendPoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// handleDashboardPlatformDistribution handles
// GET /api/v1/dashboard/platform-distribution.
// Returns a count of nodes grouped by their OS platform (combining platform
// and platform_version) across all organisations.
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

	// Resolve owned node keys when ownership filtering is active.
	var ownedKeys map[string]bool
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
			if err != nil {
				r.logf("ERROR", "resolving all owned node keys for platform distribution: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
			if err != nil {
				r.logf("ERROR", "resolving owned node keys for platform distribution: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgs, err := r.db.ListOrganisations(ctx)
	if err != nil {
		r.logf("ERROR", "listing organisations for platform distribution: %v", err)
		WriteInternalError(w, "Failed to compute platform distribution.")
		return
	}

	counts := make(map[string]int)
	totalNodes := 0
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s in platform distribution: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			if ownerFilterActive && ownedKeys != nil {
				if of.Unowned {
					if ownedKeys[n.NodeName] {
						continue
					}
				} else {
					if !ownedKeys[n.NodeName] {
						continue
					}
				}
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
	}

	type platformCount struct {
		Platform string  `json:"platform"`
		Count    int     `json:"count"`
		Percent  float64 `json:"percent"`
	}

	result := make([]platformCount, 0, len(counts))
	for p, c := range counts {
		pct := 0.0
		if totalNodes > 0 {
			pct = float64(c) / float64(totalNodes) * 100
		}
		result = append(result, platformCount{
			Platform: p,
			Count:    c,
			Percent:  pct,
		})
	}

	// Sort by count descending, then platform ascending for stability.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Platform < result[j].Platform
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"total_nodes":  totalNodes,
		"distribution": result,
	})
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
		serverCookbooks, cbErr := r.db.ListServerCookbooksByOrganisation(req.Context(), org.ID)
		if cbErr != nil {
			r.logf("WARN", "listing server cookbooks for org %s in download status: %v", org.Name, cbErr)
			continue
		}
		for _, sc := range serverCookbooks {
			totalCookbooks++
			status := sc.DownloadStatus
			if status == "" {
				status = "pending"
			}
			statusCounts[status]++

			if status == "failed" {
				failedList = append(failedList, failedCookbook{
					ID:             sc.ID,
					OrganisationID: sc.OrganisationID,
					OrgName:        orgNameByID[sc.OrganisationID],
					Name:           sc.Name,
					Version:        sc.Version,
					DownloadError:  sc.DownloadError,
					IsActive:       sc.IsActive,
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
var _ = datastore.NodeSnapshot{}
