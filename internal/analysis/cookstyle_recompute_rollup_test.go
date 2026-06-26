// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

func ts(day int) time.Time {
	return time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC)
}

func fpRow(scannedDay int, cops ...datastore.FingerprintCopEntry) datastore.CookstyleOffenceFingerprint {
	return datastore.CookstyleOffenceFingerprint{ScannedAt: ts(scannedDay), Cops: cops}
}

// FingerprintValidAt selects the latest row with scanned_at <= T (rows are
// ascending). Before the first scan, no fingerprint is valid.
func TestFingerprintValidAt(t *testing.T) {
	rows := []datastore.CookstyleOffenceFingerprint{
		fpRow(10, datastore.FingerprintCopEntry{CopName: "A", Count: 1}),
		fpRow(20, datastore.FingerprintCopEntry{CopName: "B", Count: 2}),
	}

	// Before any scan → not available.
	if _, ok := FingerprintValidAt(rows, ts(5)); ok {
		t.Error("expected no fingerprint valid before first scan")
	}
	// On/after first scan, before second → first row.
	cops, ok := FingerprintValidAt(rows, ts(15))
	if !ok || len(cops) != 1 || cops[0].CopName != "A" {
		t.Errorf("at day 15 got %v ok=%v, want [A]", cops, ok)
	}
	// Exactly on a scan boundary is inclusive.
	cops, ok = FingerprintValidAt(rows, ts(20))
	if !ok || cops[0].CopName != "B" {
		t.Errorf("at day 20 got %v ok=%v, want [B]", cops, ok)
	}
	// After the latest → latest row.
	cops, _ = FingerprintValidAt(rows, ts(99))
	if cops[0].CopName != "B" {
		t.Errorf("at day 99 got %v, want [B]", cops)
	}
	// Empty history → not available.
	if _, ok := FingerprintValidAt(nil, ts(15)); ok {
		t.Error("expected nil history to be unavailable")
	}
}

// RecomputeRollupAt rolls up re-derived status + complexity across current
// membership at time T. Members with no fingerprint valid at T are Untested.
func TestRecomputeRollupAt(t *testing.T) {
	rules := DefaultFailureRules()
	resolver := resolverAt("18.0", map[string]string{
		"Op/Blocker": ClassificationBlocker,
		"Op/Review":  ClassificationReview,
	})

	histories := []ResultFingerprintHistory{
		// Ready: only a non-failing unclassified cop.
		{Key: "ready", Rows: []datastore.CookstyleOffenceFingerprint{
			fpRow(10, datastore.FingerprintCopEntry{CopName: "Style/Unknown", Count: 1, Severity: "warning"}),
		}},
		// Needs review.
		{Key: "review", Rows: []datastore.CookstyleOffenceFingerprint{
			fpRow(10, datastore.FingerprintCopEntry{CopName: "Op/Review", Count: 2, Severity: "warning"}),
		}},
		// Blocked.
		{Key: "blocked", Rows: []datastore.CookstyleOffenceFingerprint{
			fpRow(10, datastore.FingerprintCopEntry{CopName: "Op/Blocker", Count: 1, Severity: "warning"}),
		}},
		// Untested at T=15: its first scan is day 30 (after T).
		{Key: "untested", Rows: []datastore.CookstyleOffenceFingerprint{
			fpRow(30, datastore.FingerprintCopEntry{CopName: "Op/Blocker", Count: 1}),
		}},
	}

	got := RecomputeRollupAt(histories, ts(15), rules, resolver)

	if got.Ready != 1 || got.NeedsReview != 1 || got.Blocked != 1 || got.Untested != 1 {
		t.Errorf("rollup counts = %+v, want ready=1 review=1 blocked=1 untested=1", got)
	}
	if got.MembersWithData != 3 {
		t.Errorf("MembersWithData = %d, want 3", got.MembersWithData)
	}
	wantComplexity := 2*remediation.WeightReview + 1*remediation.WeightBlocker
	if got.TotalComplexity != wantComplexity {
		t.Errorf("TotalComplexity = %d, want %d", got.TotalComplexity, wantComplexity)
	}
}

// GroupFingerprintHistories splits bulk rows into per-result histories keyed by
// kind+identity; rows for one result stay contiguous and ascending, and a shared
// name across kinds does not collide.
func TestGroupFingerprintHistories(t *testing.T) {
	rows := []datastore.CookstyleOffenceFingerprint{
		{ResultKind: datastore.FingerprintKindServerCookbook, OrganisationName: "o", CookbookName: "cb", CookbookVersion: "1", TargetChefVersion: "19", ScannedAt: ts(10)},
		{ResultKind: datastore.FingerprintKindServerCookbook, OrganisationName: "o", CookbookName: "cb", CookbookVersion: "1", TargetChefVersion: "19", ScannedAt: ts(20)},
		// Same name "cb" but git kind → separate history.
		{ResultKind: datastore.FingerprintKindGitRepo, GitRepoName: "cb", GitRepoURL: "git@example.com:o/cb", TargetChefVersion: "19", ScannedAt: ts(10)},
	}
	got := GroupFingerprintHistories(rows)
	if len(got) != 2 {
		t.Fatalf("expected 2 histories, got %d", len(got))
	}
	if len(got[0].Rows) != 2 {
		t.Errorf("server history should have 2 rows, got %d", len(got[0].Rows))
	}
	if !got[0].Rows[0].ScannedAt.Before(got[0].Rows[1].ScannedAt) {
		t.Error("server history rows should stay ascending")
	}
	if len(got[1].Rows) != 1 {
		t.Errorf("git history should have 1 row, got %d", len(got[1].Rows))
	}
	if got[0].Key == got[1].Key {
		t.Error("server and git results sharing a name must not share a key")
	}
}

// DistinctScanTimes collects the change points (distinct scanned_at) across all
// histories, ascending and de-duplicated.
func TestDistinctScanTimes(t *testing.T) {
	histories := []ResultFingerprintHistory{
		{Key: "a", Rows: []datastore.CookstyleOffenceFingerprint{fpRow(20), fpRow(10)}},
		{Key: "b", Rows: []datastore.CookstyleOffenceFingerprint{fpRow(10), fpRow(30)}},
	}
	got := DistinctScanTimes(histories)
	want := []time.Time{ts(10), ts(20), ts(30)}
	if len(got) != len(want) {
		t.Fatalf("got %d times, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("time[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// RecomputeTrend produces one point per supplied time, and the series reflects a
// reclassification applied to the CURRENT resolver across every historical point.
func TestRecomputeTrend_ReclassificationAffectsWholeSeries(t *testing.T) {
	rules := DefaultFailureRules()
	histories := []ResultFingerprintHistory{
		{Key: "r", Rows: []datastore.CookstyleOffenceFingerprint{
			fpRow(10, datastore.FingerprintCopEntry{CopName: "Op/X", Count: 1, Severity: "warning"}),
		}},
	}
	times := DistinctScanTimes(histories)

	// Under the review classification, every post-boundary point is Needs review.
	reviewResolver := resolverAt("18.0", map[string]string{"Op/X": ClassificationReview})
	pts := RecomputeTrend(histories, times, rules, reviewResolver)
	if len(pts) != 1 || pts[0].Rollup.NeedsReview != 1 {
		t.Fatalf("review series = %+v, want one NeedsReview point", pts)
	}

	// Reclassify to blocker: the same frozen fingerprints now recompute to Blocked
	// across the whole series — recompute uses TODAY's criteria for past points.
	blockerResolver := resolverAt("18.0", map[string]string{"Op/X": ClassificationBlocker})
	pts = RecomputeTrend(histories, times, rules, blockerResolver)
	if len(pts) != 1 || pts[0].Rollup.Blocked != 1 {
		t.Fatalf("blocker series = %+v, want one Blocked point", pts)
	}
}

// FingerprintDataBoundary is the earliest scanned_at across all histories — the
// point before which no trend point can be recomputed (frozen range).
func TestFingerprintDataBoundary(t *testing.T) {
	histories := []ResultFingerprintHistory{
		{Key: "a", Rows: []datastore.CookstyleOffenceFingerprint{fpRow(20), fpRow(40)}},
		{Key: "b", Rows: []datastore.CookstyleOffenceFingerprint{fpRow(10), fpRow(30)}},
		{Key: "empty"},
	}
	boundary, ok := FingerprintDataBoundary(histories)
	if !ok || !boundary.Equal(ts(10)) {
		t.Errorf("boundary = %v ok=%v, want %v", boundary, ok, ts(10))
	}
	if _, ok := FingerprintDataBoundary(nil); ok {
		t.Error("expected no boundary for empty input")
	}
}
