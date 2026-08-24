// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// The repository is not the cookbook — see journeys/scan-trust.md.
//
// A repository holds the cookbook AND the helper tasks, the pipeline and the
// test suites. Chef never executes the latter during a converge, so a finding
// there cannot decide whether the cookbook survives the upgrade. But it is
// still real work for somebody, so it must remain readable on the cookbook
// rather than being deleted — which is the failure mode this test exists to
// catch. A test asserting only the verdict would pass if the excluded findings
// were simply thrown away.

// storedOffences builds the offences JSONB exactly as the scanner persists it
// (a remediation.EnrichedOffense array, path in location.file), so this test
// exercises the real read path rather than a convenient in-memory shape.
func storedOffences(t *testing.T, offences ...remediation.EnrichedOffense) []byte {
	t.Helper()
	data, err := json.Marshal(offences)
	if err != nil {
		t.Fatalf("marshalling stored offences: %v", err)
	}
	return data
}

// fileExistsOffence is File.exists?, a curated Blocker at 19.0, carried by a cop
// whose severity is a mere warning. The same offence repeats across nearly every
// repository in a fleet because it sits in a copied Rakefile, not in cookbook
// code.
func fileExistsOffence(path string) remediation.EnrichedOffense {
	return remediation.EnrichedOffense{
		CopName:  "Lint/DeprecatedClassMethods",
		Severity: "warning",
		Message:  "File.exists? is deprecated in favor of File.exist?.",
		Location: remediation.OffenseLocation{File: path, StartLine: 12},
	}
}

func scanScopeResolver() *CopClassificationResolver {
	return &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "19.0",
	}
}

// TestRepositoryIsNotTheCookbook pins both halves of the property at once.
//
// The fixture is one repository carrying the SAME breaking finding twice: once
// in cookbook code that runs on a converging node, once in a helper task that
// never does. The two copies differ only by path, so the path is the only thing
// that can separate them.
func TestRepositoryIsNotTheCookbook(t *testing.T) {
	resolver := scanScopeResolver()

	// Sanity: the cop really is a Blocker at this target, so a status of Ready
	// below can only come from scope and never from a classification accident.
	if got := resolver.Classify("Lint/DeprecatedClassMethods"); got != ClassificationBlocker {
		t.Fatalf("Lint/DeprecatedClassMethods should classify as blocker at 19.0, got %q", got)
	}

	inCookbook := fileExistsOffence("libraries/helpers.rb")
	inHelperTask := fileExistsOffence("Rakefile")

	// --- Half one: the verdict follows only the cookbook copy. ---

	// Both copies present: the cookbook copy blocks. Nothing subtle here, but it
	// is what makes the next case a fair comparison — the two runs differ by one
	// offence whose only distinguishing feature is its path.
	both := ParseStoredOffenses(storedOffences(t, inCookbook, inHelperTask))
	if got := DeriveCookstyleStatus(both, resolver); got != StatusBlocked {
		t.Errorf("a breaking finding in cookbook code must block the cookbook, got %q", got)
	}

	// Only the helper-task copy: the converge never executes a Rakefile, so the
	// cookbook is not blocked by it. This is the ~95% case.
	helperOnly := ParseStoredOffenses(storedOffences(t, inHelperTask))
	if got := DeriveCookstyleStatus(helperOnly, resolver); got == StatusBlocked {
		t.Errorf("a breaking finding in a file the converge never executes must not block the cookbook, got %q", got)
	}

	// --- Half two: the excluded finding is still readable afterwards. ---
	// Deleting it would satisfy half one and lose real work. So it must survive
	// the read path with its path intact, be identifiable as out of scope, and
	// carry the recorded reason it was excluded.

	if len(helperOnly) != 1 {
		t.Fatalf("the helper task's finding must survive the read path: want 1 offence, got %d", len(helperOnly))
	}
	survivor := helperOnly[0]
	if survivor.CopName != "Lint/DeprecatedClassMethods" {
		t.Errorf("surviving offence lost its cop name: got %q", survivor.CopName)
	}
	if got := survivor.Path(); got != "Rakefile" {
		t.Errorf("surviving offence lost its path: want %q, got %q", "Rakefile", got)
	}

	scope := DefaultScanScope()

	exclusion, excluded := scope.Excluded(survivor.Path())
	if !excluded {
		t.Fatalf("%q must be recognised as a file the converge never executes", survivor.Path())
	}
	if exclusion.Reason == "" {
		t.Errorf("exclusion of %q must carry a recorded reason, same discipline as calling a finding harmless", survivor.Path())
	}

	// And the cookbook copy must NOT be swept up by the same mechanism, or the
	// exclusion list has become the means of making the blocked list look good.
	if _, excluded := scope.Excluded(inCookbook.Location.File); excluded {
		t.Errorf("%q is cookbook code and must not be excluded", inCookbook.Location.File)
	}
}

// TestRepositoryIsNotTheCookbook_SurvivesTheFingerprint pins the same property
// on the recompute path, which re-derives a verdict from the stored fingerprint
// rather than from the offences. The fingerprint drops source locations, so
// without the occurrence split a reclassification would quietly re-block every
// cookbook whose only blocker sits in a helper task — and the trend line would
// disagree with the cookbook page.
func TestRepositoryIsNotTheCookbook_SurvivesTheFingerprint(t *testing.T) {
	resolver := scanScopeResolver()

	helperOnly := []CookstyleOffense{
		{
			CopName:  "Lint/DeprecatedClassMethods",
			Severity: "warning",
			Message:  "File.exists? is deprecated in favor of File.exist?.",
			File:     "Rakefile",
		},
	}

	entries, _ := BuildOffenceFingerprint(helperOnly)
	if len(entries) != 1 {
		t.Fatalf("want 1 fingerprint entry, got %d", len(entries))
	}

	// The occurrence is retained and counted, not dropped — the estate needs to
	// know how widespread it is.
	if entries[0].Count != 1 {
		t.Errorf("the occurrence must still be counted: want count 1, got %d", entries[0].Count)
	}
	if entries[0].ExcludedCount != 1 {
		t.Errorf("the occurrence must be recorded as outside cookbook code: want 1, got %d", entries[0].ExcludedCount)
	}

	if got := DeriveStatusFromFingerprint(entries, resolver); got == StatusBlocked {
		t.Errorf("a recomputed verdict must not block on a finding outside cookbook code, got %q", got)
	}
}

// TestFingerprintsWrittenBeforeScopeReDeriveUnchanged guards the migration edge:
// every fingerprint already in the database predates scope and carries no
// excluded count. Those rows must keep re-deriving exactly as they did, rather
// than reading as wholly out of scope and turning the whole estate green.
func TestFingerprintsWrittenBeforeScopeReDeriveUnchanged(t *testing.T) {
	legacy := []datastore.FingerprintCopEntry{
		{CopName: "Lint/DeprecatedClassMethods", Count: 3, Severity: "warning"},
	}
	if got := DeriveStatusFromFingerprint(legacy, scanScopeResolver()); got != StatusBlocked {
		t.Errorf("a pre-scope fingerprint must re-derive as before: want %q, got %q", StatusBlocked, got)
	}
}

// TestScanScopeExclusionsAllCarryReasons pins the discipline rather than the
// list: an exclusion nobody can argue with is an exclusion nobody can check.
func TestScanScopeExclusionsAllCarryReasons(t *testing.T) {
	exclusions := DefaultScanScope().Exclusions()
	if len(exclusions) == 0 {
		t.Fatal("the curated exclusion list must not be empty")
	}
	for _, ex := range exclusions {
		if ex.Pattern == "" {
			t.Error("an exclusion with no pattern excludes nothing")
		}
		if ex.Reason == "" {
			t.Errorf("exclusion %q carries no reason", ex.Pattern)
		}
	}
}
