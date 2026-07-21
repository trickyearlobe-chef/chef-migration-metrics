// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "testing"

// Resolver-level coverage for the 2026-07-16 false-negative sweep: each newly
// curated removal must resolve to Blocker/verified_removal against the single
// active target (CC19), and the DeprecatedConstants poly cop must split into a
// Blocker (removed constants) and a Review (Net::HTTPServerException, present).

func TestFalseNegativeSweep_NewCopsResolveBlocker(t *testing.T) {
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.3.15"}
	for _, cop := range []string{
		"Lint/BigDecimalNew",
		"Lint/UnifiedInteger",
		"Chef/Deprecations/UsesChefRESTHelpers",
		"Chef/Deprecations/ChefShellout",
		"Chef/Deprecations/UsesDeprecatedMixins",
		"Chef/Deprecations/ResourceUsesDslNameMethod",
		"Chef/Deprecations/NodeSetWithoutLevel",
		"Chef/Deprecations/PartialSearchClassUsage",
		"Chef/Deprecations/PartialSearchHelperUsage",
		"Chef/Deprecations/EpicFail",
	} {
		got := r.Resolve(cop)
		if got.Classification != ClassificationBlocker || got.Source != SourceVerifiedRemoval {
			t.Errorf("%s: got %s/%s, want blocker/verified_removal", cop, got.Classification, got.Source)
		}
	}
}

func TestFalseNegativeSweep_DeprecatedConstantsSplit(t *testing.T) {
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.3.15"}

	// Removed constants → base Blocker.
	for _, msg := range []string{
		"Use `nil` instead of `NIL`, deprecated since Ruby 2.4.",
		"Do not use `Random::DEFAULT`.",
	} {
		if got := r.ResolveOffense("Lint/DeprecatedConstants", msg); got.Classification != ClassificationBlocker {
			t.Errorf("%q: got %s/%s, want blocker", msg, got.Classification, got.Source)
		}
	}

	// Net::HTTPServerException still present on Ruby 3.4 → Review, not a false-positive Blocker.
	got := r.ResolveOffense("Lint/DeprecatedConstants",
		"Use `Net::HTTPClientException` instead of `Net::HTTPServerException`.")
	if got.Classification != ClassificationReview {
		t.Errorf("Net::HTTPServerException: got %s/%s, want review", got.Classification, got.Source)
	}
}
