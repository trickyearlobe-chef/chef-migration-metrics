// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// A person's decision has to stick, and everything downstream has to follow it —
// see journeys/scan-trust.md. That applies to a decision about which files run
// during a converge exactly as it does to one about a finding.
//
// The recompute closure re-derives stored verdicts whenever something changes.
// If it re-derives under the shipped list while the pages read the operator's
// list, an operator's decision is silently undone the next time anybody
// reclassifies anything — and undone in the direction that puts a red back on a
// cookbook that is fine. Nothing would look wrong; the number would just
// disagree with the page it came from.

// scopedRef builds a stored result whose only breaking finding sits in a file
// the seeded list cannot name — the build-job script case.
func scopedRef(t *testing.T, path string) datastore.CookstyleResultRef {
	t.Helper()
	offences, err := json.Marshal([]map[string]any{
		{
			"cop_name": "Lint/DeprecatedClassMethods",
			"severity": "warning",
			"message":  "File.exists? is deprecated in favor of File.exist?.",
			"location": map[string]any{"file": path, "start_line": 12},
		},
	})
	if err != nil {
		t.Fatalf("marshalling offences: %v", err)
	}
	return datastore.CookstyleResultRef{
		OrganisationName:  "example-org",
		CookbookName:      "cb-alpha",
		CookbookVersion:   "1.0.0",
		TargetChefVersion: "19.0",
		CookstyleStatus:   analysis.StatusReady,
		Offences:          offences,
	}
}

// TestPropagate_HonoursAnOperatorScopeDecision — the operator has established
// that tooling/ci/* never runs on a node. A later reclassification must not put
// the cookbook back to blocked on the strength of a finding in there.
func TestPropagate_HonoursAnOperatorScopeDecision(t *testing.T) {
	store := &mockPropagationStore{
		serverRefs: []datastore.CookstyleResultRef{scopedRef(t, "tooling/ci/preflight.rb")},
		exclusions: []datastore.ScanScopeExclusion{
			{Pattern: "tooling/ci/*", Excluded: true, Reason: "Invoked only by the build job."},
		},
	}

	prop := NewCookstylePropagator(store, nil, nil, nil)
	if _, err := prop.PropagateReclassification(t.Context(), "Lint/DeprecatedClassMethods", "19.0"); err != nil {
		t.Fatalf("propagation failed: %v", err)
	}

	for _, u := range store.serverPassedUpdates {
		if u.status == analysis.StatusBlocked {
			t.Errorf("recompute blocked %s on a finding the operator established does not run", u.key)
		}
	}
}

// TestPropagate_HonoursAnOperatorOverturningAShippedExclusion is the other
// direction, and the one that fails dangerously. An estate whose test directory
// really does ship code that runs has said so; the recompute must then let a
// breaking finding in there block, rather than quietly keeping the cookbook
// green on the strength of our shipped default.
func TestPropagate_HonoursAnOperatorOverturningAShippedExclusion(t *testing.T) {
	store := &mockPropagationStore{
		serverRefs: []datastore.CookstyleResultRef{scopedRef(t, "test/helpers/shared.rb")},
		exclusions: []datastore.ScanScopeExclusion{
			{Pattern: "test/*", Excluded: false, Reason: "Our converge loads shared helpers from here."},
		},
	}

	prop := NewCookstylePropagator(store, nil, nil, nil)
	if _, err := prop.PropagateReclassification(t.Context(), "Lint/DeprecatedClassMethods", "19.0"); err != nil {
		t.Fatalf("propagation failed: %v", err)
	}

	var blocked bool
	for _, u := range store.serverPassedUpdates {
		if u.status == analysis.StatusBlocked {
			blocked = true
		}
	}
	if !blocked {
		t.Error("the operator established this file does run; a breaking finding in it must block the cookbook")
	}
}

// TestPropagate_ShippedListStillAppliesWithNoDecisions keeps the ordinary case
// honest: with nothing recorded, the seeded list governs as before.
func TestPropagate_ShippedListStillAppliesWithNoDecisions(t *testing.T) {
	store := &mockPropagationStore{
		serverRefs: []datastore.CookstyleResultRef{scopedRef(t, "Rakefile")},
	}

	prop := NewCookstylePropagator(store, nil, nil, nil)
	if _, err := prop.PropagateReclassification(t.Context(), "Lint/DeprecatedClassMethods", "19.0"); err != nil {
		t.Fatalf("propagation failed: %v", err)
	}

	for _, u := range store.serverPassedUpdates {
		if u.status == analysis.StatusBlocked {
			t.Errorf("a Rakefile finding must not block %s", u.key)
		}
	}
}
