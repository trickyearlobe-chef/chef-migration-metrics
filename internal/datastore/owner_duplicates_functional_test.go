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

// Two owners under the same display name is about the strongest duplicate
// signal there is, and it is the shape the committer path produces: one person
// commits under two addresses, so two owners are created with two unrelated
// names and one identical display name.
func TestFunctional_OwnerDuplicates_PairsOnDisplayName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Nothing about the names or the aliases connects these two.
	seedMergeOwner(t, db, "quenilda-stackhouse")
	seedMergeOwner(t, db, "brambleworth")
	for _, name := range []string{"quenilda-stackhouse", "brambleworth"} {
		displayName := "Perpetua Wintergreen"
		if _, err := db.UpdateOwner(ctx, name, UpdateOwnerParams{DisplayName: &displayName}); err != nil {
			t.Fatalf("UpdateOwner(%q): %v", name, err)
		}
	}

	got := scanForDuplicates(t, db)

	c, ok := candidatesByPair(t, got)[pairKey("quenilda-stackhouse", "brambleworth")]
	if !ok {
		t.Fatalf("two owners under one display name were not paired; got %d candidates", len(got))
	}
	if c.MatchedOn != "display_name" {
		t.Errorf("MatchedOn = %q, want %q", c.MatchedOn, "display_name")
	}
	if c.ValueA != "Perpetua Wintergreen" || c.ValueB != "Perpetua Wintergreen" {
		t.Errorf("the matched display names were not reported: %+v", c)
	}
}

// An owner's contact address is an identity we already hold. Left out of the
// alias table it is invisible to every lookup that resolves a person — the
// duplicate scan, the localpart signal, and an import matching on an address.
func TestFunctional_OwnerDuplicates_ContactEmailIsAnAlias(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// The name is unrelated to the address, so only the address can pair them.
	if _, err := db.InsertOwner(ctx, InsertOwnerParams{
		Name:         "wobbleton-perch",
		OwnerType:    "individual",
		ContactEmail: "gwendolen.fitzhammond@example-corp.test",
	}); err != nil {
		t.Fatalf("InsertOwner: %v", err)
	}
	t.Cleanup(func() { _, _ = db.DeleteOwner(context.Background(), "wobbleton-perch") })

	seeded, err := db.SeedAliasesFromContactEmails(ctx)
	if err != nil {
		t.Fatalf("SeedAliasesFromContactEmails: %v", err)
	}
	if seeded < 1 {
		t.Errorf("seeded %d aliases, want at least the one owner with a contact address", seeded)
	}

	owner, err := db.ResolveOwnerByAlias(ctx, "email", "gwendolen.fitzhammond@example-corp.test")
	if err != nil {
		t.Fatalf("the contact address does not resolve to its owner: %v", err)
	}
	if owner != "wobbleton-perch" {
		t.Errorf("resolved to %q", owner)
	}

	// Running it again must not fail on what it already seeded.
	if _, err := db.SeedAliasesFromContactEmails(ctx); err != nil {
		t.Errorf("seeding twice: %v", err)
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

// Owners at one company share an email domain, and a shared domain is most of
// a shared string. Comparing whole addresses made every owner look like every
// other one — measured at 0.33-0.41 against a floor of 0.3 for three names
// with nothing in common — so the view paired everybody with everybody and
// stopped being readable at exactly the scale it was built for.
func TestRecomputeOwnerDuplicateCandidates_ASharedEmailDomainIsNotASignal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, o := range []struct{ name, email string }{
		{"alice.brown", "alice.brown@example-corp.test"},
		{"bob.jones", "bob.jones@example-corp.test"},
		{"carol.white", "carol.white@example-corp.test"},
	} {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{
			Name: o.name, OwnerType: "individual", ContactEmail: o.email,
		}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", o.name, err)
		}
		defer func(n string) { _, _ = db.DeleteOwner(context.Background(), n) }(o.name)
	}

	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("RecomputeOwnerDuplicateCandidates: %v", err)
	}

	pairs, _, err := db.ListOwnerDuplicateCandidates(ctx, OwnerDuplicateFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListOwnerDuplicateCandidates: %v", err)
	}
	for _, p := range pairs {
		if isSeedOwner(p.OwnerA) && isSeedOwner(p.OwnerB) {
			t.Errorf("paired %s with %s on %s (%q vs %q, %.3f) — three unrelated people at one company",
				p.OwnerA, p.OwnerB, p.MatchedOn, p.ValueA, p.ValueB, p.Similarity)
		}
	}
}

func isSeedOwner(name string) bool {
	switch name {
	case "alice.brown", "bob.jones", "carol.white":
		return true
	}
	return false
}

// A pair somebody has looked at and rejected has to stay rejected. The scan
// rebuilds the candidate table on every run, so a dismissal that lived there
// would be swept away and the pair would come back — which is the whole
// complaint.
func TestDismissOwnerDuplicate_SurvivesARescan(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Two people who genuinely do look alike, so the scan keeps finding them.
	for _, n := range []string{"dave.taylor", "dave.tailor"} {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: n, OwnerType: "individual"}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", n, err)
		}
		defer func(n string) { _, _ = db.DeleteOwner(context.Background(), n) }(n)
	}

	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if !pairPresent(t, db, "dave.tailor", "dave.taylor") {
		t.Fatal("precondition: the scan should pair two names this alike")
	}

	if err := db.DismissOwnerDuplicate(ctx, "dave.tailor", "dave.taylor",
		"different people, confirmed with both", "tester"); err != nil {
		t.Fatalf("DismissOwnerDuplicate: %v", err)
	}
	if pairPresent(t, db, "dave.tailor", "dave.taylor") {
		t.Error("the pair is still listed immediately after being dismissed")
	}

	// The point of the whole thing.
	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if pairPresent(t, db, "dave.tailor", "dave.taylor") {
		t.Error("the dismissed pair came back after a rescan")
	}
}

// Dismissing works whichever way round the caller names the two, because the
// reader clicking it has no idea which order the scan stored them in.
func TestDismissOwnerDuplicate_OrderIndependent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, n := range []string{"erin.walsh", "erin.walshe"} {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: n, OwnerType: "individual"}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", n, err)
		}
		defer func(n string) { _, _ = db.DeleteOwner(context.Background(), n) }(n)
	}
	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Named in the reverse of the stored order.
	if err := db.DismissOwnerDuplicate(ctx, "erin.walshe", "erin.walsh", "", "tester"); err != nil {
		t.Fatalf("DismissOwnerDuplicate: %v", err)
	}
	if pairPresent(t, db, "erin.walsh", "erin.walshe") {
		t.Error("dismissing with the pair named in the other order had no effect")
	}

	// Saying it twice is not an error — a second click must not fail.
	if err := db.DismissOwnerDuplicate(ctx, "erin.walsh", "erin.walshe", "again", "tester"); err != nil {
		t.Errorf("dismissing twice: %v", err)
	}
}

func pairPresent(t *testing.T, db *DB, a, b string) bool {
	t.Helper()
	pairs, _, err := db.ListOwnerDuplicateCandidates(context.Background(), OwnerDuplicateFilter{Limit: 200})
	if err != nil {
		t.Fatalf("ListOwnerDuplicateCandidates: %v", err)
	}
	for _, p := range pairs {
		if (p.OwnerA == a && p.OwnerB == b) || (p.OwnerA == b && p.OwnerB == a) {
			return true
		}
	}
	return false
}

// A rejection has to be reversible, and a rejected pair is hidden from the
// list — so there is nothing to click unless it can be listed separately.
// Without both halves, a mis-click suppresses a pair permanently and
// invisibly, which is a worse failure than the one dismissing fixed.
func TestRestoreOwnerDuplicate_UndoesADismissal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, n := range []string{"frank.oneill", "frank.o-neill"} {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: n, OwnerType: "individual"}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", n, err)
		}
		defer func(n string) { _, _ = db.DeleteOwner(context.Background(), n) }(n)
	}
	if _, err := db.RecomputeOwnerDuplicateCandidates(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !pairPresent(t, db, "frank.oneill", "frank.o-neill") {
		t.Fatal("precondition: the scan should pair these two")
	}

	if err := db.DismissOwnerDuplicate(ctx, "frank.oneill", "frank.o-neill", "mis-click", "tester"); err != nil {
		t.Fatalf("DismissOwnerDuplicate: %v", err)
	}

	// It has to be findable, or there is nothing to undo.
	dismissed, err := db.ListOwnerDuplicateDismissals(ctx)
	if err != nil {
		t.Fatalf("ListOwnerDuplicateDismissals: %v", err)
	}
	var found *OwnerDuplicateDismissal
	for i := range dismissed {
		if dismissed[i].OwnerA == "frank.o-neill" || dismissed[i].OwnerB == "frank.o-neill" {
			found = &dismissed[i]
		}
	}
	if found == nil {
		t.Fatal("a dismissed pair is invisible — it cannot be undone")
	}
	if found.Reason != "mis-click" || found.DismissedBy != "tester" {
		t.Errorf("dismissal = %+v, want the reason and who recorded it", found)
	}

	if err := db.RestoreOwnerDuplicate(ctx, "frank.oneill", "frank.o-neill"); err != nil {
		t.Fatalf("RestoreOwnerDuplicate: %v", err)
	}
	if !pairPresent(t, db, "frank.oneill", "frank.o-neill") {
		t.Error("the pair did not come back after the dismissal was undone")
	}

	// Undoing something already undone is not an error — a second click has
	// changed nothing.
	if err := db.RestoreOwnerDuplicate(ctx, "frank.o-neill", "frank.oneill"); err != nil {
		t.Errorf("restoring twice, named the other way round: %v", err)
	}
}
