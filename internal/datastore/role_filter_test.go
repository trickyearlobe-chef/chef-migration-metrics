// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
)

// buildRoleListQuery reads the materialised role_summary table, rolls per-org
// rows up to one row per role, filters/sorts on indexed columns, and paginates
// in SQL. These tests assert the query shape.

func TestBuildRoleListQuery_ReadsRoleSummaryAndRollsUp(t *testing.T) {
	f := RoleFilter{OrganisationNames: []string{"org-a"}}
	query, _ := buildRoleListQuery(f)

	checks := []string{
		"FROM role_summary",
		"GROUP BY role_name",
		"SUM(node_count)",
		"MAX(direct_cookbook_count)",
		"MAX(transitive_cookbook_count)",
		"MAX(compatible_count)",
		"MAX(incompatible_count)",
		"MAX(untested_count)",
		"ARRAY_AGG(DISTINCT organisation_name",
		"COUNT(*) OVER() AS total_count",
	}
	for _, c := range checks {
		if !strings.Contains(query, c) {
			t.Errorf("expected %q in query, got:\n%s", c, query)
		}
	}
	// Must not fall back to the recursive transitive-dep CTE.
	if strings.Contains(query, "WITH RECURSIVE") || strings.Contains(query, "role_dependencies") {
		t.Errorf("query should read role_summary, not recompute from role_dependencies:\n%s", query)
	}
}

func TestBuildRoleListQuery_WorstOfRollups(t *testing.T) {
	query, _ := buildRoleListQuery(RoleFilter{})

	// compatibility_status worst-of: incompatible > untested > compatible.
	compatIdx := strings.Index(query, "compatibility_status = 'incompatible'")
	compatUntestedIdx := strings.Index(query, "compatibility_status = 'untested'")
	if compatIdx == -1 || compatUntestedIdx == -1 || compatIdx > compatUntestedIdx {
		t.Errorf("expected compat worst-of (incompatible before untested):\n%s", query)
	}
	// tk_status worst-of: failed > partial > passed > untested.
	for _, pair := range [][2]string{
		{"tk_status = 'failed'", "tk_status = 'partial'"},
		{"tk_status = 'partial'", "tk_status = 'passed'"},
	} {
		a, b := strings.Index(query, pair[0]), strings.Index(query, pair[1])
		if a == -1 || b == -1 || a > b {
			t.Errorf("expected tk worst-of order %s before %s:\n%s", pair[0], pair[1], query)
		}
	}
}

func TestBuildRoleListQuery_OrgAndNameFilters(t *testing.T) {
	t.Run("org + name", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a", "org-b"}, Name: "web"}
		query, args := buildRoleListQuery(f)
		if !strings.Contains(query, "organisation_name = ANY($1)") {
			t.Errorf("expected org filter, got:\n%s", query)
		}
		if !strings.Contains(query, "LOWER(role_name) LIKE") {
			t.Errorf("expected name filter, got:\n%s", query)
		}
		if len(args) != 2 {
			t.Errorf("expected 2 args (org, name), got %d", len(args))
		}
	})

	t.Run("no filters", func(t *testing.T) {
		query, args := buildRoleListQuery(RoleFilter{})
		if strings.Contains(query, "organisation_name = ANY") {
			t.Errorf("expected no org filter, got:\n%s", query)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}
	})
}

func TestBuildRoleListQuery_CompatAndTKFiltersPostRollup(t *testing.T) {
	f := RoleFilter{
		CompatibilityStatus: "incompatible",
		TKStatuses:          []string{"failed", "partial"},
	}
	query, args := buildRoleListQuery(f)

	if !strings.Contains(query, "AND compatibility_status = $1") {
		t.Errorf("expected compat filter on rolled-up status, got:\n%s", query)
	}
	if !strings.Contains(query, "AND tk_status = ANY($2)") {
		t.Errorf("expected tk filter on rolled-up status, got:\n%s", query)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args (compat, tk), got %d", len(args))
	}
}

func TestBuildRoleListQuery_CompatAllIsNoFilter(t *testing.T) {
	query, args := buildRoleListQuery(RoleFilter{CompatibilityStatus: "all"})
	if strings.Contains(query, "AND compatibility_status =") {
		t.Errorf("compatibility_status=all should not filter, got:\n%s", query)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildRoleListQuery_Sorting(t *testing.T) {
	cases := []struct {
		sort, order, wantExpr, wantDir string
	}{
		{"", "asc", "ORDER BY LOWER(role_name) ASC", "ASC"},
		{"name", "desc", "ORDER BY LOWER(role_name) DESC", "DESC"},
		{"node_count", "desc", "ORDER BY node_count DESC", "DESC"},
		{"incompatible_cookbook_count", "asc", "ORDER BY incompatible_count ASC", "ASC"},
		{"tk_status", "asc", "WHEN 'failed' THEN 0", "ASC"},
	}
	for _, c := range cases {
		f := RoleFilter{Sort: c.sort, SortOrder: c.order}
		query, _ := buildRoleListQuery(f)
		if !strings.Contains(query, c.wantExpr) {
			t.Errorf("sort=%q order=%q: expected %q in query, got:\n%s", c.sort, c.order, c.wantExpr, query)
		}
	}
}

func TestBuildRoleListQuery_Pagination(t *testing.T) {
	t.Run("with limit and offset", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}, Limit: 25, Offset: 50}
		query, args := buildRoleListQuery(f)
		if !strings.Contains(query, "LIMIT $2") {
			t.Errorf("expected LIMIT, got:\n%s", query)
		}
		if !strings.Contains(query, "OFFSET $3") {
			t.Errorf("expected OFFSET, got:\n%s", query)
		}
		// org($1), limit($2), offset($3)
		if len(args) != 3 {
			t.Errorf("expected 3 args, got %d", len(args))
		}
	})

	t.Run("no limit no offset", func(t *testing.T) {
		query, _ := buildRoleListQuery(RoleFilter{})
		if strings.Contains(query, "LIMIT") || strings.Contains(query, "OFFSET") {
			t.Errorf("expected no LIMIT/OFFSET, got:\n%s", query)
		}
	})
}
