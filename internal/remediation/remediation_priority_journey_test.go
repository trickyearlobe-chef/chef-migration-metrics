//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"reflect"
	"testing"
)

// The journey suite for journeys/remediation-priority.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// The suite lives in this package because the two numbers the ranking is made
// of — how much work a fix is, and how much of the estate it frees — are
// computed here. Parts of the journey are decided in packages this one cannot
// import without a cycle (internal/analysis imports this one, and the read path
// that renders a list is in internal/webapi). Those lines are recorded as skips
// naming where the assertion belongs, rather than left out: a line nobody
// accounted for reads exactly like a line nobody needs.

// "For each broken thing: roughly how much work it is, and how much of the
// estate it unblocks. Those two together are the ranking."
//
// Both halves on one record. Either alone is not a ranking — a cheap fix that
// frees nothing and an expensive one that frees everything are the same number
// until the two sit side by side.
func TestJourney_TheTwoNumbersTheRankingIsMadeOfSitTogether(t *testing.T) {
	rt := reflect.TypeOf(ComplexityResult{})

	if _, ok := rt.FieldByName("ComplexityScore"); !ok {
		t.Error("a broken thing does not say roughly how much work it is, so the list " +
			"cannot be ordered by what it costs me")
	}

	// How much of the estate it unblocks, in the three ways the estate is
	// organised. Machines alone understates a cookbook buried in a base role.
	for _, field := range []string{
		"AffectedNodeCount",
		"AffectedRoleCount",
	} {
		if _, ok := rt.FieldByName(field); !ok {
			t.Errorf("a broken thing does not say how much of the estate it unblocks "+
				"(%s is missing), so a one-line fix in something everything depends on "+
				"ranks the same as a fortnight of work two machines care about", field)
		}
	}
}

// "roughly how much work it is"
//
// Roughly is the word the person used: a label, not just a number. A raw score
// is not something anybody can act on without knowing the scale it sits in.
func TestJourney_HowMuchWorkItIsIsSaidInWordsNotOnlyANumber(t *testing.T) {
	if _, ok := reflect.TypeOf(ComplexityResult{}).FieldByName("ComplexityLabel"); !ok {
		t.Fatal("the effort figure carries no label, so a person reading the list has to " +
			"learn the scoring scale before the number means anything")
	}

	// The label has to actually vary with the score, or it is decoration.
	low, high := ScoreToLabel(0), ScoreToLabel(10000)
	if low == high {
		t.Errorf("a trivial fix and an enormous one are both labelled %q, so the label "+
			"tells a person nothing about what they are picking up", low)
	}
}

// "Not everything a scan reports is a migration problem. Most of what static
// analysis produces is style and modernisation advice that has nothing to do
// with whether the thing survives the version change."
//
// Asserted from the person's direction on the path the ranking actually uses:
// tidying advice must not make a fix look expensive. If it does, a genuinely
// breaking finding is pushed down the list by noise, which is the exact failure
// the journey opens with.
func TestJourney_TidyingAdviceDoesNotInflateWhatAFixCosts(t *testing.T) {
	tidyingOnly := []ClassifiedOffense{
		{CopName: "Style/StringLiterals", Severity: "convention", Classification: classNoise},
		{CopName: "Layout/TrailingWhitespace", Severity: "convention", Classification: classNoise},
		{CopName: "Style/GuardClause", Severity: "convention", Classification: classNoise},
	}
	breaking := []ClassifiedOffense{
		{CopName: "Chef/Deprecations/NodeSet", Severity: "warning", Classification: classBlocker},
	}

	tidyingCost := ComputeCookstyleComplexity(tidyingOnly)
	breakingCost := ComputeCookstyleComplexity(breaking)

	if tidyingCost >= breakingCost {
		t.Errorf("three pieces of tidying advice cost %d and one thing that will actually "+
			"break costs %d — the list is ordered by volume of advice rather than by what "+
			"stops the upgrade, and after the second false alarm I stop reading it",
			tidyingCost, breakingCost)
	}
}

// The same line, against the scoring the service falls back to when no
// classifier has been wired. The journey states the property with no condition
// on it, and on this path it does not hold: modernisation advice carries weight,
// so enough of it outranks a finding that will actually break the upgrade.
//
// Red because the condition is real, not because the fallback is wrong to
// exist. It goes green when the fallback either classifies or is removed.
func TestJourney_TidyingAdviceDoesNotInflateWhatAFixCostsWithoutAClassifier(t *testing.T) {
	tidyingOnly := ComplexityInput{
		CookbookName: "web-app",
		Cookstyle:    CookstyleOffenseSummary{ModernizeCount: 50},
	}
	breaking := ComplexityInput{
		CookbookName: "apt",
		Cookstyle:    CookstyleOffenseSummary{ErrorFatalCount: 1},
	}

	tidyingCost := ComputeComplexityScore(tidyingOnly)
	breakingCost := ComputeComplexityScore(breaking)

	if tidyingCost >= breakingCost {
		t.Errorf("with no classifier wired, fifty pieces of modernisation advice cost %d "+
			"and one thing that will actually break costs %d — a deployment that has not "+
			"reached the classified path ranks by volume of advice, and the journey states "+
			"the opposite with no condition attached", tidyingCost, breakingCost)
	}
}

// "What is actually wrong, in enough detail to start — not 'this cookbook has
// offences' but which ones"
func TestJourney_IAmToldWhichFindingsNotJustThatThereAreSome(t *testing.T) {
	rt := reflect.TypeOf(EnrichedOffense{})
	for _, field := range []string{"CopName", "Message", "Location"} {
		if _, ok := rt.FieldByName(field); !ok {
			t.Errorf("a finding does not carry %s, so the list can say a cookbook has "+
				"problems but not what they are or where to start", field)
		}
	}
}

// "and why each matters for this upgrade rather than in general."
//
// The distinction is the whole point: a general description of a lint rule is
// available from the linter. What the person needs is the version the thing was
// removed in, because that is what makes it a migration problem rather than
// advice.
func TestJourney_AFindingSaysWhyItMattersForThisUpgrade(t *testing.T) {
	rt := reflect.TypeOf(CopMapping{})
	if _, ok := rt.FieldByName("RemovedIn"); !ok {
		t.Fatal("a finding does not say which version removed the thing it names, so " +
			"nothing distinguishes a migration blocker from general lint advice")
	}
	if _, ok := rt.FieldByName("Description"); !ok {
		t.Error("a finding carries no explanation, so a person cannot start on it without " +
			"going and reading the rule themselves")
	}

	// A known removal must actually resolve, or the mapping is a shape with
	// nothing in it and every finding reads as unclassified.
	if CopMappingCount() == 0 {
		t.Error("nothing is mapped at all, so no finding can say why it matters for this " +
			"upgrade and the whole list degrades to 'the tool says so'")
	}
}

// "Where a fix can be applied automatically, what it would change, before it
// changes anything. I will not run an automatic rewrite across this estate on
// trust."
//
// Seeing the count is not seeing the change. The person said "what it would
// change", so the actual difference has to be in the preview.
func TestJourney_ISeeWhatAnAutomaticFixWouldChangeBeforeItChangesAnything(t *testing.T) {
	rt := reflect.TypeOf(AutocorrectPreviewResult{})

	if _, ok := rt.FieldByName("DiffOutput"); !ok {
		t.Fatal("a preview does not carry the change it would make, so agreeing to an " +
			"automatic rewrite across the estate is done on trust")
	}
	for _, field := range []string{"FilesModified", "CorrectableOffenses"} {
		if _, ok := rt.FieldByName(field); !ok {
			t.Errorf("a preview does not say %s, so the size of what is about to happen "+
				"is not visible before it happens", field)
		}
	}
}

// "One thing worth knowing is only half proven. A finding the mapping does not
// recognise is asserted to come back as 'no mapping' — but nothing asserts what
// the list then *does* with it. Whether an unrecognised finding is surfaced for
// a human or quietly dropped is decided elsewhere."
//
// Half of it can be held here: an unrecognised finding must come back as
// unrecognised rather than as something else.
func TestJourney_AnUnrecognisedFindingComesBackAsUnrecognised(t *testing.T) {
	if got := LookupCop("Nonexistent/Cop/NobodyHasEverWritten"); got != nil {
		t.Errorf("a finding nothing knows about resolved to %q, so an unclassified "+
			"problem is being presented as a classified one", got.CopName)
	}
}

// "Whether an unrecognised finding is surfaced for a human or quietly dropped
// is decided elsewhere and is not pinned here, and dropping it silently is the
// failure mode this journey cares about most."
func TestJourney_AnUnrecognisedFindingIsSurfacedRatherThanDropped(t *testing.T) {
	t.Skip("what the list does with an unrecognised finding is decided on the read path " +
		"in internal/webapi, which this package cannot reach; the journey names this as " +
		"the failure mode it cares about most and it is still unheld")
}

// "Which of it is mine, or my team's, so I can cut the list down to what I am
// responsible for."
func TestJourney_ICanCutTheListDownToWhatIsMine(t *testing.T) {
	t.Skip("filtering a list by owner is held in internal/datastore and internal/webapi, " +
		"which this package cannot import; journeys/ownership-attribution.md owns the " +
		"rule that ownership reaches every view")
}

// "Verify before designing against this: the ranking assumes each broken thing
// carries the verdicts of every source that had an opinion on it, tagged with
// which source, and a rule deciding which wins."
func TestJourney_EachBrokenThingCarriesEverySourcesVerdict(t *testing.T) {
	t.Skip("the structure this assumes belongs to internal/analysis, which imports this " +
		"package, so it cannot be reached from here; it is asserted there by " +
		"TestJourney_AVerdictCarriesWhatItWasBasedOn and " +
		"TestJourney_WhenTheTwoSignalsDisagreeBothSurvive")
}

// "Nothing proves the ranking itself. No test asserts that cost against benefit
// produces a sensible order, and none could — 'sensible' is the engineer's
// judgement, and the journey's own success test is a feeling about the first
// page."
func TestJourney_TheRankingPutsTheRightThingFirst(t *testing.T) {
	t.Skip("whether cost against benefit produces a sensible order is the engineer's " +
		"judgement, checked by working down the list and not finding themselves asking " +
		"why something is on it; no assertion stands in for that")
}
