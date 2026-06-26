// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
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
	CompletedAt       string `json:"completed_at"`
	TotalResults      int    `json:"total_results"`
	Ready             int    `json:"ready"`
	NeedsReview       int    `json:"needs_review"`
	Blocked           int    `json:"blocked"`
	Untested          int    `json:"untested"`
	TotalComplexity   int    `json:"total_complexity"`
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
		resolver := analysis.NewResolverFromStore(ctx, r.db, tv)
		times := analysis.DistinctScanTimes(histories)

		if boundary, ok := analysis.FingerprintDataBoundary(histories); ok {
			ts := boundary.UTC().Format(trendTimestampFormat)
			if earliest == "" || ts < earliest {
				earliest = ts
			}
		}

		for _, p := range analysis.RecomputeTrend(histories, times, rules, resolver) {
			roll := p.Rollup
			points = append(points, cookstyleRecomputeTrendPoint{
				TargetChefVersion: tv,
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
