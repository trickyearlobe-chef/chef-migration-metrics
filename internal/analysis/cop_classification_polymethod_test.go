// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "testing"

// Message-aware classification for poly-method cops. See
// specifications/cop-classification.md (Poly-method cops).

func TestResolveOffense_PolyCop_RemovedVariantIsBlocker(t *testing.T) {
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.0"}
	got := r.ResolveOffense("Lint/DeprecatedClassMethods", "`File.exists?` is deprecated in favor of `File.exist?`.")
	if got.Classification != ClassificationBlocker || got.Source != SourceVerifiedRemoval {
		t.Errorf("File.exists? variant: got %s/%s, want blocker/verified_removal", got.Classification, got.Source)
	}
}

func TestResolveOffense_PolyCop_DeprecationOnlyVariantIsReview(t *testing.T) {
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.0"}
	got := r.ResolveOffense("Lint/DeprecatedClassMethods", "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`.")
	if got.Classification != ClassificationReview || got.Source != SourceReviewDefault {
		t.Errorf("Socket.gethostbyname variant: got %s/%s, want review/review_default", got.Classification, got.Source)
	}
}

func TestResolveOffense_EmptyMessageMatchesResolve(t *testing.T) {
	// The message-free path must be identical to the legacy cop-name Resolve —
	// this is the fingerprint-recompute / non-poly behaviour that must not change.
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.0"}
	for _, cop := range []string{"Lint/DeprecatedClassMethods", "Chef/Deprecations/NodeSet", "Style/StringLiterals"} {
		off := r.ResolveOffense(cop, "")
		name := r.Resolve(cop)
		if off != name {
			t.Errorf("%s: ResolveOffense(cop,\"\") = %+v, Resolve(cop) = %+v", cop, off, name)
		}
	}
}

func TestResolveOffense_OperatorOverrideWinsRegardlessOfMessage(t *testing.T) {
	// Operator overrides are keyed by cop_name (single UNIQUE row) → they apply to
	// every variant of a poly cop.
	r := &CopClassificationResolver{
		OperatorOverrides: map[string]string{"Lint/DeprecatedClassMethods": ClassificationNoise},
		TargetChefVersion: "19.0",
	}
	got := r.ResolveOffense("Lint/DeprecatedClassMethods", "`File.exists?` is deprecated in favor of `File.exist?`.")
	if got.Classification != ClassificationNoise || got.Source != SourceOperatorOverride {
		t.Errorf("override should win: got %s/%s", got.Classification, got.Source)
	}
}

func TestClassifyOffense_DelegatesToResolveOffense(t *testing.T) {
	r := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.0"}
	if got := r.ClassifyOffense("Lint/DeprecatedClassMethods", "Socket.gethostbyname is deprecated"); got != ClassificationReview {
		t.Errorf("Socket variant classify = %q, want review", got)
	}
	if got := r.ClassifyOffense("Lint/DeprecatedClassMethods", "File.exists? is deprecated"); got != ClassificationBlocker {
		t.Errorf("File.exists? variant classify = %q, want blocker", got)
	}
}
