// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
)

func TestBuildGitRepoFilterQuery_NoFilters(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{})

	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "COUNT(*) OVER ()") {
		t.Error("query missing COUNT(*) OVER ()")
	}
	if !strings.Contains(q, "ORDER BY LOWER(name) ASC") {
		t.Errorf("query missing default ORDER BY, got: %s", q)
	}
	if strings.Contains(q, "WHERE") {
		t.Error("query should not have WHERE with no filters")
	}
	if strings.Contains(q, "LIMIT") {
		t.Error("query should not have LIMIT when Limit=0")
	}
}

func TestBuildGitRepoFilterQuery_NameFilter(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{Name: "nginx"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "%nginx%" {
		t.Errorf("expected arg %%nginx%%, got %v", args[0])
	}
	if !strings.Contains(q, "LOWER(name) LIKE LOWER($1)") {
		t.Errorf("missing name filter in query: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_CompatibilityFilter(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{CompatibilityStatus: "compatible"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "compatible" {
		t.Errorf("expected arg 'compatible', got %v", args[0])
	}
	if !strings.Contains(q, "compatibility_status = $1") {
		t.Errorf("missing compatibility filter: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_TKStatusFilter(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{TKStatus: "failed"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(q, "tk_status = $1") {
		t.Errorf("missing tk_status filter: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_CloneStatusFilter(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{CloneStatus: "ok"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(q, "clone_status = $1") {
		t.Errorf("missing clone_status filter: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_HasTestSuiteFilter(t *testing.T) {
	yes := true
	q, args := buildGitRepoFilterQuery(GitRepoFilter{HasTestSuite: &yes})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != true {
		t.Errorf("expected arg true, got %v", args[0])
	}
	if !strings.Contains(q, "has_test_suite = $1") {
		t.Errorf("missing has_test_suite filter: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_MultipleFilters(t *testing.T) {
	yes := true
	q, args := buildGitRepoFilterQuery(GitRepoFilter{
		Name:                "apache",
		CompatibilityStatus: "incompatible",
		HasTestSuite:        &yes,
	})

	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if !strings.Contains(q, "AND") {
		t.Error("multiple filters should be joined with AND")
	}
}

func TestBuildGitRepoFilterQuery_Pagination(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{
		Limit:  25,
		Offset: 50,
	})

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("missing LIMIT: %s", q)
	}
	if !strings.Contains(q, "OFFSET $2") {
		t.Errorf("missing OFFSET: %s", q)
	}
	if args[0] != 25 {
		t.Errorf("expected limit 25, got %v", args[0])
	}
	if args[1] != 50 {
		t.Errorf("expected offset 50, got %v", args[1])
	}
}

func TestBuildGitRepoFilterQuery_SortDesc(t *testing.T) {
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{
		Sort:      "tk_status",
		SortOrder: "desc",
	})

	if !strings.Contains(q, "ORDER BY tk_status DESC") {
		t.Errorf("expected tk_status DESC sort: %s", q)
	}
	// Tie-breaker should still be present.
	if !strings.Contains(q, "LOWER(name) ASC") {
		t.Errorf("missing tie-breaker: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_SortCompatibility(t *testing.T) {
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{Sort: "compatibility"})

	if !strings.Contains(q, "ORDER BY compatibility_status ASC") {
		t.Errorf("expected compatibility_status sort: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_InvalidSortFallsToDefault(t *testing.T) {
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{Sort: "bogus"})

	if !strings.Contains(q, "ORDER BY LOWER(name) ASC") {
		t.Errorf("invalid sort should fall back to name: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_TieBreaker(t *testing.T) {
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{Sort: "last_fetched_at"})

	// Should have primary sort + tie-breaker.
	idx := strings.Index(q, "ORDER BY")
	orderClause := q[idx:]
	if !strings.Contains(orderClause, "last_fetched_at ASC") {
		t.Errorf("missing primary sort: %s", orderClause)
	}
	if !strings.Contains(orderClause, "LOWER(name) ASC") {
		t.Errorf("missing tie-breaker: %s", orderClause)
	}
}

func TestBuildGitRepoFilterCountQuery(t *testing.T) {
	q, args := buildGitRepoFilterCountQuery(GitRepoFilter{
		Name:        "test",
		CloneStatus: "ok",
	})

	if !strings.Contains(q, "SELECT COUNT(*)") {
		t.Error("count query missing SELECT COUNT(*)")
	}
	if strings.Contains(q, "ORDER BY") {
		t.Error("count query should not have ORDER BY")
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}
