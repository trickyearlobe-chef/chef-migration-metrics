// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import "testing"

// mapClassifier is a test CopClassifier backed by a static map; unknown cops
// resolve to unclassified.
type mapClassifier map[string]string

func (m mapClassifier) Classify(cop string) string {
	if c, ok := m[cop]; ok {
		return c
	}
	return classUnclassified
}

func TestComputeCookstyleComplexity_ByClassification(t *testing.T) {
	tests := []struct {
		name     string
		offenses []ClassifiedOffense
		want     int
	}{
		{name: "empty is zero", offenses: nil, want: 0},
		{
			name:     "single blocker dominates",
			offenses: []ClassifiedOffense{{CopName: "A", Classification: classBlocker}},
			want:     WeightBlocker,
		},
		{
			name:     "review is low weight",
			offenses: []ClassifiedOffense{{CopName: "B", Classification: classReview}},
			want:     WeightReview,
		},
		{
			name:     "noise contributes zero",
			offenses: []ClassifiedOffense{{CopName: "C", Classification: classNoise, Severity: "error"}},
			want:     0,
		},
		{
			name: "mixed levels sum once each",
			offenses: []ClassifiedOffense{
				{CopName: "A", Classification: classBlocker},
				{CopName: "B", Classification: classReview},
				{CopName: "C", Classification: classNoise},
			},
			want: WeightBlocker + WeightReview,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeCookstyleComplexity(tt.offenses); got != tt.want {
				t.Errorf("ComputeCookstyleComplexity = %d, want %d", got, tt.want)
			}
		})
	}
}

// Unclassified offenses use the single highest applicable legacy category
// weight — never the sum of overlapping categories (no double-count).
func TestComputeCookstyleComplexity_UnclassifiedNoDoubleCount(t *testing.T) {
	tests := []struct {
		name    string
		offense ClassifiedOffense
		want    int
	}{
		{
			name:    "deprecation warning weighted as deprecation",
			offense: ClassifiedOffense{CopName: "Chef/Deprecations/Foo", Severity: "warning", Classification: classUnclassified},
			want:    WeightDeprecation,
		},
		{
			name:    "deprecation at error takes the higher error weight, once",
			offense: ClassifiedOffense{CopName: "Chef/Deprecations/Foo", Severity: "error", Classification: classUnclassified},
			want:    WeightErrorFatal, // max(error/fatal=5, deprecation=3), not 5+3
		},
		{
			name:    "correctness warning weighted as correctness",
			offense: ClassifiedOffense{CopName: "Chef/Correctness/Bar", Severity: "warning", Classification: classUnclassified},
			want:    WeightCorrectness,
		},
		{
			name:    "modernize warning weighted as modernize",
			offense: ClassifiedOffense{CopName: "Chef/Modernize/Baz", Severity: "warning", Classification: classUnclassified},
			want:    WeightModernize,
		},
		{
			name:    "uncategorised warning contributes zero",
			offense: ClassifiedOffense{CopName: "Chef/Style/Qux", Severity: "warning", Classification: classUnclassified},
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeCookstyleComplexity([]ClassifiedOffense{tt.offense}); got != tt.want {
				t.Errorf("weight = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestComputeCookstyleComplexity_CosmeticStyleWeightZero proves Chunk A's
// safety property: dropping --only floods the scan with the generic cosmetic
// tail (Style/, Layout/), but an unclassified Style cop at convention severity
// contributes 0 — widening the ruleset does not inflate complexity scores.
func TestComputeCookstyleComplexity_CosmeticStyleWeightZero(t *testing.T) {
	offenses := []ClassifiedOffense{
		{CopName: "Style/StringLiterals", Severity: "convention", Classification: classUnclassified},
		{CopName: "Layout/TrailingWhitespace", Severity: "convention", Classification: classUnclassified},
	}
	if got := ComputeCookstyleComplexity(offenses); got != 0 {
		t.Errorf("cosmetic Style/Layout cops at convention should weigh 0, got %d", got)
	}
}

func TestClassifyOffensesForComplexity(t *testing.T) {
	classifier := mapClassifier{
		"Chef/Deprecations/NodeSet":                    classBlocker,
		"Chef/Deprecations/ResourceWithoutUnifiedTrue": classReview,
	}
	jsonb := []byte(`[
		{"cop_name":"Chef/Deprecations/NodeSet","severity":"warning"},
		{"cop_name":"Chef/Deprecations/ResourceWithoutUnifiedTrue","severity":"refactor"},
		{"cop_name":"Chef/Style/Unknown","severity":"warning"}
	]`)

	got := classifyOffensesForComplexity(jsonb, classifier)
	if len(got) != 3 {
		t.Fatalf("expected 3 classified offenses, got %d", len(got))
	}
	if got[0].Classification != classBlocker || got[1].Classification != classReview || got[2].Classification != classUnclassified {
		t.Errorf("unexpected classifications: %+v", got)
	}

	// A review-only repo scores low (acceptance criterion).
	reviewOnly := classifyOffensesForComplexity([]byte(`[{"cop_name":"Chef/Deprecations/ResourceWithoutUnifiedTrue","severity":"refactor"}]`), classifier)
	score := ComputeCookstyleComplexity(reviewOnly)
	if ScoreToLabel(score) != LabelLow {
		t.Errorf("review-only repo: score %d label %q, want low", score, ScoreToLabel(score))
	}

	// Nil classifier or empty JSON yields no offenses.
	if got := classifyOffensesForComplexity(jsonb, nil); got != nil {
		t.Errorf("nil classifier should yield nil, got %v", got)
	}
	if got := classifyOffensesForComplexity(nil, classifier); got != nil {
		t.Errorf("empty json should yield nil, got %v", got)
	}
}

func TestTKWeight(t *testing.T) {
	cases := map[string]int{
		"failed":  WeightTKFail,
		"partial": WeightTKPartial,
		"passed":  0,
		"":        0,
	}
	for status, want := range cases {
		if got := tkWeight(status); got != want {
			t.Errorf("tkWeight(%q) = %d, want %d", status, got, want)
		}
	}
}
