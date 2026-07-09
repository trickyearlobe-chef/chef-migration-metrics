// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// seedRoleSummary inserts one materialised role_summary row for a test.
func seedRoleSummary(t *testing.T, db *DB, org, role string, r rsRow, compat, tk string) {
	t.Helper()
	_, err := db.pool.ExecContext(context.Background(),
		`INSERT INTO role_summary
		   (organisation_name, role_name, node_count, direct_cookbook_count, transitive_cookbook_count,
		    compatible_count, incompatible_count, untested_count, compatibility_status,
		    tk_status, tk_passed, tk_total)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		org, role, r.nodeCount, r.directCB, r.transitiveCB,
		r.compatible, r.incompatible, r.untested, compat, tk, r.tkPassed, r.tkTotal)
	if err != nil {
		t.Fatalf("seed role_summary %s/%s: %v", org, role, err)
	}
}

// TestFunctional_ListRolesFiltered_RollsUpRoleSummary seeds role_summary rows
// across two orgs and asserts ListRolesFiltered rolls them up to one row per
// role with the correct cross-org aggregation, and that org/name/compat/tk
// filters, sorting, and pagination all resolve against the materialised table.
func TestFunctional_ListRolesFiltered_RollsUpRoleSummary(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const orgA, orgB = "func-lrf-a", "func-lrf-b"

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name IN ('"+orgA+"','"+orgB+"')",
	)

	// web spans both orgs; base + db are org-a only.
	seedRoleSummary(t, db, orgA, "lrf-web",
		rsRow{nodeCount: 3, directCB: 1, transitiveCB: 3, compatible: 1, incompatible: 1, untested: 1}, "incompatible", "failed")
	seedRoleSummary(t, db, orgB, "lrf-web",
		rsRow{nodeCount: 2, directCB: 1, transitiveCB: 2, compatible: 2}, "compatible", "passed")
	seedRoleSummary(t, db, orgA, "lrf-base",
		rsRow{nodeCount: 1, directCB: 2, transitiveCB: 2, compatible: 1, untested: 1}, "untested", "passed")
	seedRoleSummary(t, db, orgA, "lrf-db",
		rsRow{nodeCount: 10, directCB: 1, transitiveCB: 1, compatible: 1}, "compatible", "untested")

	both := []string{orgA, orgB}
	byName := func(rows []RoleFilterRow) map[string]RoleFilterRow {
		m := make(map[string]RoleFilterRow, len(rows))
		for _, r := range rows {
			m[r.RoleName] = r
		}
		return m
	}

	t.Run("cross-org rollup for web", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3 (web, base, db)", total)
		}
		web := byName(rows)["lrf-web"]
		if web.NodeCount != 5 { // SUM(3, 2)
			t.Errorf("web node_count = %d, want 5 (sum across orgs)", web.NodeCount)
		}
		if web.DirectCookbookCount != 1 || web.TransitiveCookbookCount != 3 || web.TotalCookbookCount != 3 {
			t.Errorf("web cookbook counts = direct %d transitive %d total %d, want 1/3/3 (max)", web.DirectCookbookCount, web.TransitiveCookbookCount, web.TotalCookbookCount)
		}
		if web.CompatibleCount != 2 || web.IncompatibleCount != 1 || web.UntestedCount != 1 {
			t.Errorf("web compat counts = %d/%d/%d, want 2/1/1 (max)", web.CompatibleCount, web.IncompatibleCount, web.UntestedCount)
		}
		if web.CompatibilityStatus != "incompatible" { // worst-of
			t.Errorf("web compatibility_status = %q, want incompatible", web.CompatibilityStatus)
		}
		if web.TKStatus != "failed" { // worst-of
			t.Errorf("web tk_status = %q, want failed", web.TKStatus)
		}
		if len(web.Organisations) != 2 || web.Organisations[0] != orgA || web.Organisations[1] != orgB {
			t.Errorf("web organisations = %v, want [%s %s]", web.Organisations, orgA, orgB)
		}
	})

	t.Run("default sort is name asc", func(t *testing.T) {
		rows, _, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		got := []string{rows[0].RoleName, rows[1].RoleName, rows[2].RoleName}
		want := []string{"lrf-base", "lrf-db", "lrf-web"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("name-sorted order = %v, want %v", got, want)
			}
		}
	})

	t.Run("sort node_count desc", func(t *testing.T) {
		rows, _, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, Sort: "node_count", SortOrder: "desc"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// db (10) > web (5) > base (1)
		want := []string{"lrf-db", "lrf-web", "lrf-base"}
		for i := range want {
			if rows[i].RoleName != want[i] {
				t.Fatalf("node_count desc order = %v, want %v", []string{rows[0].RoleName, rows[1].RoleName, rows[2].RoleName}, want)
			}
		}
	})

	t.Run("sort tk_status asc (failed first)", func(t *testing.T) {
		rows, _, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, Sort: "tk_status", SortOrder: "asc"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// web=failed(0), base=passed(2), db=untested(3)
		if rows[0].RoleName != "lrf-web" {
			t.Errorf("tk_status asc first = %q, want lrf-web (failed)", rows[0].RoleName)
		}
		if rows[2].RoleName != "lrf-db" {
			t.Errorf("tk_status asc last = %q, want lrf-db (untested)", rows[2].RoleName)
		}
	})

	t.Run("compat filter", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, CompatibilityStatus: "incompatible"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || len(rows) != 1 || rows[0].RoleName != "lrf-web" {
			t.Errorf("compat=incompatible → %d rows total %d, want just lrf-web", len(rows), total)
		}
	})

	t.Run("tk filter multi-value", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, TKStatuses: []string{"passed", "untested"}})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// base=passed, db=untested; web=failed excluded.
		if total != 2 {
			t.Errorf("tk filter total = %d, want 2", total)
		}
		m := byName(rows)
		if _, ok := m["lrf-web"]; ok {
			t.Errorf("web (failed) should be excluded by tk filter passed,untested")
		}
	})

	t.Run("org filter restricts rollup", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: []string{orgB}})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || rows[0].RoleName != "lrf-web" {
			t.Fatalf("org-b filter → total %d rows %v, want 1 (lrf-web)", total, rows)
		}
		web := rows[0]
		if web.NodeCount != 2 { // only org-b's contribution
			t.Errorf("org-b web node_count = %d, want 2", web.NodeCount)
		}
		if len(web.Organisations) != 1 || web.Organisations[0] != orgB {
			t.Errorf("org-b web organisations = %v, want [%s]", web.Organisations, orgB)
		}
		if web.CompatibilityStatus != "compatible" || web.TKStatus != "passed" {
			t.Errorf("org-b web status = %q/%q, want compatible/passed", web.CompatibilityStatus, web.TKStatus)
		}
	})

	t.Run("name filter substring", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, Name: "we"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || rows[0].RoleName != "lrf-web" {
			t.Errorf("name=we → total %d, want 1 (lrf-web)", total)
		}
	})

	t.Run("pagination reports full filtered total", func(t *testing.T) {
		rows, total, _, err := db.ListRolesFiltered(ctx, RoleFilter{OrganisationNames: both, Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("page size = %d, want 2", len(rows))
		}
		if total != 3 {
			t.Errorf("total with limit = %d, want 3 (full filtered count via window)", total)
		}
	})

	t.Run("summary rollup", func(t *testing.T) {
		s, m, err := db.GetRoleCompatSummary(ctx, RoleFilter{OrganisationNames: both})
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		if s.TotalRoles != 3 || s.CompatibleRoles != 1 || s.IncompatibleRoles != 1 || s.UntestedRoles != 1 {
			t.Errorf("summary = %+v, want total 3 / compatible 1 / incompatible 1 / untested 1", s)
		}
		if m["lrf-web"] != "incompatible" {
			t.Errorf("compatMap web = %q, want incompatible", m["lrf-web"])
		}
	})
}

// TestFunctional_GetRoleDetail_NodeCountExcludesStale asserts the role detail
// blast radius counts only non-stale nodes, matching role_summary.node_count and
// the /nodes?role=<name>&stale=false link (shared-selection consistency).
func TestFunctional_GetRoleDetail_NodeCountExcludesStale(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const org = "func-rd-stale-org"

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name = '"+org+"'",
		"DELETE FROM role_dependencies WHERE organisation_name = '"+org+"'",
		"DELETE FROM node_snapshots WHERE organisation_name = '"+org+"'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+org+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
	)

	o, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://example.com/organizations/rd", OrgName: "rd", ClientName: "c",
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: o.Name})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := db.BulkUpsertRoleDependencies(ctx, []InsertRoleDependencyParams{
		{OrganisationName: org, RoleName: "rd-web", DependencyType: "cookbook", DependencyName: "rd-nginx"},
	}); err != nil {
		t.Fatalf("role deps: %v", err)
	}

	roles := func(rs ...string) json.RawMessage { b, _ := json.Marshal(rs); return b }
	now := time.Now().UTC()
	// 2 non-stale + 1 stale carrying rd-web. Stale must not count.
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rd-n1", Roles: roles("rd-web"), ChefEnvironment: "production", Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rd-n2", Roles: roles("rd-web"), ChefEnvironment: "production", Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rd-n3", Roles: roles("rd-web"), ChefEnvironment: "production", Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now, IsStale: true},
	}); err != nil {
		t.Fatalf("nodes: %v", err)
	}

	detail, err := db.GetRoleDetail(ctx, "rd-web", "")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.NodeCount != 2 {
		t.Errorf("detail node_count = %d, want 2 (stale excluded)", detail.NodeCount)
	}

	// The per-org / per-env / per-platform breakdowns must sum to the same
	// non-stale count so the surfaces agree.
	var orgSum int
	for _, oc := range detail.NodesByOrganisation {
		orgSum += oc.Count
	}
	if orgSum != 2 {
		t.Errorf("nodes_by_organisation sum = %d, want 2", orgSum)
	}

	// Drift guard: detail count must equal the live non-stale containment count.
	var live int
	if err := db.pool.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT organisation_name || '/' || node_name) FROM node_snapshots
		 WHERE organisation_name = $1 AND is_stale = false AND roles @> to_jsonb(ARRAY[$2::text])`,
		org, "rd-web").Scan(&live); err != nil {
		t.Fatalf("live count: %v", err)
	}
	if detail.NodeCount != live {
		t.Errorf("detail node_count %d != live non-stale count %d", detail.NodeCount, live)
	}
}
