//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/converge-testing.md. Run it with `make journey`.
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
// The suite lives in this package because the verdict rules the journey leans
// on are here, and they are unexported. Some of what the journey asks for is
// built in packages this one cannot import without a cycle (internal/gitkitchen
// and internal/batch both import internal/analysis). Those lines are recorded
// as skips naming where the assertion actually lives, rather than left out —
// a line nobody accounted for reads exactly like a line nobody needs.

// convergeJourneySelectionFields returns the json field names a saved batch
// selection can be expressed in. Selection is what the journey means by
// "choose the set", and this is the vocabulary it is stored in.
func convergeJourneySelectionFields() []string {
	t := reflect.TypeOf(datastore.BatchFilters{})
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = strings.ToLower(t.Field(i).Name)
		}
		names = append(names, name)
	}
	return names
}

// convergeJourneySelectionMentions reports whether any selection field name
// contains one of the given words.
func convergeJourneySelectionMentions(words ...string) bool {
	for _, field := range convergeJourneySelectionFields() {
		for _, w := range words {
			if strings.Contains(field, w) {
				return true
			}
		}
	}
	return false
}

// convergeJourneyResultKeys returns the json keys a finished converge result
// carries to whoever is reading it.
func convergeJourneyResultKeys(t *testing.T, v any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling a converge result: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding a converge result: %v", err)
	}
	return fields
}

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "To take a set of cookbooks and have them converged on the target version
// without standing over it."
//
// Unattended means it runs at a bounded rate with nobody having configured it
// first. A ceiling that is zero until somebody sets one is a bulk run that
// either does nothing or takes the lab down.
func TestJourney_ConvergesASetWithoutStandingOverIt(t *testing.T) {
	var unconfigured config.TestKitchenConfig
	if got := unconfigured.EffectiveMaxConcurrentVMs(); got <= 0 {
		t.Errorf("with nothing configured the converge concurrency ceiling is %d, so a "+
			"bulk run cannot be left to itself", got)
	}
}

// "Thousands of them, over days, picking up where it left off."
//
// The "thousands" half: work is taken one instance at a time from a queue, so a
// bulk run is not one long process whose death loses everything before it. The
// "over days" half is the test below.
func TestJourney_WorkIsTakenOneItemAtATime(t *testing.T) {
	if _, ok := any(&datastore.DB{}).(convergeJourneyQueue); !ok {
		t.Error("there is no per-instance queue, so a bulk run is one process and " +
			"stopping it loses everything it had done")
	}
}

// "Thousands of them, over days, picking up where it left off."
func TestJourney_PicksUpWhereItLeftOffAfterARestart(t *testing.T) {
	t.Skip("Needs the functional database (-tags functional): what a restart does to " +
		"in-flight work is a query, and the queue statuses alone cannot show whether " +
		"interrupted work returns to the queue or waits for a person to re-drive it.")
}

// "To choose the set the way I think about the estate — these repositories ..."
func TestJourney_ChooseTheSetByRepository(t *testing.T) {
	if !convergeJourneySelectionMentions("cookbook") {
		t.Errorf("a batch cannot be selected by which repositories it covers; the "+
			"selection is only %v", convergeJourneySelectionFields())
	}
}

// "To choose the set the way I think about the estate — ... these platforms ..."
func TestJourney_ChooseTheSetByPlatform(t *testing.T) {
	if !convergeJourneySelectionMentions("platform") {
		t.Errorf("a batch cannot be selected by platform; the selection is only %v",
			convergeJourneySelectionFields())
	}
}

// "To choose the set the way I think about the estate — ... the ones that block
// the most machines ..."
//
// Asserted against the saved selection rather than a sorted list, because the
// journey asks to CHOOSE this way: a report that happens to be ordered by node
// count does not let a batch be built from the top of it.
func TestJourney_ChooseTheSetByWhatBlocksTheMostMachines(t *testing.T) {
	if !convergeJourneySelectionMentions("node", "impact", "blocking", "blocked") {
		t.Errorf("a batch cannot be selected by how many machines a cookbook blocks, so "+
			"the work is chosen alphabetically after all; the selection is only %v",
			convergeJourneySelectionFields())
	}
}

// "To choose the set the way I think about the estate — ... the ones I own."
func TestJourney_ChooseTheSetByWhoOwnsIt(t *testing.T) {
	if !convergeJourneySelectionMentions("owner", "ownership") {
		t.Errorf("a batch cannot be selected by who owns the cookbooks; the selection "+
			"is only %v", convergeJourneySelectionFields())
	}
}

// "To see what happened: which converged, which did not ..."
//
// Per instance, not per cookbook. A cookbook that converged on one platform and
// failed on another has no single answer, and a rollup that gives it one hides
// the platform that is actually broken.
func TestJourney_SeeWhichConvergedAndWhichDidNot(t *testing.T) {
	passed := true
	fields := convergeJourneyResultKeys(t, datastore.GitKitchenResult{
		PlatformName: "rhel-9",
		SuiteName:    "default",
		Passed:       &passed,
	})
	for _, key := range []string{"platform_name", "suite_name", "passed"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("a converge result does not carry %q, so which converged and which "+
				"did not cannot be read per platform", key)
		}
	}
}

// "... and for the ones that did not, enough output to tell whether the cookbook
// broke or the lab did."
func TestJourney_EnoughOutputToTellWhetherTheCookbookOrTheLabBroke(t *testing.T) {
	fields := convergeJourneyResultKeys(t, datastore.GitKitchenResult{
		Output:       "$$$$$$ kitchen converge\n...",
		ErrorMessage: "probable DHCP/network timeout",
		TimedOut:     true,
	})
	if _, ok := fields["output"]; !ok {
		t.Error("a failed converge keeps no output, so nothing distinguishes a cookbook " +
			"that broke from a lab that did")
	}
	if _, ok := fields["error_message"]; !ok {
		t.Error("a failed converge records no reason beside its output")
	}
}

// "To know which cookbooks cannot be tested this way at all, because they have
// no test setup."
//
// Named as a number across the estate, so "what is my coverage" has an answer
// without reading every repository.
func TestJourney_UntestableCookbooksAreNamedAsAGap(t *testing.T) {
	fields := convergeJourneyResultKeys(t, datastore.KitchenAnalysisSummary{TotalWithoutKitchen: 3})
	if _, ok := fields["total_without_kitchen"]; !ok {
		t.Error("nothing counts the cookbooks that have no test setup, so the coverage " +
			"of converge testing is unknown")
	}
}

// "That is not a pass and it is not a failure; it is a gap."
//
// The baseline is asserted by the fixture: the cookbook's static analysis
// passes. What is being tested is that the ABSENCE of a test setup does not
// turn that into a failure — and that it is not silently promoted to a pass
// either, because a cookbook nothing converged is not one that converged.
func TestJourney_NoTestSetupIsNeitherAPassNorAFailure(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("example", "1.0.0", "org-a")
	ds.addCSResult("org-a", "example", "1.0.0", "19.0", true)
	ds.addGitRepoWithTK("example", "sha-abc", false, false) // no test setup
	ds.addGitTKStatus("example", "19.0", "failed")

	status, _, _ := checkCookbookCompatibility("example", "1.0.0", "19.0",
		ds.cookbookIDs, ds.buildFakeCache())

	if status == StatusIncompatible {
		t.Error("a cookbook with no test setup is marked failed for the absence, which " +
			"charges it for something nobody ever ran")
	}
	if status == StatusCompatible {
		t.Error("a cookbook with no test setup reads as fully compatible, so the gap in " +
			"coverage disappears into the good news")
	}
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("status = %s, want %s — the absence of a test setup should leave the "+
			"cookbook judged on the static analysis alone", status, StatusCompatibleCookstyleOnly)
	}
}

// "Before a bulk run, to know what I am about to launch against: which platforms
// the estate's test configurations actually name ..."
func TestJourney_KnowWhichPlatformsTheConfigurationsName(t *testing.T) {
	dir := t.TempDir()
	writeConvergeJourneyFile(t, filepath.Join(dir, ".kitchen.yml"), `---
driver:
  name: proxmox
platforms:
  - name: rhel-9
  - name: windows-2022
suites:
  - name: default
`)

	entry := AnalyseKitchenDir(dir)
	if entry.ErrorMessage != "" {
		t.Fatalf("the fixture proves nothing: reading it failed: %s", entry.ErrorMessage)
	}
	var named []string
	for _, p := range entry.Config.Platforms {
		named = append(named, p.Name)
	}
	if len(named) != 2 {
		t.Errorf("the platforms a repository's test configuration names are %v, want both "+
			"of them — a bulk run cannot be judged before it is launched", named)
	}
}

// "... and where a repository carries its own local configuration that would
// fight ours."
func TestJourney_KnowWhereARepositoryCarriesItsOwnLocalConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeConvergeJourneyFile(t, filepath.Join(dir, ".kitchen.yml"), `---
driver:
  name: proxmox
platforms:
  - name: rhel-9
suites:
  - name: default
`)
	writeConvergeJourneyFile(t, filepath.Join(dir, ".kitchen.local.yml"), `---
driver:
  name: vagrant
`)

	entry := AnalyseKitchenDir(dir)
	if !entry.HasLocalOverride {
		t.Fatal("a repository carrying its own local configuration is not reported as " +
			"doing so, so a bulk run launches against something nobody was shown")
	}
	// Which keys it touches is what says whether it would fight ours: a local
	// file that only sets a suite name is not the same problem as one that
	// replaces the driver.
	if len(entry.LocalOverrideKeys) == 0 {
		t.Error("the local configuration is reported but not what it changes, so there is " +
			"no way to tell whether it would fight ours")
	}
}

// ---------------------------------------------------------------------------
// The decisions behind it
// ---------------------------------------------------------------------------

// "A red has three causes, and one of them cannot be detected. ... Nothing can
// tell the third from the first, so overruling it is a person's verdict and
// always will be."
func TestJourney_AFaultyFixtureCannotBeToldFromABrokenCookbook(t *testing.T) {
	t.Skip("Not answerable from this product, and deliberately so: a cookbook whose own " +
		"tests are faulty fails exactly as one that cannot converge. The journey says not " +
		"to build detection for it — the failure register is the answer.")
}

// "So whether a converge failure can block at all is a switch an administrator
// controls. On by default ..."
func TestJourney_TheSwitchIsOnByDefault(t *testing.T) {
	var unset config.ReadinessConfig
	if !unset.TKBlocksReadinessValue() {
		t.Error("with nothing set a converge failure does not block, so an estate that " +
			"never touched the switch quietly ignores the best evidence there is")
	}
}

// "With it on, a converge failure outranks a clean scan."
func TestJourney_WithTheSwitchOnAConvergeFailureOutranksACleanScan(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("example", "1.0.0", "org-a")
	ds.addGitRepoWithTK("example", "sha-abc", true, false)
	ds.addGitCSResult("example", "19.0", true) // the scan is clean
	ds.addGitTKStatus("example", "19.0", "failed")

	cache := ds.buildFakeCache()
	cache.tkBlocksReadiness = true

	status, source, _ := checkCookbookCompatibility("example", "1.0.0", "19.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("status = %s, want %s — with the switch on a converge failure must outrank "+
			"a clean scan", status, StatusIncompatible)
	}
	if source != SourceGitTestKitchen {
		t.Errorf("source = %s, want %s — the reader cannot see which signal decided",
			source, SourceGitTestKitchen)
	}
}

// "With it off, a converge failure does not block, and is still reported."
//
// The middle behaviour, and the one that keeps a distrusted lab from silently
// deleting evidence: stop concluding, keep looking.
func TestJourney_WithTheSwitchOffAConvergeFailureIsStillReported(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("example", "1.0.0", "org-a")
	ds.addGitRepoWithTK("example", "sha-abc", true, false)
	ds.addGitCSResult("example", "19.0", true)
	ds.addGitTKStatus("example", "19.0", "failed")

	cache := ds.buildFakeCache()
	cache.tkBlocksReadiness = false

	status, _, verdicts := checkCookbookCompatibility("example", "1.0.0", "19.0", ds.cookbookIDs, cache)
	if status == StatusIncompatible {
		t.Errorf("status = %s — with the switch off a converge failure must not block", status)
	}
	var reported bool
	for _, v := range verdicts {
		if v.Source == SourceGitTestKitchen {
			reported = true
		}
	}
	if !reported {
		t.Error("with the switch off the converge result disappears rather than being " +
			"shown and not counted, so turning the switch off deletes the evidence")
	}
}

// "With it off, a converge pass stops counting too, so the switch cannot be used
// to keep the good news and drop the bad."
func TestJourney_WithTheSwitchOffAConvergePassStopsCountingToo(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("example", "1.0.0", "org-a")
	ds.addGitRepoWithTK("example", "sha-abc", true, false)
	ds.addGitCSResult("example", "19.0", true)
	ds.addGitTKStatus("example", "19.0", "passed")

	cache := ds.buildFakeCache()
	cache.tkBlocksReadiness = false

	status, _, _ := checkCookbookCompatibility("example", "1.0.0", "19.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("status = %s, want %s — with the switch off a converge PASS still counts, "+
			"so the switch keeps the good news and drops the bad",
			status, StatusCompatibleCookstyleOnly)
	}
}

// "It is deliberately not automatic, because the tool cannot tell a broken lab
// from a genuinely broken estate."
func TestJourney_TheSwitchIsNotAutomatic(t *testing.T) {
	t.Skip("Covered on the frontend, where the administrator throws it: " +
		"frontend/src/pages/AdminReadinessPage.test.tsx. Recorded here so the line is " +
		"not silently unaccounted for — nothing on this side can show the absence of " +
		"an automatic path, only that a value is read.")
}

// "Our lab's failures must never be charged to the cookbook."
func TestJourney_LabFailuresAreNotChargedToTheCookbook(t *testing.T) {
	t.Skip("The journey says nothing proves this: the switch makes the distinction " +
		"available but does not classify any individual failure. Deciding a given red " +
		"was the lab's fault is a human act, recorded elsewhere.")
}

// "A repository's own test configuration is respected, not replaced."
//
// Read the way the tool itself would: our settings merge into theirs key by key
// rather than replacing the file, so a setting they made and we say nothing
// about survives.
func TestJourney_ARepositorysOwnTestConfigurationIsRespectedNotReplaced(t *testing.T) {
	theirs := map[string]any{
		"driver": map[string]any{
			"name":     "proxmox",
			"template": "their-template",
		},
	}
	ours := map[string]any{
		"driver": map[string]any{
			"name": "vagrant",
		},
	}

	merged := MergeKitchenConfigs(theirs, ours)
	driver, ok := merged["driver"].(map[string]any)
	if !ok {
		t.Fatalf("the merged configuration has no driver block: %v", merged)
	}
	if driver["template"] != "their-template" {
		t.Errorf("a setting the team made and we say nothing about was lost (%v), so we "+
			"are testing something that is not what they ship", driver["template"])
	}
	if driver["name"] != "vagrant" {
		t.Errorf("driver name = %v, want vagrant — the fixture proves nothing if our own "+
			"settings are not applied at all", driver["name"])
	}
}

// "Whatever the team put there — including the steps they run before a converge
// — has to survive."
func TestJourney_TheStepsTheyRunBeforeAConvergeSurvive(t *testing.T) {
	t.Skip("Asserted where the overlay is generated, which this package cannot import " +
		"without a cycle (internal/gitkitchen imports internal/analysis): see " +
		"internal/gitkitchen/executor_test.go TestRunInstance_PreservesRepoLifecyclePhases.")
}

// "... locally invented platform attributes survive the reading rather than
// being normalised away."
//
// The estate names platforms in its own vocabulary. Reading only the attributes
// we recognise makes "what am I about to launch against" a guess dressed as an
// answer.
func TestJourney_LocallyInventedPlatformAttributesSurvive(t *testing.T) {
	raw, err := ParseKitchenYAML([]byte(`---
platforms:
  - name: rhel-9
    x-custom-pool: dmz
`))
	if err != nil {
		t.Fatalf("the fixture proves nothing: it does not parse: %v", err)
	}

	cfg := ExtractKitchenConfig(raw)
	if len(cfg.Platforms) != 1 {
		t.Fatalf("expected one platform, got %d", len(cfg.Platforms))
	}
	if got := cfg.Platforms[0].Extensions["x-custom-pool"]; got != "dmz" {
		t.Errorf("a locally invented platform attribute was normalised away (got %v), so "+
			"the configuration we report is not the one the repository holds", got)
	}
}

// ---------------------------------------------------------------------------
// What nothing proves
// ---------------------------------------------------------------------------

// "Nothing proves the thing the journey is actually for. No test establishes
// that a real converge on a real machine finds failures the static analysis
// missed."
func TestJourney_ConvergeFindsWhatStaticAnalysisMissed(t *testing.T) {
	t.Skip("The journey says nothing proves this: it is the entire premise and is " +
		"confirmed only by having done it. If converge testing never catches anything " +
		"static analysis did not, the answer is to switch it off and save the lab.")
}

// "The load-bearing assumption: that a converge that never reached the converge
// step is distinguishable, in what we store, from one that converged and failed."
//
// Half of this is answerable here and half is not. What IS answerable: whether
// the stored vocabulary can carry the distinction at all — a single "failed"
// with no way to say "never got that far" would settle it in the wrong
// direction on its own.
func TestJourney_ANeverStartedConvergeIsDistinguishableFromAFailedOne(t *testing.T) {
	distinct := map[string]bool{}
	for _, status := range []string{
		datastore.BatchInstanceFailed,
		datastore.BatchInstanceErrored,
		datastore.BatchInstanceTimedOut,
		datastore.BatchInstanceNetworkTimeout,
	} {
		if status == "" {
			t.Fatal("an unfinished converge has no recorded outcome at all")
		}
		distinct[status] = true
	}
	if len(distinct) < 4 {
		t.Errorf("an unfinished converge and a failed one share %d outcomes between four "+
			"names, so the balance of lab-versus-cookbook failures cannot be measured",
			len(distinct))
	}

	// And at the result grain: no verdict at all must not read as a failure.
	// A *bool is what keeps "nothing concluded" out of "concluded no".
	field, ok := reflect.TypeOf(datastore.GitKitchenResult{}).FieldByName("Passed")
	if !ok {
		t.Fatal("a converge result does not record whether it passed")
	}
	if field.Type.Kind() != reflect.Ptr {
		t.Error("a converge result that reached no verdict is stored as a plain false, so " +
			"a run that never reached the converge step is indistinguishable from one " +
			"that converged and failed")
	}
}

// "Verify that before designing anything that reports on failure causes — if the
// two look the same in the data, that balance cannot be measured again."
func TestJourney_TheFailureCauseBalanceCanBeMeasured(t *testing.T) {
	t.Skip("Cannot be answered from the stored shape: whether a real failed run is " +
		"classified into the right one of those outcomes is a property of the lab, and " +
		"measuring the balance needs a body of real runs to count.")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// convergeJourneyQueue is the seam a bulk run is driven through: one instance
// claimed at a time, and in-flight work accounted for after a restart.
type convergeJourneyQueue interface {
	ClaimNextKitchenRun(ctx context.Context) (*datastore.KitchenQueueItem, error)
	MarkInterruptedKitchenRuns(ctx context.Context) (int64, error)
}

func writeConvergeJourneyFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
