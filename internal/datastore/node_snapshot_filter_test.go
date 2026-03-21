// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
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
	if !strings.Contains(q, "WITH completed_nodes AS") {
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

func TestBuildNodeSnapshotFilterQuery_OrganisationIDs(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationIDs: []string{"org-1", "org-2"},
	})

	if !strings.Contains(q, "cn.organisation_id = ANY($1)") {
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
		OrganisationIDs: []string{"org-1"},
		NodeName:        "web",
		Environment:     "prod",
		Platform:        "ubuntu",
		ChefVersion:     "18",
		PolicyName:      "base",
		PolicyGroup:     "default",
		Role:            "webserver",
		Stale:           boolPtr(true),
		Limit:           25,
		Offset:          50,
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
		"organisation_id = ANY(",
		"LOWER(cn.node_name) LIKE",
		"LOWER(cn.chef_environment) LIKE",
		"LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, ''))",
		"LOWER(cn.chef_version) LIKE",
		"LOWER(cn.policy_name) LIKE",
		"LOWER(cn.policy_group) LIKE",
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
		OrganisationIDs: []string{"org-1"},
		Environment:     "prod",
		ChefVersion:     "18",
		Limit:           10,
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

func TestBuildNodeSnapshotFilterQuery_CTE_CollectionRunValidation(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{})

	// The CTE must enforce collection run completion.
	if !strings.Contains(q, "cr.status = 'completed'") {
		t.Error("CTE missing collection run status check")
	}
	if !strings.Contains(q, "SELECT MAX(cr2.started_at)") {
		t.Error("CTE missing MAX(started_at) correlated subquery")
	}
	// The correlated subquery must scope per-org.
	if !strings.Contains(q, "cr2.organisation_id = ns.organisation_id") {
		t.Error("CTE correlated subquery not scoped to organisation_id")
	}
}

func TestBuildNodeSnapshotFilterQuery_CollectionRunJoin(t *testing.T) {
	q, _ := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{})

	// Must JOIN to collection_runs.
	if !strings.Contains(q, "INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id") {
		t.Error("CTE missing collection_runs JOIN")
	}
}

// ---------------------------------------------------------------------------
// buildNodeSnapshotFilterParts — unit tests
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotFilterParts_NoFilters(t *testing.T) {
	cte, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{})

	if !strings.Contains(cte, "WITH completed_nodes AS") {
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
	_, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
		OrganisationIDs: []string{"org-1"},
		Environment:     "staging",
	})

	if !strings.Contains(where, "organisation_id = ANY($1)") {
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
	_, where, args := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
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

	if f.OrganisationIDs != nil {
		t.Error("zero-value OrganisationIDs should be nil")
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
		OrganisationIDs: []string{},
	})

	if strings.Contains(q, "organisation_id = ANY") {
		t.Error("empty org slice should not produce ANY clause")
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_SingleOrg(t *testing.T) {
	q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationIDs: []string{"org-only"},
	})

	if !strings.Contains(q, "organisation_id = ANY($1)") {
		t.Error("single org should still use ANY clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildNodeSnapshotFilterQuery_MultipleOrgs(t *testing.T) {
	_, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{
		OrganisationIDs: []string{"org-1", "org-2", "org-3"},
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
		{OrganisationIDs: []string{"org-1"}, Environment: "prod"},
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
	_, where, _ := buildNodeSnapshotFilterParts(NodeSnapshotFilter{})
	if !strings.HasPrefix(where, " WHERE 1=1") {
		t.Errorf("where should start with \" WHERE 1=1\", got %q", where)
	}

	_, where2, _ := buildNodeSnapshotFilterParts(NodeSnapshotFilter{
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
		{OrganisationIDs: []string{"org-1"}, Stale: boolPtr(true)},
	}

	for i, f := range tests {
		q, _ := buildNodeSnapshotFilterQuery(f)
		if !strings.Contains(q, "COUNT(*) OVER () AS total_count") {
			t.Errorf("test[%d]: query missing COUNT(*) OVER () AS total_count", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractOuterSelect returns the portion of the query between the last
// "SELECT" and "FROM completed_nodes" — i.e. the outer SELECT column list.
func extractOuterSelect(query string) string {
	// Find the last SELECT (the outer one, after the CTE).
	lastSelect := strings.LastIndex(query, "SELECT ")
	if lastSelect == -1 {
		return ""
	}
	fromIdx := strings.Index(query[lastSelect:], "FROM completed_nodes")
	if fromIdx == -1 {
		return query[lastSelect:]
	}
	return query[lastSelect : lastSelect+fromIdx]
}

// extractWhere returns the WHERE clause portion from the outer query
// (after "FROM completed_nodes").
func extractWhere(query string) string {
	fromIdx := strings.Index(query, "FROM completed_nodes cn")
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
