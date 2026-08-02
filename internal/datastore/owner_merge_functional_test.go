// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"errors"
	"testing"
)

// seedMergeOwner creates an owner and registers its removal.
func seedMergeOwner(t *testing.T, db *DB, name string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: name, OwnerType: "individual"}); err != nil {
		t.Fatalf("InsertOwner(%q): %v", name, err)
	}
	t.Cleanup(func() { _, _ = db.DeleteOwner(context.Background(), name) })
}

func seedMergeAlias(t *testing.T, db *DB, owner, aliasType, aliasValue string) {
	t.Helper()
	if _, err := db.InsertOwnerAlias(context.Background(), InsertOwnerAliasParams{
		OwnerName: owner, AliasType: aliasType, AliasValue: aliasValue, Source: "import",
	}); err != nil {
		t.Fatalf("InsertOwnerAlias(%q, %q): %v", aliasType, aliasValue, err)
	}
}

func seedMergeAssignment(t *testing.T, db *DB, owner, entityType, entityKey string) {
	t.Helper()
	if _, err := db.InsertAssignment(context.Background(), InsertAssignmentParams{
		OwnerName:        owner,
		EntityType:       entityType,
		EntityKey:        entityKey,
		AssignmentSource: "import",
		Confidence:       "definitive",
	}); err != nil {
		t.Fatalf("InsertAssignment(%q, %q): %v", owner, entityKey, err)
	}
}

func ownerAliasValues(t *testing.T, db *DB, owner string) map[string]string {
	t.Helper()
	aliases, err := db.GetOwnerAliasesByOwner(context.Background(), owner)
	if err != nil {
		t.Fatalf("GetOwnerAliasesByOwner(%q): %v", owner, err)
	}
	out := map[string]string{}
	for _, a := range aliases {
		out[a.AliasType+":"+a.AliasValue] = a.Source
	}
	return out
}

// The whole point of the merge: what the work was moved onto is where the next
// ingest puts it. That needs the alias moved, not just the assignments.
func TestFunctional_MergeOwners_MovesWorkAliasesAndTheSourceName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "merge-src-tommy")
	seedMergeOwner(t, db, "merge-dst-thomas")
	seedMergeAlias(t, db, "merge-src-tommy", "custom", "Fat Tommy")
	seedMergeAlias(t, db, "merge-src-tommy", "email", "tommy@example-corp.test")
	seedMergeAssignment(t, db, "merge-src-tommy", "git_repo", "merge-web-app")

	got, err := db.MergeOwners(ctx, "merge-src-tommy", "merge-dst-thomas")
	if err != nil {
		t.Fatalf("MergeOwners: %v", err)
	}

	if got.Reassigned != 1 || got.Skipped != 0 {
		t.Errorf("Reassigned/Skipped = %d/%d, want 1/0", got.Reassigned, got.Skipped)
	}
	if got.AliasesMoved != 2 {
		t.Errorf("AliasesMoved = %d, want 2", got.AliasesMoved)
	}
	if !got.SourceNameAliased {
		t.Error("SourceNameAliased = false; the source name must survive as an alias or a re-ingest recreates the person")
	}

	// The source owner is gone.
	if _, err := db.GetOwnerByName(ctx, "merge-src-tommy"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetOwnerByName(source) error = %v, want ErrNotFound", err)
	}

	// Every identity the source was known by now resolves to the target.
	for _, want := range []struct{ aliasType, aliasValue string }{
		{"custom", "Fat Tommy"},
		{"email", "tommy@example-corp.test"},
		{"custom", "merge-src-tommy"},
	} {
		owner, err := db.ResolveOwnerByAlias(ctx, want.aliasType, want.aliasValue)
		if err != nil {
			t.Errorf("ResolveOwnerByAlias(%q, %q): %v", want.aliasType, want.aliasValue, err)
			continue
		}
		if owner != "merge-dst-thomas" {
			t.Errorf("ResolveOwnerByAlias(%q, %q) = %q, want the merge target", want.aliasType, want.aliasValue, owner)
		}
	}

	// The work sits with the target.
	counts, err := db.CountAssignmentsByOwner(ctx, "merge-dst-thomas")
	if err != nil {
		t.Fatalf("CountAssignmentsByOwner: %v", err)
	}
	if counts["git_repo"] != 1 {
		t.Errorf("target git_repo assignments = %d, want 1", counts["git_repo"])
	}
}

// Both people already holding the same repo is the normal case for a duplicate:
// the assignment must not be duplicated onto the target, and must not be left
// behind on a source owner that is about to be deleted.
func TestFunctional_MergeOwners_AssignmentTheTargetAlreadyHasIsDropped(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "merge-dup-src")
	seedMergeOwner(t, db, "merge-dup-dst")
	seedMergeAssignment(t, db, "merge-dup-src", "git_repo", "merge-dup-repo")
	seedMergeAssignment(t, db, "merge-dup-dst", "git_repo", "merge-dup-repo")
	seedMergeAssignment(t, db, "merge-dup-src", "node", "merge-dup-node")

	got, err := db.MergeOwners(ctx, "merge-dup-src", "merge-dup-dst")
	if err != nil {
		t.Fatalf("MergeOwners: %v", err)
	}
	if got.Reassigned != 1 {
		t.Errorf("Reassigned = %d, want 1 (the node only)", got.Reassigned)
	}
	if got.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the repo the target already held)", got.Skipped)
	}

	counts, err := db.CountAssignmentsByOwner(ctx, "merge-dup-dst")
	if err != nil {
		t.Fatalf("CountAssignmentsByOwner: %v", err)
	}
	if counts["git_repo"] != 1 || counts["node"] != 1 {
		t.Errorf("target counts = %v, want one of each", counts)
	}
}

// A merge into a target that already carries the source's name as an alias
// must not fail — the seed is a convenience, not a requirement.
func TestFunctional_MergeOwners_SourceNameAlreadyAliasedIsNotAnError(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "merge-seeded-src")
	seedMergeOwner(t, db, "merge-seeded-dst")
	seedMergeAlias(t, db, "merge-seeded-dst", "custom", "merge-seeded-src")

	got, err := db.MergeOwners(ctx, "merge-seeded-src", "merge-seeded-dst")
	if err != nil {
		t.Fatalf("MergeOwners: %v", err)
	}
	if got.SourceNameAliased {
		t.Error("SourceNameAliased = true, want false — the target already had it")
	}

	aliases := ownerAliasValues(t, db, "merge-seeded-dst")
	if _, ok := aliases["custom:merge-seeded-src"]; !ok {
		t.Errorf("target aliases = %v, want the existing source-name alias kept", aliases)
	}
}

// A merge deletes a person, so it has to be answerable for afterwards. The
// audit table constrains its action column, and an action it does not know is
// rejected — which the handler only logs, so an unaudited merge would look
// exactly like an audited one.
func TestFunctional_AuditLogAcceptsAMergeAction(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db, "DELETE FROM ownership_audit_log WHERE actor = 'merge-audit-test'")

	err := db.InsertAuditEntry(ctx, InsertAuditEntryParams{
		Action:    "owner_merged",
		Actor:     "merge-audit-test",
		OwnerName: "merge-audit-target",
	})
	if err != nil {
		t.Fatalf("recording a merge in the audit log: %v", err)
	}
}

func TestFunctional_MergeOwners_RejectsUnknownAndSelfMerge(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "merge-guard-real")

	if _, err := db.MergeOwners(ctx, "merge-guard-real", "merge-guard-real"); err == nil {
		t.Error("merging an owner into itself returned no error")
	}
	if _, err := db.MergeOwners(ctx, "merge-guard-absent", "merge-guard-real"); !errors.Is(err, ErrNotFound) {
		t.Errorf("merging an absent source: error = %v, want ErrNotFound", err)
	}
	if _, err := db.MergeOwners(ctx, "merge-guard-real", "merge-guard-absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("merging into an absent target: error = %v, want ErrNotFound", err)
	}
	// The guard must not have removed the owner it refused to merge.
	if _, err := db.GetOwnerByName(ctx, "merge-guard-real"); err != nil {
		t.Errorf("GetOwnerByName after a refused merge: %v", err)
	}
}
