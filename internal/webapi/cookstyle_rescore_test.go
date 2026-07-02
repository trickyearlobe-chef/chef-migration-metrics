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
	serverResults   []datastore.CookstyleRescoreRow
	gitResults      []datastore.CookstyleRescoreRow
	serverUpdates   []datastore.CookstylePassedUpdate
	gitUpdates      []datastore.CookstylePassedUpdate
	recomputedGit   []gitRepoKey
	classifications map[string][]datastore.CopClassification // target version -> overrides
}

type gitRepoKey struct {
	Name, URL, TargetVersion string
}

func (m *mockRescoreStore) ListCopClassifications(ctx context.Context, targetChefVersion string) ([]datastore.CopClassification, error) {
	return m.classifications[targetChefVersion], nil
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

var _ CookstyleRescoreStore = (*mockRescoreStore)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRescoreCookstyleResults_NoResults(t *testing.T) {
	store := &mockRescoreStore{}
	rules := analysis.DefaultFailureRules()

	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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

func TestRescoreCookstyleResults_DefaultRulesNoChange(t *testing.T) {
	// Offenses that already correctly reflect default rules (error/fatal → fail).
	// Use an unclassified cop (generic Lint/, no curated/RemovedIn/department
	// default) so the severity-rule fallback — not classification — drives the
	// verdict; a Chef/Deprecations/* cop is now department-defaulted to Review.
	offenses := []analysis.CookstyleOffense{
		{Severity: "error", CopName: "Lint/SomeUnclassifiedRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "org1|cb1|1.0.0|18",
				Offences:        offJSON,
				ErrorMessage:    "",
				Passed:          false,     // correct: error → fail
				CookstyleStatus: "blocked", // already reflects default rules
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

	rules := analysis.DefaultFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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
}

func TestRescoreCookstyleResults_RulesChangeFlipVerdict(t *testing.T) {
	// Offense at "warning" severity on an unclassified cop (generic Lint/, no
	// curated/RemovedIn/department default) so the severity-rule fallback drives
	// the verdict — a Chef/Deprecations/* cop is now department-defaulted to
	// Review and would ignore these rules entirely.
	offenses := []analysis.CookstyleOffense{
		{Severity: "warning", CopName: "Lint/SomeUnclassifiedRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "org1|cb1|1.0.0|18",
				Offences:        offJSON,
				Passed:          true, // was passing under default rules
				CookstyleStatus: "ready",
			},
		},
		gitResults: []datastore.CookstyleRescoreRow{
			{
				ID:              "myrepo|https://git.example.com/repo|18",
				Offences:        offJSON,
				Passed:          true, // was passing under default rules
				CookstyleStatus: "ready",
			},
		},
	}

	// Tighten the catch-all so warnings now fail for any (unclassified) cop.
	rules := analysis.NewCookstyleFailureRules(map[string][]string{
		"*": {"warning", "error", "fatal"},
	})
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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

	// Check compatibility recompute was triggered
	if len(store.recomputedGit) != 1 {
		t.Fatalf("recomputedGit = %d, want 1", len(store.recomputedGit))
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

	rules := analysis.StrictFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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

	rules := analysis.StrictFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0 (nil offences rows are skipped)", result.Total)
	}
}

func TestRescoreCookstyleResults_RelaxedAllowsPreviouslyFailing(t *testing.T) {
	// Style offense at error level was failing under default, now relaxed ignores Style
	offenses := []analysis.CookstyleOffense{
		{Severity: "error", CopName: "Chef/Style/SomeRule"},
	}
	offJSON, _ := json.Marshal(offenses)

	store := &mockRescoreStore{
		serverResults: []datastore.CookstyleRescoreRow{
			{
				ID:       "org1|cb1|1.0.0|18",
				Offences: offJSON,
				Passed:   false, // was failing under default rules
			},
		},
	}

	rules := analysis.RelaxedFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed != 1 {
		t.Errorf("changed = %d, want 1", result.Changed)
	}
	if len(store.serverUpdates) != 1 {
		t.Fatalf("serverUpdates = %d, want 1", len(store.serverUpdates))
	}
	if store.serverUpdates[0].Passed != true {
		t.Errorf("serverUpdates[0].Passed = %v, want true", store.serverUpdates[0].Passed)
	}
}

func TestRescoreCookstyleResults_ClassificationBlocksDespiteRules(t *testing.T) {
	// A warning-severity offense that severity rules would PASS, but the cop is
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

	// Relaxed rules would NOT fail a Style warning — only the classification does.
	rules := analysis.RelaxedFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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
	// An error-severity offense that severity rules would FAIL, but the cop is
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

	rules := analysis.DefaultFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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

	rules := analysis.DefaultFailureRules()
	result, err := RescoreCookstyleResults(context.Background(), store, rules, nil)
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
