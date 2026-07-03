// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// verdictUpdate records one UpdateVerdict call for assertions.
type verdictUpdate struct {
	key    string
	passed bool
	status string
}

// fakeBackfillStore is an in-memory CookstyleStatusBackfillStore. It returns the
// seeded refs and records verdict updates without touching a database.
type fakeBackfillStore struct {
	serverRefs []datastore.CookstyleResultRef
	gitRefs    []datastore.CookstyleResultRef
	overrides  map[string]string // cop_name -> classification (all targets)

	serverUpdates []verdictUpdate
	gitUpdates    []verdictUpdate
}

func (s *fakeBackfillStore) ListAllServerCookbookCookstyleResultRefs(context.Context) ([]datastore.CookstyleResultRef, error) {
	return s.serverRefs, nil
}

func (s *fakeBackfillStore) ListAllGitRepoCookstyleResultRefs(context.Context) ([]datastore.CookstyleResultRef, error) {
	return s.gitRefs, nil
}

func (s *fakeBackfillStore) ListCopClassifications(_ context.Context) ([]datastore.CopClassification, error) {
	out := make([]datastore.CopClassification, 0, len(s.overrides))
	for cop, class := range s.overrides {
		out = append(out, datastore.CopClassification{CopName: cop, Classification: class})
	}
	return out, nil
}

func (s *fakeBackfillStore) UpdateServerCookbookCookstyleVerdict(_ context.Context, org, name, version, target string, passed bool, status string) error {
	s.serverUpdates = append(s.serverUpdates, verdictUpdate{key: org + "/" + name + "@" + version + ":" + target, passed: passed, status: status})
	return nil
}

func (s *fakeBackfillStore) UpdateGitRepoCookstyleVerdict(_ context.Context, name, url, target string, passed bool, status string) error {
	s.gitUpdates = append(s.gitUpdates, verdictUpdate{key: name + "|" + url + ":" + target, passed: passed, status: status})
	return nil
}

// reviewOffenceJSON is the stored flat enriched-offence array for a single
// review-classified cop (Chef/Correctness/NodeNormal is Review at all targets).
const reviewOffenceJSON = `[{"cop_name":"Chef/Correctness/NodeNormal","severity":"warning"}]`

// blockerOffenceJSON is a single blocker-classified cop at target >= 18.
const blockerOffenceJSON = `[{"cop_name":"Lint/DeprecatedClassMethods","severity":"warning"}]`

// TestBackfillCookstyleStatus_RederivesNeedsReview is the core case: a row the
// coarse SQL backfill left at "ready" (because passed=true) but whose only
// offence is review-classified must be corrected to needs_review — something SQL
// can never recover.
func TestBackfillCookstyleStatus_RederivesNeedsReview(t *testing.T) {
	store := &fakeBackfillStore{
		serverRefs: []datastore.CookstyleResultRef{{
			OrganisationName: "org-a", CookbookName: "cb", CookbookVersion: "1.0.0",
			TargetChefVersion: "18", Passed: true, CookstyleStatus: "ready",
			Offences: []byte(reviewOffenceJSON),
		}},
	}

	res, err := BackfillCookstyleStatus(context.Background(), store, DefaultFailureRules())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.ServerResultsChanged != 1 {
		t.Fatalf("ServerResultsChanged = %d, want 1", res.ServerResultsChanged)
	}
	if len(store.serverUpdates) != 1 {
		t.Fatalf("got %d server updates, want 1", len(store.serverUpdates))
	}
	u := store.serverUpdates[0]
	if u.status != StatusNeedsReview {
		t.Errorf("status = %q, want %q", u.status, StatusNeedsReview)
	}
	if !u.passed {
		t.Error("passed should remain true for a needs_review row")
	}
}

// TestBackfillCookstyleStatus_Idempotent: a row already carrying the precise
// status is left untouched, so a second run (or a row migration got right) is a
// no-op.
func TestBackfillCookstyleStatus_Idempotent(t *testing.T) {
	store := &fakeBackfillStore{
		serverRefs: []datastore.CookstyleResultRef{{
			OrganisationName: "org-a", CookbookName: "cb", CookbookVersion: "1.0.0",
			TargetChefVersion: "18", Passed: true, CookstyleStatus: "needs_review",
			Offences: []byte(reviewOffenceJSON),
		}},
		gitRefs: []datastore.CookstyleResultRef{{
			GitRepoName: "repo", GitRepoURL: "https://git.example.com/repo",
			TargetChefVersion: "19", Passed: false, CookstyleStatus: "blocked",
			Offences: []byte(blockerOffenceJSON),
		}},
	}

	res, err := BackfillCookstyleStatus(context.Background(), store, DefaultFailureRules())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Changed() != 0 {
		t.Errorf("Changed() = %d, want 0 (precise rows must not be rewritten)", res.Changed())
	}
	if len(store.serverUpdates) != 0 || len(store.gitUpdates) != 0 {
		t.Errorf("expected no updates, got %d server / %d git", len(store.serverUpdates), len(store.gitUpdates))
	}
	if res.ServerResultsScanned != 1 || res.GitResultsScanned != 1 {
		t.Errorf("scanned = %d server / %d git, want 1 / 1", res.ServerResultsScanned, res.GitResultsScanned)
	}
}

// TestBackfillCookstyleStatus_BlockerAndGit: a coarse "ready" git row whose
// offence is a blocker must be corrected to blocked (passed=false).
func TestBackfillCookstyleStatus_BlockerAndGit(t *testing.T) {
	store := &fakeBackfillStore{
		gitRefs: []datastore.CookstyleResultRef{{
			GitRepoName: "repo", GitRepoURL: "https://git.example.com/repo",
			TargetChefVersion: "19", Passed: true, CookstyleStatus: "ready",
			Offences: []byte(blockerOffenceJSON),
		}},
	}

	res, err := BackfillCookstyleStatus(context.Background(), store, DefaultFailureRules())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.GitResultsChanged != 1 {
		t.Fatalf("GitResultsChanged = %d, want 1", res.GitResultsChanged)
	}
	u := store.gitUpdates[0]
	if u.status != StatusBlocked || u.passed {
		t.Errorf("got status=%q passed=%v, want blocked / false", u.status, u.passed)
	}
}

// TestBackfillCookstyleStatus_SkipsErrorRows: inconclusive scans (error_message
// set) carry no verdict and must never be rewritten.
func TestBackfillCookstyleStatus_SkipsErrorRows(t *testing.T) {
	store := &fakeBackfillStore{
		serverRefs: []datastore.CookstyleResultRef{{
			OrganisationName: "org-a", CookbookName: "cb", CookbookVersion: "1.0.0",
			TargetChefVersion: "18", CookstyleStatus: "blocked",
			ErrorMessage: "CookStyle error (exit 2): boom",
			Offences:     []byte(reviewOffenceJSON),
		}},
	}

	res, err := BackfillCookstyleStatus(context.Background(), store, DefaultFailureRules())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.ServerResultsChanged != 0 || len(store.serverUpdates) != 0 {
		t.Errorf("error rows must be skipped: changed=%d updates=%d", res.ServerResultsChanged, len(store.serverUpdates))
	}
}

// TestBackfillCookstyleStatus_OperatorOverrideApplies: an operator override is
// honoured over the curated default, so a review cop overridden to noise rolls
// up to ready.
func TestBackfillCookstyleStatus_OperatorOverrideApplies(t *testing.T) {
	store := &fakeBackfillStore{
		overrides: map[string]string{"Chef/Correctness/NodeNormal": ClassificationNoise},
		serverRefs: []datastore.CookstyleResultRef{{
			OrganisationName: "org-a", CookbookName: "cb", CookbookVersion: "1.0.0",
			TargetChefVersion: "18", Passed: true, CookstyleStatus: "needs_review",
			Offences: []byte(reviewOffenceJSON),
		}},
	}

	res, err := BackfillCookstyleStatus(context.Background(), store, DefaultFailureRules())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.ServerResultsChanged != 1 {
		t.Fatalf("ServerResultsChanged = %d, want 1", res.ServerResultsChanged)
	}
	if got := store.serverUpdates[0].status; got != StatusReady {
		t.Errorf("status = %q, want %q (override to noise)", got, StatusReady)
	}
}

func TestParseStoredOffenses(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []CookstyleOffense
	}{
		{name: "empty", data: "", want: nil},
		{name: "empty array", data: "[]", want: []CookstyleOffense{}},
		{
			name: "flat enriched",
			data: `[{"cop_name":"Chef/Correctness/NodeNormal","severity":"warning","message":"x"}]`,
			want: []CookstyleOffense{{CopName: "Chef/Correctness/NodeNormal", Severity: "warning", Message: "x"}},
		},
		{
			name: "file grouped",
			data: `[{"path":"recipes/default.rb","offenses":[{"cop_name":"Lint/Syntax","severity":"fatal"}]}]`,
			want: []CookstyleOffense{{CopName: "Lint/Syntax", Severity: "fatal"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStoredOffenses([]byte(tt.data))
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%+v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].CopName != tt.want[i].CopName || got[i].Severity != tt.want[i].Severity {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
