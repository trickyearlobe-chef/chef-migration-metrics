// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
)

// git_repos carries materialised cookstyle_status and compatibility_status
// columns that the repo LIST view reads directly, while the DETAIL view looks
// the result up live. Deleting cookstyle results without clearing those columns
// makes the two disagree: the list keeps asserting a verdict ("ready") derived
// from data that no longer exists, while the detail correctly reads untested.
//
// At fleet scale the window is hours, not seconds, because the list only
// corrects itself as each repo is re-scanned.

func seedRepoWithVerdict(t *testing.T, db *DB, name string) {
	t.Helper()
	ctx := context.Background()

	if _, err := db.pool.ExecContext(ctx, `
		INSERT INTO git_repos (name, git_repo_url, clone_status, cookstyle_status, compatibility_status, tk_status, tk_passed, tk_total)
		VALUES ($1, $2, 'ok', 'ready', 'compatible', 'passed', 3, 3)
		ON CONFLICT (name, git_repo_url) DO UPDATE SET
			cookstyle_status = 'ready', compatibility_status = 'compatible',
			tk_status = 'passed', tk_passed = 3, tk_total = 3`,
		name, "https://git.example.com/"+name+".git"); err != nil {
		t.Fatalf("seeding git repo: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.pool.ExecContext(context.Background(), `DELETE FROM git_repos WHERE name = $1`, name)
	})
}

func repoVerdicts(t *testing.T, db *DB, name string) (cookstyle, compat, tk string, tkPassed, tkTotal int) {
	t.Helper()
	err := db.pool.QueryRowContext(context.Background(),
		`SELECT cookstyle_status, compatibility_status, tk_status, tk_passed, tk_total
		   FROM git_repos WHERE name = $1`, name).
		Scan(&cookstyle, &compat, &tk, &tkPassed, &tkTotal)
	if err != nil {
		t.Fatalf("reading verdicts for %s: %v", name, err)
	}
	return
}

// The verdicts the rescan invalidates must be cleared, so the list view stops
// asserting a status with no scan behind it.
func TestResetAllGitRepoCookstyleVerdicts_ClearsStaleVerdicts(t *testing.T) {
	db := testDB(t)
	const name = "reset-verdicts-probe"
	seedRepoWithVerdict(t, db, name)

	if err := db.ResetAllGitRepoCookstyleVerdicts(context.Background()); err != nil {
		t.Fatalf("resetting: %v", err)
	}

	cookstyle, compat, _, _, _ := repoVerdicts(t, db, name)
	if cookstyle != "untested" {
		t.Errorf("cookstyle_status = %q, want untested", cookstyle)
	}
	if compat != "untested" {
		t.Errorf("compatibility_status = %q, want untested", compat)
	}
}

// Test Kitchen results are NOT deleted by a cookstyle rescan, so their
// materialised status must survive. Resetting it would destroy a verdict that
// is still backed by real data — the mirror image of the bug being fixed.
func TestResetAllGitRepoCookstyleVerdicts_PreservesTestKitchenStatus(t *testing.T) {
	db := testDB(t)
	const name = "reset-preserves-tk-probe"
	seedRepoWithVerdict(t, db, name)

	if err := db.ResetAllGitRepoCookstyleVerdicts(context.Background()); err != nil {
		t.Fatalf("resetting: %v", err)
	}

	_, _, tk, passed, total := repoVerdicts(t, db, name)
	if tk != "passed" {
		t.Errorf("tk_status = %q, want it preserved as passed", tk)
	}
	if passed != 3 || total != 3 {
		t.Errorf("tk counts = %d/%d, want 3/3 preserved", passed, total)
	}
}
