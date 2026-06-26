// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// buildCookbookFilterQuery — unit tests
// ---------------------------------------------------------------------------

func TestBuildCookbookFilterQuery_NoFilters(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{})

	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(q, "WITH cb AS") {
		t.Error("query missing CTE")
	}
	if !strings.Contains(q, "tk AS") {
		t.Error("query missing TK CTE")
	}
	if !strings.Contains(q, "COUNT(*) OVER()") {
		t.Error("query missing COUNT(*) OVER()")
	}
	if !strings.Contains(q, "LEFT JOIN tk ON") {
		t.Error("query missing LEFT JOIN tk")
	}
	if !strings.Contains(q, "ORDER BY") {
		t.Error("query missing ORDER BY")
	}
	if strings.Contains(q, "LIMIT") {
		t.Error("query should not contain LIMIT when Limit=0")
	}
	if strings.Contains(q, "OFFSET") {
		t.Error("query should not contain OFFSET when Offset=0")
	}
}

func TestBuildCookbookFilterQuery_WithTargetVersion(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "LEFT JOIN server_cookbook_cookstyle_results csr") {
		t.Error("query missing LEFT JOIN to cookstyle results")
	}
	if !strings.Contains(q, "csr.target_chef_version") {
		t.Error("query missing csr.target_chef_version condition")
	}
	// TK CTE now reads materialised column — no gkr join needed.
	if !strings.Contains(q, "gr.tk_status") {
		t.Error("query missing materialised tk_status column reference")
	}
	// SoT rollup status surfaced from the materialised column.
	if !strings.Contains(q, "csr.cookstyle_status") {
		t.Error("query missing csr.cookstyle_status reference")
	}
	if !strings.Contains(q, "cb.cookstyle_status") {
		t.Error("outer query missing cb.cookstyle_status projection")
	}
	// $1 = cookstyle target only (TK no longer needs a param)
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
	if args[0] != "18.5.0" {
		t.Errorf("args[0] = %v, want 18.5.0", args[0])
	}
}

func TestBuildCookbookFilterQuery_WithoutTargetVersion(t *testing.T) {
	q, _ := buildCookbookFilterQuery(CookbookFilter{
		TargetChefVersion: "",
	})

	if strings.Contains(q, "server_cookbook_cookstyle_results") {
		t.Error("query should not reference server_cookbook_cookstyle_results when TargetChefVersion is empty")
	}
	if !strings.Contains(q, "'untested' AS compatibility") {
		t.Error("query missing 'untested' AS compatibility")
	}
	if !strings.Contains(q, "'untested' AS cookstyle_status") {
		t.Error("query missing 'untested' AS cookstyle_status fallback")
	}
	// TK CTE still present but with no gkr join
	if !strings.Contains(q, "tk AS") {
		t.Error("query missing TK CTE")
	}
	if !strings.Contains(q, "LEFT JOIN tk ON") {
		t.Error("query missing LEFT JOIN tk")
	}
}

func TestBuildCookbookFilterQuery_OrgFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		OrganisationNames: []string{"org-a", "org-b"},
	})

	if !strings.Contains(q, "sc.organisation_name = ANY($1)") {
		t.Errorf("query missing org filter, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
}

func TestBuildCookbookFilterQuery_NameFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		Name: "nginx",
	})

	if !strings.Contains(q, "LOWER(sc.name) LIKE") {
		t.Errorf("query missing name filter, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
	if args[0] != "nginx" {
		t.Errorf("args[0] = %v, want nginx", args[0])
	}
}

func TestBuildCookbookFilterQuery_ActiveTrue(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		Active: boolPtr(true),
	})

	if !strings.Contains(q, "sc.is_active = $1") {
		t.Errorf("query missing is_active filter, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
	if args[0] != true {
		t.Errorf("args[0] = %v, want true", args[0])
	}
}

func TestBuildCookbookFilterQuery_CompatibilityFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		Compatibility:     "incompatible",
		TargetChefVersion: "18.5.0",
	})

	// $1 = cookstyle target, $2 = compatibility
	if !strings.Contains(q, "cb.compatibility = $2") {
		t.Errorf("query missing outer compatibility filter, got:\n%s", q)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "18.5.0" {
		t.Errorf("args[0] = %v, want 18.5.0", args[0])
	}
	if args[1] != "incompatible" {
		t.Errorf("args[1] = %v, want incompatible", args[1])
	}
}

func TestBuildCookbookFilterQuery_CookstyleStatusFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		CookstyleStatus:   "ready,needs_review",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "cb.cookstyle_status = ANY($2)") {
		t.Errorf("query missing outer cookstyle_status filter, got:\n%s", q)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
}

func TestBuildCookbookFilterQuery_DownloadStatusFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		DownloadStatus: "ok",
	})

	if !strings.Contains(q, "sc.download_status = $1") {
		t.Errorf("query missing download_status filter, got:\n%s", q)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
	if args[0] != "ok" {
		t.Errorf("args[0] = %v, want ok", args[0])
	}
}

func TestBuildCookbookFilterQuery_Pagination(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		Limit:  20,
		Offset: 40,
	})

	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("query missing LIMIT clause, got:\n%s", q)
	}
	if !strings.Contains(q, "OFFSET $2") {
		t.Errorf("query missing OFFSET clause, got:\n%s", q)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != 20 {
		t.Errorf("args[0] = %v, want 20", args[0])
	}
	if args[1] != 40 {
		t.Errorf("args[1] = %v, want 40", args[1])
	}
}

func TestBuildCookbookFilterQuery_SortName(t *testing.T) {
	q, _ := buildCookbookFilterQuery(CookbookFilter{
		Sort: "name",
	})

	if !strings.Contains(q, "ORDER BY LOWER(cb.name)") {
		t.Errorf("query missing ORDER BY cb.name, got:\n%s", q)
	}
}

func TestBuildCookbookFilterQuery_SortTKStatus(t *testing.T) {
	q, _ := buildCookbookFilterQuery(CookbookFilter{
		Sort: "tk_status",
	})

	if !strings.Contains(q, "ORDER BY COALESCE(tk.tk_status, 'no_repo')") {
		t.Errorf("query missing ORDER BY tk_status, got:\n%s", q)
	}
}

func TestBuildCookbookFilterQuery_TKStatusFilter(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		TKStatus:          "passed,failed",
		TargetChefVersion: "18.5.0",
	})

	if !strings.Contains(q, "COALESCE(tk.tk_status, 'no_repo') = ANY(") {
		t.Errorf("query missing TK status filter, got:\n%s", q)
	}
	// $1 = cookstyle target, $2 = tk_status array
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
}

func TestBuildCookbookFilterQuery_SortCompatibility(t *testing.T) {
	q, _ := buildCookbookFilterQuery(CookbookFilter{
		Sort: "compatibility",
	})

	if !strings.Contains(q, "ORDER BY cb.compatibility") {
		t.Errorf("query missing ORDER BY cb.compatibility, got:\n%s", q)
	}
}

func TestBuildCookbookFilterQuery_AllFilters(t *testing.T) {
	q, args := buildCookbookFilterQuery(CookbookFilter{
		OrganisationNames: []string{"org-a", "org-b"},
		Name:              "nginx",
		Active:            boolPtr(true),
		DownloadStatus:    "ok",
		Compatibility:     "incompatible",
		TKStatus:          "passed,failed",
		TargetChefVersion: "18.5.0",
		Limit:             20,
		Offset:            40,
		Sort:              "name",
		SortOrder:         "desc",
	})

	// $1 = cookstyle target, $2 = org names, $3 = name,
	// $4 = active, $5 = download_status, $6 = compatibility,
	// $7 = tk_status array, $8 = limit, $9 = offset
	expectedArgs := 9
	if len(args) != expectedArgs {
		t.Errorf("expected %d args, got %d: %v", expectedArgs, len(args), args)
	}

	// Verify parameter numbering is sequential with no duplicates.
	for i := 1; i <= expectedArgs; i++ {
		placeholder := fmt.Sprintf("$%d", i)
		if !strings.Contains(q, placeholder) {
			t.Errorf("query missing placeholder %s", placeholder)
		}
	}
	// Next placeholder should not exist.
	if strings.Contains(q, fmt.Sprintf("$%d", expectedArgs+1)) {
		t.Errorf("query contains unexpected placeholder $%d", expectedArgs+1)
	}

	// Spot-check that all filter clauses are present.
	checks := []string{
		"sc.organisation_name = ANY(",
		"LOWER(sc.name) LIKE",
		"sc.is_active =",
		"sc.download_status =",
		"cb.compatibility =",
		"COALESCE(tk.tk_status, 'no_repo') = ANY(",
		"LEFT JOIN server_cookbook_cookstyle_results",
		"csr.target_chef_version",
		"gr.tk_status",
		"LEFT JOIN tk ON",
		"LIMIT",
		"OFFSET",
		"ORDER BY",
		"DESC",
	}
	for _, check := range checks {
		if !strings.Contains(q, check) {
			t.Errorf("query missing expected clause %q", check)
		}
	}
}
