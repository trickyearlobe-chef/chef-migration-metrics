// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "testing"

// resolverFor builds a resolver with the given operator overrides at target 18.0
// (curated defaults and RemovedIn data apply at this target).
func resolverAt(target string, overrides map[string]string) *CopClassificationResolver {
	if overrides == nil {
		overrides = map[string]string{}
	}
	return &CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: target}
}

func TestDeriveCookstyleStatus_TruthTable(t *testing.T) {
	rules := DefaultFailureRules()

	tests := []struct {
		name      string
		offenses  []CookstyleOffense
		overrides map[string]string
		want      string
	}{
		{
			name:     "no offenses is ready",
			offenses: nil,
			want:     StatusReady,
		},
		{
			name: "noise only is ready",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/ChefSpecLegacyRunner", Severity: "warning"},
			},
			want: StatusReady,
		},
		{
			name: "unclassified at warning under default rules is ready",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Style/SomeUnknownCop", Severity: "warning"},
			},
			want: StatusReady,
		},
		{
			name: "review only needs review",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Severity: "warning"},
			},
			want: StatusNeedsReview,
		},
		{
			name: "review plus noise needs review",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/ChefSpecLegacyRunner", Severity: "warning"},
				{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Severity: "warning"},
			},
			want: StatusNeedsReview,
		},
		{
			name: "blocker is blocked",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
			},
			want: StatusBlocked,
		},
		{
			name: "blocker dominates review",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Severity: "warning"},
				{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
			},
			want: StatusBlocked,
		},
		{
			// Severity is inert: a fatal-severity cop that classifies as Review
			// stays Needs review — it never severity-fails to Blocked.
			name: "fatal severity is inert — review cop stays needs review",
			offenses: []CookstyleOffense{
				{CopName: "Lint/Syntax", Severity: "fatal"},
			},
			want: StatusNeedsReview,
		},
		{
			// Two Review cops, one at fatal severity: still Needs review, because
			// severity never elevates to Blocked and neither cop is a Blocker.
			name: "fatal severity does not dominate — all review stays needs review",
			offenses: []CookstyleOffense{
				{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Severity: "warning"},
				{CopName: "Lint/Syntax", Severity: "fatal"},
			},
			want: StatusNeedsReview,
		},
		{
			name: "operator review override beats severity error",
			offenses: []CookstyleOffense{
				{CopName: "SomeCop", Severity: "error"},
			},
			overrides: map[string]string{"SomeCop": ClassificationReview},
			want:      StatusNeedsReview,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := resolverAt("18.0", tt.overrides)
			got := DeriveCookstyleStatus(tt.offenses, rules, resolver)
			if got != tt.want {
				t.Errorf("DeriveCookstyleStatus = %q, want %q", got, tt.want)
			}
			// passed is the derived convenience: passed = status != Blocked.
			passed := EvaluatePassFailWithClassification(tt.offenses, rules, resolver)
			wantPassed := tt.want != StatusBlocked
			if passed != wantPassed {
				t.Errorf("passed = %v, want %v (status %q)", passed, wantPassed, tt.want)
			}
		})
	}
}

// TestDeriveCookstyleStatus_KubernetesClusterCase reproduces the root-problem
// case from plans/cookstyle-status-consistency.md: a repo whose only offense is
// the Review-level Chef/Deprecations/ResourceWithoutUnifiedTrue cop at target
// 19.3.15 must roll up to Needs review (not Blocked / "failing"), and passed
// must be true.
func TestDeriveCookstyleStatus_KubernetesClusterCase(t *testing.T) {
	rules := DefaultFailureRules()
	resolver := resolverAt("19.3.15", nil)
	offenses := []CookstyleOffense{
		{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Severity: "refactor"},
	}

	if got := DeriveCookstyleStatus(offenses, rules, resolver); got != StatusNeedsReview {
		t.Fatalf("kubernetes-cluster case: status = %q, want %q", got, StatusNeedsReview)
	}
	if !EvaluatePassFailWithClassification(offenses, rules, resolver) {
		t.Error("kubernetes-cluster case: expected passed=true for a review-only repo")
	}
}
