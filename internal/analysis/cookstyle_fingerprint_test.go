// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"
)

func off(cop, severity string, corrected bool) CookstyleOffense {
	return CookstyleOffense{CopName: cop, Severity: severity, Corrected: corrected, Message: "ignored", File: "recipes/default.rb"}
}

// TestBuildOffenceFingerprint_GroupsAndCounts verifies offences collapse into one
// entry per (cop_name, severity, correctable) tuple with an occurrence count.
func TestBuildOffenceFingerprint_GroupsAndCounts(t *testing.T) {
	entries, hash := BuildOffenceFingerprint([]CookstyleOffense{
		off("Chef/Deprecations/ResourceWithoutUnifiedTrue", "warning", false),
		off("Chef/Deprecations/ResourceWithoutUnifiedTrue", "warning", false),
		off("Chef/Deprecations/ResourceWithoutUnifiedTrue", "warning", false),
		off("Lint/DeprecatedClassMethods", "convention", true),
	})

	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 grouped entries, got %d: %+v", len(entries), entries)
	}
	// Sorted by cop_name: Chef/... before Lint/...
	if entries[0].CopName != "Chef/Deprecations/ResourceWithoutUnifiedTrue" || entries[0].Count != 3 {
		t.Errorf("entry[0] = %+v, want Chef cop count 3", entries[0])
	}
	if entries[0].Severity != "warning" || entries[0].Correctable {
		t.Errorf("entry[0] severity/correctable = %q/%v, want warning/false", entries[0].Severity, entries[0].Correctable)
	}
	if entries[1].CopName != "Lint/DeprecatedClassMethods" || entries[1].Count != 1 {
		t.Errorf("entry[1] = %+v, want Lint cop count 1", entries[1])
	}
	if entries[1].Severity != "convention" || !entries[1].Correctable {
		t.Errorf("entry[1] severity/correctable = %q/%v, want convention/true", entries[1].Severity, entries[1].Correctable)
	}
}

// TestBuildOffenceFingerprint_OrderIndependent verifies the hash depends only on
// the multiset of offences, not their input order — so a rescan that merely
// reorders offences dedupes (no new fingerprint row).
func TestBuildOffenceFingerprint_OrderIndependent(t *testing.T) {
	a := []CookstyleOffense{
		off("Chef/A", "warning", false),
		off("Chef/B", "convention", true),
		off("Chef/A", "warning", false),
	}
	b := []CookstyleOffense{
		off("Chef/A", "warning", false),
		off("Chef/A", "warning", false),
		off("Chef/B", "convention", true),
	}
	_, ha := BuildOffenceFingerprint(a)
	_, hb := BuildOffenceFingerprint(b)
	if ha != hb {
		t.Errorf("reordered identical offences produced different hashes: %s vs %s", ha, hb)
	}
}

// TestBuildOffenceFingerprint_ChangeSensitivity verifies each input dimension that
// affects status/complexity re-derivation also changes the hash — otherwise a
// meaningful change would be silently deduped away.
func TestBuildOffenceFingerprint_ChangeSensitivity(t *testing.T) {
	base := []CookstyleOffense{off("Chef/A", "warning", false), off("Chef/A", "warning", false)}
	_, baseHash := BuildOffenceFingerprint(base)

	cases := map[string][]CookstyleOffense{
		"count":       {off("Chef/A", "warning", false)},                                  // 1 not 2
		"severity":    {off("Chef/A", "error", false), off("Chef/A", "error", false)},     // severity changed
		"correctable": {off("Chef/A", "warning", true), off("Chef/A", "warning", true)},   // correctable changed
		"cop_added":   {off("Chef/A", "warning", false), off("Chef/B", "warning", false)}, // different cop set
	}
	for name, offs := range cases {
		if _, h := BuildOffenceFingerprint(offs); h == baseHash {
			t.Errorf("change %q did not alter the fingerprint hash", name)
		}
	}
}

// TestBuildOffenceFingerprint_EmptyStable verifies a clean scan yields zero
// entries but a stable, non-empty hash (so a clean result still dedupes).
func TestBuildOffenceFingerprint_EmptyStable(t *testing.T) {
	e1, h1 := BuildOffenceFingerprint(nil)
	e2, h2 := BuildOffenceFingerprint([]CookstyleOffense{})
	if len(e1) != 0 || len(e2) != 0 {
		t.Errorf("expected no entries for empty input, got %d/%d", len(e1), len(e2))
	}
	if h1 == "" || h1 != h2 {
		t.Errorf("empty fingerprint hash unstable: %q vs %q", h1, h2)
	}
}

// TestBuildOffenceFingerprint_Contract pins the projection: a fingerprint entry
// carries exactly cop_name, count, severity, and the CookstyleOffense.Corrected
// flag (as Correctable) — and nothing derived from message/location. If the
// offence shape or this projection drifts, this test must be updated deliberately.
func TestBuildOffenceFingerprint_Contract(t *testing.T) {
	entries, _ := BuildOffenceFingerprint([]CookstyleOffense{
		{
			CopName:   "Chef/Style/FileMode",
			Severity:  "refactor",
			Corrected: true,
			Message:   "must not influence the fingerprint",
			File:      "recipes/default.rb",
			Location:  CookstyleOffenseLocation{StartLine: 10, StartColumn: 2, LastLine: 10, LastColumn: 20},
		},
	})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.CopName != "Chef/Style/FileMode" {
		t.Errorf("CopName = %q", got.CopName)
	}
	if got.Count != 1 {
		t.Errorf("Count = %d, want 1", got.Count)
	}
	if got.Severity != "refactor" {
		t.Errorf("Severity = %q, want refactor", got.Severity)
	}
	if !got.Correctable {
		t.Error("Correctable should mirror CookstyleOffense.Corrected (true)")
	}
}
