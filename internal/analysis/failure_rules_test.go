package analysis

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Preset tests
// ---------------------------------------------------------------------------

func TestDefaultFailureRules(t *testing.T) {
	rules := DefaultFailureRules()
	if len(rules.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules.Rules))
	}
	got, ok := rules.Rules["*"]
	if !ok {
		t.Fatal("expected catch-all rule")
	}
	if len(got) != 2 || got[0] != "error" || got[1] != "fatal" {
		t.Fatalf("expected [error fatal], got %v", got)
	}
}

func TestStrictFailureRules(t *testing.T) {
	rules := StrictFailureRules()
	if len(rules.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules.Rules))
	}
	dep := rules.Rules["Chef/Deprecations/"]
	if len(dep) != 3 {
		t.Fatalf("expected 3 severities for Deprecations, got %v", dep)
	}
	cor := rules.Rules["Chef/Correctness/"]
	if len(cor) != 3 {
		t.Fatalf("expected 3 severities for Correctness, got %v", cor)
	}
	catchAll := rules.Rules["*"]
	if len(catchAll) != 2 {
		t.Fatalf("expected 2 severities for catch-all, got %v", catchAll)
	}
}

func TestRelaxedFailureRules(t *testing.T) {
	rules := RelaxedFailureRules()
	if len(rules.Rules) != 5 {
		t.Fatalf("expected 5 rules, got %d", len(rules.Rules))
	}
	// Style and Modernize should never fail.
	if sev := rules.Rules["Chef/Style/"]; len(sev) != 0 {
		t.Fatalf("expected empty severities for Style, got %v", sev)
	}
	if sev := rules.Rules["Chef/Modernize/"]; len(sev) != 0 {
		t.Fatalf("expected empty severities for Modernize, got %v", sev)
	}
	// Catch-all should also be empty.
	if sev := rules.Rules["*"]; len(sev) != 0 {
		t.Fatalf("expected empty severities for catch-all, got %v", sev)
	}
}

// ---------------------------------------------------------------------------
// EffectiveRules tests
// ---------------------------------------------------------------------------

func TestEffectiveRules_PresetDefault(t *testing.T) {
	rules := EffectiveRules("default", nil)
	if len(rules.Rules) != 1 {
		t.Fatalf("expected default rules, got %d entries", len(rules.Rules))
	}
}

func TestEffectiveRules_PresetStrict(t *testing.T) {
	rules := EffectiveRules("strict", nil)
	if len(rules.Rules) != 3 {
		t.Fatalf("expected strict rules, got %d entries", len(rules.Rules))
	}
}

func TestEffectiveRules_PresetRelaxed(t *testing.T) {
	rules := EffectiveRules("relaxed", nil)
	if len(rules.Rules) != 5 {
		t.Fatalf("expected relaxed rules, got %d entries", len(rules.Rules))
	}
}

func TestEffectiveRules_ExplicitOverridesPreset(t *testing.T) {
	explicit := map[string][]string{
		"Chef/Style/": {"warning", "error"},
	}
	rules := EffectiveRules("strict", explicit)
	// Should use explicit, not strict.
	if len(rules.Rules) != 1 {
		t.Fatalf("expected 1 explicit rule, got %d", len(rules.Rules))
	}
	if _, ok := rules.Rules["Chef/Style/"]; !ok {
		t.Fatal("expected Chef/Style/ rule from explicit")
	}
}

func TestEffectiveRules_EmptyPresetDefaultsToDefault(t *testing.T) {
	rules := EffectiveRules("", nil)
	if len(rules.Rules) != 1 {
		t.Fatalf("expected default rules for empty preset, got %d entries", len(rules.Rules))
	}
}

// ---------------------------------------------------------------------------
// EvaluatePassFail tests
// ---------------------------------------------------------------------------

func TestEvaluatePassFail_NoOffenses(t *testing.T) {
	rules := DefaultFailureRules()
	if !EvaluatePassFail(nil, rules) {
		t.Fatal("expected pass with no offenses")
	}
	if !EvaluatePassFail([]CookstyleOffense{}, rules) {
		t.Fatal("expected pass with empty offenses")
	}
}

func TestEvaluatePassFail_DefaultRules_MatchesIsErrorOrFatal(t *testing.T) {
	rules := DefaultFailureRules()

	tests := []struct {
		severity string
		wantPass bool
	}{
		{"convention", true},
		{"refactor", true},
		{"warning", true},
		{"error", false},
		{"fatal", false},
	}

	for _, tt := range tests {
		offenses := []CookstyleOffense{{
			Severity: tt.severity,
			CopName:  "Chef/Style/Something",
		}}
		got := EvaluatePassFail(offenses, rules)
		if got != tt.wantPass {
			t.Errorf("severity=%q: EvaluatePassFail=%v, want %v", tt.severity, got, tt.wantPass)
		}
		// Also verify it matches the old behaviour.
		oldFail := isErrorOrFatal(tt.severity)
		if got == oldFail {
			t.Errorf("severity=%q: new logic should invert old isErrorOrFatal, got same result", tt.severity)
		}
	}
}

func TestEvaluatePassFail_LongestPrefixMatch(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{
		"Chef/Deprecations/":                          {"error", "fatal"},
		"Chef/Deprecations/ResourceWithoutUnifiedTrue": {"warning", "error", "fatal"},
		"*": {"fatal"},
	})

	// A warning in Chef/Deprecations/ generally → passes (not in severity list).
	offenses := []CookstyleOffense{{
		Severity: "warning",
		CopName:  "Chef/Deprecations/SomeOtherCop",
	}}
	if !EvaluatePassFail(offenses, rules) {
		t.Fatal("expected pass: warning not in broad Deprecations rule")
	}

	// A warning in the specific cop → fails (matches the narrower rule).
	offenses = []CookstyleOffense{{
		Severity: "warning",
		CopName:  "Chef/Deprecations/ResourceWithoutUnifiedTrue",
	}}
	if EvaluatePassFail(offenses, rules) {
		t.Fatal("expected fail: warning matches narrow Deprecations/ResourceWithoutUnifiedTrue rule")
	}
}

func TestEvaluatePassFail_CatchAll(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{
		"Chef/Deprecations/": {"error", "fatal"},
		"*":                  {"fatal"},
	})

	// An error from an unknown cop → matches catch-all (only fatal fails).
	offenses := []CookstyleOffense{{
		Severity: "error",
		CopName:  "SomeVendor/CustomCop",
	}}
	if !EvaluatePassFail(offenses, rules) {
		t.Fatal("expected pass: error not in catch-all severity list")
	}

	// A fatal from an unknown cop → fails.
	offenses = []CookstyleOffense{{
		Severity: "fatal",
		CopName:  "SomeVendor/CustomCop",
	}}
	if EvaluatePassFail(offenses, rules) {
		t.Fatal("expected fail: fatal in catch-all severity list")
	}
}

func TestEvaluatePassFail_NoCatchAll_UnmatchedPasses(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{
		"Chef/Deprecations/": {"error", "fatal"},
	})

	// A fatal from an unmatched cop → passes (no rule covers it).
	offenses := []CookstyleOffense{{
		Severity: "fatal",
		CopName:  "SomeVendor/CustomCop",
	}}
	if !EvaluatePassFail(offenses, rules) {
		t.Fatal("expected pass: no rule matches this cop")
	}
}

func TestEvaluatePassFail_EmptyRules_AlwaysPasses(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{})

	offenses := []CookstyleOffense{{
		Severity: "fatal",
		CopName:  "Chef/Correctness/Something",
	}}
	if !EvaluatePassFail(offenses, rules) {
		t.Fatal("expected pass: empty rules never trigger failure")
	}
}

func TestEvaluatePassFail_EmptySeverityList_NeverFails(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{
		"Chef/Style/": {},
		"*":           {"error", "fatal"},
	})

	// Style cop with fatal severity → passes (its rule has empty severity list).
	offenses := []CookstyleOffense{{
		Severity: "fatal",
		CopName:  "Chef/Style/AlignHashRocket",
	}}
	if !EvaluatePassFail(offenses, rules) {
		t.Fatal("expected pass: Chef/Style/ has empty severity list")
	}
}

func TestEvaluatePassFail_MultipleOffenses_OneTriggersFailure(t *testing.T) {
	rules := DefaultFailureRules()

	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Style/Foo"},
		{Severity: "convention", CopName: "Chef/Modernize/Bar"},
		{Severity: "error", CopName: "Chef/Correctness/Baz"},
	}
	if EvaluatePassFail(offenses, rules) {
		t.Fatal("expected fail: one offense has error severity")
	}
}

// ---------------------------------------------------------------------------
// Sorted prefixes
// ---------------------------------------------------------------------------

func TestNewCookstyleFailureRules_SortsPrefixes(t *testing.T) {
	rules := NewCookstyleFailureRules(map[string][]string{
		"*":            {"fatal"},
		"A/":           {"error"},
		"A/B/":         {"warning"},
		"A/B/C/D/E/F/": {"convention"},
	})

	// Sorted longest first (excluding "*").
	if len(rules.sortedPrefixes) != 3 {
		t.Fatalf("expected 3 sorted prefixes (no '*'), got %d", len(rules.sortedPrefixes))
	}
	if rules.sortedPrefixes[0] != "A/B/C/D/E/F/" {
		t.Errorf("expected longest prefix first, got %q", rules.sortedPrefixes[0])
	}
	if rules.sortedPrefixes[2] != "A/" {
		t.Errorf("expected shortest prefix last, got %q", rules.sortedPrefixes[2])
	}
}
