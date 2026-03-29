// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Dashboard — compatibility endpoints (cookbook, git repo, Test Kitchen)
// ---------------------------------------------------------------------------

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

	targetVersions := r.cfg.TargetChefVersions

	type compatSummary struct {
		TargetChefVersion     string  `json:"target_chef_version"`
		TotalCookbooks        int     `json:"total_cookbooks"`
		CompatibleCookbooks   int     `json:"compatible_cookbooks"`
		IncompatibleCookbooks int     `json:"incompatible_cookbooks"`
		ErroredCookbooks      int     `json:"errored_cookbooks"`
		UntestedCookbooks     int     `json:"untested_cookbooks"`
		UntestedInactive      int     `json:"untested_inactive_cookbooks"`
		UntestedUnscanned     int     `json:"untested_unscanned_cookbooks"`
		CompatiblePercent     float64 `json:"compatible_percent"`
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

	// Compute compatibility from server cookbook cookstyle results, aggregated
	// per target Chef version. A cookbook is "compatible" when cookstyle passed,
	// "incompatible" when it did not, and "untested" when no result exists.
	// We deduplicate by cookbook name so each name counts once per target version.
	type perVersion struct {
		total             int
		compatible        int
		incompatible      int
		errored           int
		untested          int
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

		// Derive compatibility directly from CookStyle scan results.
		// A cookbook that passed CookStyle (no error/fatal offenses) is
		// compatible; one that failed is incompatible.
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
			if cs.ErrorMessage != "" {
				pv.errored++
			} else if cs.Passed {
				pv.compatible++
			} else {
				pv.incompatible++
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
			pct = float64(pv.compatible) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion:     tv,
			TotalCookbooks:        pv.total,
			CompatibleCookbooks:   pv.compatible,
			IncompatibleCookbooks: pv.incompatible,
			ErroredCookbooks:      pv.errored,
			UntestedCookbooks:     pv.untested,
			UntestedInactive:      pv.untestedInactive,
			UntestedUnscanned:     pv.untestedUnscanned,
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

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "cookbook")
	if err != nil {
		r.logf("ERROR", "resolving cookbook ownership filter for git repo compatibility: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}
	ownerFilterActive := ownedKeys != nil

	targetVersions := r.cfg.TargetChefVersions

	type compatSummary struct {
		TargetChefVersion        string  `json:"target_chef_version"`
		TotalRepos               int     `json:"total_repos"`
		CompatibleRepos          int     `json:"compatible_repos"`
		IncompatibleRepos        int     `json:"incompatible_repos"`
		ErroredRepos             int     `json:"errored_repos"`
		UntestedRepos            int     `json:"untested_repos"`
		UntestedCloneFailedRepos int     `json:"untested_clone_failed_repos"`
		UntestedPendingScanRepos int     `json:"untested_pending_scan_repos"`
		CompatiblePercent        float64 `json:"compatible_percent"`
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

	// Compute compatibility from git repo complexity records.
	type perVersion struct {
		total               int
		compatible          int
		incompatible        int
		errored             int
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
		WriteInternalError(w, "Failed to compute git repo compatibility.")
		return
	}
	repoNameByID := make(map[string]string, len(gitRepos))
	repoCloneStatus := make(map[string]string, len(gitRepos))
	for _, gr := range gitRepos {
		repoNameByID[gr.Name] = gr.Name
		repoCloneStatus[gr.Name] = gr.CloneStatus
	}

	// Determine compatibility directly from CookStyle results.
	// Passed == true → compatible, Passed == false → incompatible,
	// no result for the target version → untested.
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
		if cs.ErrorMessage != "" {
			pv.errored++
		} else if cs.Passed {
			pv.compatible++
		} else {
			pv.incompatible++
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
			pct = float64(pv.compatible) / float64(pv.total) * 100
		}
		summaries = append(summaries, compatSummary{
			TargetChefVersion:        tv,
			TotalRepos:               pv.total,
			CompatibleRepos:          pv.compatible,
			IncompatibleRepos:        pv.incompatible,
			ErroredRepos:             pv.errored,
			UntestedRepos:            pv.untested,
			UntestedCloneFailedRepos: pv.untestedCloneFailed,
			UntestedPendingScanRepos: pv.untestedPendingScan,
			CompatiblePercent:        pct,
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

	targetVersions := r.cfg.TargetChefVersions

	type tkSummary struct {
		TargetChefVersion        string  `json:"target_chef_version"`
		TotalRepos               int     `json:"total_repos"`
		PassedRepos              int     `json:"passed_repos"`
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
		r.logf("ERROR", "listing git repos for TK compatibility: %v", err)
		WriteInternalError(w, "Failed to compute Test Kitchen compatibility.")
		return
	}
	repoNameByID := make(map[string]string, len(gitRepos))
	for _, gr := range gitRepos {
		repoNameByID[gr.Name] = gr.Name
	}

	// Load all test kitchen results.
	tkResults, err := r.db.ListAllGitRepoTestKitchenResults(ctx)
	if err != nil {
		r.logf("ERROR", "listing TK results for compatibility: %v", err)
		WriteInternalError(w, "Failed to compute Test Kitchen compatibility.")
		return
	}

	for _, tk := range tkResults {
		repoName := repoNameByID[tk.GitRepoName]
		if repoName == "" {
			continue
		}
		if allowedNames != nil && !allowedNames[repoName] {
			continue
		}
		pv, ok := byTV[tk.TargetChefVersion]
		if !ok {
			continue
		}
		key := tvName{tv: tk.TargetChefVersion, name: repoName}
		if seen[key] {
			continue
		}
		seen[key] = true
		pv.total++
		switch {
		case tk.TimedOut:
			pv.timedOut++
		case tk.Compatible:
			pv.passed++
		default:
			pv.failed++
		}
	}

	// Count untested: git repos with no TK result for a given target
	// version. Split into clone-failed vs pending-scan so the dashboard
	// can explain *why* a repo is untested.
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
