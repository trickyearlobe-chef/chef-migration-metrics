// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestFunctional_CookstyleStatus_ServerRoundTrip verifies the materialised
// cookstyle_status column on server cookbook results is persisted on upsert and
// returned on get (migration 0041 / Chunk 3 SoT surfacing).
func TestFunctional_CookstyleStatus_ServerRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM server_cookbook_cookstyle_results WHERE cookbook_name = 'func-cs-status'",
		"DELETE FROM server_cookbooks WHERE name = 'func-cs-status'",
		"DELETE FROM organisations WHERE name = 'func-cs-org'",
	)

	if _, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: "func-cs-org", ChefServerURL: "https://chef.example.com", OrgName: "func-cs-org", ClientName: "c",
	}); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := db.UpsertServerCookbook(ctx, UpsertServerCookbookParams{
		OrganisationName: "func-cs-org", Name: "func-cs-status", Version: "1.0.0", IsActive: true,
	}); err != nil {
		t.Fatalf("seed cookbook: %v", err)
	}

	_, err := db.UpsertServerCookbookCookstyleResult(ctx, UpsertServerCookbookCookstyleResultParams{
		OrganisationName:  "func-cs-org",
		CookbookName:      "func-cs-status",
		CookbookVersion:   "1.0.0",
		TargetChefVersion: "18",
		Passed:            true,
		CookstyleStatus:   "needs_review",
		Offences:          []byte("[]"),
		DurationSeconds:   1,
		ScannedAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := db.GetServerCookbookCookstyleResult(ctx, "func-cs-org", "func-cs-status", "1.0.0", "18")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected a result row")
	}
	if got.CookstyleStatus != "needs_review" {
		t.Errorf("CookstyleStatus = %q, want needs_review", got.CookstyleStatus)
	}

	// Upsert again with a changed status — ON CONFLICT must update it.
	_, err = db.UpsertServerCookbookCookstyleResult(ctx, UpsertServerCookbookCookstyleResultParams{
		OrganisationName:  "func-cs-org",
		CookbookName:      "func-cs-status",
		CookbookVersion:   "1.0.0",
		TargetChefVersion: "18",
		Passed:            false,
		CookstyleStatus:   "blocked",
		Offences:          []byte("[]"),
		DurationSeconds:   1,
		ScannedAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, err = db.GetServerCookbookCookstyleResult(ctx, "func-cs-org", "func-cs-status", "1.0.0", "18")
	if err != nil {
		t.Fatalf("get after re-upsert: %v", err)
	}
	if got.CookstyleStatus != "blocked" {
		t.Errorf("after re-upsert CookstyleStatus = %q, want blocked", got.CookstyleStatus)
	}
}

// TestFunctional_ListAllServerCookbookCookstyleResultsByTargetVersion_Scans
// guards against the SELECT column list in this cross-org query drifting from
// the shared scan helper. Migration 0041 added cookstyle_status as the 17th
// column (read by scanServerCookbookCookstyleResults); this query selected only
// 16, so it failed at runtime with "expected 16 destination arguments in Scan,
// not 17" — but only against a real DB, which mocks can't catch.
func TestFunctional_ListAllServerCookbookCookstyleResultsByTargetVersion_Scans(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM server_cookbook_cookstyle_results WHERE cookbook_name = 'func-cs-listall'",
		"DELETE FROM server_cookbooks WHERE name = 'func-cs-listall'",
		"DELETE FROM organisations WHERE name = 'func-cs-listall-org'",
	)

	if _, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: "func-cs-listall-org", ChefServerURL: "https://chef.example.com", OrgName: "func-cs-listall-org", ClientName: "c",
	}); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := db.UpsertServerCookbook(ctx, UpsertServerCookbookParams{
		OrganisationName: "func-cs-listall-org", Name: "func-cs-listall", Version: "1.0.0", IsActive: true,
	}); err != nil {
		t.Fatalf("seed cookbook: %v", err)
	}
	if _, err := db.UpsertServerCookbookCookstyleResult(ctx, UpsertServerCookbookCookstyleResultParams{
		OrganisationName:  "func-cs-listall-org",
		CookbookName:      "func-cs-listall",
		CookbookVersion:   "1.0.0",
		TargetChefVersion: "func-cs-listall-tv",
		Passed:            true,
		CookstyleStatus:   "needs_review",
		Offences:          []byte("[]"),
		DurationSeconds:   1,
		ScannedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	results, err := db.ListAllServerCookbookCookstyleResultsByTargetVersion(ctx, "func-cs-listall-tv")
	if err != nil {
		t.Fatalf("ListAllServerCookbookCookstyleResultsByTargetVersion: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].CookstyleStatus != "needs_review" {
		t.Errorf("CookstyleStatus = %q, want needs_review", results[0].CookstyleStatus)
	}
}

// TestFunctional_SchemaBackfills_Marker verifies the backfill marker round-trips
// and that marking is idempotent (migration 0043).
func TestFunctional_SchemaBackfills_Marker(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "func-test-backfill-marker"
	cleanupTestData(t, db, "DELETE FROM schema_backfills WHERE name = '"+name+"'")

	done, err := db.BackfillCompleted(ctx, name)
	if err != nil {
		t.Fatalf("BackfillCompleted: %v", err)
	}
	if done {
		t.Fatal("expected marker absent before first mark")
	}

	if err := db.MarkBackfillCompleted(ctx, name); err != nil {
		t.Fatalf("MarkBackfillCompleted: %v", err)
	}
	// Second mark must be a no-op (ON CONFLICT DO NOTHING), not an error.
	if err := db.MarkBackfillCompleted(ctx, name); err != nil {
		t.Fatalf("second MarkBackfillCompleted: %v", err)
	}

	done, err = db.BackfillCompleted(ctx, name)
	if err != nil {
		t.Fatalf("BackfillCompleted after mark: %v", err)
	}
	if !done {
		t.Error("expected marker present after mark")
	}
}

// TestFunctional_ListAllCookstyleResultRefs verifies the unscoped ref listings
// return rows across targets with their stored offences and current status — the
// inputs the precise-status backfill consumes.
func TestFunctional_ListAllCookstyleResultRefs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const org = "func-cs-refs-org"
	const cb = "func-cs-refs"
	const gitName = "func-cs-refs-git"
	const gitURL = "https://git.example.com/func-cs-refs-git"
	cleanupTestData(t, db,
		"DELETE FROM server_cookbook_cookstyle_results WHERE cookbook_name = '"+cb+"'",
		"DELETE FROM server_cookbooks WHERE name = '"+cb+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = '"+gitName+"'",
		"DELETE FROM git_repos WHERE name = '"+gitName+"'",
	)

	if _, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://chef.example.com", OrgName: org, ClientName: "c",
	}); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := db.UpsertServerCookbook(ctx, UpsertServerCookbookParams{
		OrganisationName: org, Name: cb, Version: "1.0.0", IsActive: true,
	}); err != nil {
		t.Fatalf("seed cookbook: %v", err)
	}
	const offJSON = `[{"cop_name":"Chef/Correctness/NodeNormal","severity":"warning"}]`
	if _, err := db.UpsertServerCookbookCookstyleResult(ctx, UpsertServerCookbookCookstyleResultParams{
		OrganisationName: org, CookbookName: cb, CookbookVersion: "1.0.0", TargetChefVersion: "18",
		Passed: true, CookstyleStatus: "ready", Offences: []byte(offJSON), DurationSeconds: 1, ScannedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert server result: %v", err)
	}
	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{Name: gitName, GitRepoURL: gitURL}); err != nil {
		t.Fatalf("upsert git repo: %v", err)
	}
	if _, err := db.UpsertGitRepoCookstyleResult(ctx, UpsertGitRepoCookstyleResultParams{
		GitRepoName: gitName, GitRepoURL: gitURL, TargetChefVersion: "18",
		Passed: true, CookstyleStatus: "ready", Offences: []byte(offJSON), DurationSeconds: 1, ScannedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert git result: %v", err)
	}

	const wantCop = "Chef/Correctness/NodeNormal"

	serverRefs, err := db.ListAllServerCookbookCookstyleResultRefs(ctx)
	if err != nil {
		t.Fatalf("ListAllServerCookbookCookstyleResultRefs: %v", err)
	}
	if !containsServerRef(serverRefs, org, cb, wantCop) {
		t.Errorf("seeded server ref (with its offences) not found in %d refs", len(serverRefs))
	}

	gitRefs, err := db.ListAllGitRepoCookstyleResultRefs(ctx)
	if err != nil {
		t.Fatalf("ListAllGitRepoCookstyleResultRefs: %v", err)
	}
	if !containsGitRef(gitRefs, gitName, wantCop) {
		t.Errorf("seeded git ref (with its offences) not found in %d refs", len(gitRefs))
	}
}

func containsServerRef(refs []CookstyleResultRef, org, cb, wantCop string) bool {
	for _, r := range refs {
		if r.OrganisationName == org && r.CookbookName == cb && bytes.Contains(r.Offences, []byte(wantCop)) {
			return true
		}
	}
	return false
}

func containsGitRef(refs []CookstyleResultRef, name, wantCop string) bool {
	for _, r := range refs {
		if r.GitRepoName == name && bytes.Contains(r.Offences, []byte(wantCop)) {
			return true
		}
	}
	return false
}

// TestFunctional_CookstyleStatus_GitRepoMaterialised verifies a git repo
// result's status round-trips and that the git_repos rollup column is
// recomputed from the latest result.
func TestFunctional_CookstyleStatus_GitRepoMaterialised(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "func-cs-git"
	const url = "https://git.example.com/func-cs-git"
	cleanupTestData(t, db,
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = '"+name+"'",
		"DELETE FROM git_repos WHERE name = '"+name+"'",
	)

	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{Name: name, GitRepoURL: url}); err != nil {
		t.Fatalf("upsert git repo: %v", err)
	}

	// UpsertGitRepoCookstyleResult also recomputes the materialised columns.
	if _, err := db.UpsertGitRepoCookstyleResult(ctx, UpsertGitRepoCookstyleResultParams{
		GitRepoName:       name,
		GitRepoURL:        url,
		TargetChefVersion: "18",
		Passed:            true,
		CookstyleStatus:   "needs_review",
		Offences:          []byte("[]"),
		DurationSeconds:   1,
		ScannedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert git result: %v", err)
	}

	res, err := db.GetGitRepoCookstyleResult(ctx, name, url, "18")
	if err != nil {
		t.Fatalf("get git result: %v", err)
	}
	if res == nil || res.CookstyleStatus != "needs_review" {
		t.Fatalf("git result CookstyleStatus = %v, want needs_review", res)
	}

	// The git_repos rollup column must mirror the latest result's status.
	repo, err := db.GetGitRepoByKey(ctx, name, url)
	if err != nil {
		t.Fatalf("get git repo: %v", err)
	}
	if repo.CookstyleStatus != "needs_review" {
		t.Errorf("git_repos.cookstyle_status = %q, want needs_review", repo.CookstyleStatus)
	}
}
