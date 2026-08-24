//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"reflect"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/staleness"
)

// The journey suite for journeys/node-readiness.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. That makes this the todo list for the journey, and a
// todo list made of tests cannot go stale: nobody has to remember to update it,
// because running it recomputes it.
//
// It is deliberately OUTSIDE the gating suite. Most of a journey is unbuilt for
// most of its life, so a red here is the normal state and must never block a
// build — a red that stops a release gets deleted, and then the list is gone.
//
// Two rules:
//
//   - Assert the real thing, so building the feature turns the test green with
//     no edit. A test that says "not implemented" has to be rewritten by the
//     person it was meant to help.
//   - Name the journey line it comes from, in the journey's words, so the
//     reason outlives whoever wrote it.
//
// This is not where regressions go. Something that used to work and now fails
// is a broken build, not a todo — parking it here hides it among the honest
// gaps, which are indistinguishable from it once they are in the same list.
//
// It lives in this package because that is where the verdict is decided.
// Anything that only exists at the HTTP layer cannot be reached from here:
// webapi imports analysis, so analysis cannot import webapi.

// journeyReadinessAmpleDisk is a filesystem with far more free space than any
// install needs, so a verdict built on it turns on the cookbooks alone.
func journeyReadinessAmpleDisk() map[string]linuxMount {
	return map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
	}
}

// journeyReadinessFullDisk is a filesystem too small for the install.
func journeyReadinessFullDisk() map[string]linuxMount {
	return map[string]linuxMount{
		"/dev/sda1": {KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%", Mount: "/"},
	}
}

// journeyReadinessEveryCheckFailing builds one machine that fails each of the
// three checks at once: a cookbook CookStyle says no to, a cookbook the converge
// testing says no to, and not enough disk.
func journeyReadinessEveryCheckFailing(t *testing.T) (*ReadinessEvaluator, *fakeReadinessDS, ReadinessResult) {
	t.Helper()

	ds := newFakeReadinessDS()
	// Blocked by the cookbook analysis.
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResultStatus("org-1", "apt", "7.4.0", "18.0", StatusBlocked)
	// Blocked by the converge testing: CookStyle is happy, Test Kitchen is not.
	ds.addCookbookID("nginx", "2.0.0", "org-1")
	ds.addCSResult("org-1", "nginx", "2.0.0", "18.0", true)
	ds.addGitRepoWithTK("nginx", "sha-nginx", true, false)
	ds.addGitCSResult("nginx", "18.0", true)
	ds.addGitTKStatus("nginx", "18.0", "failed")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(journeyReadinessFullDisk()))

	return e, ds, e.evaluateOne(snap, "18.0", ds.cookbookIDs, ds.buildFakeCache())
}

// "For every server, whether it is ready, and if not, which check failed — is it
// disk space, is it the cookbook analysis, is it the converge testing, or more
// than one."
//
// The point is the three checks answering separately. A single "blocked" would
// pass a test that only looked at the verdict, and would still send the reader
// into the machine to find out which check it was.
func TestJourney_SaysWhichCheckFailed(t *testing.T) {
	_, _, result := journeyReadinessEveryCheckFailing(t)

	if result.Status != StatusBlocked {
		t.Fatalf("the fixture proves nothing: a machine failing every check is %q, not %q",
			result.Status, StatusBlocked)
	}
	if result.SufficientDiskSpace == nil || *result.SufficientDiskSpace {
		t.Error("the disk check does not answer for itself, so a reader cannot tell " +
			"disk space apart from the other reasons a machine is blocked")
	}
	if result.CookstyleStatus != "failed" {
		t.Errorf("the cookbook analysis check reads %q, not failed, so it cannot be told "+
			"apart from the other reasons a machine is blocked", result.CookstyleStatus)
	}
	if result.KitchenStatus != "failed" {
		t.Errorf("the converge testing check reads %q, not failed, so it cannot be told "+
			"apart from the other reasons a machine is blocked", result.KitchenStatus)
	}
}

// "I need that on the list itself, at a glance, without opening anything."
//
// Asserted at the seam a list has to read from: the per-check verdicts are
// stored against the machine, so a list can show them without evaluating or
// opening anything. Whether the list then shows them is not answerable here.
func TestJourney_TheChecksAreStoredAgainstTheMachine(t *testing.T) {
	e, ds, result := journeyReadinessEveryCheckFailing(t)

	if err := e.persistResult(t.Context(), result); err != nil {
		t.Fatalf("storing a verdict: %v", err)
	}
	if len(ds.upserted) != 1 {
		t.Fatalf("expected one stored verdict, got %d", len(ds.upserted))
	}
	stored := ds.upserted[0]

	if stored.Status == "" {
		t.Error("the stored verdict does not say whether the machine is ready")
	}
	if stored.CookstyleStatus == "" {
		t.Error("the cookbook analysis result is not stored against the machine, so a list " +
			"cannot show which check failed without opening it")
	}
	if stored.KitchenStatus == "" {
		t.Error("the converge testing result is not stored against the machine, so a list " +
			"cannot show which check failed without opening it")
	}
	if stored.SufficientDiskSpace == nil {
		t.Error("the disk result is not stored against the machine, so a list cannot show " +
			"which check failed without opening it")
	}
}

// "Colour alone is not enough; I have colleagues who cannot rely on it and I
// read these lists tired."
func TestJourney_ColourAloneIsNotEnough(t *testing.T) {
	t.Skip("Answered where the marks are drawn, not where the verdict is decided: " +
		"frontend/src/components/CheckStatusIcons.test.tsx asserts a spoken label and a " +
		"distinct shape per check. The journey already calls that a stand-in — it is " +
		"evidence the information survives without colour, not proof it is readable.")
}

// "When I do open a machine, the specific reasons — which cookbooks are
// blocking it ..."
func TestJourney_NamesTheCookbooksBlockingTheMachine(t *testing.T) {
	_, _, result := journeyReadinessEveryCheckFailing(t)

	if len(result.BlockingCookbooks) == 0 {
		t.Fatal("a blocked machine does not say which cookbooks blocked it")
	}
	for _, bc := range result.BlockingCookbooks {
		if bc.Name == "" || bc.Version == "" {
			t.Errorf("a blocking cookbook is not named: %+v", bc)
		}
		if bc.Reason == "" {
			t.Errorf("%s %s blocks the machine without saying why", bc.Name, bc.Version)
		}
		if len(bc.Verdicts) == 0 {
			t.Errorf("%s %s blocks the machine without saying which check said so",
				bc.Name, bc.Version)
		}
	}
}

// "... how much disk it actually has against how much it needs ..."
//
// Both numbers, not a verdict: a machine short by a few megabytes and a machine
// short by a hundred gigabytes are the same "insufficient" and different jobs.
func TestJourney_ShowsTheDiskItHasAgainstTheDiskItNeeds(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(journeyReadinessAmpleDisk()))
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, ds.buildFakeCache())

	if result.AvailableDiskMB == nil {
		t.Error("a machine with filesystem data does not say how much disk it actually has")
	}
	if result.RequiredDiskMB <= 0 {
		t.Errorf("a machine does not say how much disk it needs (got %d)", result.RequiredDiskMB)
	}
}

// "... and what it pulls in, so I can see whether the problem is this machine or
// something it inherits."
func TestJourney_ShowsWhatTheMachinePullsIn(t *testing.T) {
	t.Skip("Cannot be reached from this package: the run list → roles → cookbooks tree is " +
		"assembled in the HTTP layer, and webapi imports analysis so analysis cannot " +
		"import webapi. Recorded here so the journey line is not silently unaccounted " +
		"for; it belongs in a webapi-side test.")
}

// "So I need those told apart: reporting normally, quiet for a while, and gone."
//
// Three states, three distinct answers. Two of them collapsing into one is the
// failure the journey describes — a machine that will come back on its own and a
// machine that never will, treated the same.
func TestJourney_ReportingQuietAndGoneAreToldApart(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	thresholds := staleness.Thresholds{WarningHours: 24, CriticalDays: 30}

	reporting := staleness.ComputeTier(now.Add(-30*time.Minute), now, thresholds)
	quiet := staleness.ComputeTier(now.Add(-7*24*time.Hour), now, thresholds)
	gone := staleness.ComputeTier(now.Add(-180*24*time.Hour), now, thresholds)

	if reporting == quiet || quiet == gone || reporting == gone {
		t.Errorf("the three states are not told apart: reporting=%q quiet=%q gone=%q",
			reporting, quiet, gone)
	}
	// A machine that has never reported at all is gone, not fine.
	if neverSeen := staleness.ComputeTier(time.Time{}, now, thresholds); neverSeen != gone {
		t.Errorf("a machine that has never reported reads as %q rather than gone", neverSeen)
	}
}

// "I need to be able to set the whole 'gone' pile aside without losing it."
//
// Asserted at the seam: the machine list can be selected by which of the three
// states a machine is in, so the gone pile can be taken out of view. Setting
// aside is a selection, never a deletion — nothing here proves the rows survive,
// only that removing them is not how it is done.
func TestJourney_TheGonePileCanBeSetAside(t *testing.T) {
	filter := reflect.TypeOf(datastore.NodeSnapshotFilter{})
	for i := 0; i < filter.NumField(); i++ {
		f := filter.Field(i)
		if f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String &&
			(f.Name == "StaleTiers" || f.Name == "StalenessTiers") {
			return
		}
	}
	t.Error("machines cannot be selected by which of the three states they are in, so the " +
		"gone pile can only be set aside one machine at a time")
}

// "A machine we cannot see must not be reported as a machine that is fine."
//
// The baseline is asserted first: the same machine, reporting normally, IS
// ready. Without that this passes the moment anything else stops a machine being
// ready, and the absence-of-evidence rule stops being what is under test.
func TestJourney_AMachineWeCannotSeeIsNeverReportedAsFine(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	cookbooks := cookbooksJSON(map[string]string{"apt": "7.4.0"})
	filesystem := linuxFilesystemJSON(journeyReadinessAmpleDisk())

	fresh := e.evaluateOne(makeSnapshot("org-1", "node-1", false, cookbooks, filesystem),
		"18.0", ds.cookbookIDs, ds.buildFakeCache())
	if !fresh.IsReady {
		t.Fatalf("the fixture proves nothing: the same machine reporting normally is not "+
			"ready either (status %q)", fresh.Status)
	}

	unseen := e.evaluateOne(makeSnapshot("org-1", "node-1", true, cookbooks, filesystem),
		"18.0", ds.cookbookIDs, ds.buildFakeCache())
	if unseen.IsReady || unseen.Status == StatusReady {
		t.Error("a machine we have no recent data for is reported as ready, so absence of " +
			"evidence renders as a pass")
	}
}

// "If we have no recent data, say so — do not let absence of evidence render as
// a pass."
//
// Not just "not ready": the machine has to say the data is old and the checks
// unknown, or the reader treats it as a real failure and goes looking for one.
func TestJourney_NoRecentDataSaysSo(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", true,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(journeyReadinessAmpleDisk()))
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, ds.buildFakeCache())

	if !result.StaleData {
		t.Error("a machine we cannot see does not say its data is old")
	}
	if result.SufficientDiskSpace != nil {
		t.Errorf("the disk check answers %v from data we know is old, rather than saying "+
			"it does not know", *result.SufficientDiskSpace)
	}
	if result.CookstyleStatus != "unknown" {
		t.Errorf("the cookbook analysis check reads %q from data we know is old, rather "+
			"than unknown", result.CookstyleStatus)
	}
	if result.KitchenStatus != "unknown" {
		t.Errorf("the converge testing check reads %q from data we know is old, rather "+
			"than unknown", result.KitchenStatus)
	}
}

// "That a blocked machine can never come back as ready is pinned by the
// compatibility contract."
//
// The linked contract holds it for one cookbook. This holds it for the machine,
// under both settings of the review toggle, because that is the level the
// journey speaks at and a machine rollup is a separate piece of code.
func TestJourney_ABlockedMachineNeverComesBackAsReady(t *testing.T) {
	for _, reviewBlocks := range []bool{false, true} {
		ds := newFakeReadinessDS()
		ds.addCookbookID("apt", "7.4.0", "org-1")
		ds.addCSResultStatus("org-1", "apt", "7.4.0", "18.0", StatusBlocked)

		e := NewReadinessEvaluator(ds, nil, 1, 2048)
		snap := makeSnapshot("org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(journeyReadinessAmpleDisk()))

		cache := ds.buildFakeCache()
		cache.reviewBlocksReadiness = reviewBlocks
		result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

		if result.IsReady || result.Status == StatusReady {
			t.Errorf("review_blocks_readiness=%v: a machine with a blocked cookbook comes "+
				"back as ready", reviewBlocks)
		}
	}
}

// "I can go from 'a hundred and fifty thousand servers' to 'these are the ones I
// have to deal with this week, and here is why' without opening a single
// machine."
func TestJourney_TheListIsUsableAtAGlance(t *testing.T) {
	t.Skip("The journey says nothing proves this and no assertion stands in for it: " +
		"whether the list is usable at a glance across tens of thousands of rows is a " +
		"judgement made by looking at it.")
}
