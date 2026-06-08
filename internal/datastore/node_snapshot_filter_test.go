// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// boolPtr is a helper to create a *bool for test cases.
func boolPtr(b bool) *bool { return &b }

// ---------------------------------------------------------------------------
// buildNodeSnapshotFilterQuery — unit tests
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterQuery_NoFilters(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{})

	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
	// Should contain the CTE.
	if !strings.Contains(q, "WITH current_nodes AS") {
		t.Error("query missing CTE")
	}
	// Should contain COUNT(*) OVER() for pagination total.
	if !strings.Contains(q, "COUNT(*) OVER ()") {
		t.Error("query missing COUNT(*) OVER ()")
	}
	// Should have ORDER BY.
	if !strings.Contains(q, "ORDER BY cn.node_name") {
		t.Error("query missing ORDER BY")
	}
	// Should NOT contain LIMIT or OFFSET when both are 0.
	if strings.Contains(q, "LIMIT") {
		t.Error("query should not contain LIMIT when Limit=0")
	}
	if strings.Contains(q, "OFFSET") {
		t.Error("query should not contain OFFSET when Offset=0")
	}
	// Lightweight projection: should NOT include filesystem, cookbooks,
	// custom_attributes in the outer SELECT columns.
	// The CTE always includes them, but the outer SELECT should not.
	outerSelect := extractOuterSelect(q)
	if strings.Contains(outerSelect, "cn.filesystem") {
		t.Error("lightweight query should not SELECT cn.filesystem")
	}
	if strings.Contains(outerSelect, "cn.cookbooks") {
		t.Error("lightweight query should not SELECT cn.cookbooks")
	}
	if strings.Contains(outerSelect, "cn.custom_attributes") {
		t.Error("lightweight query should not SELECT cn.custom_attributes")
	}
}

func TestBuildNodeSnapshotFilterQuery_HeavyJSON(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		IncludeHeavyJSON: true,
	})

	outerSelect := extractOuterSelect(q)
	if !strings.Contains(outerSelect, "cn.filesystem") {
		t.Error("heavy query should SELECT cn.filesystem")
	}
	if !strings.Contains(outerSelect, "cn.cookbooks") {
		t.Error("heavy query should SELECT cn.cookbooks")
	}
	if !strings.Contains(outerSelect, "cn.custom_attributes") {
		t.Error("heavy query should SELECT cn.custom_attributes")
	}
}

func TestBuildNodeSnapshotFilterQuery_OrganisationNames(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-1", "org-2"},
	})

	if !strings.Contains(q, "cn.organisation_name = ANY($1)") {
		t.Errorf("query missing org filter clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_NodeName(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		NodeName: "web01",
	})

	if !strings.Contains(q, "LOWER(cn.node_name) LIKE") {
		t.Errorf("query missing node_name filter clause")
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "web01" {
		t.Errorf("arg[0] = %v, want web01", args[0])
	}
}

func TestBuildNodeSnapshotFilterQuery_Environment(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Environment: "production",
	})

	if !strings.Contains(q, "LOWER(cn.chef_environment) LIKE") {
		t.Errorf("query missing environment filter clause")
	}
	if len(args) != 1 || args[0] != "production" {
		t.Errorf("args = %v, want [production]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_Platform(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Platform: "ubuntu 22",
	})

	if !strings.Contains(q, "LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, ''))") {
		t.Errorf("query missing combined platform filter clause")
	}
	if len(args) != 1 || args[0] != "ubuntu 22" {
		t.Errorf("args = %v, want [ubuntu 22]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_PlatformUnknown(t *testing.T) {
	// When platform filter is "unknown", the query should match nodes
	// with empty or NULL platform — not nodes whose platform literally
	// contains "unknown". The dashboard platform distribution uses
	// CASE WHEN cn.platform IS NULL OR cn.platform = '' THEN 'unknown'
	// to label these nodes, so the filter must reverse that mapping.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Platform: "unknown",
	})

	if !strings.Contains(q, "platform IS NULL OR cn.platform = ''") {
		t.Errorf("query should match NULL/empty platform for 'unknown' filter, got:\n%s", q)
	}
	// Should NOT add a positional parameter for "unknown".
	if len(args) != 0 {
		t.Errorf("args = %v, want [] (no positional params for unknown filter)", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_PlatformUnknown_CaseInsensitive(t *testing.T) {
	// "Unknown" (mixed case) should also trigger the NULL/empty match.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Platform: "Unknown",
	})

	if !strings.Contains(q, "platform IS NULL OR cn.platform = ''") {
		t.Errorf("query should match NULL/empty platform for 'Unknown' filter, got:\n%s", q)
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want [] (no positional params for Unknown filter)", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_ChefVersion(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ChefVersion: "18.0",
	})

	if !strings.Contains(q, "LOWER(cn.chef_version) LIKE LOWER(") {
		t.Errorf("query missing chef_version prefix filter clause")
	}
	// Must be a prefix match (no leading wildcard) so "17" doesn't match "13.17.4".
	if strings.Contains(q, "'%' || LOWER(cn.chef_version)") || strings.Contains(q, "LIKE '%' || LOWER($") {
		t.Errorf("chef_version filter should use prefix match, not substring match")
	}
	if len(args) != 1 || args[0] != "18.0" {
		t.Errorf("args = %v, want [18.0]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_ChefVersionUnknown(t *testing.T) {
	// When chef_version filter is "unknown", the query should match nodes
	// with empty or NULL chef_version — not nodes whose chef_version
	// literally starts with "unknown". The dashboard version distribution
	// uses COALESCE(NULLIF(chef_version, ''), 'unknown') to label these
	// nodes, so the filter must reverse that mapping.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ChefVersion: "unknown",
	})

	if !strings.Contains(q, "chef_version IS NULL OR cn.chef_version = ''") {
		t.Errorf("query should match NULL/empty chef_version for 'unknown' filter, got:\n%s", q)
	}
	// Should NOT add a positional parameter for "unknown".
	if len(args) != 0 {
		t.Errorf("args = %v, want [] (no positional params for unknown filter)", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_ChefVersionExact(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ChefVersionExact: "18.0.92",
	})

	if !strings.Contains(q, "cn.chef_version = $1") {
		t.Errorf("query missing chef_version exact match clause, got:\n%s", q)
	}
	// Should NOT contain LIKE.
	if strings.Contains(q, "LOWER(cn.chef_version) LIKE") {
		t.Error("exact version should not use LIKE")
	}
	if len(args) != 1 || args[0] != "18.0.92" {
		t.Errorf("args = %v, want [18.0.92]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_ChefVersionExact_PrecedesSubstring(t *testing.T) {
	// When both ChefVersionExact and ChefVersion are set, exact wins.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ChefVersion:      "18",
		ChefVersionExact: "18.0.92",
	})

	if !strings.Contains(q, "cn.chef_version = $1") {
		t.Errorf("query should use exact match when both are set")
	}
	if strings.Contains(q, "LOWER(cn.chef_version) LIKE") {
		t.Error("query should not use LIKE when exact is set")
	}
	if len(args) != 1 || args[0] != "18.0.92" {
		t.Errorf("args = %v, want [18.0.92]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_PolicyName(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		PolicyName: "base",
	})

	if !strings.Contains(q, "LOWER(cn.policy_name) LIKE") {
		t.Errorf("query missing policy_name filter clause")
	}
	if len(args) != 1 || args[0] != "base" {
		t.Errorf("args = %v, want [base]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_PolicyGroup(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		PolicyGroup: "staging",
	})

	if !strings.Contains(q, "LOWER(cn.policy_group) LIKE") {
		t.Errorf("query missing policy_group filter clause")
	}
	if len(args) != 1 || args[0] != "staging" {
		t.Errorf("args = %v, want [staging]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_Role(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Role: "webserver",
	})

	if !strings.Contains(q, "jsonb_typeof(cn.roles) = 'array'") {
		t.Errorf("query missing jsonb_typeof guard for roles")
	}
	if !strings.Contains(q, "EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles)") {
		t.Errorf("query missing role EXISTS subquery")
	}
	if !strings.Contains(q, "LOWER(r) LIKE") {
		t.Errorf("query missing role LOWER LIKE clause")
	}
	if len(args) != 1 || args[0] != "webserver" {
		t.Errorf("args = %v, want [webserver]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_RolesExact(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Roles: []string{"admin", "webserver"},
	})

	if !strings.Contains(q, "jsonb_typeof(cn.roles) = 'array'") {
		t.Errorf("query missing jsonb_typeof guard for roles")
	}
	if !strings.Contains(q, "EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles) r WHERE r = ANY(") {
		t.Errorf("query missing role exact-match ANY clause, got:\n%s", q)
	}
	// Should NOT contain LIKE (that's the substring path).
	if strings.Contains(q, "LIKE") {
		t.Errorf("exact-match Roles should not use LIKE, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_RolesExactTakesPrecedence(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Role:  "admin",
		Roles: []string{"admin"},
	})

	// Roles (exact) should take precedence — no LIKE clause.
	if strings.Contains(q, "LIKE") {
		t.Errorf("Roles should take precedence over Role (substring), got:\n%s", q)
	}
	if !strings.Contains(q, "r = ANY(") {
		t.Errorf("expected exact-match ANY clause, got:\n%s", q)
	}
}

func TestBuildNodeSnapshotFilterQuery_StaleTrue(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Stale: boolPtr(true),
	})

	if !strings.Contains(q, "cn.is_stale = $1") {
		t.Errorf("query missing is_stale filter clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != true {
		t.Errorf("arg[0] = %v, want true", args[0])
	}
}

func TestBuildNodeSnapshotFilterQuery_StaleFalse(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Stale: boolPtr(false),
	})

	if !strings.Contains(q, "cn.is_stale = $1") {
		t.Errorf("query missing is_stale filter clause")
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != false {
		t.Errorf("arg[0] = %v, want false", args[0])
	}
}

func TestBuildNodeSnapshotFilterQuery_StaleNil(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		// Stale is nil by default — no filter.
	})

	if strings.Contains(q, "is_stale") {
		// The CTE contains is_stale in the column list, so check WHERE clause specifically.
		where := extractWhere(q)
		if strings.Contains(where, "is_stale") {
			t.Error("query should not filter on is_stale when Stale is nil")
		}
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_StaleTier(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		StaleTiers:        []string{"stale"},
		StaleWarningHours: 24,
		StaleCriticalDays: 7,
	})

	where := extractWhere(q)
	if !strings.Contains(where, "ohai_time <=") {
		t.Errorf("stale tier should filter on ohai_time, got WHERE:\n%s", where)
	}
	if strings.Contains(where, "is_stale") {
		t.Error("stale tier should not use is_stale boolean filter")
	}
}

func TestBuildNodeSnapshotFilterQuery_StaleTierMulti(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		StaleTiers:        []string{"warning", "critical"},
		StaleWarningHours: 24,
		StaleCriticalDays: 7,
	})

	where := extractWhere(q)
	if !strings.Contains(where, " OR ") {
		t.Errorf("multi-tier filter should use OR, got WHERE:\n%s", where)
	}
	if strings.Contains(where, "is_stale") {
		t.Error("stale tier should not use is_stale boolean filter")
	}
}

func TestBuildNodeSnapshotFilterQuery_Pagination(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Limit:  25,
		Offset: 50,
	})

	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("query missing LIMIT clause, got:\n%s", q)
	}
	if !strings.Contains(q, "OFFSET $2") {
		t.Errorf("query missing OFFSET clause, got:\n%s", q)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != 25 {
		t.Errorf("args[0] = %v, want 25", args[0])
	}
	if args[1] != 50 {
		t.Errorf("args[1] = %v, want 50", args[1])
	}
}

func TestBuildNodeSnapshotFilterQuery_LimitOnly(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Limit: 10,
	})

	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("query missing LIMIT clause")
	}
	if strings.Contains(q, "OFFSET") {
		t.Error("query should not contain OFFSET when Offset=0")
	}
	if len(args) != 1 || args[0] != 10 {
		t.Errorf("args = %v, want [10]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_OffsetOnly(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Offset: 20,
	})

	// OFFSET without LIMIT should still be emitted.
	if !strings.Contains(q, "OFFSET $1") {
		t.Errorf("query missing OFFSET clause, got:\n%s", q)
	}
	// LIMIT should not be present.
	if strings.Contains(q, "LIMIT") {
		t.Error("query should not contain LIMIT when Limit=0")
	}
	if len(args) != 1 || args[0] != 20 {
		t.Errorf("args = %v, want [20]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_AllFilters(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-1"},
		NodeName:          "web",
		Environment:       "prod",
		Platform:          "ubuntu",
		ChefVersion:       "18",
		PolicyName:        "base",
		PolicyGroup:       "default",
		Role:              "webserver",
		Stale:             boolPtr(true),
		Limit:             25,
		Offset:            50,
	})

	// 9 filter args + 2 pagination args = 11 total.
	expectedArgs := 11
	if len(args) != expectedArgs {
		t.Errorf("expected %d args, got %d: %v", expectedArgs, len(args), args)
	}

	// Verify parameter numbering is sequential.
	for i := 1; i <= expectedArgs; i++ {
		placeholder := strings.Replace("$N", "N", strings.Repeat("", 0)+intToStr(i), 1)
		if !strings.Contains(q, placeholder) {
			t.Errorf("query missing placeholder %s", placeholder)
		}
	}

	// Spot-check that all filter clauses are present.
	checks := []string{
		"organisation_name = ANY(",
		"LOWER(cn.node_name) LIKE",
		"LOWER(cn.chef_environment) LIKE",
		"LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, ''))",
		"LOWER(cn.chef_version) LIKE",
		"LOWER(cn.policy_name) LIKE",
		"LOWER(cn.policy_group) LIKE",
		"jsonb_typeof(cn.roles) = 'array'",
		"jsonb_array_elements_text(cn.roles)",
		"cn.is_stale =",
		"LIMIT",
		"OFFSET",
	}
	for _, check := range checks {
		if !strings.Contains(q, check) {
			t.Errorf("query missing expected clause %q", check)
		}
	}
}

func TestBuildNodeSnapshotFilterQuery_ParameterNumbering(t *testing.T) {
	// Verify that parameter numbers are sequential and correctly ordered
	// when multiple filters are used.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-1"},
		Environment:       "prod",
		ChefVersion:       "18",
		Limit:             10,
	})

	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}

	// $1 = org IDs, $2 = environment, $3 = chef_version, $4 = limit
	if !strings.Contains(q, "ANY($1)") {
		t.Error("org filter should use $1")
	}
	if !strings.Contains(q, "LOWER($2)") {
		t.Error("environment filter should use $2")
	}
	if !strings.Contains(q, "LOWER($3)") {
		t.Error("chef_version filter should use $3")
	}
	if !strings.Contains(q, "LIMIT $4") {
		t.Error("LIMIT should use $4")
	}
}

func TestBuildNodeSnapshotFilterQuery_CTE_NoCollectionRunGating(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{})

	// The CTE must NOT gate on collection run status — node snapshots are
	// upserted in place and valid regardless of run outcome.
	if strings.Contains(q, "cr.status = 'completed'") {
		t.Error("CTE should not gate on collection run status")
	}
	if strings.Contains(q, "SELECT MAX(cr2.started_at)") {
		t.Error("CTE should not contain MAX(started_at) subquery")
	}
	if strings.Contains(q, "INNER JOIN collection_runs") {
		t.Error("CTE should not JOIN collection_runs")
	}
}

func TestBuildNodeSnapshotFilterQuery_NoCollectionRunJoin(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{})

	// Must NOT JOIN to collection_runs — snapshots are valid once upserted.
	if strings.Contains(q, "INNER JOIN collection_runs") {
		t.Error("CTE should not JOIN collection_runs")
	}
}

// ---------------------------------------------------------------------------
// buildNodeSnapshotFilterParts — unit tests
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterParts_NoFilters(t *testing.T) {
	cte, _, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{})

	if !strings.Contains(cte, "WITH current_nodes AS") {
		t.Error("CTE missing")
	}
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want \" WHERE 1=1\"", where)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterParts_WithFilters(t *testing.T) {
	_, _, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
		OrganisationNames: []string{"org-1"},
		Environment:       "staging",
	})

	if !strings.Contains(where, "organisation_name = ANY($1)") {
		t.Error("where missing org filter")
	}
	if !strings.Contains(where, "LOWER(cn.chef_environment) LIKE") {
		t.Error("where missing environment filter")
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterParts_NoPagination(t *testing.T) {
	// buildNodeSnapshotFilterParts should NOT include LIMIT/OFFSET
	// because those are only relevant for the list query, not aggregates.
	_, _, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
		Limit:  25,
		Offset: 50,
	})

	if strings.Contains(where, "LIMIT") {
		t.Error("filter parts should not include LIMIT")
	}
	if strings.Contains(where, "OFFSET") {
		t.Error("filter parts should not include OFFSET")
	}
	// Limit and Offset should not produce args.
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
}

// ---------------------------------------------------------------------------
// NodeSnapshotFilter struct tests
// ---------------------------------------------------------------------------

func TestNodeSnapshotFilter_ZeroValue(t *testing.T) {
	var f NodeSnapshotFilter

	if f.OrganisationNames != nil {
		t.Error("zero-value OrganisationNames should be nil")
	}
	if f.NodeName != "" {
		t.Error("zero-value NodeName should be empty")
	}
	if f.Stale != nil {
		t.Error("zero-value Stale should be nil")
	}
	if f.IncludeHeavyJSON {
		t.Error("zero-value IncludeHeavyJSON should be false")
	}
	if f.Limit != 0 {
		t.Error("zero-value Limit should be 0")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterQuery_EmptyOrgSlice(t *testing.T) {
	// Empty slice should be treated as "no filter" (all orgs).
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{},
	})

	if strings.Contains(q, "organisation_name = ANY") {
		t.Error("empty org slice should not produce ANY clause")
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_SingleOrg(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-only"},
	})

	if !strings.Contains(q, "organisation_name = ANY($1)") {
		t.Error("single org should still use ANY clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_MultipleOrgs(t *testing.T) {
	_, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-1", "org-2", "org-3"},
	})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_OrderByAlwaysPresent(t *testing.T) {
	// ORDER BY should always be present regardless of filters.
	tests := []NodeSnapshotFilter{
		{},
		{NodeName: "test"},
		{Limit: 10, Offset: 5},
		{OrganisationNames: []string{"org-1"}, Environment: "prod"},
	}

	for i, f := range tests {
		q, _ := buildNodeSnapshotFilterQuery(f)
		if !strings.Contains(q, "ORDER BY cn.node_name") {
			t.Errorf("test[%d]: query missing ORDER BY", i)
		}
	}
}

func TestBuildNodeSnapshotFilterQuery_LimitBeforeOffset(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Limit:  10,
		Offset: 20,
	})

	limitIdx := strings.Index(q, "LIMIT")
	offsetIdx := strings.Index(q, "OFFSET")

	if limitIdx == -1 || offsetIdx == -1 {
		t.Fatal("query missing LIMIT or OFFSET")
	}
	if limitIdx >= offsetIdx {
		t.Error("LIMIT should appear before OFFSET in the query")
	}
}

func TestBuildNodeSnapshotFilterQuery_WhereAlwaysStartsWith1Eq1(t *testing.T) {
	// The WHERE clause (in both buildNodeSnapshotFilterQuery and
	// buildNodeSnapshotFilterParts) always starts with "WHERE 1=1" so
	// that additional AND clauses can be appended safely.
	_, _, where, _ := buildNodeSnapshotFilterParts(NodeSnapshotFilter{})
	if !strings.HasPrefix(where, " WHERE 1=1") {
		t.Errorf("where should start with \" WHERE 1=1\", got %q", where)
	}

	_, _, where2, _ := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
		Environment: "prod",
	})
	if !strings.HasPrefix(where2, " WHERE 1=1") {
		t.Errorf("where should start with \" WHERE 1=1\", got %q", where2)
	}
}

func TestBuildNodeSnapshotFilterQuery_CountOverAlwaysPresent(t *testing.T) {
	tests := []NodeSnapshotFilter{
		{},
		{NodeName: "test", Limit: 10},
		{OrganisationNames: []string{"org-1"}, Stale: boolPtr(true)},
	}

	for i, f := range tests {
		q, _ := buildNodeSnapshotFilterQuery(f)
		if !strings.Contains(q, "COUNT(*) OVER () AS total_count") {
			t.Errorf("test[%d]: query missing COUNT(*) OVER () AS total_count", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Readiness filter push-down tests
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterQuery_ReadinessReady(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "ready",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query missing LEFT JOIN node_readiness")
	}
	if !strings.Contains(q, "nr.target_chef_version") {
		t.Error("query missing nr.target_chef_version")
	}
	if !strings.Contains(q, "nr.is_ready = true") {
		t.Error("query missing nr.is_ready = true")
	}
	found := false
	for _, a := range args {
		if a == "18.5.0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("args should contain 18.5.0, got %v", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessBlocked(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "blocked",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query missing LEFT JOIN node_readiness")
	}
	if !strings.Contains(q, "nr.is_ready = false OR nr.is_ready IS NULL") {
		t.Error("query missing blocked condition (nr.is_ready = false OR nr.is_ready IS NULL)")
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessCookbooksBlocked(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "cookbooks_blocked",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query missing LEFT JOIN node_readiness")
	}
	if !strings.Contains(q, "nr.all_cookbooks_compatible = false OR nr.all_cookbooks_compatible IS NULL") {
		t.Error("query missing cookbooks_blocked condition")
	}
}

// Disk readiness is version-invariant: the verdict is computed from platform
// install size and the node's free space, identical across every target-version
// row. The disk filters must therefore resolve against ANY readiness row for
// the node (a correlated EXISTS), NOT the version-scoped `nr` JOIN alias, so the
// list view agrees with the detail view regardless of the selected target.
func TestBuildNodeSnapshotFilterQuery_ReadinessDiskBlocked(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "disk_blocked",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "EXISTS (SELECT 1 FROM node_readiness nrd") {
		t.Error("query missing version-agnostic EXISTS subquery for disk_blocked")
	}
	if !strings.Contains(q, "nrd.sufficient_disk_space = false") {
		t.Error("query missing nrd.sufficient_disk_space = false")
	}
	// Disk filter must not be tied to the version-scoped alias.
	if strings.Contains(q, "nr.sufficient_disk_space") {
		t.Error("disk filter must not reference the version-scoped nr alias")
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessDiskUnknown(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "disk_unknown",
		TargetChefVersion: "18.5.0",
	})

	// "Unknown" = the node has no determinate disk verdict in ANY readiness row
	// (covers both "no rows at all" and "rows present but disk indeterminate").
	if !strings.Contains(q, "NOT EXISTS (SELECT 1 FROM node_readiness nrd") {
		t.Error("query missing version-agnostic NOT EXISTS subquery for disk_unknown")
	}
	if !strings.Contains(q, "nrd.sufficient_disk_space IS NOT NULL") {
		t.Error("query missing nrd.sufficient_disk_space IS NOT NULL")
	}
	if strings.Contains(q, "nr.sufficient_disk_space") {
		t.Error("disk filter must not reference the version-scoped nr alias")
	}
}

// Disk filters are version-agnostic and must work even when no target Chef
// version is selected (the LEFT JOIN is absent in that case).
func TestBuildNodeSnapshotFilterQuery_ReadinessDiskBlocked_NoTargetVersion(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "disk_blocked",
		TargetChefVersion: "",
	})

	if strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query should NOT join the version-scoped nr alias when no target version is set")
	}
	if !strings.Contains(q, "EXISTS (SELECT 1 FROM node_readiness nrd") {
		t.Error("disk_blocked must still apply (via EXISTS) without a target version")
	}
	if !strings.Contains(q, "nrd.sufficient_disk_space = false") {
		t.Error("query missing nrd.sufficient_disk_space = false")
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessDiskUnknown_NoTargetVersion(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "disk_unknown",
		TargetChefVersion: "",
	})

	if strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query should NOT join the version-scoped nr alias when no target version is set")
	}
	if !strings.Contains(q, "NOT EXISTS (SELECT 1 FROM node_readiness nrd") {
		t.Error("disk_unknown must still apply (via NOT EXISTS) without a target version")
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessWithoutTargetVersion(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "ready",
		TargetChefVersion: "",
	})

	if strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query should NOT join node_readiness when TargetChefVersion is empty")
	}
	if strings.Contains(q, "nr.is_ready") {
		t.Error("query should NOT contain readiness WHERE clause when TargetChefVersion is empty")
	}
}

func TestBuildNodeSnapshotFilterQuery_TargetVersionOnly(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query should join node_readiness when TargetChefVersion is set (for response enrichment)")
	}
	found := false
	for _, a := range args {
		if a == "18.5.0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("args should contain 18.5.0, got %v", args)
	}
	// No readiness WHERE filter should be applied.
	if strings.Contains(q, "nr.is_ready") {
		t.Error("query should NOT have readiness WHERE clause when ReadinessFilter is empty")
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadinessWithOtherFilters(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadinessFilter:   "ready",
		TargetChefVersion: "18.5.0",
		Environment:       "production",
	})

	// Readiness JOIN and WHERE must be present.
	if !strings.Contains(q, "LEFT JOIN node_readiness") {
		t.Error("query missing LEFT JOIN node_readiness")
	}
	if !strings.Contains(q, "nr.is_ready = true") {
		t.Error("query missing nr.is_ready = true")
	}

	// Environment filter must be present.
	if !strings.Contains(q, "LOWER(cn.chef_environment) LIKE") {
		t.Error("query missing environment filter clause")
	}

	// Verify both args are present and parameter numbering is correct.
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args, got %d: %v", len(args), args)
	}

	// Find positions of target_chef_version and environment in args.
	versionIdx := -1
	envIdx := -1
	for i, a := range args {
		if a == "18.5.0" {
			versionIdx = i
		}
		if a == "production" {
			envIdx = i
		}
	}
	if versionIdx == -1 {
		t.Errorf("args missing 18.5.0: %v", args)
	}
	if envIdx == -1 {
		t.Errorf("args missing production: %v", args)
	}
	if versionIdx != -1 && envIdx != -1 && versionIdx == envIdx {
		t.Error("target_chef_version and environment should be separate args")
	}
}

// ---------------------------------------------------------------------------
// Multi-value filter tests ([]string slice fields with ANY($N) SQL)
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterQuery_MultiEnvironments(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Environments: []string{"prod", "staging"},
	})

	if !strings.Contains(q, "chef_environment = ANY(") {
		t.Errorf("query missing chef_environment = ANY clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d: %v", len(args), args)
	}
	// Verify the arg is a pq.Array wrapping the expected slice.
	expected := fmt.Sprintf("%v", pq.Array([]string{"prod", "staging"}))
	got := fmt.Sprintf("%v", args[0])
	if got != expected {
		t.Errorf("args[0] = %v, want %v", got, expected)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiPlatforms(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Platforms: []string{"centos 7", "ubuntu 20.04"},
	})

	if !strings.Contains(q, "= ANY(") {
		t.Errorf("query missing ANY clause, got:\n%s", q)
	}
	if !strings.Contains(q, "cn.platform || ' ' || COALESCE(cn.platform_version") {
		t.Errorf("query missing platform concatenation, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d: %v", len(args), args)
	}
	expected := fmt.Sprintf("%v", pq.Array([]string{"centos 7", "ubuntu 20.04"}))
	got := fmt.Sprintf("%v", args[0])
	if got != expected {
		t.Errorf("args[0] = %v, want %v", got, expected)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiChefVersions(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ChefVersions: []string{"18.5.0", "17.10.0"},
	})

	if !strings.Contains(q, "chef_version = ANY(") {
		t.Errorf("query missing chef_version = ANY clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d: %v", len(args), args)
	}
	expected := fmt.Sprintf("%v", pq.Array([]string{"18.5.0", "17.10.0"}))
	got := fmt.Sprintf("%v", args[0])
	if got != expected {
		t.Errorf("args[0] = %v, want %v", got, expected)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiPolicyNames(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		PolicyNames: []string{"base", "web"},
	})

	if !strings.Contains(q, "policy_name = ANY(") {
		t.Errorf("query missing policy_name = ANY clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d: %v", len(args), args)
	}
	expected := fmt.Sprintf("%v", pq.Array([]string{"base", "web"}))
	got := fmt.Sprintf("%v", args[0])
	if got != expected {
		t.Errorf("args[0] = %v, want %v", got, expected)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiPolicyGroups(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		PolicyGroups: []string{"prod", "staging"},
	})

	if !strings.Contains(q, "policy_group = ANY(") {
		t.Errorf("query missing policy_group = ANY clause, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg (pq.Array), got %d: %v", len(args), args)
	}
	expected := fmt.Sprintf("%v", pq.Array([]string{"prod", "staging"}))
	got := fmt.Sprintf("%v", args[0])
	if got != expected {
		t.Errorf("args[0] = %v, want %v", got, expected)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiEnvironments_OverridesSubstring(t *testing.T) {
	// When both Environment (substring) and Environments (slice) are set,
	// the slice takes precedence — no LIKE clause for chef_environment.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Environment:  "prod",
		Environments: []string{"staging"},
	})

	if strings.Contains(q, "LOWER(cn.chef_environment) LIKE") {
		t.Error("query should NOT contain LIKE for chef_environment when Environments slice is set")
	}
	if !strings.Contains(q, "chef_environment = ANY(") {
		t.Error("query missing chef_environment = ANY clause")
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
}

func TestBuildNodeSnapshotFilterQuery_SingleEnvironment_PreservesSubstring(t *testing.T) {
	// When only Environment (substring) is set with no Environments slice,
	// existing LIKE behaviour is preserved.
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Environment: "prod",
	})

	if !strings.Contains(q, "LOWER(cn.chef_environment) LIKE") {
		t.Error("query should contain LIKE for chef_environment when only substring filter is set")
	}
	if strings.Contains(q, "chef_environment = ANY(") {
		t.Error("query should NOT contain ANY when Environments slice is empty")
	}
	if len(args) != 1 || args[0] != "prod" {
		t.Errorf("args = %v, want [prod]", args)
	}
}

func TestBuildNodeSnapshotFilterQuery_MultiValueCombined(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Environments: []string{"prod"},
		Platforms:    []string{"centos 7"},
		ChefVersions: []string{"18.5.0"},
	})

	if !strings.Contains(q, "chef_environment = ANY(") {
		t.Error("query missing chef_environment = ANY clause")
	}
	if !strings.Contains(q, "cn.platform || ' ' || COALESCE(cn.platform_version") {
		t.Error("query missing platform concatenation ANY clause")
	}
	if !strings.Contains(q, "chef_version = ANY(") {
		t.Error("query missing chef_version = ANY clause")
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args (one pq.Array per filter), got %d: %v", len(args), args)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractOuterSelect returns the portion of the query between the last
// "SELECT" and "FROM current_nodes" — i.e. the outer SELECT column list.
func extractOuterSelect(query string) string {
	// Find the last SELECT (the outer one, after the CTE).
	lastSelect := strings.LastIndex(query, "SELECT ")
	if lastSelect == -1 {
		return ""
	}
	fromIdx := strings.Index(query[lastSelect:], "FROM current_nodes")
	if fromIdx == -1 {
		return query[lastSelect:]
	}
	return query[lastSelect : lastSelect+fromIdx]
}

// extractWhere returns the WHERE clause portion from the outer query
// (after "FROM current_nodes").
func extractWhere(query string) string {
	fromIdx := strings.Index(query, "FROM current_nodes cn")
	if fromIdx == -1 {
		return ""
	}
	rest := query[fromIdx:]
	whereIdx := strings.Index(rest, "WHERE")
	if whereIdx == -1 {
		return ""
	}
	orderIdx := strings.Index(rest, "ORDER BY")
	if orderIdx == -1 {
		return rest[whereIdx:]
	}
	return rest[whereIdx:orderIdx]
}

// intToStr converts an int to its string representation without importing
// strconv (keeping test dependencies minimal within the package).
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// ---------------------------------------------------------------------------
// Parallel deployment tracking filter tests
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterQuery_MigrationStates(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		MigrationStates: []string{"hab_dormant", "hab_active"},
	})
	if !strings.Contains(q, "cn.migration_state = ANY(") {
		t.Error("query missing migration_state ANY clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_TargetConvergeStatuses(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		TargetConvergeStatuses: []string{"success"},
	})
	if !strings.Contains(q, "cn.target_converge_status = ANY(") {
		t.Error("query missing target_converge_status ANY clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_ReadyToActivate(t *testing.T) {
	v := true
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		ReadyToActivate: &v,
	})
	if !strings.Contains(q, "cn.migration_state = 'hab_dormant'") {
		t.Error("query missing hab_dormant condition for ready_to_activate")
	}
	if !strings.Contains(q, "cn.target_converge_status = 'success'") {
		t.Error("query missing target_converge_status = 'success' condition for ready_to_activate")
	}
	// ready_to_activate uses literal values, not args.
	if len(args) != 0 {
		t.Errorf("expected 0 args for ready_to_activate, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_SortMigrationState(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		Sort: "migration_state",
	})
	if !strings.Contains(q, "ORDER BY cn.migration_state") {
		t.Error("query should sort by cn.migration_state")
	}
}
