// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// fakeCopLister is a CopDescriptionLister backed by an in-memory map: cop name
// → description. A cop absent from the map is treated as not emitted.
type fakeCopLister map[string]string

func (f fakeCopLister) Lookup(name string) (CopRegistryEntry, bool) {
	desc, ok := f[name]
	if !ok {
		return CopRegistryEntry{}, false
	}
	return CopRegistryEntry{CopName: name, Description: desc}, true
}

func issueFor(issues []CurationIssue, cop string) (CurationIssue, bool) {
	for _, i := range issues {
		if i.CopName == cop {
			return i, true
		}
	}
	return CurationIssue{}, false
}

// A curated RemovedIn whose cop the binary no longer emits is flagged stale.
func TestValidateCuratedRemovals_FlagsStaleEntry(t *testing.T) {
	mappings := []remediation.CopMapping{
		{CopName: "Chef/Deprecations/Present", RemovedIn: "14.0"},
		{CopName: "Chef/Deprecations/GoneUpstream", RemovedIn: "15.0"},
	}
	reg := fakeCopLister{
		"Chef/Deprecations/Present": "node.set was removed in Chef 14.",
		// GoneUpstream absent → stale.
	}

	issues := ValidateCuratedRemovals(mappings, reg)

	got, ok := issueFor(issues, "Chef/Deprecations/GoneUpstream")
	if !ok {
		t.Fatalf("expected a stale issue for GoneUpstream, got %+v", issues)
	}
	if got.Kind != CurationStale {
		t.Errorf("kind = %q, want %q", got.Kind, CurationStale)
	}
	if _, present := issueFor(issues, "Chef/Deprecations/Present"); present {
		t.Errorf("Present cop should not be flagged: %+v", issues)
	}
}

// A description that names a removal version disagreeing with the curated
// RemovedIn is flagged.
func TestValidateCuratedRemovals_FlagsRemovalDisagreement(t *testing.T) {
	mappings := []remediation.CopMapping{
		{CopName: "Chef/Deprecations/Mismatch", RemovedIn: "14.0"},
	}
	reg := fakeCopLister{
		"Chef/Deprecations/Mismatch": "This was deprecated in Chef 12 and removed in Chef Infra Client 16.",
	}

	issues := ValidateCuratedRemovals(mappings, reg)

	got, ok := issueFor(issues, "Chef/Deprecations/Mismatch")
	if !ok {
		t.Fatalf("expected a disagreement issue, got %+v", issues)
	}
	if got.Kind != CurationRemovalDisagreement {
		t.Errorf("kind = %q, want %q", got.Kind, CurationRemovalDisagreement)
	}
}

// A curated RemovedIn that agrees with the description (or whose description
// states no removal version) is not flagged.
func TestValidateCuratedRemovals_CleanTable(t *testing.T) {
	mappings := []remediation.CopMapping{
		{CopName: "Chef/Deprecations/Agrees", RemovedIn: "14.0"},
		{CopName: "Chef/Deprecations/NoVersionInDesc", RemovedIn: "15.0"},
		{CopName: "Chef/Correctness/NotARemoval", RemovedIn: ""}, // ignored: not a removal claim
	}
	reg := fakeCopLister{
		"Chef/Deprecations/Agrees":          "node.set was removed in Chef 14.",
		"Chef/Deprecations/NoVersionInDesc": "This deprecated pattern should be replaced.",
		"Chef/Correctness/NotARemoval":      "An advisory correctness check.",
	}

	if issues := ValidateCuratedRemovals(mappings, reg); len(issues) != 0 {
		t.Errorf("expected no issues on a clean table, got %+v", issues)
	}
}

// The "deprecated in N" trap must not be read as a removal version.
func TestValidateCuratedRemovals_IgnoresDeprecatedInVersion(t *testing.T) {
	mappings := []remediation.CopMapping{
		{CopName: "Chef/Deprecations/DeprecatedOnly", RemovedIn: "14.0"},
	}
	reg := fakeCopLister{
		// Names a "deprecated in 12" version but no removal version — must not flag.
		"Chef/Deprecations/DeprecatedOnly": "This was deprecated in Chef 12; use the modern form.",
	}

	if issues := ValidateCuratedRemovals(mappings, reg); len(issues) != 0 {
		t.Errorf("a 'deprecated in' version must not be treated as a removal, got %+v", issues)
	}
}
