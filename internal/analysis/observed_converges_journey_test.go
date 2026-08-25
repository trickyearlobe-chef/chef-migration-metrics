//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"reflect"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/observed-converges.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. That makes this the todo list for the journey, and a
// todo list made of tests cannot go stale.
//
// It is deliberately OUTSIDE the gating suite. Most of this journey is unbuilt,
// so a red here is the normal state and must never block a build.

// observedConvergeFields reports the field names on a type, so a test can ask
// whether something has been carried without naming a field that does not exist
// yet.
func observedConvergeFields(v any) []string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		out = append(out, t.Field(i).Name)
	}
	return out
}

func observedConvergeHasField(v any, name string) bool {
	for _, f := range observedConvergeFields(v) {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "To have a cookbook raise itself as a blocker when it converges cleanly at the
// version the estate is on now and fails at the target version."
//
// Nothing reads converge events when deciding whether a cookbook is blocked, so
// the pairing cannot raise anything. The baseline is asserted first: the human
// verdict path — the thing an automatic entry would join — does work.
func TestJourney_APairRaisesABlockerByItself(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-apache", "abc123", true, false)
	cache := f.buildFakeCache()

	// Baseline: an entry in the register does reach the verdict. Without this
	// the test below could pass for the wrong reason.
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-apache": {SubjectName: "acme-apache", Verdict: datastore.VerdictBroken, RaisedBy: "bob"},
	}
	if status, _, _ := checkCookbookCompatibility(
		"acme-apache", "2.0.0", "19.0.0", f.cookbookIDs, cache); status != StatusIncompatible {
		t.Fatalf("a register entry does not reach the verdict at all (%q), so this test cannot "+
			"tell whether an observed pair would", status)
	}

	// The real question: is there anywhere for observed converges to enter?
	if observedConvergeHasField(readinessCache{}, "observedConverges") ||
		observedConvergeHasField(readinessCache{}, "convergeRuns") {
		return
	}
	t.Error("nothing carries observed converge outcomes into the compatibility decision, so a " +
		"cookbook that works on the current version and fails on the target cannot raise itself " +
		"— somebody has to notice and write it down")
}

// "Only the pairing raises a blocker. A failure at the target version on its own
// could be a cookbook that has been broken for a year."
func TestJourney_AFailureAtTargetAloneDoesNotRaise(t *testing.T) {
	t.Skip("Cannot be answered until observed converges reach the decision at all: with no " +
		"path in, there is nothing that could distinguish a pair from a lone failure. Held by " +
		"TestJourney_APairRaisesABlockerByItself until that goes green.")
}

// "To see the cookbooks already failing at the version the estate is on now,
// kept apart from the ones that break at the target. They are still worth
// fixing — but they are an existing problem rather than evidence about the
// upgrade."
func TestJourney_AlreadyBrokenCookbooksAreReportedSeparately(t *testing.T) {
	t.Skip("Cannot be answered until observed converges reach the decision at all: with no " +
		"path in, there is no set of already-failing cookbooks to keep apart from anything. " +
		"Held by TestJourney_APairRaisesABlockerByItself until that goes green.")
}

// "A cookbook already failing at the current version is reported, and does not
// block the upgrade. It is real work and somebody should fix it, but it says
// nothing about the version we are moving to."
func TestJourney_AlreadyBrokenDoesNotBlockTheUpgrade(t *testing.T) {
	t.Skip("Cannot be answered until observed converges reach the decision at all: nothing " +
		"can be shown not to block when nothing reaches the verdict. Held by " +
		"TestJourney_APairRaisesABlockerByItself until that goes green.")
}

// "It also cannot say anything: until it converges cleanly today there is no
// pairing to be had, so its migration verdict is unknown rather than good."
//
// The blind spot, which is the part most likely to be lost. Reporting the work
// and calling the cookbook fine are two different answers, and a cookbook that
// has never converged cleanly at the current version has earned neither a pass
// nor a blocker — it has earned "we cannot tell yet".
func TestJourney_AlreadyBrokenReadsAsUnknownNotGood(t *testing.T) {
	t.Skip("Cannot be answered until observed converges reach the decision at all: with no " +
		"path in, every cookbook's observed verdict is absent rather than unknown. Held by " +
		"TestJourney_APairRaisesABlockerByItself until that goes green.")
}

// "To overrule it myself ... and when I mark it good, for that to stand over the
// scan and the lab both."
func TestJourney_MarkingItGoodStandsOverTheScanAndTheLab(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-apache", "abc123", true, false)
	f.addGitCSResult("acme-apache", "19.0.0", false)
	f.addGitTKStatus("acme-apache", "19.0.0", "failed")

	cache := f.buildFakeCache()
	cache.tkBlocksReadiness = true
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-apache": {
			SubjectName: "acme-apache",
			Verdict:     datastore.VerdictNotBroken,
			Reason:      "seen converging at the target version across the estate",
			RaisedBy:    "bob",
		},
	}

	status, source, _ := checkCookbookCompatibility(
		"acme-apache", "2.0.0", "19.0.0", f.cookbookIDs, cache)
	if status != StatusCompatible || source != SourceHumanVerdict {
		t.Errorf("status/source = %q/%q, want %q/%q — a person who has looked outranks both "+
			"the scan and the lab", status, source, StatusCompatible, SourceHumanVerdict)
	}
}

// "with the screen saying a person decided rather than quietly showing a clean
// result."
func TestJourney_TheOverruleIsVisibleNotSilent(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-apache", "abc123", true, false)
	f.addGitCSResult("acme-apache", "19.0.0", false)
	f.addGitTKStatus("acme-apache", "19.0.0", "failed")

	cache := f.buildFakeCache()
	cache.tkBlocksReadiness = true
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-apache": {SubjectName: "acme-apache", Verdict: datastore.VerdictNotBroken, RaisedBy: "bob"},
	}

	_, _, verdicts := checkCookbookCompatibility(
		"acme-apache", "2.0.0", "19.0.0", f.cookbookIDs, cache)

	for _, src := range []string{SourceGitCookstyle, SourceGitTestKitchen} {
		v, ok := findVerdict(verdicts, src)
		if !ok {
			t.Errorf("the %s verdict was dropped, so a reader cannot tell it was overruled "+
				"rather than never consulted", src)
			continue
		}
		if v.Status != StatusIncompatible {
			t.Errorf("the losing %s verdict reads %q; it must be kept as it was reported",
				src, v.Status)
		}
	}
}

// "To be told when one of these is worth another look: across the whole estate,
// every converge at the target version we saw for it in the last stretch
// succeeded. That is a prompt to go and check, not a verdict."
func TestJourney_AQuietStretchRaisesAPromptNotAVerdict(t *testing.T) {
	if observedConvergeHasField(datastore.StandingVerdict{}, "worthAnotherLook") ||
		observedConvergeHasField(datastore.StandingVerdict{}, "recheck") {
		return
	}
	t.Error("nothing marks a register entry as worth looking at again, so a cookbook that has " +
		"started converging cleanly at the target version stays blocked until somebody happens " +
		"to revisit it")
}

// "How long a stretch is depends on how often the estate converges ... so it is
// set, not fixed."
func TestJourney_HowLongAStretchIsCanBeSet(t *testing.T) {
	var ingest config.IngestConfig
	for _, name := range observedConvergeFields(ingest) {
		if strings.Contains(strings.ToLower(name), "recheck") ||
			strings.Contains(strings.ToLower(name), "window") {
			return
		}
	}
	t.Error("the stretch a clean sweep is measured over cannot be set, so an estate converging " +
		"every half hour and one converging weekly are judged over the same span — and for one " +
		"of them a clean sweep is a handful of machines")
}

// "Whether observed success may ever clear a blocker on its own is a switch, and
// it starts off."
func TestJourney_ClearingAutomaticallyStartsOff(t *testing.T) {
	var ingest config.IngestConfig
	for _, name := range observedConvergeFields(ingest) {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "autoclear") || strings.Contains(lower, "clears") {
			return
		}
	}
	t.Error("there is no switch governing whether observed success may clear a blocker without " +
		"a person, so the question is settled by whatever the code happens to do rather than by " +
		"somebody deciding they trust it")
}

// "The same run must not count twice because both routes saw it."
func TestJourney_TheSameRunDoesNotCountTwice(t *testing.T) {
	t.Skip("Needs a real database: the guard is the stored run's identity, and it is held by " +
		"TestFunctional_ConvergeRuns_UpsertDedupAndRetention in internal/datastore.")
}

// "Others already send them somewhere that keeps them ... and we go and ask it on
// a schedule."
func TestJourney_EventsCanBeFetchedOnASchedule(t *testing.T) {
	t.Skip("Nothing fetches converge events from anywhere — the only route in is the one that " +
		"is pushed to us. There is no surface in this package to assert against; it belongs " +
		"beside the ingest path once a fetching source exists.")
}

// ---------------------------------------------------------------------------
// What nothing can prove
// ---------------------------------------------------------------------------

// "Nothing proves a quiet day means anything."
func TestJourney_AQuietStretchMeansTheCookbookIsFixed(t *testing.T) {
	t.Skip("The journey says nothing proves it: a stretch with every run passing is only as " +
		"strong as what ran in it. That is why it is a prompt for a person and not a verdict.")
}

// "Nothing proves the estate is a fair sample."
func TestJourney_TheEstateIsAFairSample(t *testing.T) {
	t.Skip("Not answerable from this product. The machines that reached the target version " +
		"first are the ones somebody chose to move, and they are rarely the difficult ones.")
}

// "The load-bearing assumption: that a failed run names the cookbook that failed,
// often enough to attribute."
func TestJourney_AFailedRunNamesTheCookbookThatFailed(t *testing.T) {
	t.Skip("Answerable only against real deliveries, not from here: a dependency that will " +
		"not resolve fails before any cookbook runs, so there is nothing to blame. Measure " +
		"what proportion arrive unattributable before trusting anything that counts them.")
}
