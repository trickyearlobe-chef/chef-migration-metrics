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

func TestResolverOperatorOverrideTakesPriority(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{
			"Chef/Deprecations/NodeSet": ClassificationNoise,
		},
		TargetChefVersion: "18.0",
	}

	// NodeSet has RemovedIn=14.0 in the cop mapping, but operator override wins.
	result := resolver.Resolve("Chef/Deprecations/NodeSet")
	if result.Classification != ClassificationNoise {
		t.Errorf("expected noise (operator override), got %s", result.Classification)
	}
	if result.Source != SourceOperatorOverride {
		t.Errorf("expected source operator_override, got %s", result.Source)
	}
}

func TestResolverRemovedInAutoSeed(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// NodeSet has RemovedIn=14.0 — should be blocker for target 18.0.
	result := resolver.Resolve("Chef/Deprecations/NodeSet")
	if result.Classification != ClassificationBlocker {
		t.Errorf("expected blocker (removed_in), got %s", result.Classification)
	}
	if result.Source != SourceRemovedIn {
		t.Errorf("expected source removed_in, got %s", result.Source)
	}
}

func TestResolverRemovedInNotApplicable(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "12.0",
	}

	// NodeSet has RemovedIn=14.0 — should NOT be a blocker for target 12.0.
	result := resolver.Resolve("Chef/Deprecations/NodeSet")
	// It won't be blocker via removed_in, but may hit curated defaults or unclassified.
	if result.Classification == ClassificationBlocker && result.Source == SourceRemovedIn {
		t.Errorf("should not be blocker via removed_in for target 12.0")
	}
}

func TestResolverCuratedDefaults(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// Lint/DeprecatedClassMethods is a curated blocker for target >= 18.0
	result := resolver.Resolve("Lint/DeprecatedClassMethods")
	if result.Classification != ClassificationBlocker {
		t.Errorf("expected blocker (curated_default), got %s", result.Classification)
	}
	if result.Source != SourceCuratedDefault {
		t.Errorf("expected source curated_default, got %s", result.Source)
	}
}

func TestResolverCuratedDefaultVersionGating(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "14.0",
	}

	// Lint/DeprecatedClassMethods has MinTargetVersion=18.0 — should not apply for target 14.0
	result := resolver.Resolve("Lint/DeprecatedClassMethods")
	if result.Classification == ClassificationBlocker && result.Source == SourceCuratedDefault {
		t.Errorf("curated default should not apply for target 14.0 (min is 18.0)")
	}
}

func TestResolverCuratedNoise(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// ChefSpec cops are noise regardless of version.
	result := resolver.Resolve("Chef/Deprecations/ChefSpecLegacyRunner")
	if result.Classification != ClassificationNoise {
		t.Errorf("expected noise for ChefSpec cop, got %s", result.Classification)
	}
	if result.Source != SourceCuratedDefault {
		t.Errorf("expected source curated_default, got %s", result.Source)
	}
}

func TestResolverUnclassifiedFallback(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// Unknown cop outside any curated prefix — should be unclassified.
	result := resolver.Resolve("Lint/SomeUnknownCop")
	if result.Classification != ClassificationUnclassified {
		t.Errorf("expected unclassified, got %s", result.Classification)
	}
	if result.Source != SourceUnclassified {
		t.Errorf("expected source unclassified, got %s", result.Source)
	}
}

func TestResolverCuratedPrefixDefaults(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// Cosmetic departments resolve to Noise via department prefix, even though
	// no exact-name curated default exists for these cops.
	cosmetic := []string{
		"Style/TrailingWhitespace",
		"Layout/IndentationWidth",
		"Chef/Style/CommentFormat",
	}
	for _, cop := range cosmetic {
		result := resolver.Resolve(cop)
		if result.Classification != ClassificationNoise {
			t.Errorf("%s: expected noise (curated prefix), got %s", cop, result.Classification)
		}
		if result.Source != SourceCuratedDefault {
			t.Errorf("%s: expected source curated_default, got %s", cop, result.Source)
		}
	}

	// Noise contributes 0 complexity weight (Classify drives the scorer).
	if got := resolver.Classify("Style/TrailingWhitespace"); got != ClassificationNoise {
		t.Errorf("Classify(Style/TrailingWhitespace) = %s, want noise", got)
	}
}

func TestResolverOperatorOverrideBeatsPrefixDefault(t *testing.T) {
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{
			"Style/TrailingWhitespace": ClassificationBlocker,
		},
		TargetChefVersion: "18.0",
	}

	// Operator override wins over the Noise department prefix.
	result := resolver.Resolve("Style/TrailingWhitespace")
	if result.Classification != ClassificationBlocker {
		t.Errorf("expected blocker (operator override), got %s", result.Classification)
	}
	if result.Source != SourceOperatorOverride {
		t.Errorf("expected source operator_override, got %s", result.Source)
	}
}

func TestEvaluatePassFailWithClassification(t *testing.T) {
	rules := DefaultFailureRules()
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	t.Run("blocker cop fails", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
		}
		if EvaluatePassFailWithClassification(offenses, rules, resolver) {
			t.Error("expected fail: NodeSet is a blocker (removed in 14.0, target 18.0)")
		}
	})

	t.Run("noise cop passes", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Chef/Deprecations/ChefSpecLegacyRunner", Severity: "warning"},
		}
		if !EvaluatePassFailWithClassification(offenses, rules, resolver) {
			t.Error("expected pass: ChefSpec cop is noise")
		}
	})

	t.Run("unclassified cop at warning passes under default rules", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Lint/SomeUnknownCop", Severity: "warning"},
		}
		if !EvaluatePassFailWithClassification(offenses, rules, resolver) {
			t.Error("expected pass: unknown cop at warning, default rules only fail on error/fatal")
		}
	})

	t.Run("unclassified cop at fatal fails under default rules", func(t *testing.T) {
		offenses := []CookstyleOffense{
			{CopName: "Lint/Syntax", Severity: "fatal"},
		}
		if EvaluatePassFailWithClassification(offenses, rules, resolver) {
			t.Error("expected fail: fatal severity triggers default rules")
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
		if !EvaluatePassFailWithClassification(offenses, rules, resolver) {
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
