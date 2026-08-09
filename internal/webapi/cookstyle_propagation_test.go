// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockPropagationStore struct {
	classifications map[string][]datastore.CopClassification
	exclusions      []datastore.ScanScopeExclusion
	serverRefs      []datastore.CookstyleResultRef
	gitRefs         []datastore.CookstyleResultRef
	orgs            []datastore.Organisation

	serverPassedUpdates  []passedUpdate
	gitPassedUpdates     []passedUpdate
	gitCompatRecomputed  []string
	roleCompatRecomputed []string
}

type passedUpdate struct {
	key    string
	passed bool
	status string
}

func (m *mockPropagationStore) ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error) {
	var out []datastore.CopClassification
	for _, cs := range m.classifications {
		out = append(out, cs...)
	}
	return out, nil
}

func (m *mockPropagationStore) ListScanScopeExclusions(ctx context.Context) ([]datastore.ScanScopeExclusion, error) {
	return m.exclusions, nil
}

func (m *mockPropagationStore) ListServerCookbookCookstyleResultsWithCop(ctx context.Context, cop, target string) ([]datastore.CookstyleResultRef, error) {
	return m.serverRefs, nil
}

func (m *mockPropagationStore) ListGitRepoCookstyleResultsWithCop(ctx context.Context, cop, target string) ([]datastore.CookstyleResultRef, error) {
	return m.gitRefs, nil
}

func (m *mockPropagationStore) UpdateServerCookbookCookstyleVerdict(ctx context.Context, org, name, version, target string, passed bool, status string) error {
	m.serverPassedUpdates = append(m.serverPassedUpdates, passedUpdate{key: org + "|" + name + "|" + version, passed: passed, status: status})
	return nil
}

func (m *mockPropagationStore) UpdateGitRepoCookstyleVerdict(ctx context.Context, name, url, target string, passed bool, status string) error {
	m.gitPassedUpdates = append(m.gitPassedUpdates, passedUpdate{key: name + "|" + url, passed: passed, status: status})
	return nil
}

func (m *mockPropagationStore) RecomputeGitRepoCompatibilityStatus(ctx context.Context, name, url, target string) error {
	m.gitCompatRecomputed = append(m.gitCompatRecomputed, name)
	return nil
}

func (m *mockPropagationStore) RecomputeAllRoleCompatStatus(ctx context.Context, target string) error {
	m.roleCompatRecomputed = append(m.roleCompatRecomputed, target)
	return nil
}

func (m *mockPropagationStore) ListOrganisations(ctx context.Context) ([]datastore.Organisation, error) {
	return m.orgs, nil
}

var _ CookstylePropagationStore = (*mockPropagationStore)(nil)

type mockComplexityRescorer struct {
	serverCalls [][]datastore.ServerCookbook
	serverOrgs  []string
	gitCalls    [][]datastore.GitRepo
	gitOrgs     []string
}

func (m *mockComplexityRescorer) ScoreServerCookbooks(ctx context.Context, cbs []datastore.ServerCookbook, target string, org string) remediation.ComplexityBatchResult {
	m.serverCalls = append(m.serverCalls, cbs)
	m.serverOrgs = append(m.serverOrgs, org)
	return remediation.ComplexityBatchResult{}
}

func (m *mockComplexityRescorer) ScoreGitRepos(ctx context.Context, repos []datastore.GitRepo, target string, org string) remediation.ComplexityBatchResult {
	m.gitCalls = append(m.gitCalls, repos)
	m.gitOrgs = append(m.gitOrgs, org)
	return remediation.ComplexityBatchResult{}
}

var _ ComplexityRescorer = (*mockComplexityRescorer)(nil)

type mockReadinessRecomputer struct {
	orgs []string
}

func (m *mockReadinessRecomputer) EvaluateOrganisation(ctx context.Context, orgID, orgName string, target string) ([]analysis.ReadinessResult, error) {
	m.orgs = append(m.orgs, orgID)
	return nil, nil
}

var _ ReadinessRecomputer = (*mockReadinessRecomputer)(nil)

// offJSON marshals offenses to the stored enriched (flat) JSON shape.
func offJSONForCop(cop, severity string) []byte {
	b, _ := json.Marshal([]map[string]any{{"cop_name": cop, "severity": severity}})
	return b
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPropagate_FlipsServerVerdictAndRecomputes(t *testing.T) {
	// A Style/warning cop reclassified to blocker: passing server cookbook must
	// flip to failing, get complexity-rescored, and trigger readiness recompute.
	store := &mockPropagationStore{
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "blocker"}},
		},
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true, CookstyleStatus: "ready"},
		},
	}
	scorer := &mockComplexityRescorer{}
	readiness := &mockReadinessRecomputer{}
	p := NewCookstylePropagator(store, scorer, readiness, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ServerResultsChanged != 1 {
		t.Errorf("ServerResultsChanged = %d, want 1", res.ServerResultsChanged)
	}
	if len(store.serverPassedUpdates) != 1 || store.serverPassedUpdates[0].passed != false || store.serverPassedUpdates[0].status != "blocked" {
		t.Errorf("expected passed=false status=blocked update, got %+v", store.serverPassedUpdates)
	}
	if res.CookbooksRescored != 1 {
		t.Errorf("CookbooksRescored = %d, want 1", res.CookbooksRescored)
	}
	if len(scorer.serverCalls) != 1 || scorer.serverOrgs[0] != "org-a" {
		t.Errorf("expected ScoreServerCookbooks for org-a, got orgs=%v", scorer.serverOrgs)
	}
	if res.OrgsReadinessRecomputed != 1 || len(readiness.orgs) != 1 || readiness.orgs[0] != "org-a" {
		t.Errorf("expected readiness recompute for org-a, got %v", readiness.orgs)
	}
	// Roles derive from the same cookstyle verdicts, so a reclassification must
	// re-materialise role compat for the target once.
	if len(store.roleCompatRecomputed) != 1 || store.roleCompatRecomputed[0] != "18" {
		t.Errorf("expected role compat recompute for target 18, got %v", store.roleCompatRecomputed)
	}
}

func TestPropagate_RescoreEvenWhenVerdictUnchanged(t *testing.T) {
	// Already needs_review, reclassified review → still needs_review: the
	// materialised verdict is unchanged so no verdict row is written, but the
	// classification-weighted complexity must still be re-scored.
	store := &mockPropagationStore{
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "review"}},
		},
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true, CookstyleStatus: "needs_review"},
		},
	}
	scorer := &mockComplexityRescorer{}
	p := NewCookstylePropagator(store, scorer, nil, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ServerResultsChanged != 0 {
		t.Errorf("ServerResultsChanged = %d, want 0 (verdict unchanged)", res.ServerResultsChanged)
	}
	if len(store.serverPassedUpdates) != 0 {
		t.Errorf("expected no passed updates, got %+v", store.serverPassedUpdates)
	}
	if res.CookbooksRescored != 1 {
		t.Errorf("CookbooksRescored = %d, want 1 (complexity must be re-scored)", res.CookbooksRescored)
	}
}

func TestPropagate_StatusChangeWithoutPassedFlip(t *testing.T) {
	// ready → needs_review: passed stays true (review is not blocking) but the
	// materialised rollup status changes, so the verdict MUST be re-written.
	// Under the old passed-only comparison this update was silently skipped,
	// leaving the materialised status stale — the SoT regression this guards.
	store := &mockPropagationStore{
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "review"}},
		},
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true, CookstyleStatus: "ready"},
		},
	}
	scorer := &mockComplexityRescorer{}
	p := NewCookstylePropagator(store, scorer, nil, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ServerResultsChanged != 1 {
		t.Errorf("ServerResultsChanged = %d, want 1 (status changed)", res.ServerResultsChanged)
	}
	if len(store.serverPassedUpdates) != 1 || store.serverPassedUpdates[0].passed != true || store.serverPassedUpdates[0].status != "needs_review" {
		t.Errorf("expected passed=true status=needs_review update, got %+v", store.serverPassedUpdates)
	}
}

func TestPropagate_GitFlipRecomputesCompat(t *testing.T) {
	store := &mockPropagationStore{
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "blocker"}},
		},
		gitRefs: []datastore.CookstyleResultRef{
			{GitRepoName: "repo1", GitRepoURL: "https://git/repo1", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true, CookstyleStatus: "ready"},
		},
		orgs: []datastore.Organisation{{Name: "org-a"}},
	}
	scorer := &mockComplexityRescorer{}
	p := NewCookstylePropagator(store, scorer, nil, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.GitResultsChanged != 1 {
		t.Errorf("GitResultsChanged = %d, want 1", res.GitResultsChanged)
	}
	if len(store.gitCompatRecomputed) != 1 {
		t.Errorf("expected git compat recompute, got %v", store.gitCompatRecomputed)
	}
	// Git-only change: complexity re-scored against the (single) organisation.
	if res.GitReposRescored != 1 || len(scorer.gitCalls) != 1 || scorer.gitOrgs[0] != "org-a" {
		t.Errorf("expected git complexity rescored for org-a, got orgs=%v rescored=%d", scorer.gitOrgs, res.GitReposRescored)
	}
}

func TestPropagate_NilScorerAndReadinessSafe(t *testing.T) {
	store := &mockPropagationStore{
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: false, CookstyleStatus: "blocked"},
		},
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "noise"}},
		},
	}
	p := NewCookstylePropagator(store, nil, nil, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// noise: error-free style → passes; status blocked → ready, passed false → true.
	if res.ServerResultsChanged != 1 || store.serverPassedUpdates[0].passed != true || store.serverPassedUpdates[0].status != "ready" {
		t.Errorf("expected passed=true status=ready flip, got changed=%d updates=%+v", res.ServerResultsChanged, store.serverPassedUpdates)
	}
	if res.CookbooksRescored != 0 || res.OrgsReadinessRecomputed != 0 {
		t.Errorf("nil scorer/readiness should skip those stages: %+v", res)
	}
}

func TestPropagate_SkipsErrorMessageRows(t *testing.T) {
	store := &mockPropagationStore{
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "blocker"}},
		},
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true, ErrorMessage: "scan crashed"},
		},
	}
	scorer := &mockComplexityRescorer{}
	readiness := &mockReadinessRecomputer{}
	p := NewCookstylePropagator(store, scorer, readiness, nil)

	res, err := p.PropagateReclassification(context.Background(), "Chef/Style/Foo", "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ServerResultsChanged != 0 || len(store.serverPassedUpdates) != 0 {
		t.Errorf("error_message rows must be skipped, got %+v", res)
	}
	if res.CookbooksRescored != 0 || res.OrgsReadinessRecomputed != 0 {
		t.Errorf("error_message rows must not trigger recompute, got %+v", res)
	}
}
