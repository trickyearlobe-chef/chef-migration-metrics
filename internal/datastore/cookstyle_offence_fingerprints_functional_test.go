// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// TestFunctional_OffenceFingerprint_AppendAndDedupe verifies the change-dedupe
// contract: an identical fingerprint appends nothing, a changed one appends a new
// row, and the per-result history is returned oldest-first (migration 0043 /
// Chunk 8 fingerprint foundation).
func TestFunctional_OffenceFingerprint_AppendAndDedupe(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const org, cb, ver, target = "func-fp-org", "func-fp-cb", "1.0.0", "19.3.15"
	cleanupTestData(t, db,
		"DELETE FROM cookstyle_offence_fingerprints WHERE organisation_name = 'func-fp-org'",
	)

	base := AppendCookstyleOffenceFingerprintParams{
		ResultKind:        FingerprintKindServerCookbook,
		OrganisationName:  org,
		CookbookName:      cb,
		CookbookVersion:   ver,
		TargetChefVersion: target,
		FingerprintHash:   "hash-A",
		Cops: []FingerprintCopEntry{
			{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Count: 5, Severity: "warning", Correctable: false},
		},
		ScannedAt: time.Now().UTC().Add(-2 * time.Hour),
	}

	// First scan: appends.
	appended, err := db.AppendCookstyleOffenceFingerprint(ctx, base)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	if !appended {
		t.Error("first append should insert a row")
	}

	// Identical fingerprint (later scan time, same hash): dedupes — no new row.
	dup := base
	dup.ScannedAt = base.ScannedAt.Add(time.Hour)
	appended, err = db.AppendCookstyleOffenceFingerprint(ctx, dup)
	if err != nil {
		t.Fatalf("dup append: %v", err)
	}
	if appended {
		t.Error("identical fingerprint should be deduped, not appended")
	}

	// Changed fingerprint: appends a second row.
	changed := base
	changed.FingerprintHash = "hash-B"
	changed.Cops = []FingerprintCopEntry{
		{CopName: "Chef/Deprecations/ResourceWithoutUnifiedTrue", Count: 2, Severity: "warning", Correctable: false},
		{CopName: "Lint/DeprecatedClassMethods", Count: 1, Severity: "convention", Correctable: true},
	}
	changed.ScannedAt = base.ScannedAt.Add(2 * time.Hour)
	appended, err = db.AppendCookstyleOffenceFingerprint(ctx, changed)
	if err != nil {
		t.Fatalf("changed append: %v", err)
	}
	if !appended {
		t.Error("changed fingerprint should append a new row")
	}

	// History: exactly two rows, oldest-first, with cops JSONB round-tripped.
	history, err := db.ListServerCookbookOffenceFingerprints(ctx, org, cb, ver, target)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 fingerprint rows after dedupe, got %d", len(history))
	}
	if history[0].FingerprintHash != "hash-A" || history[1].FingerprintHash != "hash-B" {
		t.Errorf("history order = [%s, %s], want [hash-A, hash-B]", history[0].FingerprintHash, history[1].FingerprintHash)
	}
	if !history[0].ScannedAt.Before(history[1].ScannedAt) {
		t.Error("history should be ordered oldest-first by scanned_at")
	}
	if len(history[1].Cops) != 2 {
		t.Fatalf("expected 2 cops in second fingerprint, got %d", len(history[1].Cops))
	}
	if c := history[1].Cops[1]; c.CopName != "Lint/DeprecatedClassMethods" || c.Count != 1 || c.Severity != "convention" || !c.Correctable {
		t.Errorf("cops JSONB round-trip mismatch: %+v", c)
	}
}

// TestFunctional_OffenceFingerprint_KindAndResultIsolation verifies fingerprints
// for distinct results (and distinct kinds sharing a name) don't dedupe against
// each other.
func TestFunctional_OffenceFingerprint_KindAndResultIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookstyle_offence_fingerprints WHERE organisation_name = 'func-fp-iso-org'",
		"DELETE FROM cookstyle_offence_fingerprints WHERE git_repo_name = 'func-fp-iso'",
	)

	now := time.Now().UTC()

	// A server-cookbook result and a git-repo result that share the same name and
	// hash must each get their own row (kind + identity differ).
	server := AppendCookstyleOffenceFingerprintParams{
		ResultKind: FingerprintKindServerCookbook, OrganisationName: "func-fp-iso-org",
		CookbookName: "func-fp-iso", CookbookVersion: "1.0.0", TargetChefVersion: "19",
		FingerprintHash: "shared-hash", Cops: []FingerprintCopEntry{{CopName: "Chef/A", Count: 1, Severity: "warning"}},
		ScannedAt: now,
	}
	git := AppendCookstyleOffenceFingerprintParams{
		ResultKind: FingerprintKindGitRepo, GitRepoName: "func-fp-iso",
		GitRepoURL: "git@example.com:org-a/func-fp-iso", TargetChefVersion: "19",
		FingerprintHash: "shared-hash", Cops: []FingerprintCopEntry{{CopName: "Chef/A", Count: 1, Severity: "warning"}},
		ScannedAt: now,
	}

	if appended, err := db.AppendCookstyleOffenceFingerprint(ctx, server); err != nil || !appended {
		t.Fatalf("server append: appended=%v err=%v", appended, err)
	}
	if appended, err := db.AppendCookstyleOffenceFingerprint(ctx, git); err != nil || !appended {
		t.Fatalf("git append: appended=%v err=%v", appended, err)
	}

	serverHist, err := db.ListServerCookbookOffenceFingerprints(ctx, "func-fp-iso-org", "func-fp-iso", "1.0.0", "19")
	if err != nil {
		t.Fatalf("server list: %v", err)
	}
	gitHist, err := db.ListGitRepoOffenceFingerprints(ctx, "func-fp-iso", "git@example.com:org-a/func-fp-iso", "19")
	if err != nil {
		t.Fatalf("git list: %v", err)
	}
	if len(serverHist) != 1 {
		t.Errorf("expected 1 server fingerprint, got %d", len(serverHist))
	}
	if len(gitHist) != 1 {
		t.Errorf("expected 1 git fingerprint, got %d", len(gitHist))
	}
}

// TestFunctional_OffenceFingerprint_RejectsBadKind verifies an unknown result kind
// is rejected rather than silently writing an unqueryable row.
func TestFunctional_OffenceFingerprint_RejectsBadKind(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.AppendCookstyleOffenceFingerprint(ctx, AppendCookstyleOffenceFingerprintParams{
		ResultKind: "bogus", FingerprintHash: "x", ScannedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected an error for an invalid result_kind")
	}
}
