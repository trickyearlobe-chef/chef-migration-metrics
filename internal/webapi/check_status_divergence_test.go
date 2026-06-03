// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Semantic Contract Conformance Tests — Persisted Value Precedence
//
// These tests verify that deriveCheckStatus prefers persisted status values
// (computed at analysis time with full context) over re-derivation from
// blocking cookbooks. This resolves the divergences documented in Phase 1.
// ---------------------------------------------------------------------------

// TestPersistedStatus_KitchenPartialServedDirectly verifies that when analysis
// computed kitchen_status="partial" (due to partial TK coverage), the API
// serves "partial" instead of re-deriving "passed" from blocking cookbooks.
//
// This is the fix for Bug #1: "Node readiness shows TK passes but no TK runs
// have actually passed."
func TestPersistedStatus_KitchenPartialServedDirectly(t *testing.T) {
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: true,
		BlockingCookbooks:      json.RawMessage(`[]`),
		CookstyleStatus:        "passed",
		KitchenStatus:          "partial", // persisted by analysis (has full TK coverage context)
	}

	got := deriveCheckStatus(nr, "/hab")

	if got.KitchenStatus != "partial" {
		t.Errorf("KitchenStatus = %q, want %q (persisted value)", got.KitchenStatus, "partial")
	}
	// Detail string must be consistent with status.
	if got.KitchenDetail == nil || *got.KitchenDetail != "Test Kitchen: partially tested" {
		detail := "<nil>"
		if got.KitchenDetail != nil {
			detail = *got.KitchenDetail
		}
		t.Errorf("KitchenDetail = %q, want %q", detail, "Test Kitchen: partially tested")
	}
}

// TestPersistedStatus_KitchenPassedWithBlocking verifies that when analysis
// determined kitchen_status="passed" (all TK tests passed) but blocking
// exists for cookstyle reasons, the API serves "passed".
func TestPersistedStatus_KitchenPassedWithBlocking(t *testing.T) {
	blocking := []testBlocking{{
		Name:   "legacy-cookbook",
		Reason: "incompatible",
		Source: "cookstyle",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
		KitchenStatus:          "passed", // persisted by analysis
	}

	got := deriveCheckStatus(nr, "/hab")

	if got.KitchenStatus != "passed" {
		t.Errorf("KitchenStatus = %q, want %q (persisted value)", got.KitchenStatus, "passed")
	}
	if got.KitchenDetail == nil || *got.KitchenDetail != "Test Kitchen: all passed" {
		detail := "<nil>"
		if got.KitchenDetail != nil {
			detail = *got.KitchenDetail
		}
		t.Errorf("KitchenDetail = %q, want %q", detail, "Test Kitchen: all passed")
	}
}

// TestPersistedStatus_FallbackWhenEmpty verifies that legacy records without
// persisted status values fall back to re-derivation from blocking cookbooks.
func TestPersistedStatus_FallbackWhenEmpty(t *testing.T) {
	blocking := []testBlocking{{
		Name:   "apt",
		Reason: "incompatible",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
		CookstyleStatus:        "", // legacy empty — triggers fallback
		KitchenStatus:          "", // legacy empty — triggers fallback
	}

	got := deriveCheckStatus(nr, "/hab")

	if got.CookstyleStatus != CookstyleStatusFailed {
		t.Errorf("CookstyleStatus fallback = %q, want %q", got.CookstyleStatus, CookstyleStatusFailed)
	}
	if got.KitchenStatus != KitchenStatusFailed {
		t.Errorf("KitchenStatus fallback = %q, want %q", got.KitchenStatus, KitchenStatusFailed)
	}
}

// TestPersistedStatus_CookstyleOverridesDerivation verifies cookstyle status
// is also served from persisted values.
func TestPersistedStatus_CookstyleOverridesDerivation(t *testing.T) {
	// Blocking exists with only TK failures. Re-derivation would say "passed".
	// But persisted says "unknown" (analysis had a reason we can't see from blocking alone).
	blocking := []testBlocking{{
		Name:   "web-app",
		Reason: "incompatible",
		Verdicts: []testVerdict{
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
		CookstyleStatus:        "unknown", // persisted — analysis had reason
	}

	got := deriveCheckStatus(nr, "/hab")

	if got.CookstyleStatus != "unknown" {
		t.Errorf("CookstyleStatus = %q, want %q (persisted value)", got.CookstyleStatus, "unknown")
	}
}
