// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock re-score store
// ---------------------------------------------------------------------------

type mockRescoreStore struct {
	exclusions            []datastore.ScanScopeExclusion
	serverResults         []datastore.CookstyleRescoreRow
	gitResults            []datastore.CookstyleRescoreRow
	serverUpdates         []datastore.CookstylePassedUpdate
	gitUpdates            []datastore.CookstylePassedUpdate
	recomputedGit         []gitRepoKey
	recomputedAllTargets  []string                                 // targets passed to RecomputeAllGitRepoCookstyleStatus
	recomputedRoleTargets []string                                 // targets passed to RecomputeAllRoleCompatStatus
	classifications       map[string][]datastore.CopClassification // target version -> overrides
}

type gitRepoKey struct {
	Name, URL, TargetVersion string
}

func (m *mockRescoreStore) ListScanScopeExclusions(ctx context.Context) ([]datastore.ScanScopeExclusion, error) {
	return m.exclusions, nil
}

func (m *mockRescoreStore) ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error) {
	var out []datastore.CopClassification
	for _, cs := range m.classifications {
		out = append(out, cs...)
	}
	return out, nil
}

func (m *mockRescoreStore) ListServerCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error) {
	return m.serverResults, nil
}

func (m *mockRescoreStore) ListGitRepoCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error) {
	return m.gitResults, nil
}

func (m *mockRescoreStore) BatchUpdateServerCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error {
	m.serverUpdates = append(m.serverUpdates, updates...)
	return nil
}

func (m *mockRescoreStore) BatchUpdateGitRepoCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error {
	m.gitUpdates = append(m.gitUpdates, updates...)
	return nil
}

func (m *mockRescoreStore) RecomputeGitRepoCompatibilityStatus(ctx context.Context, name, url, targetVersion string) error {
	m.recomputedGit = append(m.recomputedGit, gitRepoKey{name, url, targetVersion})
	return nil
}

func (m *mockRescoreStore) RecomputeAllGitRepoCookstyleStatus(ctx context.Context, targetChefVersion string) error {
	m.recomputedAllTargets = append(m.recomputedAllTargets, targetChefVersion)
	return nil
}

func (m *mockRescoreStore) RecomputeAllRoleCompatStatus(ctx context.Context, targetChefVersion string) error {
	m.recomputedRoleTargets = append(m.recomputedRoleTargets, targetChefVersion)
	return nil
}

var _ CookstyleRescoreStore = (*mockRescoreStore)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRescoreCookstyleResults_NoResults(t *testing.T) {
	store := &mockRescoreStore{}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
	if result.Changed != 0 {
		t.Errorf("changed = %d, want 0", result.Changed)
	}
}

func TestRescoreCookstyleResults_ConsistentStatusNoChange(t *testing.T) {
	// A result whose stored status already matches its classification-derived
	// status is not rewritten. NodeSet is a verified-removal Blocker (RemovedIn
	// 14.0 ≤ the ID's target 18), so a stored "blocked" is already correct.
	offenses := []analysis.CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "org1|cb1|1.0.0|18",
				Offences:        offJSON,
				ErrorMessage:    "",
				Passed:          false,     // correct: Blocker → fail
				CookstyleStatus: "blocked", // already reflects classification
			},
		},
		gitResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "myrepo|https://git.example.com/repo|18",
				Offences:        offJSON,
				ErrorMessage:    "",
				Passed:          false, // correct
				CookstyleStatus: "blocked",
			},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.Changed != 0 {
		t.Errorf("changed = %d, want 0", result.Changed)
	}
	if len(store.serverUpdates) != 0 {
		t.Errorf("serverUpdates = %d, want 0", len(store.serverUpdates))
	}
	if len(store.gitUpdates) != 0 {
		t.Errorf("gitUpdates = %d, want 0", len(store.gitUpdates))
	}
	// Regression (git-repo cookstyle drift): even though NO result changed, the
	// bulk git-repo re-materialisation must still run. A repo whose result status
	// is unchanged but whose materialised git_repos.cookstyle_status is stale
	// (e.g. blanked by a target-version reset) would otherwise never be healed,
	// which is what made the Git Repos list disagree with the dashboard summary.
	if len(store.recomputedAllTargets) != 1 || store.recomputedAllTargets[0] != "18" {
		t.Errorf("recomputedAllTargets = %v, want [18] even with no result changes", store.recomputedAllTargets)
	}
	// Roles derive from the same results, so their compat must be re-materialised
	// for the same target too — else the roles list would drift from the results.
	if len(store.recomputedRoleTargets) != 1 || store.recomputedRoleTargets[0] != "18" {
		t.Errorf("recomputedRoleTargets = %v, want [18]", store.recomputedRoleTargets)
	}
}

func TestRescoreCookstyleResults_ClassificationFlipsVerdict(t *testing.T) {
	// A result stored ready/passed but containing a Blocker cop flips to blocked
	// when re-derived from classification. NodeSet is a verified-removal Blocker
	// (RemovedIn 14.0 ≤ target 18). Severity and failure rules play no part.
	offenses := []analysis.CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "org1|cb1|1.0.0|18",
				Offences:        offJSON,
				Passed:          true, // stale: stored before classification said Blocker
				CookstyleStatus: "ready",
			},
		},
		gitResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "myrepo|https://git.example.com/repo|18",
				Offences:        offJSON,
				Passed:          true,
				CookstyleStatus: "ready",
			},
		},
	}

	// The flip is driven purely by classification.
	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.Changed != 2 {
		t.Errorf("changed = %d, want 2", result.Changed)
	}

	// Check server updates
	if len(store.serverUpdates) != 1 {
		t.Fatalf("serverUpdates = %d, want 1", len(store.serverUpdates))
	}
	if store.serverUpdates[0].ID != "org1|cb1|1.0.0|18" {
		t.Errorf("serverUpdates[0].ID = %q", store.serverUpdates[0].ID)
	}
	if store.serverUpdates[0].Passed != false {
		t.Errorf("serverUpdates[0].Passed = %v, want false", store.serverUpdates[0].Passed)
	}
	if store.serverUpdates[0].Status != "blocked" {
		t.Errorf("serverUpdates[0].Status = %q, want blocked", store.serverUpdates[0].Status)
	}

	// Check git updates
	if len(store.gitUpdates) != 1 {
		t.Fatalf("gitUpdates = %d, want 1", len(store.gitUpdates))
	}
	if store.gitUpdates[0].Passed != false {
		t.Errorf("gitUpdates[0].Passed = %v, want false", store.gitUpdates[0].Passed)
	}

	// The bulk git-repo status re-materialisation runs for the target present in
	// the results (unconditional, not gated on which results changed).
	if len(store.recomputedAllTargets) != 1 || store.recomputedAllTargets[0] != "18" {
		t.Errorf("recomputedAllTargets = %v, want [18]", store.recomputedAllTargets)
	}
}

func TestRescoreCookstyleResults_SkipsErrorMessageRows(t *testing.T) {
	offenses := []analysis.CookstyleOffense{
		{Severity: "error", CopName: "Chef/Deprecations/SomeRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:           "org1|cb1|1.0.0|18",
				Offences:     offJSON,
				ErrorMessage: "scan crashed",
				Passed:       true, // inconclusive — should not be changed
			},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0 (error_message rows are skipped)", result.Total)
	}
	if result.Changed != 0 {
		t.Errorf("changed = %d, want 0", result.Changed)
	}
}

func TestRescoreCookstyleResults_SkipsNilOffences(t *testing.T) {
	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:       "org1|cb1|1.0.0|18",
				Offences: nil, // no offences stored (legacy row)
				Passed:   true,
			},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0 (nil offences rows are skipped)", result.Total)
	}
}

func TestRescoreCookstyleResults_ClassificationBlocksDespiteRules(t *testing.T) {
	// A warning-severity offense is passed at the offense level, but the cop is
	// classified as a blocker for this target — rescore must flip passed=false.
	offenses := []analysis.CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Style/SomeRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{ID: "org1|cb1|1.0.0|18", Offences: offJSON, Passed: true},
		},
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/SomeRule", Classification: "blocker"}},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed != 1 {
		t.Fatalf("changed = %d, want 1 (classification should block)", result.Changed)
	}
	if len(store.serverUpdates) != 1 || store.serverUpdates[0].Passed != false {
		t.Errorf("expected passed=false update, got %+v", store.serverUpdates)
	}
}

func TestRescoreCookstyleResults_NoiseClassificationPasses(t *testing.T) {
	// An error-severity offense is failed at the offense level, but the cop is
	// classified as noise — rescore must flip passed=true.
	offenses := []analysis.CookstyleOffense{
		{Severity: "error", CopName: "Chef/Style/SomeRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{ID: "org1|cb1|1.0.0|18", Offences: offJSON, Passed: false},
		},
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/SomeRule", Classification: "noise"}},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed != 1 {
		t.Fatalf("changed = %d, want 1 (noise should pass)", result.Changed)
	}
	if len(store.serverUpdates) != 1 || store.serverUpdates[0].Passed != true {
		t.Errorf("expected passed=true update, got %+v", store.serverUpdates)
	}
}

func TestRescoreCookstyleResults_EmptyOffencesArrayPasses(t *testing.T) {
	offJSON, _ := json.Marshal([]analysis.CookstyleOffense{})

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:       "org1|cb1|1.0.0|18",
				Offences: offJSON,
				Passed:   false, // incorrectly marked as failed
			},
		},
	}

	result, err := RescoreCookstyleResults(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed != 1 {
		t.Errorf("changed = %d, want 1", result.Changed)
	}
	if store.serverUpdates[0].Passed != true {
		t.Errorf("should pass with empty offenses")
	}
}
