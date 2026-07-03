// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"sort"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// Fingerprint re-derivation engine — the trend-recompute counterpart of the
// scan-time single source of truth. Given a result's stored offence fingerprint
// (the per-cop projection persisted in cookstyle_offence_fingerprints), it
// re-derives the same rollup status and weighted complexity DeriveCookstyleStatus
// and the classification-weighted complexity produce at scan time, but under
// WHATEVER classification is current at recompute time. This is what lets a trend
// point captured after fingerprint history ships be recomputed under today's
// criteria after a reclassification (no rescan). See
// specifications/enriched-metric-snapshots.md → Trend Recompute Under Current
// Criteria and specifications/cop-classification.md → History.

// DeriveStatusFromFingerprint re-derives the rollup status from a result's stored
// offence fingerprint under the given failure rules + resolver. It is exactly the
// scan-time derivation (DeriveCookstyleStatus delegates to the same core): a cop's
// occurrence count is collapsed in the fingerprint but does not affect status —
// one blocker blocks, one review needs review, one unclassified severity-fail
// blocks. An empty fingerprint is Ready (a clean scan). Untested is the caller's
// concern when no fingerprint exists at all for a result at time T.
func DeriveStatusFromFingerprint(cops []datastore.FingerprintCopEntry, rules CookstyleFailureRules, resolver *CopClassificationResolver) string {
	// rules is retained in the signature for call-site stability but no longer
	// participates in the verdict: severity is the signal this feature exists to
	// distrust, so it never produces a red. Only a Blocker classification blocks.
	_ = rules
	hasReview := false
	for i := range cops {
		c := &cops[i]
		switch resolver.Resolve(c.CopName).Classification {
		case ClassificationBlocker:
			return StatusBlocked
		case ClassificationReview:
			hasReview = true
		case ClassificationNoise:
			// Noise contributes nothing to the rollup.
		}
	}
	if hasReview {
		return StatusNeedsReview
	}
	return StatusReady
}

// ComplexityFromFingerprint re-derives the classification-weighted CookStyle
// complexity contribution from a result's stored offence fingerprint under the
// current classifier. Each cop's weight is applied once per occurrence (×count),
// mirroring ComputeCookstyleComplexity over the original offences.
//
// This is the CookStyle portion ONLY. Test Kitchen status is not fingerprinted,
// so a recomputed trend point carries the CookStyle-derived complexity; the TK
// weight added at scan time (tkWeight) has no historical counterpart to recompute
// from. A nil classifier yields 0 (no classification context).
func ComplexityFromFingerprint(cops []datastore.FingerprintCopEntry, classifier remediation.CopClassifier) int {
	if classifier == nil {
		return 0
	}
	// Expand each per-cop entry back to its occurrence count and reuse the exact
	// scan-time complexity function, so recompute can never drift from the SoT.
	total := 0
	for _, c := range cops {
		off := remediation.ClassifiedOffense{
			CopName:        c.CopName,
			Severity:       c.Severity,
			Classification: classifier.Classify(c.CopName),
		}
		if c.Count <= 0 {
			continue
		}
		total += remediation.ComputeCookstyleComplexity([]remediation.ClassifiedOffense{off}) * c.Count
	}
	return total
}

// ---------------------------------------------------------------------------
// Rollup over current membership × fingerprint-valid-at-T
// ---------------------------------------------------------------------------

// ResultFingerprintHistory is one result's (cookbook×target or repo×target)
// chronological fingerprint history, oldest first — as returned by the datastore
// List...OffenceFingerprints queries. Key is an opaque, caller-chosen result
// identity used only for membership/dedup bookkeeping.
type ResultFingerprintHistory struct {
	Key  string
	Rows []datastore.CookstyleOffenceFingerprint
}

// FingerprintResultKey is the canonical, collision-free identity for a fingerprint
// row's result, used to group bulk rows into per-result histories. Server-cookbook
// and git-repo identities are namespaced by kind so a shared name never collides.
func FingerprintResultKey(r datastore.CookstyleOffenceFingerprint) string {
	switch r.ResultKind {
	case datastore.FingerprintKindServerCookbook:
		return "server\x00" + r.OrganisationName + "\x00" + r.CookbookName + "\x00" + r.CookbookVersion + "\x00" + r.TargetChefVersion
	default: // git_repo
		return "git\x00" + r.GitRepoName + "\x00" + r.GitRepoURL + "\x00" + r.TargetChefVersion
	}
}

// GroupFingerprintHistories splits bulk fingerprint rows into per-result
// histories keyed by FingerprintResultKey. Rows are assumed ordered so each
// result's rows are contiguous and ascending by scanned_at (as
// ListOffenceFingerprintsByTarget returns them); the grouping is a single linear
// pass that preserves that ordering within each history.
func GroupFingerprintHistories(rows []datastore.CookstyleOffenceFingerprint) []ResultFingerprintHistory {
	var out []ResultFingerprintHistory
	index := make(map[string]int)
	for i := range rows {
		key := FingerprintResultKey(rows[i])
		pos, ok := index[key]
		if !ok {
			index[key] = len(out)
			out = append(out, ResultFingerprintHistory{Key: key, Rows: []datastore.CookstyleOffenceFingerprint{rows[i]}})
			continue
		}
		out[pos].Rows = append(out[pos].Rows, rows[i])
	}
	return out
}

// RecomputedRollup is the re-derived status breakdown and weighted complexity for
// a set of results (current membership) at one point in time, under the current
// classification. Untested counts members that have no fingerprint valid at T
// (their first scan is later than T), so the totals always partition membership.
type RecomputedRollup struct {
	Ready           int
	NeedsReview     int
	Blocked         int
	Untested        int
	TotalComplexity int
	// MembersWithData is the number of members that had a fingerprint valid at T
	// (= Ready+NeedsReview+Blocked); Untested members are excluded.
	MembersWithData int
}

// FingerprintValidAt returns the cops of the fingerprint valid at time t — the
// latest row whose scanned_at is <= t — and whether one exists. Rows must be
// ascending by scanned_at (the datastore List queries guarantee this). Before the
// first scan (or for an empty history) it reports ok=false: the result has no
// recomputable state at t and the caller treats it as Untested.
func FingerprintValidAt(rows []datastore.CookstyleOffenceFingerprint, t time.Time) ([]datastore.FingerprintCopEntry, bool) {
	idx := -1
	for i := range rows {
		if rows[i].ScannedAt.After(t) {
			break
		}
		idx = i
	}
	if idx < 0 {
		return nil, false
	}
	return rows[idx].Cops, true
}

// RecomputeRollupAt re-derives each member's rollup status and weighted CookStyle
// complexity from its fingerprint valid at time t, under the given failure rules
// and resolver, and aggregates them. Membership is the supplied set of histories
// (bounded to CURRENT membership — membership-at-T history does not exist; see
// specifications/enriched-metric-snapshots.md → Limitations). The resolver must be
// the one for the members' target version.
func RecomputeRollupAt(histories []ResultFingerprintHistory, t time.Time, rules CookstyleFailureRules, resolver *CopClassificationResolver) RecomputedRollup {
	var out RecomputedRollup
	classifier := resolver // *CopClassificationResolver satisfies remediation.CopClassifier
	for i := range histories {
		cops, ok := FingerprintValidAt(histories[i].Rows, t)
		if !ok {
			out.Untested++
			continue
		}
		out.MembersWithData++
		switch DeriveStatusFromFingerprint(cops, rules, resolver) {
		case StatusBlocked:
			out.Blocked++
		case StatusNeedsReview:
			out.NeedsReview++
		default:
			out.Ready++
		}
		out.TotalComplexity += ComplexityFromFingerprint(cops, classifier)
	}
	return out
}

// RecomputeTrendPoint pairs a point in time with the rollup recomputed at it.
type RecomputeTrendPoint struct {
	At     time.Time
	Rollup RecomputedRollup
}

// DistinctScanTimes returns every distinct scanned_at across all histories,
// sorted ascending. These are the change points of the recomputed series: the
// rollup only changes when some result's fingerprint changes, so evaluating at
// exactly these times reproduces the full step function with no redundant points.
func DistinctScanTimes(histories []ResultFingerprintHistory) []time.Time {
	seen := make(map[int64]struct{})
	var out []time.Time
	for i := range histories {
		for _, row := range histories[i].Rows {
			k := row.ScannedAt.UnixNano()
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, row.ScannedAt)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// RecomputeTrend evaluates RecomputeRollupAt at each supplied time, producing one
// trend point per time (ascending order preserved). It is the assembly the trend
// endpoint serialises; current-membership and target scoping are the caller's
// responsibility (the histories passed in are the membership).
func RecomputeTrend(histories []ResultFingerprintHistory, times []time.Time, rules CookstyleFailureRules, resolver *CopClassificationResolver) []RecomputeTrendPoint {
	points := make([]RecomputeTrendPoint, 0, len(times))
	for _, t := range times {
		points = append(points, RecomputeTrendPoint{At: t, Rollup: RecomputeRollupAt(histories, t, rules, resolver)})
	}
	return points
}

// FingerprintDataBoundary returns the earliest scanned_at across all histories —
// the point in time before which no trend point can be recomputed, because no
// fingerprint inputs were captured yet. Trend consumers use it to mark the
// frozen/recomputable boundary on a mixed-range chart. Reports ok=false when no
// history has any rows.
func FingerprintDataBoundary(histories []ResultFingerprintHistory) (time.Time, bool) {
	var earliest time.Time
	found := false
	for i := range histories {
		rows := histories[i].Rows
		if len(rows) == 0 {
			continue
		}
		first := rows[0].ScannedAt // ascending → first row is earliest
		if !found || first.Before(earliest) {
			earliest = first
			found = true
		}
	}
	return earliest, found
}
