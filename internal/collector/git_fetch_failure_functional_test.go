// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package collector

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// gitTestDB opens the functional test database and runs migrations.
// CMM_TEST_DATABASE_URL must be set, e.g.
//
//	postgres://user:pass@localhost:5432/cmm_test?sslmode=disable
func gitTestDB(t *testing.T) *datastore.DB {
	t.Helper()

	url := os.Getenv("CMM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CMM_TEST_DATABASE_URL not set — skipping functional test")
	}

	db, err := datastore.Open(url)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.MigrateUp(context.Background(), "../../migrations"); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	return db
}

func gitTestLogger() *logging.ScopedLogger {
	l := logging.New(logging.Options{
		Level:   logging.ERROR,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	return l.WithScope(logging.ScopeGitOperation)
}

// A repo that lives at an earlier base URL must not gain a second row when a
// fetch fails. The collector tries each base URL in turn and only knows that
// *all* of them failed — attributing the failure to the last one configured
// would insert a row under a URL the repo never lived at, leaving two rows for
// one cookbook. The remediation handlers take gitRepos[0] from an unordered
// list, so a healthy, fully-scanned cookbook could then render as failed.
func TestFunctional_FetchGitCookbooks_FailedFetchDoesNotDuplicateExistingRepo(t *testing.T) {
	db := gitTestDB(t)
	ctx := context.Background()

	const name = "test-fgc-cookbook"
	const realURL = "git@example.com:org-a/test-fgc-cookbook"

	t.Cleanup(func() {
		if _, err := db.Pool().ExecContext(context.Background(),
			"DELETE FROM git_repos WHERE name = $1", name); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	if _, err := db.Pool().ExecContext(ctx, "DELETE FROM git_repos WHERE name = $1", name); err != nil {
		t.Fatalf("pre-test cleanup: %v", err)
	}

	// The repo lives at org-a and was cloned and scanned on an earlier run.
	if _, err := db.UpsertGitRepo(ctx, datastore.UpsertGitRepoParams{
		Name:          name,
		GitRepoURL:    realURL,
		HeadCommitSHA: "abc123def456",
		DefaultBranch: "main",
		HasTestSuite:  true,
		LastFetchedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seeding cloned repo: %v", err)
	}

	// Every git command fails — the repo is unreachable from every base URL
	// (server down, network blip, credentials expired).
	fake := newFakeGitExecutor()
	fake.defaultErr = errors.New("fatal: could not read from remote repository")
	mgr := NewGitCookbookManager(t.TempDir(), fake)

	// org-a is where the repo actually lives; org-z is merely the last one tried.
	baseURLs := []string{"git@example.com:org-a", "git@example.com:org-z"}

	result := fetchGitCookbooks(ctx, mgr, db, gitTestLogger(), baseURLs,
		map[string]bool{name: true}, 1)

	if result.Failed != 1 {
		t.Errorf("result.Failed: got %d, want 1", result.Failed)
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
		t.Fatalf("a failed fetch duplicated the repo row: got %d rows for %q, want 1: %v",
			len(rows), name, urls)
	}

	got := rows[0]
	if got.GitRepoURL != realURL {
		t.Errorf("git_repo_url: got %q, want %q (where the repo actually lives)",
			got.GitRepoURL, realURL)
	}
	if got.CloneStatus != datastore.CloneStatusFailed {
		t.Errorf("clone_status: got %q, want %q", got.CloneStatus, datastore.CloneStatusFailed)
	}
	if got.CloneError == "" {
		t.Error("clone_error: empty, want the failure reason (the UI shows it)")
	}
	if got.HeadCommitSHA != "abc123def456" {
		t.Errorf("head_commit_sha: got %q, want the previously-fetched SHA preserved",
			got.HeadCommitSHA)
	}
}

// A cookbook with no git repo anywhere still gets a failed row, so it shows up
// in the UI as missing with its reason rather than vanishing.
func TestFunctional_FetchGitCookbooks_UnknownRepoGetsFailedRow(t *testing.T) {
	db := gitTestDB(t)
	ctx := context.Background()

	const name = "test-fgc-unknown"

	t.Cleanup(func() {
		if _, err := db.Pool().ExecContext(context.Background(),
			"DELETE FROM git_repos WHERE name = $1", name); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	if _, err := db.Pool().ExecContext(ctx, "DELETE FROM git_repos WHERE name = $1", name); err != nil {
		t.Fatalf("pre-test cleanup: %v", err)
	}

	fake := newFakeGitExecutor()
	fake.defaultErr = errors.New("ERROR: Repository not found")
	mgr := NewGitCookbookManager(t.TempDir(), fake)

	baseURLs := []string{"git@example.com:org-a", "git@example.com:org-z"}

	result := fetchGitCookbooks(ctx, mgr, db, gitTestLogger(), baseURLs,
		map[string]bool{name: true}, 1)

	if result.Failed != 1 {
		t.Errorf("result.Failed: got %d, want 1", result.Failed)
	}

	rows, err := db.ListGitReposByName(ctx, name)
	if err != nil {
		t.Fatalf("listing repos by name: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 failed row for an unknown repo, got %d", len(rows))
	}
	if rows[0].CloneStatus != datastore.CloneStatusFailed {
		t.Errorf("clone_status: got %q, want %q", rows[0].CloneStatus, datastore.CloneStatusFailed)
	}
	// With no row to learn from, the last base URL tried is the best guess.
	if rows[0].GitRepoURL != "git@example.com:org-z/"+name {
		t.Errorf("git_repo_url: got %q, want the last base URL tried", rows[0].GitRepoURL)
	}
}
