// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
)

// --- Mock implementations ---

type mockAnalysisLoader struct {
	results map[string]*datastore.KitchenAnalysisResult
	errors  map[string]error
}

func (m *mockAnalysisLoader) GetKitchenAnalysisResultByName(_ context.Context, repoName string) (*datastore.KitchenAnalysisResult, error) {
	if err, ok := m.errors[repoName]; ok {
		return nil, err
	}
	r, ok := m.results[repoName]
	if !ok {
		return nil, nil
	}
	return r, nil
}

type mockExclusionsLoader struct {
	exclusions map[string][]gitkitchen.InstanceExclusion
	errors     map[string]error
}

func (m *mockExclusionsLoader) LoadInstanceExclusions(_ context.Context, repoName string) ([]gitkitchen.InstanceExclusion, error) {
	if err, ok := m.errors[repoName]; ok {
		return nil, err
	}
	return m.exclusions[repoName], nil
}

// --- Helpers ---

func makeAnalysisResult(name, url string, platforms []analysis.KitchenPlatform, suites []analysis.KitchenSuite) *datastore.KitchenAnalysisResult {
	pJSON, _ := json.Marshal(platforms)
	sJSON, _ := json.Marshal(suites)
	return &datastore.KitchenAnalysisResult{
		GitRepoName:   name,
		GitRepoURL:    url,
		HeadCommitSHA: "abc123",
		Platforms:     pJSON,
		Suites:        sJSON,
	}
}

func defaultPlatformMap() []config.PlatformMapEntry {
	return []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "ubuntu-24.04", Image: "ubuntu-2404-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
		{KitchenName: "windows-2022", Image: "win-2022-img", Skip: true},
	}
}

// --- Tests ---

func TestPlanBatch_PlannedCookbook(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"apache2": makeAnalysisResult("apache2", "https://git.example.com/apache2.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}, {Name: "centos-7"}},
				[]analysis.KitchenSuite{{Name: "default"}, {Name: "integration"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.TotalCookbooks != 1 {
		t.Errorf("expected TotalCookbooks=1, got %d", result.TotalCookbooks)
	}
	if result.SkippedCookbooks != 0 {
		t.Errorf("expected SkippedCookbooks=0, got %d", result.SkippedCookbooks)
	}
	// 2 platforms × 2 suites = 4 total, all mapped
	if result.TotalEstimatedVMs != 4 {
		t.Errorf("expected TotalEstimatedVMs=4, got %d", result.TotalEstimatedVMs)
	}
	if len(result.Cookbooks) != 1 {
		t.Fatalf("expected 1 cookbook, got %d", len(result.Cookbooks))
	}

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusPlanned {
		t.Errorf("expected planning_status=%q, got %q", PlanningStatusPlanned, cb.PlanningStatus)
	}
	if cb.EstimatedVMs != 4 {
		t.Errorf("expected estimated_vms=4, got %d", cb.EstimatedVMs)
	}
	if cb.TotalInstances != 4 {
		t.Errorf("expected total_instances=4, got %d", cb.TotalInstances)
	}
	if cb.Unmapped != 0 {
		t.Errorf("expected unmapped=0, got %d", cb.Unmapped)
	}
}

func TestPlanBatch_MixedStatuses(t *testing.T) {
	// ubuntu-22.04 → mapped, centos-7 → mapped, windows-2022 → skipped (skip=true)
	// Also add an unmapped platform (rocky-8)
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"mixed": makeAnalysisResult("mixed", "https://git.example.com/mixed.git",
				[]analysis.KitchenPlatform{
					{Name: "ubuntu-22.04"},
					{Name: "centos-7"},
					{Name: "windows-2022"},
					{Name: "rocky-8"},
				},
				[]analysis.KitchenSuite{{Name: "default"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "mixed", GitRepoURL: "https://git.example.com/mixed.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusPlanned {
		t.Errorf("expected planned, got %q", cb.PlanningStatus)
	}
	if cb.TotalInstances != 4 {
		t.Errorf("expected total_instances=4, got %d", cb.TotalInstances)
	}
	// mapped: ubuntu-22.04 + centos-7 = 2
	if cb.EstimatedVMs != 2 {
		t.Errorf("expected estimated_vms=2, got %d", cb.EstimatedVMs)
	}
	if cb.Skipped != 1 {
		t.Errorf("expected skipped=1 (windows-2022), got %d", cb.Skipped)
	}
	if cb.Unmapped != 1 {
		t.Errorf("expected unmapped=1 (rocky-8), got %d", cb.Unmapped)
	}
	if result.TotalEstimatedVMs != 2 {
		t.Errorf("expected TotalEstimatedVMs=2, got %d", result.TotalEstimatedVMs)
	}
}

func TestPlanBatch_NoAnalysis(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "unscanned", GitRepoURL: "https://git.example.com/unscanned.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.TotalCookbooks != 1 {
		t.Errorf("expected TotalCookbooks=1, got %d", result.TotalCookbooks)
	}
	if result.SkippedCookbooks != 1 {
		t.Errorf("expected SkippedCookbooks=1, got %d", result.SkippedCookbooks)
	}
	if result.TotalEstimatedVMs != 0 {
		t.Errorf("expected TotalEstimatedVMs=0, got %d", result.TotalEstimatedVMs)
	}

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusNoAnalysis {
		t.Errorf("expected planning_status=%q, got %q", PlanningStatusNoAnalysis, cb.PlanningStatus)
	}
	if cb.PlanningNote == "" {
		t.Error("expected non-empty planning_note for no_analysis")
	}
}

func TestPlanBatch_ExclusionError(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"broken-excl": makeAnalysisResult("broken-excl", "https://git.example.com/broken-excl.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}},
				[]analysis.KitchenSuite{{Name: "default"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{
		errors: map[string]error{
			"broken-excl": errors.New("database timeout"),
		},
	}

	cookbooks := []ResolvedCookbook{
		{Name: "broken-excl", GitRepoURL: "https://git.example.com/broken-excl.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.SkippedCookbooks != 1 {
		t.Errorf("expected SkippedCookbooks=1, got %d", result.SkippedCookbooks)
	}

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusExclusionError {
		t.Errorf("expected planning_status=%q, got %q", PlanningStatusExclusionError, cb.PlanningStatus)
	}
	if cb.EstimatedVMs != 0 {
		t.Errorf("expected estimated_vms=0, got %d", cb.EstimatedVMs)
	}
}

func TestPlanBatch_PlanError(t *testing.T) {
	// Invalid JSON in platforms causes PlanRepo to error
	badResult := &datastore.KitchenAnalysisResult{
		GitRepoName:   "bad-json",
		GitRepoURL:    "https://git.example.com/bad-json.git",
		HeadCommitSHA: "abc123",
		Platforms:     []byte(`not valid json`),
		Suites:        []byte(`[]`),
	}
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"bad-json": badResult,
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "bad-json", GitRepoURL: "https://git.example.com/bad-json.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.SkippedCookbooks != 1 {
		t.Errorf("expected SkippedCookbooks=1, got %d", result.SkippedCookbooks)
	}

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusPlanError {
		t.Errorf("expected planning_status=%q, got %q", PlanningStatusPlanError, cb.PlanningStatus)
	}
}

func TestPlanBatch_PlannedButEmpty(t *testing.T) {
	// Analysis has platforms but no suites
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"empty": makeAnalysisResult("empty", "https://git.example.com/empty.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}},
				[]analysis.KitchenSuite{},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "empty", GitRepoURL: "https://git.example.com/empty.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	cb := result.Cookbooks[0]
	if cb.PlanningStatus != PlanningStatusPlanned {
		t.Errorf("expected planned, got %q", cb.PlanningStatus)
	}
	if cb.TotalInstances != 0 {
		t.Errorf("expected total_instances=0, got %d", cb.TotalInstances)
	}
	if cb.EstimatedVMs != 0 {
		t.Errorf("expected estimated_vms=0, got %d", cb.EstimatedVMs)
	}
	// Should NOT be counted as skipped
	if result.SkippedCookbooks != 0 {
		t.Errorf("expected SkippedCookbooks=0, got %d", result.SkippedCookbooks)
	}
}

func TestPlanBatch_UserExclusion(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"excl-test": makeAnalysisResult("excl-test", "https://git.example.com/excl-test.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}, {Name: "centos-7"}},
				[]analysis.KitchenSuite{{Name: "default"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{
		exclusions: map[string][]gitkitchen.InstanceExclusion{
			"excl-test": {
				{SuiteName: "default", PlatformName: "centos-7", Reason: "known failure"},
			},
		},
	}

	cookbooks := []ResolvedCookbook{
		{Name: "excl-test", GitRepoURL: "https://git.example.com/excl-test.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	cb := result.Cookbooks[0]
	if cb.EstimatedVMs != 1 {
		t.Errorf("expected estimated_vms=1 (ubuntu only), got %d", cb.EstimatedVMs)
	}
	if cb.UserExcluded != 1 {
		t.Errorf("expected user_excluded=1, got %d", cb.UserExcluded)
	}
	if cb.TotalInstances != 2 {
		t.Errorf("expected total_instances=2, got %d", cb.TotalInstances)
	}
}

func TestPlanBatch_PerPlatformBreakdown(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"repo1": makeAnalysisResult("repo1", "https://git.example.com/repo1.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}, {Name: "centos-7"}},
				[]analysis.KitchenSuite{{Name: "default"}},
			),
			"repo2": makeAnalysisResult("repo2", "https://git.example.com/repo2.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}},
				[]analysis.KitchenSuite{{Name: "default"}, {Name: "ha"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "repo1", GitRepoURL: "https://git.example.com/repo1.git"},
		{Name: "repo2", GitRepoURL: "https://git.example.com/repo2.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.TotalEstimatedVMs != 4 {
		t.Errorf("expected TotalEstimatedVMs=4, got %d", result.TotalEstimatedVMs)
	}
	// ubuntu-22.04: 1 (repo1) + 2 (repo2) = 3
	if result.PerPlatform["ubuntu-22.04"] != 3 {
		t.Errorf("expected per_platform[ubuntu-22.04]=3, got %d", result.PerPlatform["ubuntu-22.04"])
	}
	// centos-7: 1 (repo1)
	if result.PerPlatform["centos-7"] != 1 {
		t.Errorf("expected per_platform[centos-7]=1, got %d", result.PerPlatform["centos-7"])
	}
}

func TestPlanBatch_MultipleCookbooks(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"good": makeAnalysisResult("good", "https://git.example.com/good.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}},
				[]analysis.KitchenSuite{{Name: "default"}},
			),
			// "missing" has no analysis
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "good", GitRepoURL: "https://git.example.com/good.git"},
		{Name: "missing", GitRepoURL: "https://git.example.com/missing.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.TotalCookbooks != 2 {
		t.Errorf("expected TotalCookbooks=2, got %d", result.TotalCookbooks)
	}
	if result.SkippedCookbooks != 1 {
		t.Errorf("expected SkippedCookbooks=1, got %d", result.SkippedCookbooks)
	}
	if result.TotalEstimatedVMs != 1 {
		t.Errorf("expected TotalEstimatedVMs=1, got %d", result.TotalEstimatedVMs)
	}
}

func TestPlanBatch_SuiteExcludesPopulate(t *testing.T) {
	// Suite "linux-only" excludes "windows-2022" via Excludes field.
	// windows-2022 is also skip=true in platform map, so it counts as skipped (not excluded)
	// because skip is evaluated before suite exclude in the planner.
	// Use a mapped platform instead to test suite excludes properly.
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"suite-excl": makeAnalysisResult("suite-excl", "https://git.example.com/suite-excl.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}, {Name: "centos-7"}},
				[]analysis.KitchenSuite{{Name: "linux-only", Excludes: []string{"centos-7"}}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "suite-excl", GitRepoURL: "https://git.example.com/suite-excl.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	cb := result.Cookbooks[0]
	// 2 platforms × 1 suite = 2 total, but centos-7 excluded by suite
	if cb.TotalInstances != 2 {
		t.Errorf("expected total_instances=2, got %d", cb.TotalInstances)
	}
	if cb.Excluded != 1 {
		t.Errorf("expected excluded=1, got %d", cb.Excluded)
	}
	if cb.EstimatedVMs != 1 {
		t.Errorf("expected estimated_vms=1, got %d", cb.EstimatedVMs)
	}
}

func TestPlanBatch_PlatformsAndSuitesPopulated(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{
		results: map[string]*datastore.KitchenAnalysisResult{
			"info-test": makeAnalysisResult("info-test", "https://git.example.com/info-test.git",
				[]analysis.KitchenPlatform{{Name: "ubuntu-22.04"}, {Name: "centos-7"}},
				[]analysis.KitchenSuite{{Name: "default"}, {Name: "smoke"}},
			),
		},
	}
	exclusionsLoader := &mockExclusionsLoader{}

	cookbooks := []ResolvedCookbook{
		{Name: "info-test", GitRepoURL: "https://git.example.com/info-test.git"},
	}

	result := PlanBatch(context.Background(), cookbooks, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	cb := result.Cookbooks[0]
	if len(cb.Platforms) != 2 {
		t.Errorf("expected 2 platforms, got %d", len(cb.Platforms))
	}
	if len(cb.Suites) != 2 {
		t.Errorf("expected 2 suites, got %d", len(cb.Suites))
	}
}

func TestPlanBatch_EmptyInput(t *testing.T) {
	analysisLoader := &mockAnalysisLoader{}
	exclusionsLoader := &mockExclusionsLoader{}

	result := PlanBatch(context.Background(), nil, defaultPlatformMap(), analysisLoader, exclusionsLoader)

	if result.TotalCookbooks != 0 {
		t.Errorf("expected TotalCookbooks=0, got %d", result.TotalCookbooks)
	}
	if result.TotalEstimatedVMs != 0 {
		t.Errorf("expected TotalEstimatedVMs=0, got %d", result.TotalEstimatedVMs)
	}
	if len(result.Cookbooks) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(result.Cookbooks))
	}
}
