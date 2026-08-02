// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
)

// pairKey names a candidate pair in the order the query reports it, so a test
// can look one up without caring which side it landed on.
func pairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

func candidatesByPair(t *testing.T, got []OwnerDuplicateCandidate) map[string]OwnerDuplicateCandidate {
	t.Helper()
	out := map[string]OwnerDuplicateCandidate{}
	for _, c := range got {
		out[pairKey(c.OwnerA, c.OwnerB)] = c
	}
	return out
}

// scanForDuplicates rebuilds the stored candidates and reads them back.
func scanForDuplicates(t *testing.T, db *DB) []OwnerDuplicateCandidate {
	t.Helper()
	ctx := context.Background()
	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("RecomputeOwnerDuplicateCandidates: %v", err)
	}
	got, _, err := db.ListOwnerDuplicateCandidates(ctx, OwnerDuplicateFilter{Limit: 500})
	if err != nil {
		t.Fatalf("ListOwnerDuplicateCandidates: %v", err)
	}
	return got
}

// The import invents people, so the catalogue fills up with near-duplicates.
// This is the standing list that pairs one with who they might already be.
func TestFunctional_OwnerDuplicates_PairsSimilarOwners(t *testing.T) {
	db := testDB(t)

	// Two spellings of one person, and an unrelated third.
	seedMergeOwner(t, db, "dupe-thomas-smithson")
	seedMergeOwner(t, db, "dupe-thomas-smithsen")
	seedMergeOwner(t, db, "dupe-quentin-farnsworth")

	got := scanForDuplicates(t, db)

	pairs := candidatesByPair(t, got)
	c, ok := pairs[pairKey("dupe-thomas-smithson", "dupe-thomas-smithsen")]
	if !ok {
		t.Fatalf("the two spellings were not paired; got %d candidates", len(got))
	}
	if c.Similarity <= 0 || c.Similarity > 1 {
		t.Errorf("Similarity = %v, want a fraction", c.Similarity)
	}
	if c.MatchedOn != "name" {
		t.Errorf("MatchedOn = %q, want %q", c.MatchedOn, "name")
	}

	for key := range pairs {
		if key == pairKey("dupe-thomas-smithson", "dupe-quentin-farnsworth") ||
			key == pairKey("dupe-thomas-smithsen", "dupe-quentin-farnsworth") {
			t.Errorf("paired two unrelated people: %s", key)
		}
	}
}

// An owner created by the committer path has no alias, so an alias-only search
// cannot see it. Comparing owner names is what keeps the whole catalogue in
// scope rather than only the half that was imported.
func TestFunctional_OwnerDuplicates_MatchesOnAliasValueToo(t *testing.T) {
	db := testDB(t)

	// The two owner names share nothing, so only the aliases can pair them.
	seedMergeOwner(t, db, "aliasmatch-quenby")
	seedMergeOwner(t, db, "wobblefish-krumhorn")
	seedMergeAlias(t, db, "aliasmatch-quenby", "custom", "Wilhelmina Fotheringay")
	seedMergeAlias(t, db, "wobblefish-krumhorn", "custom", "Wilhelmina Fotheringey")

	got := scanForDuplicates(t, db)

	c, ok := candidatesByPair(t, got)[pairKey("aliasmatch-quenby", "wobblefish-krumhorn")]
	if !ok {
		t.Fatalf("two owners with near-identical aliases were not paired; got %d candidates", len(got))
	}
	if c.MatchedOn != "alias" {
		t.Errorf("MatchedOn = %q, want %q", c.MatchedOn, "alias")
	}
	if c.ValueA == "" || c.ValueB == "" {
		t.Errorf("the matched values were not reported: %+v", c)
	}
}

// Which way round to merge depends on how much work each side holds, so the
// list has to carry it — otherwise the operator has to open both people.
func TestFunctional_OwnerDuplicates_CarriesAssignmentCounts(t *testing.T) {
	db := testDB(t)

	seedMergeOwner(t, db, "dupe-counted-hendricks")
	seedMergeOwner(t, db, "dupe-counted-hendrickz")
	seedMergeAssignment(t, db, "dupe-counted-hendricks", "git_repo", "dupe-counted-repo-one")
	seedMergeAssignment(t, db, "dupe-counted-hendricks", "node", "dupe-counted-node-one")

	got := scanForDuplicates(t, db)

	c, ok := candidatesByPair(t, got)[pairKey("dupe-counted-hendricks", "dupe-counted-hendrickz")]
	if !ok {
		t.Fatalf("pair not found; got %d candidates", len(got))
	}

	withWork, withoutWork := c.AssignmentsA, c.AssignmentsB
	if c.OwnerA != "dupe-counted-hendricks" {
		withWork, withoutWork = c.AssignmentsB, c.AssignmentsA
	}
	if withWork != 2 {
		t.Errorf("assignment count for the owner holding work = %d, want 2", withWork)
	}
	if withoutWork != 0 {
		t.Errorf("assignment count for the owner holding none = %d, want 0", withoutWork)
	}
}

// The same pair turning up under several aliases is one duplicate to resolve,
// not five rows to read.
func TestFunctional_OwnerDuplicates_ReportsEachPairOnce(t *testing.T) {
	db := testDB(t)

	seedMergeOwner(t, db, "dupe-once-bartholomew")
	seedMergeOwner(t, db, "dupe-once-barthollomew")
	seedMergeAlias(t, db, "dupe-once-bartholomew", "custom", "Bartholomew Pemberton")
	seedMergeAlias(t, db, "dupe-once-barthollomew", "custom", "Bartholomew Pembertonn")
	seedMergeAlias(t, db, "dupe-once-bartholomew", "email", "bartholomew.pemberton@example-corp.test")
	seedMergeAlias(t, db, "dupe-once-barthollomew", "email", "bartholomew.pembertonn@example-corp.test")

	got := scanForDuplicates(t, db)

	seen := 0
	for _, c := range got {
		if pairKey(c.OwnerA, c.OwnerB) == pairKey("dupe-once-bartholomew", "dupe-once-barthollomew") {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("the pair appears %d times, want exactly 1", seen)
	}
}

// A resolved duplicate has to leave the list straight away. Waiting for the
// next scan would show the operator a pair they just dealt with, and one side
// of it no longer exists.
func TestFunctional_OwnerDuplicates_MergedPairLeavesTheListWithoutRescanning(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "dupe-gone-archibald")
	seedMergeOwner(t, db, "dupe-gone-archibauld")

	before := scanForDuplicates(t, db)
	if _, ok := candidatesByPair(t, before)[pairKey("dupe-gone-archibald", "dupe-gone-archibauld")]; !ok {
		t.Fatal("the pair was not listed before the merge")
	}

	if _, err := db.MergeOwners(ctx, "dupe-gone-archibauld", "dupe-gone-archibald"); err != nil {
		t.Fatalf("MergeOwners: %v", err)
	}

	after, _, err := db.ListOwnerDuplicateCandidates(ctx, OwnerDuplicateFilter{Limit: 500})
	if err != nil {
		t.Fatalf("ListOwnerDuplicateCandidates after merge: %v", err)
	}
	if _, ok := candidatesByPair(t, after)[pairKey("dupe-gone-archibald", "dupe-gone-archibauld")]; ok {
		t.Error("the merged pair is still listed")
	}
}

// An empty list after a scan means "nothing looks alike". An empty list
// because nobody has ever scanned means something else entirely, and the two
// must not read the same.
func TestFunctional_OwnerDuplicates_ScanRecordIsSeparateFromTheResult(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	found, err := db.RecomputeOwnerDuplicateCandidates(ctx)
	if err != nil {
		t.Fatalf("RecomputeOwnerDuplicateCandidates: %v", err)
	}

	scan, err := db.GetOwnerDuplicateScan(ctx)
	if err != nil {
		t.Fatalf("GetOwnerDuplicateScan after a scan: %v", err)
	}
	if scan.ScannedAt.IsZero() {
		t.Error("ScannedAt is zero after a scan")
	}
	if scan.PairsFound != found {
		t.Errorf("PairsFound = %d, want the %d the scan reported", scan.PairsFound, found)
	}
}

// Rescanning must replace the previous result, not add to it.
func TestFunctional_OwnerDuplicates_RescanIsIdempotent(t *testing.T) {
	db := testDB(t)

	seedMergeOwner(t, db, "dupe-twice-mortimer")
	seedMergeOwner(t, db, "dupe-twice-mortimeer")

	first := scanForDuplicates(t, db)
	second := scanForDuplicates(t, db)

	if len(first) != len(second) {
		t.Errorf("scanning twice gave %d then %d pairs", len(first), len(second))
	}
}

// The view says how much of the catalogue it can see. Silently omitting owners
// is worse than not having the report.
func TestFunctional_CountOwnersMissingAliases(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seedMergeOwner(t, db, "dupe-coverage-with-alias")
	seedMergeOwner(t, db, "dupe-coverage-without-alias")
	seedMergeAlias(t, db, "dupe-coverage-with-alias", "custom", "Coverage With Alias")

	total, missing, err := db.CountOwnersMissingAliases(ctx)
	if err != nil {
		t.Fatalf("CountOwnersMissingAliases: %v", err)
	}
	if total < 2 {
		t.Fatalf("total owners = %d, want at least the two seeded here", total)
	}
	if missing < 1 {
		t.Errorf("owners without an alias = %d, want at least the one seeded here", missing)
	}
	if missing > total {
		t.Errorf("owners without an alias (%d) exceeds the total (%d)", missing, total)
	}
}
