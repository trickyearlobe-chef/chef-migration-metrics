// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// rollupBucketForScan maps a CookStyle scan result to one of the 4-state rollup
// buckets the dashboard summaries report (ready / needs_review / blocked /
// untested), so every surface shares the cop-classification.md vocabulary. An
// inconclusive scan (error_message set) and a row with no materialised status
// fall to untested — a scan that produced no verdict. This deliberately ignores
// the legacy passed boolean: a needs_review result has passed=true but must not
// read as ready/"compatible".
func rollupBucketForScan(cookstyleStatus, errorMessage string) string {
	if errorMessage != "" {
		return analysis.StatusUntested
	}
	switch cookstyleStatus {
	case analysis.StatusReady, analysis.StatusNeedsReview, analysis.StatusBlocked:
		return cookstyleStatus
	default:
		return analysis.StatusUntested
	}
}

// ---------------------------------------------------------------------------
// Dashboard — compatibility endpoints (cookbook, git repo, Test Kitchen)
// ---------------------------------------------------------------------------

// handleDashboardCookbookCompatibility handles
// GET /api/v1/dashboard/cookbook-compatibility.
// Returns the CookStyle rollup-status breakdown (ready / needs_review / blocked
// / untested) for server cookbooks across all organisations and target Chef
// versions, sourced from the materialised cookstyle_status.
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

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "cookbook")
	if err != nil {
		r.logf("ERROR", "resolving cookbook ownership filter for compatibility: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbook compatibility: %v", err)
		WriteInternalError(w, "Failed to compute cookbook compatibility.")
		return
	}

	targetVersions := r.liveConfig().TargetChefVersionList()

	// CookStyle rollup summary (cop-classification.md 4-state vocabulary). The
	// untested segment is sub-split (errored scan / inactive / not-yet-scanned)
	// for the card tooltip; the three add up to UntestedCookbooks.
	type compatSummary struct {
		TargetChefVersion    string  `json:"target_chef_version"`
		TotalCookbooks       int     `json:"total_cookbooks"`
		ReadyCookbooks       int     `json:"ready_cookbooks"`
		NeedsReviewCookbooks int     `json:"needs_review_cookbooks"`
		BlockedCookbooks     int     `json:"blocked_cookbooks"`
		UntestedCookbooks    int     `json:"untested_cookbooks"`
		UntestedErrored      int     `json:"untested_errored_cookbooks"`
		UntestedInactive     int     `json:"untested_inactive_cookbooks"`
		UntestedUnscanned    int     `json:"untested_unscanned_cookbooks"`
		ReadyPercent         float64 `json:"ready_percent"`
	}

	// Build an allowed-names set for ownership filtering (nil = no filter).
	var allowedNames map[string]bool
	if ownerFilterActive {
		if of.Unowned {
			allCookbooks := make(map[string]bool)
			for _, org := range orgs {
				cbs, err := r.db.ListServerCookbooksByOrganisation(ctx, org.Name)
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

	// Aggregate the materialised CookStyle rollup status per target Chef version,
	// deduplicating by cookbook name so each name counts once per target version.
	type perVersion struct {
		total             int
		ready             int
		needsReview       int
		blocked           int
		untested          int
		untestedErrored   int
		untestedInactive  int
		untestedUnscanned int
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
		cookstyleResults, err := r.db.ListServerCookbookCookstyleResultsByOrganisation(ctx, org.Name)
		if err != nil {
			r.logf("WARN", "listing server cookbook cookstyle results for org %s: %v", org.Name, err)
			continue
		}

		// Also need the cookbook metadata to get the name.
		serverCookbooks, scErr := r.db.ListServerCookbooksByOrganisation(ctx, org.Name)
		if scErr != nil {
			r.logf("WARN", "listing server cookbooks for org %s: %v", org.Name, scErr)
			continue
		}
		cookbookNameByID := make(map[string]string, len(serverCookbooks))
		for _, sc := range serverCookbooks {
			cookbookNameByID[sc.OrganisationName+"/"+sc.Name+"/"+sc.Version] = sc.Name
		}

		// Bucket each scanned cookbook by its materialised CookStyle rollup
		// status (ready / needs_review / blocked / untested).
		for _, cs := range cookstyleResults {
			cbName := cookbookNameByID[cs.OrganisationName+"/"+cs.CookbookName+"/"+cs.CookbookVersion]
			if cbName == "" {
				continue
			}
			if allowedNames != nil && !allowedNames[cbName] {
				continue
			}
			pv, ok := byTV[cs.TargetChefVersion]
			if !ok {
				continue
			}
			key := tvName{tv: cs.TargetChefVersion, name: cbName}
			if seen[key] {
				continue
			}
			seen[key] = true
			pv.total++
			switch rollupBucketForScan(cs.CookstyleStatus, cs.ErrorMessage) {
			case analysis.StatusReady:
				pv.ready++
			case analysis.StatusNeedsReview:
				pv.needsReview++
			case analysis.StatusBlocked:
				pv.blocked++
			default: // untested — an errored scan has no verdict.
				pv.untested++
				pv.untestedErrored++
			}
		}

		// Count untested: server cookbooks with no cookstyle result for a
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
				if sc.IsActive {
					pv.untestedUnscanned++
				} else {
					pv.untestedInactive++
				}
			}
		}
	}

	var summaries []compatSummary
	for _, tv := range targetVersions {
		pv := byTV[tv]
		pct := 0.0
		if pv.total > 0 {
			pct = float64(pv.ready) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion:    tv,
			TotalCookbooks:       pv.total,
			ReadyCookbooks:       pv.ready,
			NeedsReviewCookbooks: pv.needsReview,
			BlockedCookbooks:     pv.blocked,
			UntestedCookbooks:    pv.untested,
			UntestedErrored:      pv.untestedErrored,
			UntestedInactive:     pv.untestedInactive,
			UntestedUnscanned:    pv.untestedUnscanned,
			ReadyPercent:         pct,
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

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "cookbook")
	if err != nil {
		r.logf("ERROR", "resolving cookbook ownership filter for git repo compatibility: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	targetVersions := r.liveConfig().TargetChefVersionList()

	// CookStyle rollup summary (4-state). UntestedRepos is sub-split into errored
	// scan / clone-failed / cloned-but-not-yet-scanned for the card tooltip.
	type compatSummary struct {
		TargetChefVersion        string  `json:"target_chef_version"`
		TotalRepos               int     `json:"total_repos"`
		ReadyRepos               int     `json:"ready_repos"`
		NeedsReviewRepos         int     `json:"needs_review_repos"`
		BlockedRepos             int     `json:"blocked_repos"`
		UntestedRepos            int     `json:"untested_repos"`
		UntestedErroredRepos     int     `json:"untested_errored_repos"`
		UntestedCloneFailedRepos int     `json:"untested_clone_failed_repos"`
		UntestedPendingScanRepos int     `json:"untested_pending_scan_repos"`
		ReadyPercent             float64 `json:"ready_percent"`
	}

	// Build an allowed-names set for ownership filtering (nil = no filter).
	var allowedNames map[string]bool
	if ownerFilterActive {
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

	// Aggregate the materialised CookStyle rollup status per target Chef version.
	type perVersion struct {
		total               int
		ready               int
		needsReview         int
		blocked             int
		untested            int
		untestedErrored     int
		untestedCloneFailed int
		untestedPendingScan int
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

	// Load git repos — one per cookbook name.
	gitRepos, err := r.db.ListGitRepos(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repos for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute git repo compatibility.")
		return
	}
	repoNameByID := make(map[string]string, len(gitRepos))
	repoCloneStatus := make(map[string]string, len(gitRepos))
	for _, gr := range gitRepos {
		repoNameByID[gr.Name] = gr.Name
		repoCloneStatus[gr.Name] = gr.CloneStatus
	}

	// Bucket each scanned repo by its materialised CookStyle rollup status; a
	// repo with no result for the target version is counted as untested below.
	allCookstyle, err := r.db.ListAllGitRepoCookstyleResults(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repo cookstyle results for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute git repo compatibility.")
		return
	}

	for _, cs := range allCookstyle {
		repoName := repoNameByID[cs.GitRepoName]
		if repoName == "" {
			continue
		}
		if allowedNames != nil && !allowedNames[repoName] {
			continue
		}
		pv, ok := byTV[cs.TargetChefVersion]
		if !ok {
			continue
		}
		key := tvName{tv: cs.TargetChefVersion, name: repoName}
		if seen[key] {
			continue
		}
		seen[key] = true
		pv.total++
		// A repo we can't clone can't be verified — count it untested regardless
		// of any stale result, matching the materialised list column and the repo
		// detail (a Missing repo must not show a ready/needs_review/blocked verdict).
		if repoCloneStatus[repoName] == "failed" {
			pv.untested++
			pv.untestedCloneFailed++
			continue
		}
		switch rollupBucketForScan(cs.CookstyleStatus, cs.ErrorMessage) {
		case analysis.StatusReady:
			pv.ready++
		case analysis.StatusNeedsReview:
			pv.needsReview++
		case analysis.StatusBlocked:
			pv.blocked++
		default: // untested — an errored scan has no verdict.
			pv.untested++
			pv.untestedErrored++
		}
	}

	// Count untested: git repos with no complexity record for a given
	// target version. Split into clone-failed vs pending-scan so the
	// dashboard can explain *why* a repo is untested.
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
			if gr.CloneStatus == "failed" {
				pv.untestedCloneFailed++
			} else {
				pv.untestedPendingScan++
			}
		}
	}

	var summaries []compatSummary
	for _, tv := range targetVersions {
		pv := byTV[tv]
		pct := 0.0
		if pv.total > 0 {
			pct = float64(pv.ready) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion:        tv,
			TotalRepos:               pv.total,
			ReadyRepos:               pv.ready,
			NeedsReviewRepos:         pv.needsReview,
			BlockedRepos:             pv.blocked,
			UntestedRepos:            pv.untested,
			UntestedErroredRepos:     pv.untestedErrored,
			UntestedCloneFailedRepos: pv.untestedCloneFailed,
			UntestedPendingScanRepos: pv.untestedPendingScan,
			ReadyPercent:             pct,
		})
	}

	if summaries == nil {
		summaries = []compatSummary{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

// handleDashboardTestKitchenCompatibility handles
// GET /api/v1/dashboard/test-kitchen-compatibility.
// Returns per-target-version counts of git repos whose latest Test Kitchen
// run passed, failed, timed out, or has not been tested yet. The response
// shape mirrors the cookbook/git-repo compatibility cards so the frontend
// can render an identical stacked-bar summary.
func (r *Router) handleDashboardTestKitchenCompatibility(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "cookbook")
	if err != nil {
		r.logf("ERROR", "resolving cookbook ownership filter for TK compatibility: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	targetVersions := r.liveConfig().TargetChefVersionList()

	type tkSummary struct {
		TargetChefVersion        string  `json:"target_chef_version"`
		TotalRepos               int     `json:"total_repos"`
		PassedRepos              int     `json:"passed_repos"`
		PartialRepos             int     `json:"partial_repos"`
		FailedRepos              int     `json:"failed_repos"`
		TimedOutRepos            int     `json:"timed_out_repos"`
		UntestedRepos            int     `json:"untested_repos"`
		UntestedCloneFailedRepos int     `json:"untested_clone_failed_repos"`
		UntestedPendingScanRepos int     `json:"untested_pending_scan_repos"`
		PassedPercent            float64 `json:"passed_percent"`
	}

	// Build an allowed-names set for ownership filtering (nil = no filter).
	var allowedNames map[string]bool
	if ownerFilterActive {
		gitRepos, err := r.db.ListGitRepos(ctx)
		if err != nil {
			r.logf("ERROR", "listing git repos for TK compatibility ownership filter: %v", err)
			WriteInternalError(w, "Failed to compute Test Kitchen compatibility.")
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

	// Tally per-target-version counts.
	type perVersion struct {
		total               int
		passed              int
		partial             int
		failed              int
		timedOut            int
		untested            int
		untestedCloneFailed int
		untestedPendingScan int
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

	// Load git repos — one per cookbook name.
	gitRepos, err := r.db.ListGitRepos(ctx)
	if err != nil {
		r.logf("ERROR", "listing git repos for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute compatibility.")
		return
	}

	// Load kitchen results to compute per-repo TK status.
	type repoTKInfo struct {
		passed int
		failed int
		total  int
	}
	tkByRepo := make(map[string]*repoTKInfo)
	allResults, tkErr := r.db.ListActiveGitKitchenResults(ctx)
	if tkErr != nil {
		r.logf("WARN", "listing git kitchen results for TK dashboard: %v", tkErr)
	} else {
		for _, res := range allResults {
			// A lab failure is not a verdict about the cookbook — see the
			// rule on datastore.ListGitKitchenCountsByTargetVersions.
			isPass := res.Passed != nil && *res.Passed
			isFail := tkstatus.CountsAsCookbookFailure(res.Passed, res.FailureKind)
			if !isPass && !isFail {
				continue
			}
			s := tkByRepo[res.GitRepoName]
			if s == nil {
				s = &repoTKInfo{}
				tkByRepo[res.GitRepoName] = s
			}
			s.total++
			if isPass {
				s.passed++
			} else {
				s.failed++
			}
		}
	}

	// Count repos per target version. Start all as untested, then
	// reclassify based on kitchen results.
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

			s := tkByRepo[gr.Name]
			if s != nil && s.total > 0 {
				switch tkstatus.ComputeTKStatus(s.passed, s.failed) {
				case "partial":
					pv.partial++
				case "failed":
					pv.failed++
				case "passed":
					pv.passed++
				}
			} else {
				pv.untested++
				if gr.CloneStatus == "failed" {
					pv.untestedCloneFailed++
				} else {
					pv.untestedPendingScan++
				}
			}
		}
	}

	var summaries []tkSummary
	for _, tv := range targetVersions {
		pv := byTV[tv]
		pct := 0.0
		if pv.total > 0 {
			pct = float64(pv.passed) / float64(pv.total) * 100
		}
		summaries = append(summaries, tkSummary{
			TargetChefVersion:        tv,
			TotalRepos:               pv.total,
			PassedRepos:              pv.passed,
			PartialRepos:             pv.partial,
			FailedRepos:              pv.failed,
			TimedOutRepos:            pv.timedOut,
			UntestedRepos:            pv.untested,
			UntestedCloneFailedRepos: pv.untestedCloneFailed,
			UntestedPendingScanRepos: pv.untestedPendingScan,
			PassedPercent:            pct,
		})
	}

	if summaries == nil {
		summaries = []tkSummary{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}
