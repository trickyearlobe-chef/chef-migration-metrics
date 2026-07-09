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

// rsRow is a materialised role_summary row read back for assertions.
type rsRow struct {
	nodeCount, directCB, transitiveCB  int
	compatible, incompatible, untested int
	compatStatus, tkStatus             string
	tkPassed, tkTotal                  int
}

func getRoleSummary(t *testing.T, db *DB, org, role string) rsRow {
	t.Helper()
	var r rsRow
	err := db.pool.QueryRowContext(context.Background(),
		`SELECT node_count, direct_cookbook_count, transitive_cookbook_count,
		        compatible_count, incompatible_count, untested_count,
		        compatibility_status, tk_status, tk_passed, tk_total
		 FROM role_summary WHERE organisation_name = $1 AND role_name = $2`,
		org, role,
	).Scan(&r.nodeCount, &r.directCB, &r.transitiveCB,
		&r.compatible, &r.incompatible, &r.untested,
		&r.compatStatus, &r.tkStatus, &r.tkPassed, &r.tkTotal)
	if err != nil {
		t.Fatalf("reading role_summary %s/%s: %v", org, role, err)
	}
	return r
}

// TestFunctional_RecomputeRoleSummary_MatchesLiveDerivation seeds a realistic
// nested-role scenario (nesting, incompatible precedence, untested cookbook,
// stale-node exclusion, TK worst-of rollup) and asserts the materialised
// role_summary columns equal the hand-computed live derivation after the bulk
// recompute functions run. This is the consistency contract for chunk 2.
func TestFunctional_RecomputeRoleSummary_MatchesLiveDerivation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const (
		org    = "func-rsr-org"
		target = "19.3.15"
	)

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name = '"+org+"'",
		"DELETE FROM role_dependencies WHERE organisation_name = '"+org+"'",
		"DELETE FROM node_snapshots WHERE organisation_name = '"+org+"'",
		"DELETE FROM server_cookbook_cookstyle_results WHERE organisation_name = '"+org+"'",
		"DELETE FROM server_cookbooks WHERE organisation_name = '"+org+"'",
		"DELETE FROM git_repos WHERE name LIKE 'rsr-%'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+org+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
	)

	// --- Organisation + collection run (FK parents for node_snapshots). ---
	o, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://example.com/organizations/rsr", OrgName: "rsr", ClientName: "c",
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: o.Name})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// --- Role dependencies: web → {cookbook nginx, role base}; base → {apt, ssl}. ---
	if _, err := db.BulkUpsertRoleDependencies(ctx, []InsertRoleDependencyParams{
		{OrganisationName: org, RoleName: "rsr-web", DependencyType: "cookbook", DependencyName: "rsr-nginx"},
		{OrganisationName: org, RoleName: "rsr-web", DependencyType: "role", DependencyName: "rsr-base"},
		{OrganisationName: org, RoleName: "rsr-base", DependencyType: "cookbook", DependencyName: "rsr-apt"},
		{OrganisationName: org, RoleName: "rsr-base", DependencyType: "cookbook", DependencyName: "rsr-ssl"},
	}); err != nil {
		t.Fatalf("role deps: %v", err)
	}

	// --- Nodes: 3 non-stale + 1 stale. Stale node3 must NOT count. ---
	roles := func(rs ...string) json.RawMessage { b, _ := json.Marshal(rs); return b }
	now := time.Now().UTC()
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rsr-n1", Roles: roles("rsr-web"), CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rsr-n2", Roles: roles("rsr-web", "rsr-base"), CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rsr-n3", Roles: roles("rsr-base"), CollectedAt: now, IsStale: true},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "rsr-n4", Roles: roles("rsr-web"), CollectedAt: now},
	}); err != nil {
		t.Fatalf("nodes: %v", err)
	}

	// --- Cookbooks + cookstyle results: nginx incompatible, apt compatible,
	//     ssl has no cookbook/result (→ untested). ---
	for _, cb := range []struct{ name, ver string }{{"rsr-nginx", "1.0.0"}, {"rsr-apt", "1.0.0"}} {
		if _, err := db.UpsertServerCookbook(ctx, UpsertServerCookbookParams{
			OrganisationName: org, Name: cb.name, Version: cb.ver, IsActive: true, LastFetchedAt: now,
		}); err != nil {
			t.Fatalf("cookbook %s: %v", cb.name, err)
		}
	}
	for _, r := range []struct {
		name, ver string
		passed    bool
	}{{"rsr-nginx", "1.0.0", false}, {"rsr-apt", "1.0.0", true}} {
		if _, err := db.UpsertServerCookbookCookstyleResult(ctx, UpsertServerCookbookCookstyleResultParams{
			OrganisationName: org, CookbookName: r.name, CookbookVersion: r.ver, TargetChefVersion: target,
			Passed: r.passed, DurationSeconds: 1, ScannedAt: now,
		}); err != nil {
			t.Fatalf("cookstyle %s: %v", r.name, err)
		}
	}

	// --- git_repos for TK rollup (by cookbook name): nginx failed, apt passed,
	//     ssl untested (filtered out of the rollup). ---
	for _, g := range []struct{ name, tk string }{{"rsr-nginx", "failed"}, {"rsr-apt", "passed"}, {"rsr-ssl", "untested"}} {
		url := "git@example.com:rsr/" + g.name
		if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{Name: g.name, GitRepoURL: url, LastFetchedAt: now}); err != nil {
			t.Fatalf("git repo %s: %v", g.name, err)
		}
		if _, err := db.pool.ExecContext(ctx,
			"UPDATE git_repos SET tk_status = $1 WHERE name = $2 AND git_repo_url = $3", g.tk, g.name, url); err != nil {
			t.Fatalf("set tk %s: %v", g.name, err)
		}
	}

	// --- Recompute all three column groups. ---
	if err := db.RecomputeAllRoleStructural(ctx); err != nil {
		t.Fatalf("structural: %v", err)
	}
	if err := db.RecomputeAllRoleCompatStatus(ctx, target); err != nil {
		t.Fatalf("compat: %v", err)
	}
	if err := db.RecomputeAllRoleTKStatus(ctx); err != nil {
		t.Fatalf("tk: %v", err)
	}

	// --- Assert materialised == expected live derivation. ---
	web := getRoleSummary(t, db, org, "rsr-web")
	wantWeb := rsRow{
		nodeCount: 3, directCB: 1, transitiveCB: 3, // {nginx, apt, ssl}
		compatible: 1, incompatible: 1, untested: 1, // apt / nginx / ssl
		compatStatus: "incompatible", tkStatus: "failed",
	}
	assertRS(t, "rsr-web", web, wantWeb)

	base := getRoleSummary(t, db, org, "rsr-base")
	wantBase := rsRow{
		nodeCount: 1, directCB: 2, transitiveCB: 2, // {apt, ssl}, stale n3 excluded
		compatible: 1, incompatible: 0, untested: 1,
		compatStatus: "untested", tkStatus: "passed",
	}
	assertRS(t, "rsr-base", base, wantBase)

	// --- Drift guard: node_count must equal a live non-stale containment count. ---
	for _, role := range []string{"rsr-web", "rsr-base"} {
		var live int
		if err := db.pool.QueryRowContext(ctx,
			`SELECT COUNT(DISTINCT node_name) FROM node_snapshots
			 WHERE organisation_name = $1 AND is_stale = false
			   AND roles @> to_jsonb(ARRAY[$2::text])`, org, role).Scan(&live); err != nil {
			t.Fatalf("live node count %s: %v", role, err)
		}
		mat := getRoleSummary(t, db, org, role).nodeCount
		if mat != live {
			t.Errorf("%s node_count drift: materialised %d != live %d", role, mat, live)
		}
	}
}

func assertRS(t *testing.T, role string, got, want rsRow) {
	t.Helper()
	if got != want {
		t.Errorf("role %s:\n  got  %+v\n  want %+v", role, got, want)
	}
}

// TestFunctional_ResetAllRoleStatuses_PreservesStructural verifies the target-
// change reset blanks active-target columns but keeps the version-independent
// structural columns intact.
func TestFunctional_ResetAllRoleStatuses_PreservesStructural(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const org = "func-rsr-reset-org"

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name = '"+org+"'",
	)

	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary
		   (organisation_name, role_name, node_count, direct_cookbook_count, transitive_cookbook_count,
		    compatible_count, incompatible_count, untested_count, compatibility_status,
		    tk_status, tk_passed, tk_total)
		 VALUES ($1, 'rsr-reset', 5, 2, 4, 3, 1, 0, 'incompatible', 'failed', 2, 5)`, org); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	if err := db.ResetAllRoleStatuses(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}

	r := getRoleSummary(t, db, org, "rsr-reset")
	// Structural preserved.
	if r.nodeCount != 5 || r.directCB != 2 || r.transitiveCB != 4 {
		t.Errorf("structural not preserved: %+v", r)
	}
	// Active-target blanked.
	want := rsRow{nodeCount: 5, directCB: 2, transitiveCB: 4, compatStatus: "untested", tkStatus: "untested"}
	if r != want {
		t.Errorf("after reset:\n  got  %+v\n  want %+v", r, want)
	}
}

// TestFunctional_RecomputeRoleStructural_PrunesRemovedRoles verifies that a role
// removed from role_dependencies is dropped from role_summary on the next
// structural recompute.
func TestFunctional_RecomputeRoleStructural_PrunesRemovedRoles(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const org = "func-rsr-prune-org"

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name = '"+org+"'",
		"DELETE FROM role_dependencies WHERE organisation_name = '"+org+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
	)
	if _, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://example.com/organizations/prune", OrgName: "prune", ClientName: "c",
	}); err != nil {
		t.Fatalf("org: %v", err)
	}

	if _, err := db.BulkUpsertRoleDependencies(ctx, []InsertRoleDependencyParams{
		{OrganisationName: org, RoleName: "rsr-keep", DependencyType: "cookbook", DependencyName: "rsr-cb"},
		{OrganisationName: org, RoleName: "rsr-gone", DependencyType: "cookbook", DependencyName: "rsr-cb"},
	}); err != nil {
		t.Fatalf("deps: %v", err)
	}
	if err := db.RecomputeAllRoleStructural(ctx); err != nil {
		t.Fatalf("recompute 1: %v", err)
	}

	// Remove rsr-gone from role_dependencies, then recompute.
	if _, err := db.pool.ExecContext(ctx,
		"DELETE FROM role_dependencies WHERE organisation_name = $1 AND role_name = 'rsr-gone'", org); err != nil {
		t.Fatalf("delete dep: %v", err)
	}
	if err := db.RecomputeAllRoleStructural(ctx); err != nil {
		t.Fatalf("recompute 2: %v", err)
	}

	var count int
	if err := db.pool.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM role_summary WHERE organisation_name = $1 AND role_name = 'rsr-gone'", org).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("rsr-gone should be pruned from role_summary, found %d rows", count)
	}
	// rsr-keep must remain.
	if err := db.pool.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM role_summary WHERE organisation_name = $1 AND role_name = 'rsr-keep'", org).Scan(&count); err != nil {
		t.Fatalf("count keep: %v", err)
	}
	if count != 1 {
		t.Errorf("rsr-keep should remain, found %d rows", count)
	}
}
