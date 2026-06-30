// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dashboard — CookStyle rollup recompute trend
// ---------------------------------------------------------------------------

// cookstyleRecomputeTrendPoint is one recomputed CookStyle rollup point: the
// per-result status breakdown and weighted complexity re-derived from the offence
// fingerprints valid at that time, under the CURRENT classification. Vocabulary is
// the canonical rollup set (ready / needs_review / blocked / untested).
type cookstyleRecomputeTrendPoint struct {
	TargetChefVersion string `json:"target_chef_version"`
	// Source is the result kind this point aggregates: "server" (server
	// cookbooks) or "git" (git repos). The two are charted as separate series.
	Source          string `json:"source"`
	CompletedAt     string `json:"completed_at"`
	TotalResults    int    `json:"total_results"`
	Ready           int    `json:"ready"`
	NeedsReview     int    `json:"needs_review"`
	Blocked         int    `json:"blocked"`
	Untested        int    `json:"untested"`
	TotalComplexity int    `json:"total_complexity"`
}

// handleDashboardCookstyleRecomputeTrend handles
// GET /api/v1/dashboard/cookstyle/recompute-trend.
//
// Unlike the frozen node_metrics-backed trends, this series is RECOMPUTED from
// the change-deduped offence fingerprint history under today's classification —
// so a reclassification is reflected across every post-fingerprint point without a
// rescan. Each target version's series is built from the distinct fingerprint
// change points; recompute is bounded to CURRENT membership (membership-at-T
// history does not exist). The response carries `recompute_available_from`: the
// earliest fingerprint timestamp, i.e. the boundary before which points are frozen
// and cannot be recomputed. Consumers MUST mark that boundary on a mixed-range
// chart rather than implying the whole series reflects current criteria.
func (r *Router) handleDashboardCookstyleRecomputeTrend(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()
	rules := r.cookstyleFailureRules()
	targetVersions := r.liveConfig().TargetChefVersions

	// Live result keys per target — the current-membership set the fingerprint
	// feed is intersected with. A target absent from the map means its live
	// membership could not be determined (filtering is skipped for it).
	liveKeys := r.liveCookstyleResultKeys(ctx, targetVersions)

	points := []cookstyleRecomputeTrendPoint{}
	var earliest string

	for _, tv := range targetVersions {
		rows, err := r.db.ListOffenceFingerprintsByTarget(ctx, tv)
		if err != nil {
			r.logf("WARN", "listing offence fingerprints for recompute trend, target %s: %v", tv, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}

		histories := analysis.GroupFingerprintHistories(rows)

		// Bound to CURRENT membership: intersect the fingerprint feed with the live
		// result set so a removed-but-still-fingerprinted result (deleted cookbook,
		// dropped repo) does not over-count earlier points. See
		// specifications/enriched-metric-snapshots.md → Trend Recompute / Limitations.
		// When live membership cannot be determined, recompute over the full feed
		// rather than show nothing (the over-count is bounded and self-heals).
		if live, ok := liveKeys[tv]; ok {
			histories = filterHistoriesToLive(histories, live)
		}

		resolver := analysis.NewResolverFromStore(ctx, r.db, tv)

		if boundary, ok := analysis.FingerprintDataBoundary(histories); ok {
			ts := boundary.UTC().Format(trendTimestampFormat)
			if earliest == "" || ts < earliest {
				earliest = ts
			}
		}

		// Chart server cookbooks and git repos as separate series: partition the
		// histories by result kind and recompute each over its own change points.
		serverHist, gitHist := partitionHistoriesBySource(histories)
		for _, part := range []struct {
			source    string
			histories []analysis.ResultFingerprintHistory
		}{
			{source: "server", histories: serverHist},
			{source: "git", histories: gitHist},
		} {
			if len(part.histories) == 0 {
				continue
			}
			times := analysis.DistinctScanTimes(part.histories)
			for _, p := range analysis.RecomputeTrend(part.histories, times, rules, resolver) {
				roll := p.Rollup
				points = append(points, cookstyleRecomputeTrendPoint{
					TargetChefVersion: tv,
					Source:            part.source,
					CompletedAt:       p.At.UTC().Format(trendTimestampFormat),
					TotalResults:      roll.Ready + roll.NeedsReview + roll.Blocked + roll.Untested,
					Ready:             roll.Ready,
					NeedsReview:       roll.NeedsReview,
					Blocked:           roll.Blocked,
					Untested:          roll.Untested,
					TotalComplexity:   roll.TotalComplexity,
				})
			}
		}
	}

	resp := map[string]any{"data": points}
	// recompute_available_from is null when no fingerprint history exists yet —
	// the whole series is still in the frozen (pre-fingerprint) era.
	if earliest != "" {
		resp["recompute_available_from"] = earliest
	} else {
		resp["recompute_available_from"] = nil
	}
	WriteJSON(w, http.StatusOK, resp)
}

// liveCookstyleResultKeys builds, per target, the set of FingerprintResultKeys
// for results that CURRENTLY exist — the live server-cookbook and git-repo
// cookstyle result sets. The recompute handler intersects the fingerprint feed
// with this so removed-but-still-fingerprinted results are excluded.
//
// A target is included in the returned map only when its live membership was
// loaded without a top-level failure; an absent target signals "could not
// determine — do not filter" (the handler then recomputes over the full feed
// rather than dropping everything). Per-org / per-target read errors are logged
// and skipped: those results simply do not appear in the live set.
func (r *Router) liveCookstyleResultKeys(ctx context.Context, targets []string) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(targets))
	ensure := func(tv string) map[string]struct{} {
		if out[tv] == nil {
			out[tv] = make(map[string]struct{})
		}
		return out[tv]
	}

	// Git repos are fleet-wide; query per target (already target-filtered).
	for _, tv := range targets {
		gitRows, err := r.db.ListGitRepoCookstyleResultsByTargetVersion(ctx, tv)
		if err != nil {
			r.logf("WARN", "recompute trend: listing live git results for target %s: %v", tv, err)
			continue
		}
		set := ensure(tv)
		for _, g := range gitRows {
			set[analysis.FingerprintResultKey(datastore.CookstyleOffenceFingerprint{
				ResultKind:        datastore.FingerprintKindGitRepo,
				GitRepoName:       g.GitRepoName,
				GitRepoURL:        g.GitRepoURL,
				TargetChefVersion: tv,
			})] = struct{}{}
		}
	}

	// Server cookbooks: fetch each org once (all targets) and bucket by target.
	orgs, err := r.db.ListOrganisations(ctx)
	if err != nil {
		// Without the org list we cannot enumerate live server cookbooks; mark
		// every target's membership as undeterminable so the handler does not
		// wrongly drop all server-cookbook results.
		r.logf("WARN", "recompute trend: listing organisations for live membership: %v", err)
		return map[string]map[string]struct{}{}
	}
	targetSet := make(map[string]struct{}, len(targets))
	for _, tv := range targets {
		targetSet[tv] = struct{}{}
		ensure(tv) // a target with zero live results is still "determined" (empty set).
	}
	for _, org := range orgs {
		scRows, err := r.db.ListServerCookbookCookstyleResultsByOrganisation(ctx, org.Name)
		if err != nil {
			r.logf("WARN", "recompute trend: listing live server results for org %s: %v", org.Name, err)
			continue
		}
		for _, sc := range scRows {
			if _, ok := targetSet[sc.TargetChefVersion]; !ok {
				continue
			}
			ensure(sc.TargetChefVersion)[analysis.FingerprintResultKey(datastore.CookstyleOffenceFingerprint{
				ResultKind:        datastore.FingerprintKindServerCookbook,
				OrganisationName:  sc.OrganisationName,
				CookbookName:      sc.CookbookName,
				CookbookVersion:   sc.CookbookVersion,
				TargetChefVersion: sc.TargetChefVersion,
			})] = struct{}{}
		}
	}
	return out
}

// partitionHistoriesBySource splits histories into server-cookbook and git-repo
// groups by the result kind of their fingerprint rows, preserving order. A
// history with no rows is dropped (it has no kind and contributes nothing).
func partitionHistoriesBySource(histories []analysis.ResultFingerprintHistory) (server, git []analysis.ResultFingerprintHistory) {
	for _, h := range histories {
		if len(h.Rows) == 0 {
			continue
		}
		if h.Rows[0].ResultKind == datastore.FingerprintKindServerCookbook {
			server = append(server, h)
		} else {
			git = append(git, h)
		}
	}
	return server, git
}

// filterHistoriesToLive drops histories whose result key is not in the live set,
// preserving order.
func filterHistoriesToLive(histories []analysis.ResultFingerprintHistory, live map[string]struct{}) []analysis.ResultFingerprintHistory {
	kept := histories[:0:0]
	for _, h := range histories {
		if _, ok := live[h.Key]; ok {
			kept = append(kept, h)
		}
	}
	return kept
}
