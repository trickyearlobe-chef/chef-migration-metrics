//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/scan-trust.md. Run it with `make journey`.
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
// The journey already points at assertions elsewhere. Where it does, the same
// property is asserted here directly rather than by reference, so each test
// stands on its own and a rename somewhere else cannot quietly empty this list.

const (
	// A finding whose removal is recorded, and earlier than the version being
	// moved to: the "we know the thing it names is gone" case.
	scanTrustRemovedCop = "Chef/Deprecations/NodeSet"
	// A finding the tool has never seen.
	scanTrustUnseenCop = "Lint/NobodyHasEverCuratedThis"
	// Cosmetic by RuboCop's own taxonomy, which is the positive reason.
	scanTrustCosmeticCop = "Style/TrailingWhitespace"
	// Test tooling rather than the code that runs on a machine.
	scanTrustToolingCop = "ChefSpec/CoverageReport"

	scanTrustTarget = "19.0"
)

func scanTrustJourneyResolver() *CopClassificationResolver {
	return &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: scanTrustTarget,
	}
}

// scanTrustFinding builds an offence as the read path hands it over: a cop, the
// scanner's severity, and the file it sits in.
func scanTrustFinding(cop, severity, file string) CookstyleOffense {
	return CookstyleOffense{CopName: cop, Severity: severity, File: file}
}

// scanTrustExclusionStore stands in for the operator's recorded decisions.
type scanTrustExclusionStore struct {
	rows []datastore.ScanScopeExclusion
}

func (s scanTrustExclusionStore) ListScanScopeExclusions(context.Context) ([]datastore.ScanScopeExclusion, error) {
	return s.rows, nil
}

// "**This will break** — we know the thing it names is gone in the version we
// are moving to."
func TestJourney_ThisWillBreak(t *testing.T) {
	got := scanTrustJourneyResolver().Resolve(scanTrustRemovedCop)
	if got.Classification != ClassificationBlocker {
		t.Errorf("a finding recorded as removed before %s is not called breaking: got %q from %q",
			scanTrustTarget, got.Classification, got.Source)
	}
	if got.Source != SourceVerifiedRemoval {
		t.Errorf("the breaking call is not made on the recorded removal: got %q", got.Source)
	}

	// The same finding, moving to a version before it was removed, must not be
	// breaking — or the red above says nothing about the version at all.
	earlier := &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "13.0"}
	if earlier.Resolve(scanTrustRemovedCop).Classification == ClassificationBlocker {
		t.Errorf("%s is called breaking even for a version it still exists in", scanTrustRemovedCop)
	}
}

// "**Somebody has to decide** — it might matter and nobody has established that
// it does."
func TestJourney_SomebodyHasToDecide(t *testing.T) {
	got := scanTrustJourneyResolver().Resolve(scanTrustUnseenCop)
	if got.Classification != ClassificationReview {
		t.Errorf("a finding nobody has established anything about resolves to %q, not to somebody having to decide",
			got.Classification)
	}
	if got.Source != SourceReviewDefault {
		t.Errorf("an unproven finding is not recorded as unproven: source %q", got.Source)
	}
}

// "**This is harmless** — and there is a positive reason for saying so, not just
// an absence of evidence that it is dangerous."
func TestJourney_ThisIsHarmlessOnlyForAPositiveReason(t *testing.T) {
	r := scanTrustJourneyResolver()

	for _, cop := range []string{scanTrustCosmeticCop, scanTrustToolingCop} {
		got := r.Resolve(cop)
		if got.Classification != ClassificationNoise {
			t.Errorf("%s is not called harmless (got %q), so the positive reason for saying so is not being used",
				cop, got.Classification)
		}
		if got.Source != SourceStructuralNoise {
			t.Errorf("%s is called harmless for the reason %q, which is not a positive structural one", cop, got.Source)
		}
	}

	// And absence of evidence is not the reason: a finding nothing is known
	// about must never arrive at harmless.
	if got := r.Resolve(scanTrustUnseenCop); got.Classification == ClassificationNoise {
		t.Errorf("a finding nobody knows anything about is called harmless, on no positive reason at all (source %q)", got.Source)
	}
}

// "Three plain answers per kind of finding, and no fourth one that dresses a
// guess up as an answer"
func TestJourney_NoFourthAnswer(t *testing.T) {
	r := scanTrustJourneyResolver()
	answers := map[string]bool{
		ClassificationBlocker: true,
		ClassificationReview:  true,
		ClassificationNoise:   true,
	}
	for _, cop := range []string{
		scanTrustRemovedCop, scanTrustUnseenCop, scanTrustCosmeticCop, scanTrustToolingCop,
		"Custom/SomethingWeWrote", "Chef/Correctness/NodeNormal", "", "Metrics/BlockLength",
	} {
		if got := r.Resolve(cop).Classification; !answers[got] {
			t.Errorf("%q is answered with %q, which is a fourth answer", cop, got)
		}
	}
}

// "For anything marked as breaking or harmless, why — on what basis that call
// was made."
func TestJourney_BreakingAndHarmlessSayOnWhatBasis(t *testing.T) {
	r := scanTrustJourneyResolver()
	bases := map[string]string{
		scanTrustRemovedCop:       SourceVerifiedRemoval,
		scanTrustCosmeticCop:      SourceStructuralNoise,
		scanTrustToolingCop:       SourceStructuralNoise,
		"Custom/SomethingWeWrote": SourceCustomCop,
	}
	for cop, want := range bases {
		got := r.Resolve(cop)
		if got.Classification == ClassificationReview {
			t.Fatalf("the fixture proves nothing: %s is neither breaking nor harmless", cop)
		}
		if got.Source != want {
			t.Errorf("%s is called %q on the basis %q, and the journey's basis for it is %q — "+
				`"the tool says so" is not an answer that survives being asked to justify a red`,
				cop, got.Classification, got.Source, want)
		}
	}
}

// "**Severity from the scanner is not a migration verdict, ever.**"
func TestJourney_SeverityIsNeverAMigrationVerdict(t *testing.T) {
	r := scanTrustJourneyResolver()

	// The loudest severity the scanner has, on a finding nobody has established
	// anything about, must not produce a red.
	for _, severity := range []string{"error", "fatal", "warning", "convention", "refactor", "info"} {
		offences := []CookstyleOffense{scanTrustFinding(scanTrustUnseenCop, severity, "recipes/default.rb")}
		if got := DeriveCookstyleStatus(offences, r); got == StatusBlocked {
			t.Errorf("severity %q alone blocks a cookbook, which is the ranking this exists to replace", severity)
		}
	}

	// And the quietest severity on a finding we know is gone must still be a red,
	// so the rule above cannot be satisfied by nothing ever blocking.
	quiet := []CookstyleOffense{scanTrustFinding(scanTrustRemovedCop, "convention", "recipes/default.rb")}
	if got := DeriveCookstyleStatus(quiet, r); got != StatusBlocked {
		t.Errorf("a finding we know is gone at the target does not block when its severity is quiet: got %q", got)
	}
}

// "**anything uncertain becomes somebody has to decide — never harmless.**"
func TestJourney_AnythingUncertainBecomesSomebodyHasToDecide(t *testing.T) {
	r := scanTrustJourneyResolver()
	// A finding the tool has never seen, a generic one from the underlying
	// linter, and a known one with no recorded removal.
	for _, cop := range []string{
		scanTrustUnseenCop,
		"Lint/UselessAssignment",
		"Chef/Correctness/NodeNormal",
		"Chef/Deprecations/HWRPWithoutUnifiedTrue",
	} {
		got := r.Resolve(cop)
		if got.Classification == ClassificationNoise {
			t.Errorf("%s is unproven and is called harmless anyway (source %q) — the failure nobody reports", cop, got.Source)
		}
		if got.Classification != ClassificationReview {
			t.Errorf("%s is unproven but does not land in the worklist: got %q", cop, got.Classification)
		}
	}
}

// "**A person's decision outranks everything the tool worked out**, including
// its own confident reds."
func TestJourney_APersonsDecisionOutranksTheTool(t *testing.T) {
	// Over a red the tool made on recorded evidence.
	overRed := &CopClassificationResolver{
		OperatorOverrides: map[string]string{scanTrustRemovedCop: ClassificationNoise},
		TargetChefVersion: scanTrustTarget,
	}
	if got := overRed.Resolve(scanTrustRemovedCop); got.Classification != ClassificationNoise ||
		got.Source != SourceOperatorOverride {
		t.Errorf("a person cannot overturn the tool's red: got %q from %q", got.Classification, got.Source)
	}

	// And over a harmless verdict, which is the direction that matters more.
	overHarmless := &CopClassificationResolver{
		OperatorOverrides: map[string]string{scanTrustCosmeticCop: ClassificationBlocker},
		TargetChefVersion: scanTrustTarget,
	}
	if got := overHarmless.Resolve(scanTrustCosmeticCop); got.Classification != ClassificationBlocker ||
		got.Source != SourceOperatorOverride {
		t.Errorf("a person cannot overturn a harmless verdict: got %q from %q", got.Classification, got.Source)
	}
}

// "When I have established what a finding really means, my decision sticks and
// everything downstream follows it, so that the next person does not rediscover
// the same thing."
func TestJourney_MyDecisionIsFollowedDownstream(t *testing.T) {
	decided := &CopClassificationResolver{
		OperatorOverrides: map[string]string{scanTrustRemovedCop: ClassificationNoise},
		TargetChefVersion: scanTrustTarget,
	}
	offences := []CookstyleOffense{scanTrustFinding(scanTrustRemovedCop, "warning", "recipes/default.rb")}

	if got := DeriveCookstyleStatus(offences, scanTrustJourneyResolver()); got != StatusBlocked {
		t.Fatalf("the fixture proves nothing: this finding does not block before the decision is recorded (got %q)", got)
	}
	if got := DeriveCookstyleStatus(offences, decided); got == StatusBlocked {
		t.Error("the cookbook's verdict ignores the decision a person recorded about the finding")
	}

	// And the same when the verdict is worked out again later from what was kept.
	kept := []datastore.FingerprintCopEntry{{CopName: scanTrustRemovedCop, Count: 1}}
	if got := DeriveStatusFromFingerprint(kept, decided); got == StatusBlocked {
		t.Error("a verdict re-derived later rediscovers the red a person had already overturned")
	}
}

// "**One target version at a time.** Findings are judged per finding, not per
// version ... A per-version matrix was tried and removed."
func TestJourney_OneTargetVersionAtATime(t *testing.T) {
	encoded, err := json.Marshal(datastore.CopClassification{CopName: scanTrustRemovedCop, Classification: ClassificationNoise})
	if err != nil {
		t.Fatalf("marshalling a recorded decision: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding a recorded decision: %v", err)
	}
	for key := range fields {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "version") || strings.Contains(lower, "target") {
			t.Errorf("a person's decision about a finding is recorded against %q as well as the finding, "+
				"so the same decision has to be made again per version", key)
		}
	}

	// A decision recorded against the finding alone takes effect.
	r := NewResolverFromStore(context.Background(), nil, scanTrustTarget)
	if r == nil {
		t.Fatal("no resolver can be built without a store, so classification needs operator input to work at all")
	}
}

// "**So a cookbook's verdict is about the code that ships and runs.** A finding
// in a file the converge never executes does not block the cookbook."
func TestJourney_AFindingInAFileTheConvergeNeverExecutesDoesNotBlock(t *testing.T) {
	r := scanTrustJourneyResolver()

	inCookbook := []CookstyleOffense{scanTrustFinding(scanTrustRemovedCop, "warning", "recipes/default.rb")}
	if got := DeriveCookstyleStatus(inCookbook, r); got != StatusBlocked {
		t.Fatalf("the fixture proves nothing: the same finding in cookbook code does not block either (got %q)", got)
	}

	inHelperTask := []CookstyleOffense{scanTrustFinding(scanTrustRemovedCop, "warning", "Rakefile")}
	if got := DeriveCookstyleStatus(inHelperTask, r); got == StatusBlocked {
		t.Error("a finding in a helper task blocks the cookbook, so nearly every cookbook looks broken")
	}
}

// "So a finding outside the cookbook stays visible on the cookbook, marked as
// not blocking"
func TestJourney_AFindingOutsideTheCookbookStaysVisible(t *testing.T) {
	offences := []CookstyleOffense{
		scanTrustFinding(scanTrustRemovedCop, "warning", "Rakefile"),
		scanTrustFinding(scanTrustRemovedCop, "warning", "recipes/default.rb"),
	}
	DeriveCookstyleStatus(offences, scanTrustJourneyResolver())

	// Deciding the verdict must not throw the helper task's work away.
	if len(offences) != 2 {
		t.Fatalf("deriving the verdict discarded findings: %d left of 2", len(offences))
	}
	if offences[0].Path() != "Rakefile" {
		t.Errorf("the helper task's finding lost the path that says whose problem it is: got %q", offences[0].Path())
	}

	// And it must be identifiable as the non-blocking kind, with the reason.
	scope := DefaultScanScope()
	ex, excluded := scope.Excluded(offences[0].Path())
	if !excluded {
		t.Error("nothing marks the helper task's finding as not blocking, so a reader cannot tell it apart")
	}
	if ex.Reason == "" {
		t.Error("the finding is marked not blocking with no reason recorded against it")
	}
	if scope.ExcludesOffense(offences[1]) {
		t.Error("the cookbook's own finding is marked not blocking, which is how a blocked list is made to look good")
	}
}

// "and it is counted across the estate — because the thing I most need to know
// about it is how widespread it is"
//
// Counted where the number is kept: what is retained about a finding must
// separate what blocks a cookbook from what is merely everywhere, or the number
// cannot be split anywhere downstream.
func TestJourney_CountedAcrossTheEstate(t *testing.T) {
	offences := []CookstyleOffense{
		scanTrustFinding(scanTrustRemovedCop, "warning", "Rakefile"),
		scanTrustFinding(scanTrustRemovedCop, "warning", "spec/default_spec.rb"),
		scanTrustFinding(scanTrustRemovedCop, "warning", "recipes/default.rb"),
	}
	entries, _ := BuildOffenceFingerprint(offences)
	if len(entries) != 1 {
		t.Fatalf("the fixture proves nothing: want one entry for one cop, got %d", len(entries))
	}
	if entries[0].Count != 3 {
		t.Errorf("the estate-wide count loses occurrences outside cookbook code: counted %d of 3", entries[0].Count)
	}
	if entries[0].ExcludedCount != 2 {
		t.Errorf("nothing separates what blocks a cookbook from what is merely everywhere: "+
			"%d of %d recorded as outside cookbook code, want 2",
			entries[0].ExcludedCount, entries[0].Count)
	}
}

// "**Which files are excluded is a decision somebody makes and can see, not a
// rule inferred.**"
func TestJourney_TheListOfExcludedFilesIsReadable(t *testing.T) {
	scope := NewScanScopeFromStore(context.Background(), scanTrustExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "tooling/ci/*", Excluded: true, Reason: "Only ever started by a build job."},
		},
	})
	listed := scope.Exclusions()
	if len(listed) == 0 {
		t.Fatal("the list of excluded files cannot be read, so a cookbook is judged by a list nobody can see")
	}

	var sawShipped, sawLocal bool
	for _, ex := range listed {
		if ex.Pattern == "Rakefile" {
			sawShipped = true
		}
		if ex.Pattern == "tooling/ci/*" {
			sawLocal = true
		}
	}
	if !sawShipped || !sawLocal {
		t.Errorf("the whole list is not readable together: shipped entry present=%v, local entry present=%v",
			sawShipped, sawLocal)
	}
}

// "Inferring the set of files the converge *could* reach does not work either
// ... any allowlist quietly discards whatever nobody thought of"
func TestJourney_AnythingNotNamedOnTheListStillCounts(t *testing.T) {
	scope := DefaultScanScope()
	for _, p := range []string{
		"libraries/helpers.rb",
		"some/place/nobody/thought/of.rb",
		"vendor/thing.rb",
	} {
		if scope.ExcludesPath(p) {
			t.Errorf("%q is excluded without being named, so files nobody thought of are discarded quietly", p)
		}
	}
	// A path we know nothing about is the same case.
	if scope.ExcludesPath("") {
		t.Error("a finding with no recorded path is excluded, which hides a real blocker in the direction nobody reports")
	}
}

// "**A file we ignore because a repository told us to is a file we ignore for
// the wrong reason.**"
//
// Asserted as the property that makes it true: the effective list is exactly the
// curated entries plus the operator's, so nothing a repository declares about
// itself can add to it.
func TestJourney_ARepositorysOwnDeclarationIsNotRead(t *testing.T) {
	const operatorPattern = "tooling/ci/*"
	scope := NewScanScopeFromStore(context.Background(), scanTrustExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: operatorPattern, Excluded: true, Reason: "Only ever started by a build job."},
		},
	})

	allowed := map[string]bool{operatorPattern: true}
	for _, ex := range DefaultScanScopeExclusions() {
		allowed[ex.Pattern] = true
	}
	for _, ex := range scope.Exclusions() {
		if !allowed[ex.Pattern] {
			t.Errorf("%q is excluded and came from neither the shipped list nor a person, "+
				"so somebody else's mistake is being presented as our verdict", ex.Pattern)
		}
	}
}

// "**And every exclusion needs a reason recorded against it**"
func TestJourney_EveryExclusionCarriesARecordedReason(t *testing.T) {
	for _, ex := range DefaultScanScopeExclusions() {
		if strings.TrimSpace(ex.Reason) == "" {
			t.Errorf("%q is excluded with no reason recorded against it, so it cannot be argued with", ex.Pattern)
		}
	}
	// And a person's own entry keeps the reason they gave, rather than the
	// prose it replaced.
	scope := NewScanScopeFromStore(context.Background(), scanTrustExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "Rakefile", Excluded: true, Reason: "Ours is generated by the build and never shipped."},
		},
	})
	ex, excluded := scope.Excluded("Rakefile")
	if !excluded {
		t.Fatal("a person's exclusion of Rakefile did not take effect")
	}
	if !strings.Contains(ex.Reason, "generated by the build") {
		t.Errorf("the reason shown is not the one the person stands behind: %q", ex.Reason)
	}
}

// "I can [name a file the shipped list never could] — the script that only runs
// because a build job starts it"
func TestJourney_ICanNameAFileTheShippedListNeverCould(t *testing.T) {
	const buildScript = "tooling/ci/preflight_checks.rb"
	if DefaultScanScope().ExcludesPath(buildScript) {
		t.Fatalf("the fixture proves nothing: the shipped list already names %q", buildScript)
	}
	scope := NewScanScopeFromStore(context.Background(), scanTrustExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "tooling/ci/*", Excluded: true, Reason: "Nothing loads it during a converge; the build job starts it."},
		},
	})
	if !scope.ExcludesPath(buildScript) {
		t.Errorf("a person cannot exclude %q, and no shipped list can name it for them", buildScript)
	}
}

// "and I can [overturn a shipped one] where it is simply wrong for us"
func TestJourney_ICanOverturnAShippedExclusion(t *testing.T) {
	const inTestDir = "test/helpers/shared.rb"
	if !DefaultScanScope().ExcludesPath(inTestDir) {
		t.Fatalf("the fixture proves nothing: the shipped list does not exclude %q", inTestDir)
	}
	scope := NewScanScopeFromStore(context.Background(), scanTrustExclusionStore{
		rows: []datastore.ScanScopeExclusion{
			{Pattern: "test/*", Excluded: false, Reason: "Our converge loads shared helpers from here."},
		},
	})
	if scope.ExcludesPath(inTestDir) {
		t.Errorf("a person cannot say %q really does run here and be believed", inTestDir)
	}
	if !scope.ExcludesPath("Rakefile") {
		t.Error("overturning one shipped entry took the rest of the list with it")
	}
}

// "Nothing takes effect [without a reason recorded against it]"
func TestJourney_NothingTakesEffectWithoutAReason(t *testing.T) {
	t.Skip("The reason is required where an exclusion is recorded, which is the API — " +
		"internal/webapi/handle_cookstyle_scan_scope_test.go. Nothing on this side can " +
		"refuse a row that reached the store without one.")
}

// "The same thing holds [when a verdict is worked out again later] from what was
// kept rather than from the findings themselves"
func TestJourney_AVerdictWorkedOutAgainLaterAgrees(t *testing.T) {
	r := scanTrustJourneyResolver()

	// Every occurrence sat outside cookbook code: the same answer the scan-time
	// path gives for the same findings.
	outside := []datastore.FingerprintCopEntry{{CopName: scanTrustRemovedCop, Count: 2, ExcludedCount: 2}}
	if got := DeriveStatusFromFingerprint(outside, r); got == StatusBlocked {
		t.Error("a verdict re-derived later re-blocks a cookbook the scan correctly passed")
	}

	// One of them was in cookbook code, and that one still decides.
	mixed := []datastore.FingerprintCopEntry{{CopName: scanTrustRemovedCop, Count: 2, ExcludedCount: 1}}
	if got := DeriveStatusFromFingerprint(mixed, r); got != StatusBlocked {
		t.Errorf("a verdict re-derived later loses a real blocker in cookbook code: got %q", got)
	}
}

// "and everything recorded before this existed [still reads as it did], rather
// than a whole estate turning green on the day it shipped"
func TestJourney_EverythingRecordedBeforeThisStillReadsAsItDid(t *testing.T) {
	// A record written before anything knew about scope carries no count of what
	// sat outside cookbook code.
	old := []datastore.FingerprintCopEntry{{CopName: scanTrustRemovedCop, Count: 4}}
	if got := DeriveStatusFromFingerprint(old, scanTrustJourneyResolver()); got != StatusBlocked {
		t.Errorf("a record written before scope existed now reads as %q instead of blocked, "+
			"so the estate turned green on the day this shipped", got)
	}
}

// "**The load-bearing assumption:** that every finding reaching a person carries
// the reason for its classification with it."
func TestJourney_EveryFindingCarriesTheReasonForItsClassification(t *testing.T) {
	r := &CopClassificationResolver{
		OperatorOverrides: map[string]string{"Chef/Style/SomethingDecided": ClassificationBlocker},
		TargetChefVersion: scanTrustTarget,
	}
	for _, cop := range []string{
		"Chef/Style/SomethingDecided", // a person's decision
		"Custom/SomethingWeWrote",     // written by us
		scanTrustRemovedCop,           // recorded removal
		scanTrustCosmeticCop,          // cosmetic
		scanTrustUnseenCop,            // unproven
	} {
		if got := r.Resolve(cop); got.Source == "" {
			t.Errorf("%s is classified %q with no reason attached, which is where this started: "+
				`"the tool says so"`, cop, got.Classification)
		}
	}
}

// "**Nothing proves the list of files we ignore is the right list.**"
func TestJourney_NothingProvesTheIgnoreListIsRight(t *testing.T) {
	t.Skip("The journey says nothing proves it. Every entry carries a reason, which is " +
		"asserted above; whether a reason is TRUE is a curated claim and fails in the " +
		"direction nobody reports.")
}

// "**And the Chef server side still misses code that does run** but sits outside
// the places we look, so its counts stay low in a way a reader cannot see."
func TestJourney_TheChefServerSideMissesCodeThatRuns(t *testing.T) {
	t.Skip("Named by the journey as a known gap, not a requirement. A test here would " +
		"assert the gap rather than close it.")
}

// "**Nothing proves the knowledge itself is right.** ... a confidently wrong
// entry produces a confidently wrong red."
func TestJourney_NothingProvesTheKnowledgeIsRight(t *testing.T) {
	t.Skip("The journey says no test here touches it: the rules decide what happens once a " +
		"finding is classified, and whether the recorded evidence about a finding is " +
		"accurate is a question about curated data.")
}

// "**Nothing proves the worklist gets worked.** ... the list simply grows —
// visible to anyone who looks at it, asserted by nothing."
func TestJourney_NothingProvesTheWorklistGetsWorked(t *testing.T) {
	t.Skip("Whether somebody triages the pile is not a property of the code. It would need " +
		"a measure of the worklist over time, and a target somebody has agreed to.")
}
