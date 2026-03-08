// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ScoreToLabel
// ---------------------------------------------------------------------------

func TestScoreToLabel_None(t *testing.T) {
	if got := ScoreToLabel(0); got != LabelNone {
		t.Errorf("ScoreToLabel(0) = %q, want %q", got, LabelNone)
	}
}

func TestScoreToLabel_NegativeIsNone(t *testing.T) {
	if got := ScoreToLabel(-5); got != LabelNone {
		t.Errorf("ScoreToLabel(-5) = %q, want %q", got, LabelNone)
	}
}

func TestScoreToLabel_Low(t *testing.T) {
	for _, score := range []int{1, 5, 10} {
		if got := ScoreToLabel(score); got != LabelLow {
			t.Errorf("ScoreToLabel(%d) = %q, want %q", score, got, LabelLow)
		}
	}
}

func TestScoreToLabel_Medium(t *testing.T) {
	for _, score := range []int{11, 20, 30} {
		if got := ScoreToLabel(score); got != LabelMedium {
			t.Errorf("ScoreToLabel(%d) = %q, want %q", score, got, LabelMedium)
		}
	}
}

func TestScoreToLabel_High(t *testing.T) {
	for _, score := range []int{31, 45, 60} {
		if got := ScoreToLabel(score); got != LabelHigh {
			t.Errorf("ScoreToLabel(%d) = %q, want %q", score, got, LabelHigh)
		}
	}
}

func TestScoreToLabel_Critical(t *testing.T) {
	for _, score := range []int{61, 100, 999} {
		if got := ScoreToLabel(score); got != LabelCritical {
			t.Errorf("ScoreToLabel(%d) = %q, want %q", score, got, LabelCritical)
		}
	}
}

func TestScoreToLabel_BoundaryValues(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{0, LabelNone},
		{1, LabelLow},
		{10, LabelLow},
		{11, LabelMedium},
		{30, LabelMedium},
		{31, LabelHigh},
		{60, LabelHigh},
		{61, LabelCritical},
	}
	for _, tt := range tests {
		if got := ScoreToLabel(tt.score); got != tt.want {
			t.Errorf("ScoreToLabel(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — zero input
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_ZeroInput(t *testing.T) {
	input := ComplexityInput{}
	if got := ComputeComplexityScore(input); got != 0 {
		t.Errorf("ComputeComplexityScore(zero) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — individual weight factors
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_ErrorFatalOnly(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{ErrorFatalCount: 3},
	}
	want := 3 * WeightErrorFatal
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("ErrorFatal score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_DeprecationOnly(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{DeprecationCount: 4},
	}
	want := 4 * WeightDeprecation
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("Deprecation score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_CorrectnessOnly(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{CorrectnessCount: 2},
	}
	want := 2 * WeightCorrectness
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("Correctness score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_ManualFixOnly(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{ManualFixCount: 5},
	}
	want := 5 * WeightNonAutoCorrectable
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("ManualFix score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_ModernizeOnly(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{ModernizeCount: 10},
	}
	want := 10 * WeightModernize
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("Modernize score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_TKConvergeFailOnly(t *testing.T) {
	input := ComplexityInput{
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: false,
			TestsPassed:    false,
		},
	}
	want := WeightTKConvergeFail
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("TK converge fail score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_TKTestFailOnly(t *testing.T) {
	input := ComplexityInput{
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: true,
			TestsPassed:    false,
		},
	}
	want := WeightTKTestFail
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("TK test fail score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_TKBothPassNoContribution(t *testing.T) {
	input := ComplexityInput{
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: true,
			TestsPassed:    true,
		},
	}
	if got := ComputeComplexityScore(input); got != 0 {
		t.Errorf("TK both pass score = %d, want 0", got)
	}
}

func TestComputeComplexityScore_TKNoResultNoContribution(t *testing.T) {
	input := ComplexityInput{
		TestKitchen: TestKitchenSummary{
			HasResult:      false,
			ConvergePassed: false,
			TestsPassed:    false,
		},
	}
	if got := ComputeComplexityScore(input); got != 0 {
		t.Errorf("TK no result score = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — combined inputs
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_AllFactorsCombined(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{
			ErrorFatalCount:  2,
			DeprecationCount: 3,
			CorrectnessCount: 1,
			ModernizeCount:   4,
			ManualFixCount:   2,
		},
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: false,
		},
	}

	want := 2*WeightErrorFatal +
		3*WeightDeprecation +
		1*WeightCorrectness +
		4*WeightModernize +
		2*WeightNonAutoCorrectable +
		WeightTKConvergeFail

	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("combined score = %d, want %d", got, want)
	}
}

func TestComputeComplexityScore_CookstyleAndTKTestFail(t *testing.T) {
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{
			DeprecationCount: 5,
		},
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: true,
			TestsPassed:    false,
		},
	}

	want := 5*WeightDeprecation + WeightTKTestFail
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("cookstyle + TK test fail score = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — label integration
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_LabelNone(t *testing.T) {
	input := ComplexityInput{} // All zeros.
	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)
	if label != LabelNone {
		t.Errorf("label = %q, want %q for score %d", label, LabelNone, score)
	}
}

func TestComputeComplexityScore_LabelLow(t *testing.T) {
	// 2 deprecations = 6 → low (1-10)
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{DeprecationCount: 2},
	}
	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)
	if label != LabelLow {
		t.Errorf("label = %q, want %q for score %d", label, LabelLow, score)
	}
}

func TestComputeComplexityScore_LabelMedium(t *testing.T) {
	// 5 deprecations = 15 → medium (11-30)
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{DeprecationCount: 5},
	}
	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)
	if label != LabelMedium {
		t.Errorf("label = %q, want %q for score %d", label, LabelMedium, score)
	}
}

func TestComputeComplexityScore_LabelHigh(t *testing.T) {
	// 10 deprecations = 30 → medium, plus 1 error = 35 → high
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{
			DeprecationCount: 10,
			ErrorFatalCount:  1,
		},
	}
	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)
	if label != LabelHigh {
		t.Errorf("label = %q, want %q for score %d", label, LabelHigh, score)
	}
}

func TestComputeComplexityScore_LabelCritical(t *testing.T) {
	// TK converge fail (20) + 5 errors (25) + 5 deprecations (15) = 60 → high
	// Add 1 more to get 61 → critical
	input := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{
			ErrorFatalCount:  5,
			DeprecationCount: 5,
			ModernizeCount:   2,
		},
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: false,
		},
	}
	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)
	// 5*5 + 5*3 + 2*1 + 20 = 25 + 15 + 2 + 20 = 62 → critical
	if label != LabelCritical {
		t.Errorf("label = %q, want %q for score %d", label, LabelCritical, score)
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — blast radius is NOT included in score
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_BlastRadiusDoesNotAffectScore(t *testing.T) {
	base := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{DeprecationCount: 3},
	}
	withBlast := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{DeprecationCount: 3},
		Blast: BlastRadius{
			AffectedNodeCount:   1000,
			AffectedRoleCount:   50,
			AffectedPolicyCount: 20,
		},
	}

	baseScore := ComputeComplexityScore(base)
	blastScore := ComputeComplexityScore(withBlast)

	if baseScore != blastScore {
		t.Errorf("blast radius affected score: base=%d, withBlast=%d", baseScore, blastScore)
	}
}

// ---------------------------------------------------------------------------
// ComputeComplexityScore — auto-correctable does NOT contribute to score
// (only ManualFixCount contributes)
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_AutoCorrectableNotScored(t *testing.T) {
	withAuto := ComplexityInput{
		Cookstyle: CookstyleOffenseSummary{
			AutoCorrectableCount: 100,
		},
	}
	if got := ComputeComplexityScore(withAuto); got != 0 {
		t.Errorf("auto-correctable should not contribute to score, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Weight constants sanity
// ---------------------------------------------------------------------------

func TestWeightConstants(t *testing.T) {
	if WeightErrorFatal != 5 {
		t.Errorf("WeightErrorFatal = %d, want 5", WeightErrorFatal)
	}
	if WeightDeprecation != 3 {
		t.Errorf("WeightDeprecation = %d, want 3", WeightDeprecation)
	}
	if WeightCorrectness != 3 {
		t.Errorf("WeightCorrectness = %d, want 3", WeightCorrectness)
	}
	if WeightNonAutoCorrectable != 4 {
		t.Errorf("WeightNonAutoCorrectable = %d, want 4", WeightNonAutoCorrectable)
	}
	if WeightModernize != 1 {
		t.Errorf("WeightModernize = %d, want 1", WeightModernize)
	}
	if WeightTKConvergeFail != 20 {
		t.Errorf("WeightTKConvergeFail = %d, want 20", WeightTKConvergeFail)
	}
	if WeightTKTestFail != 10 {
		t.Errorf("WeightTKTestFail = %d, want 10", WeightTKTestFail)
	}
}

// ---------------------------------------------------------------------------
// Label constants sanity
// ---------------------------------------------------------------------------

func TestLabelConstants(t *testing.T) {
	if LabelNone != "none" {
		t.Errorf("LabelNone = %q, want %q", LabelNone, "none")
	}
	if LabelLow != "low" {
		t.Errorf("LabelLow = %q, want %q", LabelLow, "low")
	}
	if LabelMedium != "medium" {
		t.Errorf("LabelMedium = %q, want %q", LabelMedium, "medium")
	}
	if LabelHigh != "high" {
		t.Errorf("LabelHigh = %q, want %q", LabelHigh, "high")
	}
	if LabelCritical != "critical" {
		t.Errorf("LabelCritical = %q, want %q", LabelCritical, "critical")
	}
}

// ---------------------------------------------------------------------------
// Namespace classification helpers
// ---------------------------------------------------------------------------

func TestIsDeprecation(t *testing.T) {
	tests := []struct {
		copName string
		want    bool
	}{
		{"ChefDeprecations/NodeSet", true},
		{"ChefDeprecations/ResourceWithoutUnifiedTrue", true},
		{"ChefCorrectness/BlockGuardWithOnlyString", false},
		{"ChefModernize/CronDFileOrTemplate", false},
		{"ChefStyle/TrueClassFalseClassResourceProperties", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isDeprecation(tt.copName); got != tt.want {
			t.Errorf("isDeprecation(%q) = %v, want %v", tt.copName, got, tt.want)
		}
	}
}

func TestIsCorrectness(t *testing.T) {
	tests := []struct {
		copName string
		want    bool
	}{
		{"ChefCorrectness/BlockGuardWithOnlyString", true},
		{"ChefCorrectness/CookbookUsesNodeSave", true},
		{"ChefDeprecations/NodeSet", false},
		{"ChefModernize/CronDFileOrTemplate", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isCorrectness(tt.copName); got != tt.want {
			t.Errorf("isCorrectness(%q) = %v, want %v", tt.copName, got, tt.want)
		}
	}
}

func TestIsModernize(t *testing.T) {
	tests := []struct {
		copName string
		want    bool
	}{
		{"ChefModernize/CronDFileOrTemplate", true},
		{"ChefModernize/UnnecessaryMixlibShelloutRequire", true},
		{"ChefDeprecations/NodeSet", false},
		{"ChefCorrectness/BlockGuardWithOnlyString", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isModernize(tt.copName); got != tt.want {
			t.Errorf("isModernize(%q) = %v, want %v", tt.copName, got, tt.want)
		}
	}
}

func TestIsErrorOrFatal(t *testing.T) {
	tests := []struct {
		severity string
		want     bool
	}{
		{"error", true},
		{"fatal", true},
		{"warning", false},
		{"convention", false},
		{"refactor", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isErrorOrFatal(tt.severity); got != tt.want {
			t.Errorf("isErrorOrFatal(%q) = %v, want %v", tt.severity, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// countJSONBStringArray
// ---------------------------------------------------------------------------

func TestCountJSONBStringArray_ValidArray(t *testing.T) {
	data := []byte(`["prod","staging","dev"]`)
	if got := countJSONBStringArray(data); got != 3 {
		t.Errorf("countJSONBStringArray = %d, want 3", got)
	}
}

func TestCountJSONBStringArray_EmptyArray(t *testing.T) {
	data := []byte(`[]`)
	if got := countJSONBStringArray(data); got != 0 {
		t.Errorf("countJSONBStringArray = %d, want 0", got)
	}
}

func TestCountJSONBStringArray_NilData(t *testing.T) {
	if got := countJSONBStringArray(nil); got != 0 {
		t.Errorf("countJSONBStringArray(nil) = %d, want 0", got)
	}
}

func TestCountJSONBStringArray_EmptyData(t *testing.T) {
	if got := countJSONBStringArray([]byte{}); got != 0 {
		t.Errorf("countJSONBStringArray(empty) = %d, want 0", got)
	}
}

func TestCountJSONBStringArray_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	if got := countJSONBStringArray(data); got != 0 {
		t.Errorf("countJSONBStringArray(invalid) = %d, want 0", got)
	}
}

func TestCountJSONBStringArray_ObjectNotArray(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	if got := countJSONBStringArray(data); got != 0 {
		t.Errorf("countJSONBStringArray(object) = %d, want 0", got)
	}
}

func TestCountJSONBStringArray_SingleElement(t *testing.T) {
	data := []byte(`["only_one"]`)
	if got := countJSONBStringArray(data); got != 1 {
		t.Errorf("countJSONBStringArray = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// ComplexityResult type sanity
// ---------------------------------------------------------------------------

func TestComplexityResult_Fields(t *testing.T) {
	r := ComplexityResult{
		CookbookID:           "cb-1",
		CookbookName:         "apache2",
		CookbookVersion:      "5.2.1",
		TargetChefVersion:    "18.0",
		ComplexityScore:      35,
		ComplexityLabel:      LabelHigh,
		ErrorCount:           2,
		DeprecationCount:     5,
		CorrectnessCount:     1,
		ModernizeCount:       3,
		AutoCorrectableCount: 6,
		ManualFixCount:       2,
		AffectedNodeCount:    100,
		AffectedRoleCount:    5,
		AffectedPolicyCount:  3,
	}

	if r.CookbookName != "apache2" {
		t.Errorf("CookbookName = %q", r.CookbookName)
	}
	if r.ComplexityLabel != LabelHigh {
		t.Errorf("ComplexityLabel = %q", r.ComplexityLabel)
	}
	if r.AffectedNodeCount != 100 {
		t.Errorf("AffectedNodeCount = %d", r.AffectedNodeCount)
	}
}

// ---------------------------------------------------------------------------
// TK scoring edge cases: converge fail takes precedence over test fail
// ---------------------------------------------------------------------------

func TestComputeComplexityScore_TKConvergeFailTakesPrecedence(t *testing.T) {
	// When converge fails, we should get the converge fail weight (20),
	// NOT the test fail weight (10), even though tests also failed.
	input := ComplexityInput{
		TestKitchen: TestKitchenSummary{
			HasResult:      true,
			ConvergePassed: false,
			TestsPassed:    false,
		},
	}
	want := WeightTKConvergeFail
	if got := ComputeComplexityScore(input); got != want {
		t.Errorf("TK converge+test fail score = %d, want %d (converge fail only)", got, want)
	}
}
