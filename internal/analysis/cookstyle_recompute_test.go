// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// The fingerprint re-derivation engine must agree exactly with the scan-time
// single source of truth: re-deriving status from a result's stored offence
// fingerprint yields the same rollup as DeriveCookstyleStatus over the original
// offences. Occurrence count collapses in the fingerprint but does not change
// status (one blocker blocks; one review needs review).
func TestDeriveStatusFromFingerprint_MatchesScanDerivation(t *testing.T) {
	resolver := resolverAt("18.0", map[string]string{
		"Op/Blocker": ClassificationBlocker,
		"Op/Review":  ClassificationReview,
		"Op/Noise":   ClassificationNoise,
	})

	cases := []struct {
		name     string
		offenses []CookstyleOffense
	}{
		{"clean", nil},
		{"noise only", []CookstyleOffense{
			{CopName: "Op/Noise", Severity: "warning"},
			{CopName: "Op/Noise", Severity: "warning"},
		}},
		{"review repeated collapses but still needs review", []CookstyleOffense{
			{CopName: "Op/Review", Severity: "warning"},
			{CopName: "Op/Review", Severity: "warning"},
			{CopName: "Op/Review", Severity: "warning"},
		}},
		{"blocker dominates", []CookstyleOffense{
			{CopName: "Op/Review", Severity: "warning"},
			{CopName: "Op/Blocker", Severity: "warning"},
		}},
		{"unclassified fatal severity-fails", []CookstyleOffense{
			{CopName: "Lint/Syntax", Severity: "fatal"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := DeriveCookstyleStatus(tc.offenses, resolver)
			cops, _ := BuildOffenceFingerprint(tc.offenses)
			got := DeriveStatusFromFingerprint(cops, resolver)
			if got != want {
				t.Errorf("DeriveStatusFromFingerprint = %q, want %q (scan derivation)", got, want)
			}
		})
	}
}

// ComplexityFromFingerprint weights each cop's occurrence count by its resolved
// classification, exactly as the scan-time classification-weighted complexity
// does. It is the CookStyle contribution only (Test Kitchen is not fingerprinted).
func TestComplexityFromFingerprint_CountWeighted(t *testing.T) {
	resolver := resolverAt("18.0", map[string]string{
		"Op/Blocker": ClassificationBlocker,
		"Op/Review":  ClassificationReview,
		"Op/Noise":   ClassificationNoise,
	})

	cops := []datastore.FingerprintCopEntry{
		{CopName: "Op/Blocker", Count: 2, Severity: "warning"}, // 2 * WeightBlocker
		{CopName: "Op/Review", Count: 3, Severity: "warning"},  // 3 * WeightReview
		{CopName: "Op/Noise", Count: 5, Severity: "warning"},   // 0
	}
	want := 2*remediation.WeightBlocker + 3*remediation.WeightReview

	if got := ComplexityFromFingerprint(cops, resolver); got != want {
		t.Errorf("ComplexityFromFingerprint = %d, want %d", got, want)
	}
}

// A nil classifier yields zero complexity (no classification context available).
func TestComplexityFromFingerprint_NilClassifier(t *testing.T) {
	cops := []datastore.FingerprintCopEntry{{CopName: "Op/Blocker", Count: 2}}
	if got := ComplexityFromFingerprint(cops, nil); got != 0 {
		t.Errorf("ComplexityFromFingerprint(nil classifier) = %d, want 0", got)
	}
}

// The whole point of fingerprint history: the SAME stored fingerprint re-derives
// to a DIFFERENT status/complexity once a cop is reclassified. A trend point
// captured before the reclassification recomputes under today's criteria.
func TestRecompute_ReflectsReclassification(t *testing.T) {
	cops := []datastore.FingerprintCopEntry{{CopName: "Op/X", Count: 4, Severity: "warning"}}

	// Before: X is a review-level cop.
	before := resolverAt("18.0", map[string]string{"Op/X": ClassificationReview})
	if got := DeriveStatusFromFingerprint(cops, before); got != StatusNeedsReview {
		t.Fatalf("before reclassification: status = %q, want %q", got, StatusNeedsReview)
	}
	if got := ComplexityFromFingerprint(cops, before); got != 4*remediation.WeightReview {
		t.Fatalf("before reclassification: complexity = %d, want %d", got, 4*remediation.WeightReview)
	}

	// After: an operator reclassifies X as a blocker. The frozen fingerprint now
	// recomputes to Blocked with blocker weighting — no rescan required.
	after := resolverAt("18.0", map[string]string{"Op/X": ClassificationBlocker})
	if got := DeriveStatusFromFingerprint(cops, after); got != StatusBlocked {
		t.Fatalf("after reclassification: status = %q, want %q", got, StatusBlocked)
	}
	if got := ComplexityFromFingerprint(cops, after); got != 4*remediation.WeightBlocker {
		t.Fatalf("after reclassification: complexity = %d, want %d", got, 4*remediation.WeightBlocker)
	}
}
