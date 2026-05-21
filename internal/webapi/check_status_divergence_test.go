// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Semantic Contract Divergence Tests
//
// These tests document KNOWN divergences between the webapi read-time
// derivation (check_status.go) and the analysis write-time derivation
// (analysis/readiness.go). They serve as a regression guard and roadmap
// for Phase 2 fixes.
//
// Each test documents:
// - What the analysis (write-time) would produce
// - What the webapi (read-time) re-derivation produces
// - Why they differ
// ---------------------------------------------------------------------------

// TestDivergence_KitchenStatus_PartialCoverageNotVisible documents that the
// webapi deriveKitchenStatus cannot detect partial TK coverage because it only
// sees blocking cookbooks, not all cookbooks on the node.
//
// Scenario: A node has 3 TK-eligible cookbooks. Only 1 has been tested (passed).
// None are blocking (all compatible). The analysis write-time correctly sets
// kitchen_status = "partial" because tkTested < tkEligible. But the webapi
// re-derivation sees AllCookbooksCompatible=true + no blocking → returns "passed".
//
// Phase 2 fix: serve the persisted kitchen_status from the DB instead of
// re-deriving it.
func TestDivergence_KitchenStatus_PartialCoverageNotVisible(t *testing.T) {
	// This represents a node where analysis computed kitchen_status="partial"
	// but we stored AllCookbooksCompatible=true because no cookbooks are blocking.
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: true,
		BlockingCookbooks:      json.RawMessage(`[]`),
		CookstyleStatus:        "passed",
		KitchenStatus:          "partial", // ← what analysis computed (persisted)
	}

	// The webapi re-derivation disagrees with what was persisted.
	got := deriveKitchenStatus(nr)

	// KNOWN DIVERGENCE: webapi says "passed", analysis said "partial".
	if got != KitchenStatusPassed {
		t.Fatalf("expected webapi to return %q (the divergent value), got %q",
			KitchenStatusPassed, got)
	}

	// The persisted value is correct per the semantic contract.
	if nr.KitchenStatus != "partial" {
		t.Fatalf("persisted kitchen_status should be %q, got %q", "partial", nr.KitchenStatus)
	}

	// This test PASSES today documenting the divergence.
	// When Phase 2 lands (serve persisted values), this test should be updated
	// to verify the API returns nr.KitchenStatus ("partial") directly.
	t.Log("KNOWN DIVERGENCE: webapi derives 'passed' but persisted value is 'partial'")
	t.Log("Phase 2 will fix by serving persisted kitchen_status directly")
}

// TestDivergence_KitchenStatus_AllTestedPassedButBlockingExists documents
// another scenario where webapi and analysis can disagree.
//
// Scenario: All TK-eligible cookbooks passed, but one cookbook is blocking
// due to cookstyle failure (not TK). The analysis says kitchen_status="passed"
// because all TK tests passed. The webapi sees blocking entries without TK
// verdicts and says "unknown".
func TestDivergence_KitchenStatus_AllTestedPassedButBlockingExists(t *testing.T) {
	// A cookbook is blocking due to cookstyle but TK was fine for all eligible.
	blocking := []testBlocking{{
		Name:   "legacy-cookbook",
		Reason: "incompatible",
		Source: "cookstyle",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
			// No git_test_kitchen verdict — this cookbook has no test suite
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
		KitchenStatus:          "passed", // ← analysis determined all TK passed
	}

	got := deriveKitchenStatus(nr)

	// KNOWN DIVERGENCE: webapi sees blocking without TK verdicts → "unknown"
	if got != KitchenStatusUnknown {
		t.Fatalf("expected webapi to return %q (the divergent value), got %q",
			KitchenStatusUnknown, got)
	}

	if nr.KitchenStatus != "passed" {
		t.Fatalf("persisted kitchen_status should be %q, got %q", "passed", nr.KitchenStatus)
	}

	t.Log("KNOWN DIVERGENCE: webapi derives 'unknown' but persisted value is 'passed'")
	t.Log("Phase 2 will fix by serving persisted kitchen_status directly")
}
