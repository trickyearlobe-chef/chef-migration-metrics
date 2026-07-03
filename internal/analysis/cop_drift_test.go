// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"
)

// driftResolver builds a resolver with the given operator overrides at a fixed
// target version. Uses synthetic Chef/* cop names not present in the embedded
// RemovedIn mapping so resolution is driven only by prefix/exact defaults and
// overrides under test.
func driftResolver(overrides map[string]string) *CopClassificationResolver {
	if overrides == nil {
		overrides = map[string]string{}
	}
	return &CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: "18.0"}
}

func TestComputeCopDriftStale(t *testing.T) {
	// Registry emits one live cop; the static tables reference two cops the
	// binary no longer has plus one it still does.
	reg := NewCopRegistry([]CopRegistryEntry{
		{CopName: "Chef/Deprecations/LiveCop", Department: "Chef/Deprecations", TopNamespace: "Chef", Enabled: true},
	}, "8.6.10")

	static := []StaticCopSource{
		{CopName: "Chef/Deprecations/LiveCop", Source: StaticSourceMapping},
		{CopName: "Chef/Deprecations/GoneMapping", Source: StaticSourceMapping},
		{CopName: "Lint/GoneMapped", Source: StaticSourceMapping},
	}

	report := ComputeCopDrift(reg, driftResolver(nil), static)

	if !report.RegistryAvailable {
		t.Fatal("RegistryAvailable = false, want true")
	}
	if report.RegistryVersion != "8.6.10" {
		t.Errorf("RegistryVersion = %q, want 8.6.10", report.RegistryVersion)
	}

	staleByName := map[string]string{}
	for _, s := range report.Stale {
		staleByName[s.CopName] = s.Source
	}
	if len(staleByName) != 2 {
		t.Fatalf("Stale = %d entries, want 2: %+v", len(staleByName), report.Stale)
	}
	if staleByName["Chef/Deprecations/GoneMapping"] != StaticSourceMapping {
		t.Errorf("GoneMapping stale source = %q, want %q", staleByName["Chef/Deprecations/GoneMapping"], StaticSourceMapping)
	}
	if staleByName["Lint/GoneMapped"] != StaticSourceMapping {
		t.Errorf("GoneMapped stale source = %q, want %q", staleByName["Lint/GoneMapped"], StaticSourceMapping)
	}
	if _, ok := staleByName["Chef/Deprecations/LiveCop"]; ok {
		t.Error("LiveCop should not be stale — the binary still emits it")
	}
}

func TestComputeCopDriftCoverageGaps(t *testing.T) {
	reg := NewCopRegistry([]CopRegistryEntry{
		// Chef/* cop that nothing specifically classifies → resolves to the
		// Review default → coverage gap.
		{CopName: "Chef/Modernize/ZzzTestCop", Department: "Chef/Modernize", TopNamespace: "Chef", Enabled: true},
		// Covered by a positive structural-noise reason (Chef/Style/ prefix) →
		// source structural_noise, not review_default → not a gap.
		{CopName: "Chef/Style/ZzzTestCop", Department: "Chef/Style", TopNamespace: "Chef", Enabled: true},
		// Covered by an operator override → not a gap.
		{CopName: "Chef/Sharing/ZzzTestCop", Department: "Chef/Sharing", TopNamespace: "Chef", Enabled: true},
		// Generic Ruby cop → excluded from coverage entirely.
		{CopName: "Style/ZzzTestCop", Department: "Style", TopNamespace: "Style", Enabled: false},
	}, "8.6.10")

	resolver := driftResolver(map[string]string{"Chef/Sharing/ZzzTestCop": ClassificationReview})
	report := ComputeCopDrift(reg, resolver, nil)

	if len(report.CoverageGaps) != 1 {
		t.Fatalf("CoverageGaps = %d, want 1: %+v", len(report.CoverageGaps), report.CoverageGaps)
	}
	gap := report.CoverageGaps[0]
	if gap.CopName != "Chef/Modernize/ZzzTestCop" {
		t.Errorf("gap = %q, want Chef/Modernize/ZzzTestCop", gap.CopName)
	}
	if gap.Department != "Chef/Modernize" {
		t.Errorf("gap Department = %q, want Chef/Modernize", gap.Department)
	}
}

func TestComputeCopDriftNilRegistry(t *testing.T) {
	report := ComputeCopDrift(nil, driftResolver(nil), []StaticCopSource{
		{CopName: "Chef/Deprecations/Whatever", Source: StaticSourceMapping},
	})
	if report.RegistryAvailable {
		t.Error("RegistryAvailable = true for nil registry, want false")
	}
	if len(report.Stale) != 0 || len(report.CoverageGaps) != 0 {
		t.Errorf("nil registry should yield no drift, got stale=%d gaps=%d", len(report.Stale), len(report.CoverageGaps))
	}
}

func TestComputeCopDriftDeterministicOrder(t *testing.T) {
	reg := NewCopRegistry([]CopRegistryEntry{
		{CopName: "Chef/Modernize/BravoCop", Department: "Chef/Modernize", TopNamespace: "Chef", Enabled: true},
		{CopName: "Chef/Modernize/AlphaCop", Department: "Chef/Modernize", TopNamespace: "Chef", Enabled: true},
	}, "8.6.10")
	static := []StaticCopSource{
		{CopName: "Chef/Gone/Zulu", Source: StaticSourceMapping},
		{CopName: "Chef/Gone/Alpha", Source: StaticSourceMapping},
	}
	report := ComputeCopDrift(reg, driftResolver(nil), static)

	if len(report.CoverageGaps) != 2 || report.CoverageGaps[0].CopName != "Chef/Modernize/AlphaCop" {
		t.Errorf("CoverageGaps not sorted ascending: %+v", report.CoverageGaps)
	}
	if len(report.Stale) != 2 || report.Stale[0].CopName != "Chef/Gone/Alpha" {
		t.Errorf("Stale not sorted ascending: %+v", report.Stale)
	}
}
