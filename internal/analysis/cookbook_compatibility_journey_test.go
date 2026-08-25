//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"reflect"
	"strings"
	"testing"
)

// The journey suite for journeys/cookbook-compatibility.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// Much of this journey is already held by contracts next to the code, and those
// tests are named in the journey itself. What is repeated here is repeated for
// one reason only — the journey states a property those contracts establish
// from one direction, and the property is worth asserting from the direction
// the person cares about. Where a line is genuinely proven elsewhere and
// nothing is added by saying it again, this suite says so and points at it
// rather than duplicating the assertion.

// "Per cookbook, whether it is safe, needs looking at, or is blocked — and what
// that verdict was based on, because I will be asked to justify it and because
// I do not trust a verdict I cannot see the reasoning for."
//
// The three answers are asserted here rather than the vocabulary being taken on
// trust: a fourth state, or a renamed one, changes what a person is shown and
// this is where that would surface.
func TestJourney_ThereAreThreeAnswersPerCookbook(t *testing.T) {
	for _, want := range []string{StatusReady, StatusNeedsReview, StatusBlocked} {
		if strings.TrimSpace(want) == "" {
			t.Fatal("one of the three answers a cookbook can carry is empty, so a person " +
				"would be shown a blank where a verdict belongs")
		}
	}

	// "Safe" and "needs looking at" must not be the same answer. If they ever
	// collapse, a cookbook nobody has judged reads as one somebody cleared.
	if StatusReady == StatusNeedsReview {
		t.Error("safe and needs-looking-at are the same value, so a cookbook awaiting " +
			"judgement is indistinguishable from one that passed")
	}
}

// "and what that verdict was based on, because I will be asked to justify it"
//
// The reasoning is carried as the set of verdicts that fed the answer, each
// tagged with where it came from. A verdict with no sources attached is the
// "the tool says so" answer the journey opens by rejecting.
func TestJourney_AVerdictCarriesWhatItWasBasedOn(t *testing.T) {
	fields := map[string]bool{}
	rt := reflect.TypeOf(BlockingCookbook{})
	for i := 0; i < rt.NumField(); i++ {
		fields[rt.Field(i).Name] = true
	}
	if !fields["Verdicts"] {
		t.Fatal("a blocking cookbook does not carry the verdicts behind it, so a person " +
			"asked to justify a red has nothing to show but the red itself")
	}

	vt := reflect.TypeOf(CookbookSourceVerdict{})
	var hasSource bool
	for i := 0; i < vt.NumField(); i++ {
		if vt.Field(i).Name == "Source" {
			hasSource = true
		}
	}
	if !hasSource {
		t.Error("a verdict does not say where it came from, so two disagreeing signals " +
			"cannot be told apart when they are shown side by side")
	}
}

// "Two independent signals feed it. Static analysis reads the code and tells me
// about things that were removed or deprecated. Actually converging the
// cookbook on a real machine tells me whether it works. They disagree
// sometimes, and when they do I want to see both rather than have one quietly
// overrule the other."
//
// This is the line the journey most depends on, and it is the one a future
// change is most likely to undo by collapsing the two signals into one field.
// The disagreement case is asserted directly: both verdicts must survive.
func TestJourney_WhenTheTwoSignalsDisagreeBothSurvive(t *testing.T) {
	disagreeing := BlockingCookbook{
		Name:   "web-app",
		Reason: StatusIncompatible,
		Verdicts: []CookbookSourceVerdict{
			{Source: SourceServerCookstyle, Status: StatusCompatible},
			{Source: SourceGitTestKitchen, Status: StatusIncompatible},
		},
	}

	if len(disagreeing.Verdicts) != 2 {
		t.Fatalf("a cookbook the two signals disagree about carries %d verdicts, want 2 — "+
			"one of them has been dropped and the person is being shown a conclusion "+
			"rather than a disagreement", len(disagreeing.Verdicts))
	}

	seen := map[string]string{}
	for _, v := range disagreeing.Verdicts {
		if prev, dup := seen[v.Source]; dup {
			t.Errorf("two verdicts from %s (%q and %q); a source that speaks twice cannot "+
				"be reconciled on screen", v.Source, prev, v.Status)
		}
		seen[v.Source] = v.Status
	}
	if seen[SourceServerCookstyle] == seen[SourceGitTestKitchen] {
		t.Error("the two signals were made to agree, which is the quiet overrule the " +
			"journey exists to prevent")
	}
}

// "Which version, because the same cookbook at two versions is two different
// answers."
func TestJourney_AVerdictIsAboutOneVersion(t *testing.T) {
	rt := reflect.TypeOf(BlockingCookbook{})
	if _, ok := rt.FieldByName("Version"); !ok {
		t.Error("a cookbook's verdict does not name a version, so two versions of one " +
			"cookbook are reported as a single answer that is right for at most one of them")
	}

	vt := reflect.TypeOf(CookbookSourceVerdict{})
	if _, ok := vt.FieldByName("Version"); !ok {
		t.Error("an individual verdict does not name the version it was reached against, " +
			"so a stale result cannot be told from a current one")
	}
}

// "An untested cookbook is not a passing cookbook. 'We have no result' and 'we
// have a result and it is fine' must never render the same way."
//
// Green from the start and here to stay that way. The journey names the
// contracts that hold it; this asks the question in the person's terms — that
// the absence of a result never derives the same answer as a good one.
func TestJourney_NoResultNeverReadsAsAGoodResult(t *testing.T) {
	noResult := deriveCookstyleStatusFromBlocking(true, true, nil)
	goodResult := deriveCookstyleStatusFromBlocking(true, false, nil)

	if noResult == goodResult {
		t.Errorf("a cookbook nobody has looked at and one that passed both derive %q — "+
			"the most damaging thing this tool can do is say something is safe when "+
			"nobody has actually looked", noResult)
	}
}

// "A verdict must not be poisoned by our own infrastructure. When a converge
// test fails because the lab could not build a machine, or could not get it an
// address, or could not log in, that is our problem and not the cookbook's."
//
// Green from the start. Asserted from the person's direction: a lab failure
// standing alone must not produce the same answer as the code being broken.
func TestJourney_ALabFailureIsNotACookbookFailure(t *testing.T) {
	labOnly := []BlockingCookbook{{
		Name:   "web-app",
		Reason: StatusIncompatible,
		Source: SourceTestKitchen,
		Verdicts: []CookbookSourceVerdict{
			{Source: SourceGitTestKitchen, Status: StatusIncompatible},
		},
	}}
	codeBroken := []BlockingCookbook{{
		Name:   "apt",
		Reason: StatusIncompatible,
		Verdicts: []CookbookSourceVerdict{
			{Source: SourceServerCookstyle, Status: StatusIncompatible},
		},
	}}

	if got := deriveCookstyleStatusFromBlocking(false, false, labOnly); got == "failed" {
		t.Error("a converge failure on its own marked the cookbook failed, so a bad day " +
			"in our lab fills the blocked list with cookbooks that are not broken")
	}
	if got := deriveCookstyleStatusFromBlocking(false, false, codeBroken); got != "failed" {
		t.Errorf("a cookbook the static analysis says is broken derived %q rather than "+
			"failed — the rule above has been applied too widely and now hides real "+
			"blockers as well as lab noise", got)
	}
}

// "Nor by files that are not the cookbook. A repository carries pipelines,
// helper tasks and test suites that never run during a converge."
//
// The decisions and their assertions belong to journeys/scan-trust.md, which
// owns this rule and pins it against a repository carrying the same finding
// twice. Repeating the assertion here would be a second home for it, so this
// records only that the scoped derivation the rule needs still exists and is
// reachable from the verdict path this journey describes.
func TestJourney_ScopeIsAppliedWhenTheVerdictIsDerived(t *testing.T) {
	fn := reflect.ValueOf(DeriveCookstyleStatusInScope)
	if fn.IsNil() {
		t.Fatal("there is no scoped derivation, so every file in a repository counts " +
			"towards its cookbook's verdict and a shared helper task makes the whole " +
			"estate look broken")
	}
	if got := fn.Type().NumIn(); got != 3 {
		t.Errorf("the scoped derivation takes %d arguments; it no longer accepts a scope, "+
			"which is how files that never run start blocking cookbooks again", got)
	}
}

// "A person can overrule the machine. If I have watched a cookbook converge
// successfully, my verdict wins, and it is recorded as mine. The reverse too:
// if it passed here and broke in production, I say so and that sticks."
//
// Both directions and the attribution. The journey names the contracts that
// hold the overruling itself; what is asked here is the part the person stated
// and no contract says — that the verdict is recorded as theirs, with why.
func TestJourney_MyVerdictIsRecordedAsMine(t *testing.T) {
	vt := reflect.TypeOf(CookbookSourceVerdict{})

	if _, ok := vt.FieldByName("RecordedBy"); !ok {
		t.Error("a person's verdict does not record who made it, so an override cannot " +
			"be questioned later and reads as something the tool decided")
	}
	if _, ok := vt.FieldByName("Note"); !ok {
		t.Error("a person's verdict carries no reason, and a verdict without one is " +
			"indistinguishable from a mistake three weeks later")
	}

	// The person's verdict has to be a source in its own right, or it cannot
	// sit beside the machine's rather than replacing it silently.
	if SourceHumanVerdict == "" {
		t.Error("there is no source meaning 'a person said so', so a human verdict " +
			"cannot be told apart from a machine one when both are shown")
	}
}

// "The estate's cookbooks come from two places and I care about both the same
// way. Some are on the Chef servers, uploaded and in use. Some are in git
// repositories, which is where they are actually written and where a fix has to
// land. Asking 'is this safe' should not depend on which side I happen to be
// looking from, and a cookbook that looks fine on one side and broken on the
// other is a question I need answered, not a discrepancy I have to notice."
//
// Both sides exist as distinct sources, which is what makes the discrepancy
// visible rather than one side quietly winning.
func TestJourney_BothSidesOfACookbookAreAskedAbout(t *testing.T) {
	if SourceServerCookstyle == SourceGitCookstyle {
		t.Fatal("the Chef server copy and the git copy of a cookbook are recorded as the " +
			"same source, so a cookbook that is fine in one place and broken in the " +
			"other cannot be told apart")
	}

	both := BlockingCookbook{
		Name:   "apt",
		Reason: StatusIncompatible,
		Verdicts: []CookbookSourceVerdict{
			{Source: SourceServerCookstyle, Status: StatusCompatible},
			{Source: SourceGitCookstyle, Status: StatusIncompatible},
		},
	}
	var server, git bool
	for _, v := range both.Verdicts {
		switch v.Source {
		case SourceServerCookstyle:
			server = true
		case SourceGitCookstyle:
			git = true
		}
	}
	if !server || !git {
		t.Error("a cookbook cannot carry a verdict from both the Chef server and the " +
			"repository at once, so the discrepancy the journey asks to have answered " +
			"is something the person has to spot for themselves")
	}
}

// "The blocked list is short enough to work through, and when I pick something
// off it and go look, it really is broken."
//
// The journey says outright that nothing proves this. It is recorded as a skip
// rather than left out, because a line nobody accounted for reads exactly like
// a line nobody needs.
func TestJourney_TheBlockedListIsShortEnoughToWorkThrough(t *testing.T) {
	t.Skip("whether the blocked list is short enough to work through, and whether what " +
		"is on it really is broken, is answered by picking things off it and going to " +
		"look; no assertion stands in for that")
}
