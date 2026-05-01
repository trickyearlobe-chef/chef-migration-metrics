// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
)

func TestBuildRolePageQuery(t *testing.T) {
	t.Run("basic org filter no name default sort", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a", "org-b"},
		}
		query, args := buildRolePageQuery(f)

		if !strings.Contains(query, "COUNT(*) OVER()") {
			t.Errorf("expected COUNT(*) OVER() in query, got:\n%s", query)
		}
		if !strings.Contains(query, "SELECT DISTINCT role_name") {
			t.Errorf("expected SELECT DISTINCT role_name in query, got:\n%s", query)
		}
		if !strings.Contains(query, "organisation_name = ANY($1)") {
			t.Errorf("expected org filter $1 in query, got:\n%s", query)
		}
		if !strings.Contains(query, "ASC") {
			t.Errorf("expected ASC order in query, got:\n%s", query)
		}
		if strings.Contains(query, "LIMIT") {
			t.Errorf("expected no LIMIT clause, got:\n%s", query)
		}
		if len(args) != 1 {
			t.Errorf("expected 1 arg (org), got %d", len(args))
		}
	})

	t.Run("with name filter", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Name:              "web",
		}
		query, args := buildRolePageQuery(f)

		if !strings.Contains(query, "LOWER(role_name) LIKE") {
			t.Errorf("expected LIKE clause in query, got:\n%s", query)
		}
		if len(args) != 2 {
			t.Errorf("expected 2 args (org, name), got %d", len(args))
		}
	})

	t.Run("sort desc", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			SortOrder:         "desc",
		}
		query, _ := buildRolePageQuery(f)

		if !strings.Contains(query, "DESC") {
			t.Errorf("expected DESC in query, got:\n%s", query)
		}
	})

	t.Run("no limit", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Limit:             0,
		}
		query, _ := buildRolePageQuery(f)

		if strings.Contains(query, "LIMIT") {
			t.Errorf("expected no LIMIT clause, got:\n%s", query)
		}
	})

	t.Run("with limit and offset", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Limit:             25,
			Offset:            50,
		}
		query, args := buildRolePageQuery(f)

		if !strings.Contains(query, "LIMIT") {
			t.Errorf("expected LIMIT in query, got:\n%s", query)
		}
		if !strings.Contains(query, "OFFSET") {
			t.Errorf("expected OFFSET in query, got:\n%s", query)
		}
		// args: org($1), limit($2), offset($3)
		if len(args) != 3 {
			t.Errorf("expected 3 args (org, limit, offset), got %d", len(args))
		}
	})

	t.Run("no org filter", func(t *testing.T) {
		f := RoleFilter{Name: "base"}
		query, args := buildRolePageQuery(f)

		if strings.Contains(query, "organisation_name") {
			t.Errorf("expected no org filter when OrganisationNames empty, got:\n%s", query)
		}
		if !strings.Contains(query, "LOWER(role_name) LIKE") {
			t.Errorf("expected name LIKE filter, got:\n%s", query)
		}
		if len(args) != 1 {
			t.Errorf("expected 1 arg (name), got %d", len(args))
		}
	})
}

func TestBuildRolePageQueryDistinctCount(t *testing.T) {
	f := RoleFilter{OrganisationNames: []string{"org-a"}}
	query, _ := buildRolePageQuery(f)

	// Structure must be: outer SELECT ... FROM ( inner SELECT DISTINCT ... ) q
	fromIdx := strings.Index(query, "FROM (")
	if fromIdx == -1 {
		t.Fatalf("expected FROM ( subquery structure, got:\n%s", query)
	}
	outerPart := query[:fromIdx]
	innerPart := query[fromIdx:]

	if !strings.Contains(outerPart, "COUNT(*) OVER()") {
		t.Errorf("COUNT(*) OVER() should be in outer SELECT, outer part:\n%s", outerPart)
	}
	if !strings.Contains(innerPart, "DISTINCT") {
		t.Errorf("inner subquery should have DISTINCT, inner part:\n%s", innerPart)
	}
	if strings.Contains(innerPart, "COUNT(*) OVER()") {
		t.Errorf("COUNT(*) OVER() should not appear in inner subquery, inner part:\n%s", innerPart)
	}
}

func TestBuildRoleFilterQuerySeeded(t *testing.T) {
	seeds := []string{"role-alpha", "role-beta"}

	t.Run("with seed roles has role filter in base case", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}}
		query, _ := buildRoleFilterQuery(f, seeds)

		if !strings.Contains(query, "AND rd.role_name = ANY(") {
			t.Errorf("expected seed filter AND in base case, got:\n%s", query)
		}
	})

	t.Run("with seed roles no org uses WHERE for seed filter in base case", func(t *testing.T) {
		f := RoleFilter{}
		query, _ := buildRoleFilterQuery(f, seeds)

		if !strings.Contains(query, "WHERE rd.role_name = ANY(") {
			t.Errorf("expected seed filter WHERE in base case (no org), got:\n%s", query)
		}
	})

	t.Run("with seed roles has all_seed_roles CTE", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}}
		query, _ := buildRoleFilterQuery(f, seeds)

		if !strings.Contains(query, "all_seed_roles") {
			t.Errorf("expected all_seed_roles CTE, got:\n%s", query)
		}
	})

	t.Run("with seed roles no LIMIT OFFSET but has array_position ORDER BY", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Limit:             25,
			Offset:            50,
		}
		query, _ := buildRoleFilterQuery(f, seeds)

		if strings.Contains(query, "LIMIT") {
			t.Errorf("expected no LIMIT in seeded query, got:\n%s", query)
		}
		if strings.Contains(query, "OFFSET") {
			t.Errorf("expected no OFFSET in seeded query, got:\n%s", query)
		}
		if !strings.Contains(query, "array_position") {
			t.Errorf("expected array_position ORDER BY, got:\n%s", query)
		}
	})

	t.Run("without seed roles nil has LIMIT OFFSET and COUNT OVER", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Limit:             25,
			Offset:            50,
		}
		query, _ := buildRoleFilterQuery(f, nil)

		if !strings.Contains(query, "LIMIT") {
			t.Errorf("expected LIMIT in unseeded query, got:\n%s", query)
		}
		if !strings.Contains(query, "OFFSET") {
			t.Errorf("expected OFFSET in unseeded query, got:\n%s", query)
		}
		if !strings.Contains(query, "COUNT(*) OVER()") {
			t.Errorf("expected COUNT(*) OVER() in unseeded query, got:\n%s", query)
		}
	})

	t.Run("with seed roles direct_counts has seed filter", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}}
		query, _ := buildRoleFilterQuery(f, seeds)

		dcStart := strings.Index(query, "direct_counts AS (")
		if dcStart == -1 {
			t.Fatal("direct_counts CTE not found in query")
		}
		remainder := query[dcStart:]
		dcEnd := strings.Index(remainder, "),\n")
		if dcEnd == -1 {
			t.Fatal("could not find end of direct_counts CTE")
		}
		dcSection := remainder[:dcEnd]

		if !strings.Contains(dcSection, "role_name = ANY(") {
			t.Errorf("expected seed filter in direct_counts section:\n%s", dcSection)
		}
	})

	t.Run("with seed roles uses 0 AS total_count not COUNT OVER", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}}
		query, _ := buildRoleFilterQuery(f, seeds)

		if strings.Contains(query, "COUNT(*) OVER()") {
			t.Errorf("seeded query should not have COUNT(*) OVER(), got:\n%s", query)
		}
		if !strings.Contains(query, "0 AS total_count") {
			t.Errorf("seeded query should have 0 AS total_count, got:\n%s", query)
		}
	})

	t.Run("seeded query reuses same placeholder for seeds in all_seed_roles and ORDER BY", func(t *testing.T) {
		f := RoleFilter{OrganisationNames: []string{"org-a"}}
		query, args := buildRoleFilterQuery(f, seeds)

		// The seed arg placeholder (e.g. $2) should appear in:
		// - base case WHERE
		// - direct_counts WHERE
		// - all_seed_roles unnest(...)
		// - ORDER BY array_position(...)
		// But seeds should appear exactly ONCE in args (not re-added for each reuse).
		seedArgCount := 0
		for _, a := range args {
			if arr, ok := a.(interface{ Value() (interface{}, error) }); ok {
				_ = arr
			}
		}
		// We verify by checking args length: with org, seeds should add only 1 arg
		// Expected args: org($1), seeds($2), org($3), org($4) = 4 total (no version/name/compat)
		if len(args) != 4 {
			t.Errorf("expected 4 args (org, seeds, org for dc, org for nc), got %d: %v", len(args), args)
		}
		_ = seedArgCount
		_ = query
	})

	t.Run("unseeded query arg count with limit offset", func(t *testing.T) {
		f := RoleFilter{
			OrganisationNames: []string{"org-a"},
			Limit:             25,
			Offset:            50,
		}
		query, args := buildRoleFilterQuery(f, nil)
		// org($1), org($2), org($3), limit($4), offset($5)
		if len(args) != 5 {
			t.Errorf("expected 5 args (3x org + limit + offset), got %d", len(args))
		}
		_ = query
	})
}
