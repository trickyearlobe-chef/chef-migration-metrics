// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"
)

func TestVersionLessOrEqual(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"14.0", "18.0", true},
		{"18.0", "14.0", false},
		{"14.0", "14.0", true},
		{"14.0", "14.5", true},
		{"14.5", "14.0", false},
		{"13.0", "18.5.0", true},
		{"19.0", "18.5.0", false},
	}
	for _, tt := range tests {
		got := versionLessOrEqual(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("versionLessOrEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"14.0", 14},
		{"18.5.0", 18},
		{"", 0},
		{"3", 3},
	}
	for _, tt := range tests {
		got := parseMajorVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseMajorVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestResolverOperatorOverrideWinsOverEverything asserts source 1 (operator
// override) beats even a verified-removal Blocker.
func TestResolverOperatorOverrideWinsOverEverything(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{
			// NodeSet has RemovedIn=14.0 (would be a verified_removal Blocker),
			// but the operator's verdict wins.
			"Chef/Deprecations/NodeSet": ClassificationNoise,
		},
		TargetChefVersion: "18.0",
	}

	result := resolver.Resolve("Chef/Deprecations/NodeSet")
	if result.Classification != ClassificationNoise {
		t.Errorf("expected noise (operator override), got %s", result.Classification)
	}
	if result.Source != SourceOperatorOverride {
		t.Errorf("expected source operator_override, got %s", result.Source)
	}
}

// TestResolverCustomCop asserts source 2: any Custom/ cop is a Blocker by intent.
func TestResolverCustomCop(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	result := resolver.Resolve("Custom/Anything")
	if result.Classification != ClassificationBlocker {
		t.Errorf("expected blocker (custom cop), got %s", result.Classification)
	}
	if result.Source != SourceCustomCop {
		t.Errorf("expected source custom_cop, got %s", result.Source)
	}
}

// TestResolverVerifiedRemoval asserts source 3: a curated RemovedIn ≤ target
// resolves to Blocker/verified_removal.
func TestResolverVerifiedRemoval(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// NodeSet RemovedIn=14.0, WindowsFeatureServermanagercmd=15.0,
	// Lint/DeprecatedClassMethods=18.0 — all ≤ target 18.0.
	for _, cop := range []string{
		"Chef/Deprecations/NodeSet",
		"Chef/Deprecations/WindowsFeatureServermanagercmd",
		"Lint/DeprecatedClassMethods",
	} {
		result := resolver.Resolve(cop)
		if result.Classification != ClassificationBlocker {
			t.Errorf("%s: expected blocker (verified_removal), got %s", cop, result.Classification)
		}
		if result.Source != SourceVerifiedRemoval {
			t.Errorf("%s: expected source verified_removal, got %s", cop, result.Source)
		}
	}
}

// TestResolverRemovedInGreaterThanTargetFallsThrough asserts a RemovedIn
// GREATER than the target must NOT produce a Blocker — it falls through to the
// honest Review default.
func TestResolverRemovedInGreaterThanTargetFallsThrough(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "14.0",
	}

	// Lint/DeprecatedClassMethods RemovedIn=18.0 > target 14.0. Not custom, not
	// structural noise → Review default, never a verified_removal Blocker.
	result := resolver.Resolve("Lint/DeprecatedClassMethods")
	if result.Classification == ClassificationBlocker {
		t.Errorf("RemovedIn 18.0 > target 14.0 must not blocker, got %s/%s", result.Classification, result.Source)
	}
	if result.Classification != ClassificationReview || result.Source != SourceReviewDefault {
		t.Errorf("expected review/review_default, got %s/%s", result.Classification, result.Source)
	}
}

// TestResolverStructuralNoise asserts source 4: cosmetic departments and
// test/CI-tooling markers resolve to Noise/structural_noise.
func TestResolverStructuralNoise(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	noise := []string{
		"Style/TrailingWhitespace",
		"Layout/IndentationWidth",
		"Chef/Style/CommentFormat",
		"Chef/Deprecations/ChefSpecLegacyRunner", // contains "ChefSpec"
		"Chef/Deprecations/FoodcriticComments",   // contains "Foodcritic"
		"Chef/Deprecations/DeliveryConfig",       // contains "Delivery"
		"Chef/Deprecations/LibrarianInclude",     // contains "Librarian"
		"Chef/Deprecations/BerksConfig",          // contains "Berks"
	}
	for _, cop := range noise {
		result := resolver.Resolve(cop)
		if result.Classification != ClassificationNoise {
			t.Errorf("%s: expected noise (structural), got %s", cop, result.Classification)
		}
		if result.Source != SourceStructuralNoise {
			t.Errorf("%s: expected source structural_noise, got %s", cop, result.Source)
		}
	}

	// Noise contributes 0 complexity weight (Classify drives the scorer).
	if got := resolver.Classify("Style/TrailingWhitespace"); got != ClassificationNoise {
		t.Errorf("Classify(Style/TrailingWhitespace) = %s, want noise", got)
	}
}

// TestResolverReviewDefault asserts source 5: everything unproven — an unknown
// Chef cop, a generic Lint cop, and formerly-"curated" Chef deprecation/
// correctness cops with NO RemovedIn — resolves to Review/review_default.
func TestResolverReviewDefault(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	review := []string{
		"Lint/SomeUnknownCop",                      // generic Lint, no mapping
		"Chef/Deprecations/SomeBrandNewCop",        // unknown Chef deprecation cop
		"Chef/Correctness/SomeBrandNewCop",         // unknown Chef correctness cop
		"Chef/Correctness/NodeNormal",              // mapped but RemovedIn=""
		"Chef/Deprecations/HWRPWithoutUnifiedTrue", // mapped but RemovedIn=""
	}
	for _, cop := range review {
		result := resolver.Resolve(cop)
		if result.Classification != ClassificationReview {
			t.Errorf("%s: expected review, got %s", cop, result.Classification)
		}
		if result.Source != SourceReviewDefault {
			t.Errorf("%s: expected source review_default, got %s", cop, result.Source)
		}
	}
}

// TestResolverOperatorOverrideBeatsStructuralNoise asserts the override (source
// 1) beats a would-be structural Noise verdict (source 4).
func TestResolverOperatorOverrideBeatsStructuralNoise(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{
			"Style/TrailingWhitespace": ClassificationBlocker,
		},
		TargetChefVersion: "18.0",
	}

	result := resolver.Resolve("Style/TrailingWhitespace")
	if result.Classification != ClassificationBlocker {
		t.Errorf("expected blocker (operator override), got %s", result.Classification)
	}
	if result.Source != SourceOperatorOverride {
		t.Errorf("expected source operator_override, got %s", result.Source)
	}
}

func TestEvaluatePassFailWithClassification(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	t.Run("blocker cop fails", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
		}
		if EvaluatePassFailWithClassification(offenses, resolver) {
			t.Error("expected fail: NodeSet is a blocker (removed in 14.0, target 18.0)")
		}
	})

	t.Run("noise cop passes", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Chef/Deprecations/ChefSpecLegacyRunner", Severity: "warning"},
		}
		if !EvaluatePassFailWithClassification(offenses, resolver) {
			t.Error("expected pass: ChefSpec cop is noise")
		}
	})

	t.Run("review cop passes", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Lint/SomeUnknownCop", Severity: "warning"},
		}
		if !EvaluatePassFailWithClassification(offenses, resolver) {
			t.Error("expected pass: unknown cop resolves to Review, which never blocks")
		}
	})

	t.Run("severity is inert — fatal alone never fails", func(t *testing.T) {
		// Under the trustworthy-reds model severity never produces Blocked. A
		// fatal-severity cop that classifies as Review must still pass.
		offenses := []CookstyleOffense{
			{CopName: "Lint/Syntax", Severity: "fatal"},
		}
		if !EvaluatePassFailWithClassification(offenses, resolver) {
			t.Error("expected pass: severity is inert, Lint/Syntax resolves to Review not Blocker")
		}
	})

	t.Run("review cop passes even at error severity", func(t *testing.T) {
		resolver := &CopClassificationResolver{
			OperatorOverrides: map[string]string{
				"SomeCop": ClassificationReview,
			},
			TargetChefVersion: "18.0",
		}
		offenses := []CookstyleOffense{
			{CopName: "SomeCop", Severity: "error"},
		}
		if !EvaluatePassFailWithClassification(offenses, resolver) {
			t.Error("expected pass: cop classified as review should not fail regardless of severity")
		}
	})
}

func TestIsBlocker(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	if !resolver.IsBlocker("Chef/Deprecations/NodeSet") {
		t.Error("NodeSet should be a blocker for target 18.0")
	}
	if resolver.IsBlocker("Chef/Deprecations/ChefSpecLegacyRunner") {
		t.Error("ChefSpecLegacyRunner should not be a blocker")
	}
}
