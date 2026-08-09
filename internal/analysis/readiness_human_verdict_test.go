// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// A person's verdict is a third source inside the existing rollup, not a
// parallel list. Behaviour: journeys/human-verdict.md.
//
// The automated signals are wrong in both directions — CookStyle marks
// cookbooks blocked that demonstrably run fine, and Test Kitchen reports the
// test environment falling over as a cookbook that does not work. Where a
// person disagrees with either, the person wins.
// ---------------------------------------------------------------------------

// plentyOfDisk is a filesystem attribute with enough headroom that disk never
// decides these tests — the cookbook verdict is what is under test.
func plentyOfDisk() json.RawMessage {
	return linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
	})
}

// findVerdict returns the verdict recorded by one source, and whether there
// was one.
func findVerdict(verdicts []CookbookSourceVerdict, source string) (CookbookSourceVerdict, bool) {
	for _, v := range verdicts {
		if v.Source == source {
			return v, true
		}
	}
	return CookbookSourceVerdict{}, false
}

// A CookStyle pass and a clean Test Kitchen run say the cookbook is fine.
// A person who watched it break on a real converge says otherwise, and nothing
// else in the product can record that.
func TestHumanVerdict_BrokenOverridesACleanScan(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-nginx", "abc123", true, false)
	f.addGitCSResult("acme-nginx", "19.0.0", true)
	f.addGitTKStatus("acme-nginx", "19.0.0", "passed")
	f.addCookbookID("acme-nginx", "1.0.0", "org-a")
	f.addCSResult("org-a", "acme-nginx", "1.0.0", "19.0.0", true)

	cache := f.buildFakeCache()
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-nginx": {
			SubjectName:  "acme-nginx",
			CookbookName: "nginx",
			Verdict:      datastore.VerdictBroken,
			Reason:       "the service resource fails on a real converge; no scan sees it",
			RaisedBy:     "alice",
		},
	}

	status, source, verdicts := checkCookbookCompatibility(
		"acme-nginx", "1.0.0", "19.0.0", f.cookbookIDs, cache)

	if status != StatusIncompatible {
		t.Errorf("status = %q, want %q — a person who saw it break outranks a clean scan", status, StatusIncompatible)
	}
	if source != SourceHumanVerdict {
		t.Errorf("source = %q, want %q", source, SourceHumanVerdict)
	}

	// The losing verdicts are retained rather than overwritten: somebody
	// looking at this cookbook can see that both scans passed and a person
	// overruled them.
	if _, ok := findVerdict(verdicts, SourceGitCookstyle); !ok {
		t.Error("the CookStyle verdict was dropped; the disagreement must stay visible")
	}
	if _, ok := findVerdict(verdicts, SourceGitTestKitchen); !ok {
		t.Error("the Test Kitchen verdict was dropped; the disagreement must stay visible")
	}

	hv, ok := findVerdict(verdicts, SourceHumanVerdict)
	if !ok {
		t.Fatal("the human verdict is not in the verdicts array")
	}
	if hv.Status != StatusIncompatible {
		t.Errorf("human verdict status = %q, want %q", hv.Status, StatusIncompatible)
	}
	if hv.Note == "" {
		t.Error("the human verdict carries no reason; the reason is what makes it survive")
	}
	if hv.RecordedBy != "alice" {
		t.Errorf("recorded by %q, want alice", hv.RecordedBy)
	}
}

// The expensive direction. A cookbook is marked incompatible and production
// has been running it for months; dispatching somebody to fix it costs their
// time and the tool's credibility.
func TestHumanVerdict_NotBrokenOverridesAFailingScan(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-apache", "abc123", true, false)
	f.addGitCSResult("acme-apache", "19.0.0", false)
	f.addGitTKStatus("acme-apache", "19.0.0", "failed")

	cache := f.buildFakeCache()
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-apache": {
			SubjectName:  "acme-apache",
			CookbookName: "apache",
			Verdict:      datastore.VerdictNotBroken,
			Reason:       "kitchen never got as far as converging; this has run in production for months",
			RaisedBy:     "bob",
		},
	}

	status, source, verdicts := checkCookbookCompatibility(
		"acme-apache", "2.0.0", "19.0.0", f.cookbookIDs, cache)

	if status != StatusCompatible {
		t.Errorf("status = %q, want %q — the person who watched it run outranks both scans", status, StatusCompatible)
	}
	if source != SourceHumanVerdict {
		t.Errorf("source = %q, want %q", source, SourceHumanVerdict)
	}

	tk, ok := findVerdict(verdicts, SourceGitTestKitchen)
	if !ok {
		t.Fatal("the Test Kitchen verdict was dropped")
	}
	if tk.Status != StatusIncompatible {
		t.Errorf("the losing Test Kitchen verdict was rewritten to %q; it must be retained as it was", tk.Status)
	}
	cs, ok := findVerdict(verdicts, SourceGitCookstyle)
	if !ok {
		t.Fatal("the CookStyle verdict was dropped")
	}
	if cs.Status != StatusIncompatible {
		t.Errorf("the losing CookStyle verdict was rewritten to %q", cs.Status)
	}
}

// A cookbook nothing has ever scanned is untested, not broken. A person
// saying it is broken is the only signal there is.
func TestHumanVerdict_BrokenOnAnUntestedCookbook(t *testing.T) {
	f := newFakeReadinessDS()
	cache := f.buildFakeCache()
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-legacy": {
			SubjectName:  "acme-legacy",
			CookbookName: "legacy",
			Verdict:      datastore.VerdictBroken,
			Reason:       "known broken; never scanned because the clone fails",
			RaisedBy:     "alice",
		},
	}

	status, source, verdicts := checkCookbookCompatibility(
		"acme-legacy", "1.0.0", "19.0.0", f.cookbookIDs, cache)

	if status != StatusIncompatible {
		t.Errorf("status = %q, want %q", status, StatusIncompatible)
	}
	if source != SourceHumanVerdict {
		t.Errorf("source = %q, want %q", source, SourceHumanVerdict)
	}
	if len(verdicts) != 1 {
		t.Errorf("got %d verdicts, want just the human one", len(verdicts))
	}
}

// A repo nobody has an opinion about must behave exactly as it did before the
// register existed.
func TestHumanVerdict_AbsentChangesNothing(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepoWithTK("acme-mysql", "abc123", true, false)
	f.addGitCSResult("acme-mysql", "19.0.0", false)

	cache := f.buildFakeCache()
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"some-other-repo": {
			SubjectName: "some-other-repo",
			Verdict:     datastore.VerdictNotBroken,
			Reason:      "unrelated",
			RaisedBy:    "alice",
		},
	}

	status, _, verdicts := checkCookbookCompatibility(
		"acme-mysql", "1.0.0", "19.0.0", f.cookbookIDs, cache)

	if status != StatusIncompatible {
		t.Errorf("status = %q, want %q", status, StatusIncompatible)
	}
	if _, ok := findVerdict(verdicts, SourceHumanVerdict); ok {
		t.Error("a verdict about another repo was applied to this one")
	}
}

// A nil map is the normal state of a deployment where nobody has recorded
// anything yet, and must not panic or change any verdict.
func TestHumanVerdict_NoRegisterAtAll(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepo("acme-redis", "abc123")
	f.addGitCSResult("acme-redis", "19.0.0", true)

	cache := f.buildFakeCache()
	cache.humanVerdicts = nil

	status, _, _ := checkCookbookCompatibility("acme-redis", "1.0.0", "19.0.0", f.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("status = %q, want %q", status, StatusCompatibleCookstyleOnly)
	}
}

// The whole point of joining the existing rollup rather than sitting beside
// it: node readiness reflects the human verdict without any consumer being
// changed. A person recording a failure blocks the nodes running it.
func TestHumanVerdict_BlocksTheNodesRunningIt(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepo("acme-ntp", "abc123")
	f.addGitCSResult("acme-ntp", "19.0.0", true)
	f.addCookbookID("acme-ntp", "1.0.0", "org-a")
	f.addCSResult("org-a", "acme-ntp", "1.0.0", "19.0.0", true)

	cache := f.buildFakeCache()
	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-ntp": {
			SubjectName:  "acme-ntp",
			CookbookName: "ntp",
			Verdict:      datastore.VerdictBroken,
			Reason:       "drifts the clock on RHEL 9 and the converge never completes",
			RaisedBy:     "alice",
		},
	}

	e := NewReadinessEvaluator(f, nil, 1, 2048)
	snapshot := makeSnapshot("org-a", "node-1", false,
		[]byte(`{"acme-ntp": {"version": "1.0.0"}}`), plentyOfDisk())

	result := e.evaluateOne(snapshot, "19.0.0", f.cookbookIDs, cache)

	if result.IsReady {
		t.Error("the node is ready despite a recorded failure on a cookbook it runs")
	}
	if len(result.BlockingCookbooks) != 1 {
		t.Fatalf("got %d blocking cookbooks, want 1", len(result.BlockingCookbooks))
	}
	bc := result.BlockingCookbooks[0]
	if bc.Source != SourceHumanVerdict {
		t.Errorf("blocking source = %q, want %q", bc.Source, SourceHumanVerdict)
	}
	hv, ok := findVerdict(bc.Verdicts, SourceHumanVerdict)
	if !ok {
		t.Fatal("the human verdict did not reach the persisted blocking entry")
	}
	if hv.Note == "" {
		t.Error("the reason did not reach the persisted blocking entry — the standup reads it there")
	}
}

// Overruling a false blocker unblocks the node. This is the case that costs
// an engineer a wasted day and the tool its credibility.
func TestHumanVerdict_NotBrokenUnblocksTheNode(t *testing.T) {
	f := newFakeReadinessDS()
	f.addGitRepo("acme-sudo", "abc123")
	f.addGitCSResult("acme-sudo", "19.0.0", false)
	f.addCookbookID("acme-sudo", "1.0.0", "org-a")
	f.addCSResult("org-a", "acme-sudo", "1.0.0", "19.0.0", false)

	cache := f.buildFakeCache()
	snapshot := makeSnapshot("org-a", "node-1", false,
		[]byte(`{"acme-sudo": {"version": "1.0.0"}}`), plentyOfDisk())
	e := NewReadinessEvaluator(f, nil, 1, 2048)

	blocked := e.evaluateOne(snapshot, "19.0.0", f.cookbookIDs, cache)
	if blocked.IsReady {
		t.Fatal("precondition: the node should be blocked before anybody overrules the scan")
	}

	cache.humanVerdicts = map[string]datastore.StandingVerdict{
		"acme-sudo": {
			SubjectName:  "acme-sudo",
			CookbookName: "sudo",
			Verdict:      datastore.VerdictNotBroken,
			Reason:       "the offence is a false positive; this runs on 4000 nodes today",
			RaisedBy:     "bob",
		},
	}

	after := e.evaluateOne(snapshot, "19.0.0", f.cookbookIDs, cache)
	if !after.IsReady {
		t.Errorf("the node is still blocked after a person overruled the scan (status %q, %d blocking)",
			after.Status, len(after.BlockingCookbooks))
	}
	if len(after.BlockingCookbooks) != 0 {
		t.Errorf("got %d blocking cookbooks, want none", len(after.BlockingCookbooks))
	}
}
