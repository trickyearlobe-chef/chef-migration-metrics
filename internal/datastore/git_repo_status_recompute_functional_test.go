// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
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
