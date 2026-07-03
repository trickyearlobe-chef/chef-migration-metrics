// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestFunctional_RecomputeAllGitRepoCookstyleStatus_HealsDrift reproduces the
// git-repo cookstyle drift bug and proves the bulk recompute fixes it: a repo
// whose cookstyle result says needs_review but whose materialised
// git_repos.cookstyle_status is stale (e.g. blanked to 'untested' by a
// target-version reset) is re-materialised to needs_review.
func TestFunctional_RecomputeAllGitRepoCookstyleStatus_HealsDrift(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const (
		name   = "func-drift-cookbook"
		url    = "git@example.com:org-a/func-drift-cookbook"
		target = "19.3.15"
	)
	cleanupTestData(t, db,
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = '"+name+"'",
		"DELETE FROM git_repos WHERE name = '"+name+"'",
	)

	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name: name, GitRepoURL: url, LastFetchedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upserting git repo: %v", err)
	}

	// A needs_review result (the source of truth the dashboard summary reads).
	if _, err := db.UpsertGitRepoCookstyleResult(ctx, UpsertGitRepoCookstyleResultParams{
		GitRepoName: name, GitRepoURL: url, TargetChefVersion: target,
		Passed: true, CookstyleStatus: "needs_review", DurationSeconds: 1, ScannedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upserting cookstyle result: %v", err)
	}

	// Force the materialised column stale, as a target-version reset would.
	if _, err := db.pool.ExecContext(ctx,
		"UPDATE git_repos SET cookstyle_status = 'untested' WHERE name = $1 AND git_repo_url = $2",
		name, url); err != nil {
		t.Fatalf("forcing stale status: %v", err)
	}

	// Sanity: the list column is now stale relative to the result.
	repos, err := db.ListGitReposByName(ctx, name)
	if err != nil || len(repos) != 1 {
		t.Fatalf("listing repo: %v (n=%d)", err, len(repos))
	}
	if repos[0].CookstyleStatus != "untested" {
		t.Fatalf("precondition: cookstyle_status = %q, want stale 'untested'", repos[0].CookstyleStatus)
	}

	// The bulk recompute must heal it back to needs_review.
	if err := db.RecomputeAllGitRepoCookstyleStatus(ctx, target); err != nil {
		t.Fatalf("RecomputeAllGitRepoCookstyleStatus: %v", err)
	}

	repos, err = db.ListGitReposByName(ctx, name)
	if err != nil || len(repos) != 1 {
		t.Fatalf("re-listing repo: %v (n=%d)", err, len(repos))
	}
	if repos[0].CookstyleStatus != "needs_review" {
		t.Errorf("after recompute: git_repos.cookstyle_status = %q, want needs_review", repos[0].CookstyleStatus)
	}
}

// TestFunctional_GitRepoCookstyleStatus_ListReconcilesWithResults is the drift
// guard: the Git Repos list filters git_repos.cookstyle_status while the
// dashboard summary reads git_repo_cookstyle_results directly. This test seeds a
// mix of statuses, runs the drift-prone path (a target-version reset blanks the
// materialised column, then the bulk recompute heals it), and asserts the two
// sources reconcile per status. If a future change lets the materialised column
// drift again, this fails in CI rather than showing a wrong number on a dashboard.
func TestFunctional_GitRepoCookstyleStatus_ListReconcilesWithResults(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const target = "19.3.15"

	cleanupTestData(t, db,
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name LIKE 'recon-%'",
		"DELETE FROM git_repos WHERE name LIKE 'recon-%'",
	)

	seed := []struct{ name, status string }{
		{"recon-nr-1", "needs_review"}, {"recon-nr-2", "needs_review"}, {"recon-nr-3", "needs_review"},
		{"recon-rd-1", "ready"}, {"recon-rd-2", "ready"},
		{"recon-bl-1", "blocked"},
	}
	for _, s := range seed {
		url := "git@example.com:org/" + s.name
		if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{Name: s.name, GitRepoURL: url, LastFetchedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("upsert repo %s: %v", s.name, err)
		}
		if _, err := db.UpsertGitRepoCookstyleResult(ctx, UpsertGitRepoCookstyleResultParams{
			GitRepoName: s.name, GitRepoURL: url, TargetChefVersion: target,
			Passed: s.status != "blocked", CookstyleStatus: s.status, DurationSeconds: 1, ScannedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("upsert result %s: %v", s.name, err)
		}
	}

	// Drift trigger (reset) then heal (bulk recompute).
	if err := db.ResetAllGitRepoStatuses(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := db.RecomputeAllGitRepoCookstyleStatus(ctx, target); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	// List source: the materialised column the Git Repos list filters.
	listCounts := reconGroupCount(t, db,
		`SELECT cookstyle_status, count(*) FROM git_repos WHERE name LIKE 'recon-%' GROUP BY 1`)
	// Summary source: the latest-result rollup per repo (what the dashboard reads).
	resultCounts := reconGroupCount(t, db, `
		SELECT rollup, count(*) FROM (
			SELECT DISTINCT ON (git_repo_name, git_repo_url)
				CASE WHEN error_message != '' THEN 'untested'
				     ELSE COALESCE(NULLIF(cookstyle_status, ''), 'untested') END AS rollup
			FROM git_repo_cookstyle_results
			WHERE git_repo_name LIKE 'recon-%' AND target_chef_version = $1
			ORDER BY git_repo_name, git_repo_url, scanned_at DESC
		) x GROUP BY rollup`, target)

	if !reflect.DeepEqual(listCounts, resultCounts) {
		t.Errorf("cookstyle drift: list (git_repos) counts %v != summary (results) counts %v", listCounts, resultCounts)
	}
	want := map[string]int{"needs_review": 3, "ready": 2, "blocked": 1}
	if !reflect.DeepEqual(listCounts, want) {
		t.Errorf("list counts %v, want %v", listCounts, want)
	}
}

// reconGroupCount runs a "SELECT key, count" query and returns it as a map.
func reconGroupCount(t *testing.T, db *DB, query string, args ...any) map[string]int {
	t.Helper()
	rows, err := db.pool.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("group count query: %v", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[k] = n
	}
	return out
}

// TestFunctional_GitRepoCloneFailed_ForcesUntested verifies the clone-failure
// invariant: a repo we can't clone can't be verified, so its materialised
// cookstyle/compatibility status is forced to 'untested' on clone failure and
// stays untested even when a later rescore-driven recompute runs against a stale
// result (a Missing repo must never show a ready/needs_review/blocked verdict).
func TestFunctional_GitRepoCloneFailed_ForcesUntested(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const (
		name   = "func-clonefail-cookbook"
		url    = "git@example.com:org-a/func-clonefail-cookbook"
		target = "19.3.15"
	)
	cleanupTestData(t, db,
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = '"+name+"'",
		"DELETE FROM git_repos WHERE name = '"+name+"'",
	)

	// Repo cloned OK and scanned needs_review.
	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{Name: name, GitRepoURL: url, LastFetchedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert repo: %v", err)
	}
	if _, err := db.UpsertGitRepoCookstyleResult(ctx, UpsertGitRepoCookstyleResultParams{
		GitRepoName: name, GitRepoURL: url, TargetChefVersion: target,
		Passed: true, CookstyleStatus: "needs_review", DurationSeconds: 1, ScannedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert result: %v", err)
	}
	if err := db.RecomputeGitRepoCompatibilityStatus(ctx, name, url, target); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	repos, err := db.ListGitReposByName(ctx, name)
	if err != nil || len(repos) != 1 {
		t.Fatalf("list: %v (n=%d)", err, len(repos))
	}
	if repos[0].CookstyleStatus != "needs_review" {
		t.Fatalf("precondition: cookstyle_status = %q, want needs_review", repos[0].CookstyleStatus)
	}

	// Clone now fails — must reset both materialised verdicts to untested.
	if _, err := db.MarkGitRepoCloneFailed(ctx, name, url, "Repository not found"); err != nil {
		t.Fatalf("mark clone failed: %v", err)
	}
	repos, _ = db.ListGitReposByName(ctx, name)
	if repos[0].CookstyleStatus != "untested" {
		t.Errorf("after clone failure: cookstyle_status = %q, want untested", repos[0].CookstyleStatus)
	}
	if repos[0].CompatibilityStatus != "untested" {
		t.Errorf("after clone failure: compatibility_status = %q, want untested", repos[0].CompatibilityStatus)
	}

	// A later rescore-driven bulk recompute must NOT resurrect the stale verdict:
	// the result still says needs_review, but clone_status='failed' forces untested.
	if err := db.RecomputeAllGitRepoCookstyleStatus(ctx, target); err != nil {
		t.Fatalf("recompute all: %v", err)
	}
	repos, _ = db.ListGitReposByName(ctx, name)
	if repos[0].CookstyleStatus != "untested" {
		t.Errorf("after rescore recompute: cookstyle_status = %q, want untested (clone still failed)", repos[0].CookstyleStatus)
	}
}
