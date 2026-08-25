// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Which files are excluded is a decision somebody makes and can see, not a rule
// inferred — and being able to disagree with it is the point. See
// journeys/scan-trust.md.
//
// The curated list reaches files with predictable names. It cannot reach a
// script that only runs because a build job invokes it: that sits at a
// different path in every repository, and nothing in the file says what runs it.
// For that case the operator's list IS the feature, so these pin the merge
// rather than the seed.

type fakeExclusionStore struct {
	rows []datastore.ScanScopeExclusion
	err  error
}

func (f fakeExclusionStore) ListScanScopeExclusions(context.Context) ([]datastore.ScanScopeExclusion, error) {
	return f.rows, f.err
}

// TestOperatorCanExcludeAFileTheSeedListCannotName is the case the seed list
// exists to admit it cannot handle.
func TestOperatorCanExcludeAFileTheSeedListCannotName(t *testing.T) {
	const buildScript = "tooling/ci/preflight_checks.rb"

	if _, excluded := DefaultScanScope().Excluded(buildScript); excluded {
		t.Fatalf("precondition: the curated list should not reach %q", buildScript)
	}

	scope := NewScanScopeFromStore(context.Background(), fakeExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "tooling/ci/*", Excluded: true, Reason: "Invoked only by the build job; nothing loads it during a converge."},
		},
	})

	ex, excluded := scope.Excluded(buildScript)
	if !excluded {
		t.Fatalf("an operator exclusion must take effect for %q", buildScript)
	}
	if ex.Reason == "" {
		t.Error("the operator's recorded reason must travel with the exclusion")
	}
}

// TestOperatorCanDisagreeWithACuratedExclusion is the other direction, and the
// more important one: an operator whose test directory really does ship code
// that runs must be able to say so, and be believed.
func TestOperatorCanDisagreeWithACuratedExclusion(t *testing.T) {
	const inTestDir = "test/helpers/shared.rb"

	if _, excluded := DefaultScanScope().Excluded(inTestDir); !excluded {
		t.Fatalf("precondition: the curated list should exclude %q", inTestDir)
	}

	scope := NewScanScopeFromStore(context.Background(), fakeExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "test/*", Excluded: false, Reason: "Our converge loads shared helpers from here."},
		},
	})

	if _, excluded := scope.Excluded(inTestDir); excluded {
		t.Errorf("a person's decision must outrank the curated default for %q", inTestDir)
	}

	// Overturning one default must not disturb the others.
	if _, excluded := scope.Excluded("Rakefile"); !excluded {
		t.Error("overturning test/* must leave the rest of the curated list standing")
	}
}

// TestOperatorReasonReplacesTheCuratedOne — when an operator restates a curated
// pattern, the reason a reader sees must be the one somebody actually stands
// behind, not the seeded prose it replaced.
func TestOperatorReasonReplacesTheCuratedOne(t *testing.T) {
	scope := NewScanScopeFromStore(context.Background(), fakeExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "Rakefile", Excluded: true, Reason: "Checked with the platform team."},
		},
	})

	ex, excluded := scope.Excluded("Rakefile")
	if !excluded {
		t.Fatal("Rakefile must remain excluded")
	}
	if ex.Reason != "Checked with the platform team." {
		t.Errorf("the operator's reason must be the one shown, got %q", ex.Reason)
	}
}

// TestUnreadableExclusionsFallBackToCurated pins the failure direction. If the
// operator list cannot be read, the seed list stands rather than the scope
// collapsing to "nothing is excluded" — but crucially NOT to "everything is",
// which would hide blockers wholesale.
func TestUnreadableExclusionsFallBackToCurated(t *testing.T) {
	scope := NewScanScopeFromStore(context.Background(), fakeExclusionStore{
		err: errors.New("database unavailable"),
	})

	if _, excluded := scope.Excluded("Rakefile"); !excluded {
		t.Error("an unreadable operator list must leave the curated list in force")
	}
	if _, excluded := scope.Excluded("recipes/default.rb"); excluded {
		t.Error("an unreadable operator list must never widen what is excluded")
	}
}

// TestNilStoreIsTheCuratedList keeps the zero-configuration path honest: a
// deployment that has never touched the list behaves exactly as the seed.
func TestNilStoreIsTheCuratedList(t *testing.T) {
	scope := NewScanScopeFromStore(context.Background(), nil)
	if len(scope.Exclusions()) != len(DefaultScanScopeExclusions()) {
		t.Errorf("want the curated list, got %d entries", len(scope.Exclusions()))
	}
}
