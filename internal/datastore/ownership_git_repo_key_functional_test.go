// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// A git_repo assignment is keyed by the repo NAME.
//
// The product used to disagree with itself: the repo list matched ownership on
// the name while the owner's page, the cookbook's inherited owner and the
// committers panel all matched on the git URL. A repo somebody had claimed read
// as unowned on the list, and the owner's own page showed nothing for it — with
// the assignment count agreeing with neither.
//
// The name is canonical because repo URLs are volatile. These tests hold the
// three URL-assuming readers to it.
// ---------------------------------------------------------------------------

const (
	keyTestRepo  = "func-key-repo"
	keyTestURL   = "https://git.example.com/cookbooks/func-key-repo.git"
	keyTestOwner = "func-key-owner"
	keyTestVer   = "18.5.0"
)

func seedNameKeyedRepoOwnership(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM ownership_assignments WHERE owner_name = '"+keyTestOwner+"'",
		"DELETE FROM git_repo_complexity WHERE git_repo_name = '"+keyTestRepo+"'",
		"DELETE FROM git_repos WHERE name = '"+keyTestRepo+"'",
		"DELETE FROM owners WHERE name = '"+keyTestOwner+"'",
	)

	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:          keyTestRepo,
		GitRepoURL:    keyTestURL,
		DefaultBranch: "main",
		LastFetchedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed git repo: %v", err)
	}
	if _, err := db.InsertOwner(ctx, InsertOwnerParams{
		Name:         keyTestOwner,
		OwnerType:    "individual",
		ContactEmail: "func-key@example.com",
	}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	// Keyed by the repo NAME — what the import writes and what the repo list
	// reads.
	if _, err := db.InsertAssignment(ctx, InsertAssignmentParams{
		OwnerName:        keyTestOwner,
		EntityType:       "git_repo",
		EntityKey:        keyTestRepo,
		AssignmentSource: "manual",
		Confidence:       "definitive",
	}); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
}

// The owner's own page resolved the assignment against git_repo_url, so a
// name-keyed assignment found no repo — and a repo with no complexity record is
// reported as incompatible, which is the same shape as a real answer.
func TestFunctional_OwnerGitRepoSummary_ResolvesByRepoName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedNameKeyedRepoOwnership(t, db)

	if _, err := db.UpsertGitRepoComplexity(ctx, UpsertGitRepoComplexityParams{
		GitRepoName:       keyTestRepo,
		GitRepoURL:        keyTestURL,
		TargetChefVersion: keyTestVer,
		ErrorCount:        0,
		ComplexityLabel:   "none",
	}); err != nil {
		t.Fatalf("seed complexity: %v", err)
	}

	summary, err := db.GetOwnerGitRepoSummary(ctx, keyTestOwner, keyTestVer)
	if err != nil {
		t.Fatalf("GetOwnerGitRepoSummary: %v", err)
	}
	if summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", summary.Total)
	}
	if summary.Compatible != 1 {
		t.Errorf("Compatible = %d, Incompatible = %d — a clean repo the owner owns reads as a problem",
			summary.Compatible, summary.Incompatible)
	}
}

// A cookbook with no owner of its own inherits its repo's owner. That lookup
// converted the repo name into a URL before matching, so it inherited nothing.
func TestFunctional_CookbookInheritsRepoOwnerByName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedNameKeyedRepoOwnership(t, db)

	results, err := db.LookupOwnership(ctx, "cookbook", keyTestRepo, "")
	if err != nil {
		t.Fatalf("LookupOwnership: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d owners, want the repo's owner inherited", len(results))
	}
	if results[0].OwnerName != keyTestOwner {
		t.Errorf("OwnerName = %q, want %q", results[0].OwnerName, keyTestOwner)
	}
}
