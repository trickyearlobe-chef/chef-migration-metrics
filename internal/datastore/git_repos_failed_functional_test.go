// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// A failed fetch must not invent a URL for a repo we already know about.
// The collector tries each configured base URL in turn and reports the
// failure against the last one; if that were used as an insert key, a repo
// living at an earlier base URL would gain a second row, and the remediation
// handlers (which take gitRepos[0] from an unordered list) would then pick
// arbitrarily between them.
func TestFunctional_MarkGitRepoFailedByName_MarksExistingRowWithoutDuplicating(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "test-mfn-cookbook"
	const realURL = "git@example.com:org-a/test-mfn-cookbook"
	const lastConfiguredURL = "git@example.com:org-z/test-mfn-cookbook"

	cleanupTestData(t, db,
		"DELETE FROM git_repos WHERE name = 'test-mfn-cookbook'",
	)

	// The repo lives at org-a and has been cloned and scanned.
	_, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:          name,
		GitRepoURL:    realURL,
		HeadCommitSHA: "abc123def456",
		DefaultBranch: "main",
		HasTestSuite:  true,
		LastFetchedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seeding cloned repo: %v", err)
	}

	// Every base URL fails; the collector reports the last configured one.
	_, err = db.MarkGitRepoFailedByName(ctx, name, lastConfiguredURL, "connection timed out")
	if err != nil {
		t.Fatalf("marking failed by name: %v", err)
	}

	rows, err := db.ListGitReposByName(ctx, name)
	if err != nil {
		t.Fatalf("listing repos by name: %v", err)
	}
	if len(rows) != 1 {
		urls := make([]string, 0, len(rows))
		for _, r := range rows {
			urls = append(urls, r.GitRepoURL)
		}
		t.Fatalf("expected 1 row for %q, got %d: %v", name, len(rows), urls)
	}

	got := rows[0]
	if got.GitRepoURL != realURL {
		t.Errorf("git_repo_url: got %q, want %q (the URL the repo actually lives at)",
			got.GitRepoURL, realURL)
	}
	if got.CloneStatus != CloneStatusFailed {
		t.Errorf("clone_status: got %q, want %q", got.CloneStatus, CloneStatusFailed)
	}
	if got.CloneError != "connection timed out" {
		t.Errorf("clone_error: got %q, want %q", got.CloneError, "connection timed out")
	}
	// A repo we can't clone can't be verified.
	if got.CookstyleStatus != "untested" {
		t.Errorf("cookstyle_status: got %q, want untested", got.CookstyleStatus)
	}
	if got.CompatibilityStatus != "untested" {
		t.Errorf("compatibility_status: got %q, want untested", got.CompatibilityStatus)
	}
	// The row's history is preserved — we know where it lives and what we last saw.
	if got.HeadCommitSHA != "abc123def456" {
		t.Errorf("head_commit_sha: got %q, want the previously-fetched SHA", got.HeadCommitSHA)
	}
}

// A repo that clones nowhere and has no row yet is genuinely new: insert a
// failed row so it is visible in the UI with its reason.
func TestFunctional_MarkGitRepoFailedByName_InsertsWhenNoRowExists(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "test-mfn-newcookbook"
	const fallbackURL = "git@example.com:org-z/test-mfn-newcookbook"

	cleanupTestData(t, db,
		"DELETE FROM git_repos WHERE name = 'test-mfn-newcookbook'",
	)

	repo, err := db.MarkGitRepoFailedByName(ctx, name, fallbackURL, "repository not found")
	if err != nil {
		t.Fatalf("marking failed by name: %v", err)
	}
	if repo.GitRepoURL != fallbackURL {
		t.Errorf("returned git_repo_url: got %q, want %q", repo.GitRepoURL, fallbackURL)
	}

	rows, err := db.ListGitReposByName(ctx, name)
	if err != nil {
		t.Fatalf("listing repos by name: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for %q, got %d", name, len(rows))
	}
	if rows[0].CloneStatus != CloneStatusFailed {
		t.Errorf("clone_status: got %q, want %q", rows[0].CloneStatus, CloneStatusFailed)
	}
	if rows[0].CloneError != "repository not found" {
		t.Errorf("clone_error: got %q, want %q", rows[0].CloneError, "repository not found")
	}
}

// Repeated failures must stay idempotent — a second failing run does not add
// another row.
func TestFunctional_MarkGitRepoFailedByName_RepeatedFailureIsIdempotent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "test-mfn-repeat"
	const fallbackURL = "git@example.com:org-z/test-mfn-repeat"

	cleanupTestData(t, db,
		"DELETE FROM git_repos WHERE name = 'test-mfn-repeat'",
	)

	if _, err := db.MarkGitRepoFailedByName(ctx, name, fallbackURL, "first failure"); err != nil {
		t.Fatalf("first failure: %v", err)
	}
	if _, err := db.MarkGitRepoFailedByName(ctx, name, fallbackURL, "second failure"); err != nil {
		t.Fatalf("second failure: %v", err)
	}

	rows, err := db.ListGitReposByName(ctx, name)
	if err != nil {
		t.Fatalf("listing repos by name: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after two failures, got %d", len(rows))
	}
	if rows[0].CloneError != "second failure" {
		t.Errorf("clone_error: got %q, want the latest failure", rows[0].CloneError)
	}
}

// Duplicate rows left behind by the old failure path must all be marked
// failed, not just one of them — the handlers pick arbitrarily between rows,
// so a stale 'ok' twin would keep the repo looking healthy.
func TestFunctional_MarkGitRepoFailedByName_MarksAllPreExistingRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const name = "test-mfn-dupes"
	const urlA = "git@example.com:org-a/test-mfn-dupes"
	const urlB = "git@example.com:org-b/test-mfn-dupes"

	cleanupTestData(t, db,
		"DELETE FROM git_repos WHERE name = 'test-mfn-dupes'",
	)

	for _, u := range []string{urlA, urlB} {
		if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
			Name:          name,
			GitRepoURL:    u,
			LastFetchedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seeding repo at %s: %v", u, err)
		}
	}

	if _, err := db.MarkGitRepoFailedByName(ctx, name, urlB, "gone"); err != nil {
		t.Fatalf("marking failed by name: %v", err)
	}

	rows, err := db.ListGitReposByName(ctx, name)
	if err != nil {
		t.Fatalf("listing repos by name: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected the 2 pre-existing rows and no new one, got %d", len(rows))
	}
	for _, r := range rows {
		if r.CloneStatus != CloneStatusFailed {
			t.Errorf("row %s: clone_status got %q, want %q", r.GitRepoURL, r.CloneStatus, CloneStatusFailed)
		}
	}
}
