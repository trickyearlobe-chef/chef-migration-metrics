// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
	"time"
)

func TestBuildConvergeRunFilterQuery_NoFilters(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{})

	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "COUNT(*) OVER ()") {
		t.Error("query missing COUNT(*) OVER ()")
	}
	if !strings.Contains(q, "ORDER BY end_time DESC") {
		t.Errorf("query missing default ORDER BY end_time DESC, got: %s", q)
	}
	if strings.Contains(q, "WHERE") {
		t.Error("query should not have WHERE with no filters")
	}
	if strings.Contains(q, "LIMIT") {
		t.Error("query should not have LIMIT when Limit=0")
	}
}

func TestBuildConvergeRunFilterQuery_OrganisationFilter(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{Organisation: "org-a"})

	if len(args) != 1 || args[0] != "org-a" {
		t.Fatalf("expected 1 arg 'org-a', got %v", args)
	}
	if !strings.Contains(q, "organisation = $1") {
		t.Errorf("missing organisation filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_StatusMultiSelect(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{Status: "failure,success"})

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(q, "status = ANY($1)") {
		t.Errorf("missing status ANY filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_NodeNameSubstring(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{NodeName: "web01"})

	if len(args) != 1 || args[0] != "%web01%" {
		t.Fatalf("expected 1 arg '%%web01%%', got %v", args)
	}
	if !strings.Contains(q, "LOWER(node_name) LIKE LOWER($1)") {
		t.Errorf("missing node_name substring filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_ChefVersionFilter(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{ChefVersion: "19.0.12"})

	if len(args) != 1 || args[0] != "19.0.12" {
		t.Fatalf("expected 1 arg '19.0.12', got %v", args)
	}
	if !strings.Contains(q, "chef_version = $1") {
		t.Errorf("missing chef_version filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_TimeRange(t *testing.T) {
	from := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{EndTimeFrom: from, EndTimeTo: to})

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "end_time >= $1") {
		t.Errorf("missing end_time lower bound: %s", q)
	}
	if !strings.Contains(q, "end_time <= $2") {
		t.Errorf("missing end_time upper bound: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_SortWhitelist(t *testing.T) {
	// Unknown sort key falls back to the default end_time column.
	q, _ := buildConvergeRunFilterQuery(ConvergeRunFilter{Sort: "; DROP TABLE", SortOrder: "asc"})
	if !strings.Contains(q, "ORDER BY end_time ASC") {
		t.Errorf("unknown sort key should fall back to end_time, got: %s", q)
	}
	if strings.Contains(q, "DROP TABLE") {
		t.Errorf("sort key must never be interpolated: %s", q)
	}

	q2, _ := buildConvergeRunFilterQuery(ConvergeRunFilter{Sort: "node_name", SortOrder: "asc"})
	if !strings.Contains(q2, "ORDER BY LOWER(node_name) ASC") {
		t.Errorf("node_name sort not honoured: %s", q2)
	}
}

func TestBuildConvergeRunFilterQuery_Pagination(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{Limit: 25, Offset: 50})

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("missing LIMIT: %s", q)
	}
	if !strings.Contains(q, "OFFSET $2") {
		t.Errorf("missing OFFSET: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_CookbookFilter(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{Cookbook: "chef-client"})
	if len(args) != 1 || args[0] != "chef-client" {
		t.Fatalf("expected 1 arg 'chef-client', got %v", args)
	}
	if !strings.Contains(q, "jsonb_exists(cookbooks, $1)") {
		t.Errorf("missing cookbook key-exists filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_FailureMessageFilter(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{FailureMessage: "not enough space"})
	if len(args) != 1 || args[0] != "%not enough space%" {
		t.Fatalf("expected 1 arg '%%not enough space%%', got %v", args)
	}
	if !strings.Contains(q, "error->>'message' ILIKE $1") {
		t.Errorf("missing failure-message filter: %s", q)
	}
}

func TestBuildConvergeRunFilterQuery_MultipleFilters(t *testing.T) {
	q, args := buildConvergeRunFilterQuery(ConvergeRunFilter{
		Organisation: "org-a",
		Status:       "failure",
		NodeName:     "web",
		ChefVersion:  "19.0.12",
	})

	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "AND") {
		t.Error("multiple filters should be joined with AND")
	}
	if !strings.Contains(q, "WHERE") {
		t.Error("multiple filters should produce a WHERE")
	}
}

// --- Node rollup builder (the top-level Nodes tab) ---

func TestBuildConvergeRunNodeFilterQuery_NoFilters(t *testing.T) {
	q, args := buildConvergeRunNodeFilterQuery(ConvergeRunFilter{})

	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "DISTINCT ON (organisation, node_name)") {
		t.Errorf("node rollup must collapse to one row per node: %s", q)
	}
	if !strings.Contains(q, "COUNT(*) OVER ()") {
		t.Error("node rollup must expose the distinct-node count")
	}
	// Inner ordering drives DISTINCT ON (latest run per node).
	if !strings.Contains(q, "ORDER BY organisation, node_name, end_time DESC") {
		t.Errorf("inner ordering must pick latest run per node: %s", q)
	}
	if strings.Contains(q, "WHERE") {
		t.Error("no filters should produce no WHERE")
	}
}

func TestBuildConvergeRunNodeFilterQuery_FiltersAndPagination(t *testing.T) {
	// EXISTS semantics: filters apply to the runs BEFORE the per-node collapse.
	q, args := buildConvergeRunNodeFilterQuery(ConvergeRunFilter{
		Status:      "failure",
		ChefVersion: "19.0.12",
		Cookbook:    "prereq",
		Limit:       25,
		Offset:      25,
	})
	if len(args) != 5 {
		t.Fatalf("expected 5 args (3 filters + limit + offset), got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "WHERE") || !strings.Contains(q, "status = ANY($1)") {
		t.Errorf("filters must be inside the matching CTE: %s", q)
	}
	if !strings.Contains(q, "jsonb_exists(cookbooks, $3)") {
		t.Errorf("cookbook filter missing: %s", q)
	}
	if !strings.Contains(q, "LIMIT $4") || !strings.Contains(q, "OFFSET $5") {
		t.Errorf("pagination applies to the distinct-node set: %s", q)
	}
}

func TestBuildConvergeRunNodeCountQuery_DistinctNodes(t *testing.T) {
	q, args := buildConvergeRunNodeCountQuery(ConvergeRunFilter{Status: "failure"})
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if !strings.Contains(q, "GROUP BY organisation, node_name") {
		t.Errorf("count must be over distinct nodes: %s", q)
	}
	if !strings.Contains(q, "COUNT(*)") {
		t.Errorf("missing COUNT(*): %s", q)
	}
}
