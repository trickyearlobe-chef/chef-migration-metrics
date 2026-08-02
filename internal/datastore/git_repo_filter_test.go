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

func TestBuildGitRepoFilterQuery_CookstyleStatusFilter(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{CookstyleStatus: "ready,needs_review"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(q, "cookstyle_status = ANY($1)") {
		t.Errorf("missing cookstyle_status filter: %s", q)
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
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{Sort: "last_fetched"})

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

// ---------------------------------------------------------------------------
// The failure register as a filter
//
// A repo somebody has overruled still carries the materialised cookstyle and
// tk statuses that were overruled — those columns report what each tool said
// and are not rewritten. So finding the overruled ones has to go to the
// register, not to those columns.
// ---------------------------------------------------------------------------

func TestBuildGitRepoFilterQuery_HumanVerdictBroken(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{HumanVerdict: "broken"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "broken" {
		t.Errorf("expected arg 'broken', got %v", args[0])
	}
	if !strings.Contains(q, "failure_register_entries") {
		t.Errorf("the filter does not reach the register: %s", q)
	}
	// Only standing verdicts: a superseded or resolved one is history, not
	// somebody's current opinion.
	if !strings.Contains(q, "status = 'open'") {
		t.Errorf("the filter does not restrict to standing verdicts: %s", q)
	}
	// Keyed on the repo name, because repo URLs are volatile and the name is
	// the stable part.
	if !strings.Contains(q, "subject_name = git_repos.name") {
		t.Errorf("the filter does not join on the repo name: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_HumanVerdictAny(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{HumanVerdict: "any"})

	// "any" asks whether a person has an opinion at all, so it must not
	// constrain which way the verdict went.
	if len(args) != 0 {
		t.Fatalf("expected no args for 'any', got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "EXISTS") {
		t.Errorf("expected an EXISTS check: %s", q)
	}
	if strings.Contains(q, "verdict =") {
		t.Errorf("'any' must not filter on which verdict it was: %s", q)
	}
}

func TestBuildGitRepoFilterQuery_HumanVerdictNone(t *testing.T) {
	q, _ := buildGitRepoFilterQuery(GitRepoFilter{HumanVerdict: "none"})

	if !strings.Contains(q, "NOT EXISTS") {
		t.Errorf("expected a NOT EXISTS check: %s", q)
	}
}

// An unrecognised value must not silently return the whole estate as though
// no filter had been asked for.
func TestBuildGitRepoFilterQuery_HumanVerdictRejectsNonsense(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{HumanVerdict: "probably"})

	if strings.Contains(q, "failure_register_entries") {
		t.Errorf("an unrecognised verdict reached the query: %s", q)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

// The filter has to compose with the ones already there rather than replacing
// them — the git repo list is busy and people arrive at it already filtered.
func TestBuildGitRepoFilterQuery_HumanVerdictComposesWithOthers(t *testing.T) {
	q, args := buildGitRepoFilterQuery(GitRepoFilter{
		CookstyleStatus: "blocked",
		HumanVerdict:    "not_broken",
	})

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "cookstyle_status = ANY($1)") {
		t.Errorf("the cookstyle filter was lost: %s", q)
	}
	if !strings.Contains(q, "AND") {
		t.Errorf("the filters are not combined: %s", q)
	}
	// This combination is the interesting one: CookStyle says blocked and a
	// person says it is not. That is the false-positive list.
	if !strings.Contains(q, "verdict = $2") {
		t.Errorf("the verdict filter did not take the second placeholder: %s", q)
	}
}
