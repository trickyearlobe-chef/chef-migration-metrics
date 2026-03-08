// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"testing"
)

// ---------------------------------------------------------------------------
// LookupCop
// ---------------------------------------------------------------------------

func TestLookupCop_KnownDeprecation(t *testing.T) {
	m := LookupCop("Chef/Deprecations/ResourceWithoutUnifiedTrue")
	if m == nil {
		t.Fatal("expected non-nil mapping for ChefDeprecations/ResourceWithoutUnifiedTrue")
	}
	if m.CopName != "Chef/Deprecations/ResourceWithoutUnifiedTrue" {
		t.Errorf("CopName = %q, want %q", m.CopName, "Chef/Deprecations/ResourceWithoutUnifiedTrue")
	}
	if m.Description == "" {
		t.Error("Description should not be empty")
	}
	if m.MigrationURL == "" {
		t.Error("MigrationURL should not be empty")
	}
	if m.IntroducedIn == "" {
		t.Error("IntroducedIn should not be empty")
	}
	if m.ReplacementPattern == "" {
		t.Error("ReplacementPattern should not be empty")
	}
}

func TestLookupCop_KnownCorrectness(t *testing.T) {
	m := LookupCop("Chef/Correctness/BlockGuardWithOnlyString")
	if m == nil {
		t.Fatal("expected non-nil mapping for ChefCorrectness/BlockGuardWithOnlyString")
	}
	if m.CopName != "Chef/Correctness/BlockGuardWithOnlyString" {
		t.Errorf("CopName = %q, want %q", m.CopName, "Chef/Correctness/BlockGuardWithOnlyString")
	}
	if m.Description == "" {
		t.Error("Description should not be empty")
	}
	if m.MigrationURL == "" {
		t.Error("MigrationURL should not be empty")
	}
}

func TestLookupCop_UnknownCop(t *testing.T) {
	m := LookupCop("Chef/Deprecations/CompletelyFakeCop")
	if m != nil {
		t.Errorf("expected nil for unknown cop, got %+v", m)
	}
}

func TestLookupCop_EmptyString(t *testing.T) {
	m := LookupCop("")
	if m != nil {
		t.Errorf("expected nil for empty cop name, got %+v", m)
	}
}

func TestLookupCop_PartialMatch(t *testing.T) {
	// Ensure partial names don't accidentally match.
	m := LookupCop("Chef/Deprecations/Resource")
	if m != nil {
		t.Errorf("expected nil for partial cop name, got %+v", m)
	}
}

func TestLookupCop_CaseSensitive(t *testing.T) {
	// Lookups should be case-sensitive since cop names are case-sensitive.
	m := LookupCop("chefdeprecations/resourcewithoutunifiedtrue")
	if m != nil {
		t.Errorf("expected nil for wrong-case cop name, got %+v", m)
	}
}

// ---------------------------------------------------------------------------
// LookupCop — spot-check several specific cops
// ---------------------------------------------------------------------------

func TestLookupCop_SpecificCops(t *testing.T) {
	tests := []struct {
		copName        string
		wantURL        string
		wantIntro      string
		wantRemoved    string
		hasReplacement bool
	}{
		{
			copName:        "Chef/Deprecations/NodeSet",
			wantURL:        "https://docs.chef.io/deprecations_attributes/",
			wantIntro:      "12.12",
			wantRemoved:    "14.0",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Deprecations/ChefRewind",
			wantURL:        "https://docs.chef.io/deprecations/",
			wantIntro:      "12.10",
			wantRemoved:    "14.0",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Deprecations/RequireRecipe",
			wantURL:        "https://docs.chef.io/deprecations/",
			wantIntro:      "10.0",
			wantRemoved:    "14.0",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Correctness/CookbookUsesNodeSave",
			wantURL:        "https://docs.chef.io/resources/",
			wantIntro:      "12.0",
			wantRemoved:    "",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Correctness/MetadataMissingName",
			wantURL:        "https://docs.chef.io/config_rb_metadata/",
			wantIntro:      "12.0",
			wantRemoved:    "",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Correctness/LazyEvalNodeAttributeDefaults",
			wantURL:        "https://docs.chef.io/custom_resources/",
			wantIntro:      "12.0",
			wantRemoved:    "",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Deprecations/UseAutomaticResourceName",
			wantURL:        "https://docs.chef.io/deprecations/",
			wantIntro:      "16.0",
			wantRemoved:    "",
			hasReplacement: true,
		},
		{
			copName:        "Chef/Deprecations/UserDeprecatedSupportsProperty",
			wantURL:        "https://docs.chef.io/deprecations/",
			wantIntro:      "12.14",
			wantRemoved:    "15.0",
			hasReplacement: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.copName, func(t *testing.T) {
			m := LookupCop(tt.copName)
			if m == nil {
				t.Fatalf("expected non-nil mapping for %s", tt.copName)
			}
			if m.CopName != tt.copName {
				t.Errorf("CopName = %q, want %q", m.CopName, tt.copName)
			}
			if m.MigrationURL != tt.wantURL {
				t.Errorf("MigrationURL = %q, want %q", m.MigrationURL, tt.wantURL)
			}
			if m.IntroducedIn != tt.wantIntro {
				t.Errorf("IntroducedIn = %q, want %q", m.IntroducedIn, tt.wantIntro)
			}
			if m.RemovedIn != tt.wantRemoved {
				t.Errorf("RemovedIn = %q, want %q", m.RemovedIn, tt.wantRemoved)
			}
			if tt.hasReplacement && m.ReplacementPattern == "" {
				t.Error("expected non-empty ReplacementPattern")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AllCopMappings
// ---------------------------------------------------------------------------

func TestAllCopMappings_ReturnsNonEmpty(t *testing.T) {
	all := AllCopMappings()
	if len(all) == 0 {
		t.Fatal("AllCopMappings() returned empty slice")
	}
}

func TestAllCopMappings_ReturnsCopy(t *testing.T) {
	all1 := AllCopMappings()
	all2 := AllCopMappings()

	// Mutating the returned slice should not affect subsequent calls.
	if len(all1) == 0 {
		t.Fatal("AllCopMappings() returned empty slice")
	}
	originalName := all1[0].CopName
	all1[0].CopName = "MUTATED"

	if all2[0].CopName != originalName {
		t.Error("AllCopMappings() did not return a copy — mutation leaked")
	}

	// Also verify the index is unaffected.
	m := LookupCop(originalName)
	if m == nil {
		t.Errorf("LookupCop(%q) returned nil after mutating AllCopMappings result", originalName)
	}
}

func TestAllCopMappings_ContainsExpectedCops(t *testing.T) {
	all := AllCopMappings()
	copSet := make(map[string]bool, len(all))
	for _, m := range all {
		copSet[m.CopName] = true
	}

	expected := []string{
		"Chef/Deprecations/ResourceWithoutUnifiedTrue",
		"Chef/Deprecations/NodeSet",
		"Chef/Deprecations/NodeSetUnless",
		"Chef/Deprecations/ChefRewind",
		"Chef/Deprecations/RequireRecipe",
		"Chef/Correctness/BlockGuardWithOnlyString",
		"Chef/Correctness/CookbookUsesNodeSave",
		"Chef/Correctness/MetadataMissingName",
		"Chef/Correctness/ScopedFileExist",
	}

	for _, cop := range expected {
		if !copSet[cop] {
			t.Errorf("AllCopMappings() is missing expected cop %q", cop)
		}
	}
}

func TestAllCopMappings_NoDuplicates(t *testing.T) {
	all := AllCopMappings()
	seen := make(map[string]bool, len(all))
	for _, m := range all {
		if seen[m.CopName] {
			t.Errorf("duplicate cop_name in AllCopMappings: %q", m.CopName)
		}
		seen[m.CopName] = true
	}
}

func TestAllCopMappings_AllFieldsPopulated(t *testing.T) {
	all := AllCopMappings()
	for _, m := range all {
		if m.CopName == "" {
			t.Error("found mapping with empty CopName")
		}
		if m.Description == "" {
			t.Errorf("cop %q has empty Description", m.CopName)
		}
		if m.MigrationURL == "" {
			t.Errorf("cop %q has empty MigrationURL", m.CopName)
		}
		if m.IntroducedIn == "" {
			t.Errorf("cop %q has empty IntroducedIn", m.CopName)
		}
		if m.ReplacementPattern == "" {
			t.Errorf("cop %q has empty ReplacementPattern", m.CopName)
		}
		// RemovedIn may be legitimately empty for cops not yet removed.
	}
}

func TestAllCopMappings_AllCopsInCorrectNamespace(t *testing.T) {
	all := AllCopMappings()
	for _, m := range all {
		isDeprecation := len(m.CopName) > len("Chef/Deprecations/") && m.CopName[:len("Chef/Deprecations/")] == "Chef/Deprecations/"
		isCorrectness := len(m.CopName) > len("Chef/Correctness/") && m.CopName[:len("Chef/Correctness/")] == "Chef/Correctness/"

		if !isDeprecation && !isCorrectness {
			t.Errorf("cop %q is not in ChefDeprecations/ or ChefCorrectness/ namespace", m.CopName)
		}
	}
}

// ---------------------------------------------------------------------------
// CopMappingCount
// ---------------------------------------------------------------------------

func TestCopMappingCount_MatchesAllCopMappings(t *testing.T) {
	count := CopMappingCount()
	all := AllCopMappings()

	if count != len(all) {
		t.Errorf("CopMappingCount() = %d, but AllCopMappings() has %d entries", count, len(all))
	}
}

func TestCopMappingCount_Positive(t *testing.T) {
	count := CopMappingCount()
	if count <= 0 {
		t.Errorf("CopMappingCount() = %d, expected positive", count)
	}
}

func TestCopMappingCount_MatchesIndexSize(t *testing.T) {
	count := CopMappingCount()

	// Verify the index has the same number of entries.
	indexCount := 0
	for _, m := range AllCopMappings() {
		if LookupCop(m.CopName) != nil {
			indexCount++
		}
	}

	if count != indexCount {
		t.Errorf("CopMappingCount() = %d, but index lookup found %d entries", count, indexCount)
	}
}

// ---------------------------------------------------------------------------
// LookupCop — returned pointer stability
// ---------------------------------------------------------------------------

func TestLookupCop_ReturnsSamePointer(t *testing.T) {
	// Multiple lookups of the same cop should return the same pointer
	// (into the index), not copies.
	m1 := LookupCop("Chef/Deprecations/NodeSet")
	m2 := LookupCop("Chef/Deprecations/NodeSet")

	if m1 == nil || m2 == nil {
		t.Fatal("expected non-nil for ChefDeprecations/NodeSet")
	}
	if m1 != m2 {
		t.Error("LookupCop returned different pointers for the same cop name")
	}
}

// ---------------------------------------------------------------------------
// Coverage of both namespaces
// ---------------------------------------------------------------------------

func TestAllCopMappings_ContainsBothNamespaces(t *testing.T) {
	all := AllCopMappings()
	hasDeprecation := false
	hasCorrectness := false

	for _, m := range all {
		if len(m.CopName) > len("Chef/Deprecations/") && m.CopName[:len("Chef/Deprecations/")] == "Chef/Deprecations/" {
			hasDeprecation = true
		}
		if len(m.CopName) > len("Chef/Correctness/") && m.CopName[:len("Chef/Correctness/")] == "Chef/Correctness/" {
			hasCorrectness = true
		}
		if hasDeprecation && hasCorrectness {
			break
		}
	}

	if !hasDeprecation {
		t.Error("AllCopMappings() contains no Chef/Deprecations/* entries")
	}
	if !hasCorrectness {
		t.Error("AllCopMappings() contains no Chef/Correctness/* entries")
	}
}

// ---------------------------------------------------------------------------
// Migration URLs are valid
// ---------------------------------------------------------------------------

func TestAllCopMappings_MigrationURLsStartWithHTTPS(t *testing.T) {
	all := AllCopMappings()
	for _, m := range all {
		if m.MigrationURL == "" {
			continue // Already tested above.
		}
		if len(m.MigrationURL) < 8 || m.MigrationURL[:8] != "https://" {
			t.Errorf("cop %q has MigrationURL %q that does not start with https://", m.CopName, m.MigrationURL)
		}
	}
}

// ---------------------------------------------------------------------------
// EnrichedOffense type sanity
// ---------------------------------------------------------------------------

func TestEnrichedOffense_CanHoldMapping(t *testing.T) {
	m := LookupCop("Chef/Deprecations/NodeSet")
	if m == nil {
		t.Fatal("expected non-nil mapping")
	}

	eo := EnrichedOffense{
		CopName:  m.CopName,
		Severity: "warning",
		Message:  "test message",
		Location: OffenseLocation{
			StartLine:   10,
			StartColumn: 1,
			LastLine:    10,
			LastColumn:  40,
		},
		Remediation: m,
	}

	if eo.Remediation == nil {
		t.Error("Remediation should not be nil")
	}
	if eo.Remediation.CopName != m.CopName {
		t.Errorf("Remediation.CopName = %q, want %q", eo.Remediation.CopName, m.CopName)
	}
}

func TestEnrichedOffense_NilRemediation(t *testing.T) {
	eo := EnrichedOffense{
		CopName:     "Chef/Style/SomeUnmappedCop",
		Severity:    "convention",
		Message:     "test",
		Remediation: nil,
	}

	if eo.Remediation != nil {
		t.Error("Remediation should be nil for unmapped cop")
	}
}
