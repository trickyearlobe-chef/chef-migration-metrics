// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// recordEntry records a verdict and registers its removal. Entries are keyed
// on a repo name, and the register never deletes in production — the cleanup
// exists so a shared test database does not accumulate rows across runs.
func recordEntry(t *testing.T, db *DB, p RecordFailureVerdictParams) FailureRegisterEntry {
	t.Helper()
	if p.RaisedBy == "" {
		p.RaisedBy = "tester"
	}
	entry, err := db.RecordFailureVerdict(context.Background(), p)
	if err != nil {
		t.Fatalf("RecordFailureVerdict(%q): %v", p.GitRepoName, err)
	}
	t.Cleanup(func() { deleteRegisterEntriesForRepo(db, p.GitRepoName) })
	return entry
}

// deleteRegisterEntriesForRepo clears a repo's entries between runs. The
// register never deletes in production — resolution is recorded instead — so
// this lives in the test rather than in the datastore.
func deleteRegisterEntriesForRepo(db *DB, gitRepoName string) {
	_, _ = db.pool.ExecContext(context.Background(),
		`DELETE FROM failure_register_entries WHERE git_repo_name = $1`, gitRepoName)
}

// A verdict with no reason is an opinion. The register exists to hold verdicts
// that survive the next person who disagrees, so the reason is not optional and
// the rejection has to come from the store rather than from a form.
func TestRecordFailureVerdict_RequiresAReason(t *testing.T) {
	db := testDB(t)

	for _, reason := range []string{"", "   ", "\t\n"} {
		_, err := db.RecordFailureVerdict(context.Background(), RecordFailureVerdictParams{
			GitRepoName:  "acme-apache",
			CookbookName: "apache",
			Verdict:      VerdictBroken,
			Reason:       reason,
			RaisedBy:     "tester",
		})
		if err == nil {
			t.Fatalf("recording a verdict with reason %q was accepted; it must be rejected", reason)
		}
	}
}

// Both sides of the verdict are first class: one says "this is broken and you
// missed it", the other says "this is not actually broken whatever the scan
// says". Anything else is not a verdict this register holds.
func TestRecordFailureVerdict_BothSidesAndNothingElse(t *testing.T) {
	db := testDB(t)

	for _, verdict := range []string{VerdictBroken, VerdictNotBroken} {
		entry := recordEntry(t, db, RecordFailureVerdictParams{
			GitRepoName:  "acme-" + verdict,
			CookbookName: "acme",
			Verdict:      verdict,
			Reason:       "seen on a real converge",
		})
		if entry.Verdict != verdict {
			t.Errorf("verdict = %q, want %q", entry.Verdict, verdict)
		}
		if entry.Status != FailureStatusOpen {
			t.Errorf("a new entry has status %q, want %q", entry.Status, FailureStatusOpen)
		}
	}

	if _, err := db.RecordFailureVerdict(context.Background(), RecordFailureVerdictParams{
		GitRepoName:  "acme-maybe",
		CookbookName: "acme",
		Verdict:      "probably_fine",
		Reason:       "a hunch",
		RaisedBy:     "tester",
	}); err == nil {
		t.Fatal("an unrecognised verdict was accepted")
	}
}

// The subject is the repo, labelled with the cookbook. A version must not
// reach the record at all — several are in use at once and the failure is
// discussed version-agnostically.
func TestRecordFailureVerdict_KeyedOnRepoLabelledWithCookbook(t *testing.T) {
	db := testDB(t)

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-nginx-cookbook",
		CookbookName: "nginx",
		Verdict:      VerdictBroken,
		Reason:       "fails to compile on the target version",
	})

	if entry.GitRepoName != "acme-nginx-cookbook" {
		t.Errorf("git repo = %q", entry.GitRepoName)
	}
	if entry.CookbookName != "nginx" {
		t.Errorf("cookbook = %q", entry.CookbookName)
	}
	if entry.ID == "" {
		t.Error("entry has no id")
	}
	if entry.RaisedAt.IsZero() {
		t.Error("entry has no raised-at timestamp")
	}
}

// A second verdict on a repo that already has one is a reversal, not an
// overwrite. Who said what, when and why is the point of the register, so the
// first verdict stays readable and points at the one that replaced it.
func TestRecordFailureVerdict_SupersedesRatherThanReplaces(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	first := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-tomcat",
		CookbookName: "tomcat",
		Verdict:      VerdictBroken,
		Reason:       "converge fails resolving the java dependency",
		RaisedBy:     "alice",
	})

	second, err := db.RecordFailureVerdict(ctx, RecordFailureVerdictParams{
		GitRepoName:  "acme-tomcat",
		CookbookName: "tomcat",
		Verdict:      VerdictNotBroken,
		Reason:       "the dependency was pinned; it has run in production for a month",
		RaisedBy:     "bob",
	})
	if err != nil {
		t.Fatalf("recording the reversal: %v", err)
	}

	reread, err := db.GetFailureRegisterEntry(ctx, first.ID)
	if err != nil {
		t.Fatalf("re-reading the first verdict: %v", err)
	}
	if reread.Status != FailureStatusSuperseded {
		t.Errorf("the first verdict has status %q, want %q", reread.Status, FailureStatusSuperseded)
	}
	if reread.SupersededBy != second.ID {
		t.Errorf("the first verdict points at %q, want the reversal %q", reread.SupersededBy, second.ID)
	}
	if reread.Reason == "" || reread.RaisedBy != "alice" {
		t.Error("the superseded verdict lost its reason or its author; it must stay readable")
	}
	if second.Status != FailureStatusOpen {
		t.Errorf("the reversal has status %q, want %q", second.Status, FailureStatusOpen)
	}

	// One repo, one standing verdict — and the losing one still on the record.
	history, err := db.ListFailureRegisterHistory(ctx, "acme-tomcat")
	if err != nil {
		t.Fatalf("ListFailureRegisterHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d entries, want 2 — the disagreement must stay visible", len(history))
	}
}

// Resolution is recorded, not deleted. Journey 6 needs the direction of
// travel, and an entry that disappears when fixed makes the list look
// permanently static.
func TestResolveFailureEntry_RecordsRatherThanDeletes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-mysql",
		CookbookName: "mysql",
		Verdict:      VerdictBroken,
		Reason:       "the service resource never starts",
	})

	resolved, err := db.ResolveFailureEntry(ctx, entry.ID, "carol", "fixed in 4.2.0 and rolled out")
	if err != nil {
		t.Fatalf("ResolveFailureEntry: %v", err)
	}
	if resolved.Status != FailureStatusResolved {
		t.Errorf("status = %q, want %q", resolved.Status, FailureStatusResolved)
	}
	if resolved.ResolvedBy != "carol" || resolved.ResolvedAt == nil {
		t.Error("a resolved entry must say who resolved it and when")
	}
	if resolved.Reason == "" {
		t.Error("resolving must not erase the reason the entry was raised")
	}

	if _, err := db.GetFailureRegisterEntry(ctx, entry.ID); err != nil {
		t.Fatalf("the resolved entry is unreadable: %v", err)
	}

	// Resolving frees the repo for a new verdict — the same cookbook can break
	// again, and that is a new entry rather than a reopening.
	again, err := db.RecordFailureVerdict(ctx, RecordFailureVerdictParams{
		GitRepoName:  "acme-mysql",
		CookbookName: "mysql",
		Verdict:      VerdictBroken,
		Reason:       "broke again on the next release",
		RaisedBy:     "dave",
	})
	if err != nil {
		t.Fatalf("recording a failure after resolution: %v", err)
	}
	if again.Status != FailureStatusOpen {
		t.Errorf("the new entry has status %q, want %q", again.Status, FailureStatusOpen)
	}

	// The resolved entry must not have been dragged into the reversal chain.
	reread, err := db.GetFailureRegisterEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("re-reading the resolved entry: %v", err)
	}
	if reread.Status != FailureStatusResolved {
		t.Errorf("the resolved entry became %q — resolving and superseding are different things", reread.Status)
	}
}

// Resolving something already resolved is a mistake, not an update: it would
// overwrite who resolved it and when.
func TestResolveFailureEntry_OnlyOpenEntries(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-redis",
		CookbookName: "redis",
		Verdict:      VerdictBroken,
		Reason:       "template renders an invalid config",
	})
	if _, err := db.ResolveFailureEntry(ctx, entry.ID, "carol", ""); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := db.ResolveFailureEntry(ctx, entry.ID, "dave", ""); err == nil {
		t.Fatal("resolving an already-resolved entry was accepted")
	}
}

// A failure is worth recording the moment it is seen. The diagnosis, the plan
// and the target date arrive later, so they are revisable — but the verdict
// and the reason are not, because a reversal is a new verdict.
func TestReviseFailureEntry_PlanAndHolderOnly(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-haproxy",
		CookbookName: "haproxy",
		Verdict:      VerdictBroken,
		Reason:       "the config template uses a removed DSL method",
	})
	if entry.Plan != "" || entry.TargetDate != nil || entry.HolderRef != "" {
		t.Error("a new entry should carry no plan, holder or target date unless one was given")
	}

	target := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	revised, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		Diagnosis:  strPtr("removed in Chef 18; needs the resource rewritten"),
		Plan:       strPtr("rewrite the template and re-release"),
		TargetDate: &target,
		HolderType: strPtr(HolderTypeTicket),
		HolderRef:  strPtr("PLAT-4821"),
	})
	if err != nil {
		t.Fatalf("ReviseFailureEntry: %v", err)
	}
	if revised.Plan != "rewrite the template and re-release" {
		t.Errorf("plan = %q", revised.Plan)
	}
	if revised.TargetDate == nil || !revised.TargetDate.Equal(target) {
		t.Errorf("target date = %v, want %v", revised.TargetDate, target)
	}
	if revised.HolderType != HolderTypeTicket || revised.HolderRef != "PLAT-4821" {
		t.Errorf("holder = %q/%q", revised.HolderType, revised.HolderRef)
	}
	if revised.Verdict != entry.Verdict || revised.Reason != entry.Reason {
		t.Error("revising changed the verdict or the reason; a reversal must be a new verdict")
	}

	// An omitted field is left alone rather than blanked — a revision that
	// only sets a target date must not wipe the plan somebody else wrote.
	only, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		TargetDate: &target,
	})
	if err != nil {
		t.Fatalf("second ReviseFailureEntry: %v", err)
	}
	if only.Plan != "rewrite the template and re-release" || only.Diagnosis == "" {
		t.Error("a partial revision blanked a field it was not given")
	}
}

// A holder is a type and a reference together, or neither. Half a reference
// cannot be looked up and cannot be chased.
func TestReviseFailureEntry_RejectsHalfAHolder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-consul",
		CookbookName: "consul",
		Verdict:      VerdictBroken,
		Reason:       "agent fails to register",
	})

	if _, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		HolderType: strPtr(HolderTypeOwner),
	}); err == nil {
		t.Error("a holder type with no reference was accepted")
	}
	if _, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		HolderRef: strPtr("someone"),
	}); err == nil {
		t.Error("a holder reference with no type was accepted")
	}
	if _, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		HolderType: strPtr("slack_channel"),
		HolderRef:  strPtr("#platform"),
	}); err == nil {
		t.Error("an unrecognised holder type was accepted")
	}
}

// A stacktrace is unbounded text, and the failure mode that has bitten this
// project is unbounded text reaching an index. Storing it must work and
// reading it back must be lossless.
func TestRecordFailureVerdict_StoresAnUnboundedStacktrace(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	trace := strings.Repeat("        from /var/chef/cache/cookbooks/acme/recipes/default.rb:42:in `block in from_file'\n", 400)

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-bigtrace",
		CookbookName: "bigtrace",
		Verdict:      VerdictBroken,
		Reason:       "compile error on the target version",
		Evidence:     trace,
	})

	reread, err := db.GetFailureRegisterEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetFailureRegisterEntry: %v", err)
	}
	if reread.Evidence != trace {
		t.Errorf("evidence came back %d bytes, want %d", len(reread.Evidence), len(trace))
	}
}

// The readiness evaluator reads the standing verdicts as a lookup keyed on the
// repo name. Superseded and resolved verdicts must not be in it — they are
// history, not the current view.
func TestListOpenFailureVerdicts_OnlyTheStandingOnes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	standing := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-standing",
		CookbookName: "standing",
		Verdict:      VerdictBroken,
		Reason:       "breaks on every converge",
	})
	overruled := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-overruled",
		CookbookName: "overruled",
		Verdict:      VerdictNotBroken,
		Reason:       "cookstyle is wrong; this has run for months",
	})
	fixed := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-fixed",
		CookbookName: "fixed",
		Verdict:      VerdictBroken,
		Reason:       "was broken",
	})
	if _, err := db.ResolveFailureEntry(ctx, fixed.ID, "carol", "fixed"); err != nil {
		t.Fatalf("resolving: %v", err)
	}

	verdicts, err := db.ListOpenFailureVerdicts(ctx)
	if err != nil {
		t.Fatalf("ListOpenFailureVerdicts: %v", err)
	}

	got, ok := verdicts["acme-standing"]
	if !ok {
		t.Fatal("the standing broken verdict is missing")
	}
	if got.Verdict != VerdictBroken || got.Reason != standing.Reason {
		t.Errorf("standing verdict = %+v", got)
	}
	if got.CookbookName != "standing" {
		t.Errorf("cookbook label = %q", got.CookbookName)
	}

	if v, ok := verdicts["acme-overruled"]; !ok || v.Verdict != VerdictNotBroken {
		t.Errorf("the overruling verdict is missing or wrong: %+v", v)
	}
	if _, ok := verdicts["acme-fixed"]; ok {
		t.Error("a resolved entry is still being served as a standing verdict")
	}
	_ = overruled
}

// The standup view reads the open list with the reason and the commitment
// visible, not a click away.
func TestListFailureRegisterEntries_TheStandupList(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	broken := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-list-broken",
		CookbookName: "listbroken",
		Verdict:      VerdictBroken,
		Reason:       "the recipe raises on compile",
		Plan:         "rewrite and re-release",
	})
	recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-list-ok",
		CookbookName: "listok",
		Verdict:      VerdictNotBroken,
		Reason:       "kitchen never converged; the cookbook is fine",
	})
	done := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-list-done",
		CookbookName: "listdone",
		Verdict:      VerdictBroken,
		Reason:       "was broken",
	})
	if _, err := db.ResolveFailureEntry(ctx, done.ID, "carol", "shipped"); err != nil {
		t.Fatalf("resolving: %v", err)
	}

	open, _, err := db.ListFailureRegisterEntries(ctx, FailureRegisterFilter{Status: FailureStatusOpen})
	if err != nil {
		t.Fatalf("ListFailureRegisterEntries: %v", err)
	}
	names := map[string]FailureRegisterEntry{}
	for _, e := range open {
		names[e.GitRepoName] = e
	}
	if _, ok := names["acme-list-done"]; ok {
		t.Error("a resolved entry is in the open list")
	}
	got, ok := names["acme-list-broken"]
	if !ok {
		t.Fatal("the open broken entry is missing from the list")
	}
	if got.Reason != broken.Reason || got.Plan != "rewrite and re-release" {
		t.Error("the list must carry the reason and the plan; the standup reads them at a glance")
	}
	if got.CookbookName == "" {
		t.Error("the list must carry the cookbook label")
	}

	// Filtering by verdict is what makes the list readable as an accuracy
	// report of the automated signals.
	onlyOverruled, _, err := db.ListFailureRegisterEntries(ctx, FailureRegisterFilter{
		Status:  FailureStatusOpen,
		Verdict: VerdictNotBroken,
	})
	if err != nil {
		t.Fatalf("filtering by verdict: %v", err)
	}
	for _, e := range onlyOverruled {
		if e.Verdict != VerdictNotBroken {
			t.Errorf("verdict filter returned a %q entry", e.Verdict)
		}
	}
}

// Whether the list is getting too large. The size and the direction matter as
// much as the contents — a register that is growing is a different message
// from one that is shrinking.
func TestFailureRegisterSummary_SizeAndDirection(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	before, err := db.FailureRegisterSummary(ctx, 7)
	if err != nil {
		t.Fatalf("FailureRegisterSummary: %v", err)
	}

	a := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-sum-a",
		CookbookName: "suma",
		Verdict:      VerdictBroken,
		Reason:       "broken on converge",
	})
	recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-sum-b",
		CookbookName: "sumb",
		Verdict:      VerdictNotBroken,
		Reason:       "the scan is wrong",
	})
	recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-sum-c",
		CookbookName: "sumc",
		Verdict:      VerdictBroken,
		Reason:       "also broken",
	})
	if _, err := db.ResolveFailureEntry(ctx, a.ID, "carol", "fixed"); err != nil {
		t.Fatalf("resolving: %v", err)
	}

	after, err := db.FailureRegisterSummary(ctx, 7)
	if err != nil {
		t.Fatalf("FailureRegisterSummary: %v", err)
	}

	if got := after.Open - before.Open; got != 2 {
		t.Errorf("open rose by %d, want 2", got)
	}
	if got := after.RaisedInWindow - before.RaisedInWindow; got != 3 {
		t.Errorf("raised-in-window rose by %d, want 3", got)
	}
	if got := after.ResolvedInWindow - before.ResolvedInWindow; got != 1 {
		t.Errorf("resolved-in-window rose by %d, want 1", got)
	}

	// The accuracy report: what the tools missed, and what they got wrong.
	if got := after.OpenBroken - before.OpenBroken; got != 1 {
		t.Errorf("open broken rose by %d, want 1", got)
	}
	if got := after.OpenNotBroken - before.OpenNotBroken; got != 1 {
		t.Errorf("open overruled rose by %d, want 1", got)
	}

	// Nobody has been put on either of the open entries, which is a standup
	// question in its own right.
	if got := after.OpenWithoutHolder - before.OpenWithoutHolder; got != 2 {
		t.Errorf("open-without-holder rose by %d, want 2", got)
	}
}

// A target date that has passed while the entry is still open is the thing
// the standup most needs pointed out.
func TestFailureRegisterSummary_CountsOverdueCommitments(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	before, err := db.FailureRegisterSummary(ctx, 7)
	if err != nil {
		t.Fatalf("FailureRegisterSummary: %v", err)
	}

	entry := recordEntry(t, db, RecordFailureVerdictParams{
		GitRepoName:  "acme-overdue",
		CookbookName: "overdue",
		Verdict:      VerdictBroken,
		Reason:       "still broken",
	})
	past := time.Now().AddDate(0, 0, -3)
	if _, err := db.ReviseFailureEntry(ctx, entry.ID, ReviseFailureEntryParams{
		TargetDate: &past,
		HolderType: strPtr(HolderTypeTicket),
		HolderRef:  strPtr("PLAT-1"),
	}); err != nil {
		t.Fatalf("ReviseFailureEntry: %v", err)
	}

	after, err := db.FailureRegisterSummary(ctx, 7)
	if err != nil {
		t.Fatalf("FailureRegisterSummary: %v", err)
	}
	if got := after.OpenOverdue - before.OpenOverdue; got != 1 {
		t.Errorf("overdue rose by %d, want 1", got)
	}
	if got := after.OpenWithoutHolder - before.OpenWithoutHolder; got != 0 {
		t.Errorf("open-without-holder rose by %d; this entry has a holder", got)
	}
}

// Reading, revising or resolving something that is not there must say so
// rather than reporting a confident nothing.
func TestFailureRegister_MissingEntry(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const missing = "00000000-0000-0000-0000-000000000000"

	if _, err := db.GetFailureRegisterEntry(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFailureRegisterEntry on a missing id returned %v, want ErrNotFound", err)
	}
	if _, err := db.ResolveFailureEntry(ctx, missing, "carol", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("ResolveFailureEntry on a missing id returned %v, want ErrNotFound", err)
	}
	if _, err := db.ReviseFailureEntry(ctx, missing, ReviseFailureEntryParams{
		Plan: strPtr("something"),
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReviseFailureEntry on a missing id returned %v, want ErrNotFound", err)
	}
}

func strPtr(s string) *string { return &s }
