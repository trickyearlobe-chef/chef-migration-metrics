// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type mockRepoLister struct {
	repos []GitRepo
	err   error
}

func (m *mockRepoLister) ListGitRepos(_ context.Context) ([]GitRepo, error) {
	return m.repos, m.err
}

type mockAnalysisProvider struct {
	platforms map[string][]string
}

func (m *mockAnalysisProvider) GetKitchenAnalysisPlatforms(_ context.Context, repoName string) ([]string, error) {
	p, ok := m.platforms[repoName]
	if !ok {
		return nil, fmt.Errorf("no analysis for %s", repoName)
	}
	return p, nil
}

type mockResultProvider struct {
	statuses map[string]string
}

func (m *mockResultProvider) GetLatestTestKitchenStatus(_ context.Context, repoName string) (string, error) {
	s, ok := m.statuses[repoName]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return s, nil
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func testRepos() []GitRepo {
	return []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
		{Name: "mysql", GitRepoURL: "https://git.example.com/mysql.git", HasTestSuite: false},
		{Name: "redis", GitRepoURL: "https://git.example.com/redis.git", HasTestSuite: true},
		{Name: "b_win_iis", GitRepoURL: "https://git.example.com/b_win_iis.git", HasTestSuite: true},
		{Name: "b_win_dns", GitRepoURL: "https://git.example.com/b_win_dns.git", HasTestSuite: true},
		{Name: "aett_fx_base", GitRepoURL: "https://git.example.com/aett_fx_base.git", HasTestSuite: true},
		{Name: "legacy_broken", GitRepoURL: "https://git.example.com/legacy_broken.git", HasTestSuite: false, KitchenExcluded: true},
		{Name: "deprecated_app", GitRepoURL: "https://git.example.com/deprecated_app.git", HasTestSuite: true, KitchenExcluded: true},
		{Name: "java", GitRepoURL: "https://git.example.com/java.git", HasTestSuite: true},
	}
}

func cookbookNames(cookbooks []ResolvedCookbook) []string {
	names := make([]string, len(cookbooks))
	for i, c := range cookbooks {
		names[i] = c.Name
	}
	return names
}

func containsName(cookbooks []ResolvedCookbook, name string) bool {
	for _, c := range cookbooks {
		if c.Name == name {
			return true
		}
	}
	return false
}

func TestResolveBatch_NoFilters(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 10 repos minus 2 excluded = 8
	if est.TotalCookbooks != 8 {
		t.Errorf("expected 8 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if containsName(est.Cookbooks, "legacy_broken") {
		t.Error("excluded repo legacy_broken should not be in results")
	}
	if containsName(est.Cookbooks, "deprecated_app") {
		t.Error("excluded repo deprecated_app should not be in results")
	}
}

func TestResolveBatch_HasTestSuiteTrue(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{HasTestSuite: boolPtr(true)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-excluded repos with HasTestSuite=true: apache2, nginx, redis, b_win_iis, b_win_dns, aett_fx_base, java = 7
	if est.TotalCookbooks != 7 {
		t.Errorf("expected 7 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if containsName(est.Cookbooks, "mysql") {
		t.Error("mysql (HasTestSuite=false) should not be in results")
	}
}

func TestResolveBatch_HasTestSuiteFalse(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{HasTestSuite: boolPtr(false)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-excluded repos with HasTestSuite=false: mysql = 1
	if est.TotalCookbooks != 1 {
		t.Errorf("expected 1 cookbook, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "mysql") {
		t.Error("mysql should be in results")
	}
}

func TestResolveBatch_CookbookNameExact(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{CookbookNames: []string{"nginx"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 1 {
		t.Errorf("expected 1 cookbook, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "nginx") {
		t.Error("nginx should be in results")
	}
}

func TestResolveBatch_CookbookNameGlob(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{CookbookNames: []string{"b_win_*"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 2 {
		t.Errorf("expected 2 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "b_win_iis") {
		t.Error("b_win_iis should be in results")
	}
	if !containsName(est.Cookbooks, "b_win_dns") {
		t.Error("b_win_dns should be in results")
	}
}

func TestResolveBatch_MultipleGlobPatterns(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{CookbookNames: []string{"b_win_*", "aett_*"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 3 {
		t.Errorf("expected 3 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "b_win_iis") {
		t.Error("b_win_iis should be in results")
	}
	if !containsName(est.Cookbooks, "b_win_dns") {
		t.Error("b_win_dns should be in results")
	}
	if !containsName(est.Cookbooks, "aett_fx_base") {
		t.Error("aett_fx_base should be in results")
	}
}

func TestResolveBatch_ExcludeCookbooks(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{ExcludeCookbooks: []string{"nginx", "redis"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 non-excluded minus nginx and redis = 6
	if est.TotalCookbooks != 6 {
		t.Errorf("expected 6 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if containsName(est.Cookbooks, "nginx") {
		t.Error("nginx should not be in results")
	}
	if containsName(est.Cookbooks, "redis") {
		t.Error("redis should not be in results")
	}
}

func TestResolveBatch_ExcludeGlobPattern(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{ExcludeCookbooks: []string{"b_win_*"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 non-excluded minus 2 b_win = 6
	if est.TotalCookbooks != 6 {
		t.Errorf("expected 6 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if containsName(est.Cookbooks, "b_win_iis") {
		t.Error("b_win_iis should not be in results")
	}
	if containsName(est.Cookbooks, "b_win_dns") {
		t.Error("b_win_dns should not be in results")
	}
}

func TestResolveBatch_PersistentExclusion(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	t.Run("default excludes kitchen-excluded repos", func(t *testing.T) {
		est, err := r.ResolveBatch(context.Background(), Filters{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if est.TotalCookbooks != 8 {
			t.Errorf("expected 8 cookbooks, got %d", est.TotalCookbooks)
		}
		if containsName(est.Cookbooks, "legacy_broken") {
			t.Error("legacy_broken should be excluded by default")
		}
		if containsName(est.Cookbooks, "deprecated_app") {
			t.Error("deprecated_app should be excluded by default")
		}
	})

	t.Run("IncludeExcluded returns all repos", func(t *testing.T) {
		est, err := r.ResolveBatch(context.Background(), Filters{IncludeExcluded: true}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if est.TotalCookbooks != 10 {
			t.Errorf("expected 10 cookbooks, got %d", est.TotalCookbooks)
		}
		if !containsName(est.Cookbooks, "legacy_broken") {
			t.Error("legacy_broken should be included when IncludeExcluded is true")
		}
		if !containsName(est.Cookbooks, "deprecated_app") {
			t.Error("deprecated_app should be included when IncludeExcluded is true")
		}
	})
}

func TestResolveBatch_MaxCount(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{}, intPtr(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 3 {
		t.Errorf("expected 3 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
}

func TestResolveBatch_CombinedFilters(t *testing.T) {
	mock := &mockRepoLister{repos: testRepos()}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{
		HasTestSuite:  boolPtr(true),
		CookbookNames: []string{"b_win_*", "aett_*"},
	}, intPtr(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 2 {
		t.Errorf("expected 2 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	// All 3 matches have test suites, but maxCount caps to 2.
	for _, c := range est.Cookbooks {
		if c.Name != "b_win_iis" && c.Name != "b_win_dns" && c.Name != "aett_fx_base" {
			t.Errorf("unexpected cookbook in results: %s", c.Name)
		}
	}
}

func TestResolveBatch_RepoListError(t *testing.T) {
	mock := &mockRepoLister{err: errors.New("database connection failed")}
	r := NewResolver(mock)

	_, err := r.ResolveBatch(context.Background(), Filters{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, mock.err) {
		t.Errorf("expected wrapped error containing %q, got %q", mock.err, err)
	}
}

func TestResolveBatch_EmptyRepoList(t *testing.T) {
	mock := &mockRepoLister{repos: []GitRepo{}}
	r := NewResolver(mock)

	est, err := r.ResolveBatch(context.Background(), Filters{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 0 {
		t.Errorf("expected 0 cookbooks, got %d", est.TotalCookbooks)
	}
	if len(est.Cookbooks) != 0 {
		t.Errorf("expected empty cookbooks slice, got %d items", len(est.Cookbooks))
	}
}

func TestResolveBatch_PlatformFilter(t *testing.T) {
	repos := []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
		{Name: "iis", GitRepoURL: "https://git.example.com/iis.git", HasTestSuite: true},
	}
	mock := &mockRepoLister{repos: repos}
	analysis := &mockAnalysisProvider{
		platforms: map[string][]string{
			"apache2": {"ubuntu-22.04", "centos-8"},
			"nginx":   {"ubuntu-22.04"},
			"iis":     {"windows-2019"},
		},
	}
	r := NewResolver(mock, WithAnalysisProvider(analysis))

	est, err := r.ResolveBatch(context.Background(), Filters{Platforms: []string{"ubuntu*"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 2 {
		t.Errorf("expected 2 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "apache2") {
		t.Error("apache2 should be in results")
	}
	if !containsName(est.Cookbooks, "nginx") {
		t.Error("nginx should be in results")
	}
	if containsName(est.Cookbooks, "iis") {
		t.Error("iis should not be in results")
	}
	// Verify Platforms field is populated.
	for _, c := range est.Cookbooks {
		if len(c.Platforms) == 0 {
			t.Errorf("cookbook %s should have platforms populated", c.Name)
		}
	}
}

func TestResolveBatch_PlatformFilterWithoutProvider(t *testing.T) {
	repos := []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
		{Name: "iis", GitRepoURL: "https://git.example.com/iis.git", HasTestSuite: true},
	}
	mock := &mockRepoLister{repos: repos}
	r := NewResolver(mock) // No analysis provider.

	est, err := r.ResolveBatch(context.Background(), Filters{Platforms: []string{"ubuntu*"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Filter ignored when provider is nil — all 3 returned.
	if est.TotalCookbooks != 3 {
		t.Errorf("expected 3 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
}

func TestResolveBatch_PreviousStatusFilter(t *testing.T) {
	repos := []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
		{Name: "iis", GitRepoURL: "https://git.example.com/iis.git", HasTestSuite: true},
	}
	mock := &mockRepoLister{repos: repos}
	results := &mockResultProvider{
		statuses: map[string]string{
			"apache2": "passed",
			"nginx":   "failed",
			"iis":     "passed",
		},
	}
	r := NewResolver(mock, WithResultProvider(results))

	est, err := r.ResolveBatch(context.Background(), Filters{PreviousStatus: "passed"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 2 {
		t.Errorf("expected 2 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
	if !containsName(est.Cookbooks, "apache2") {
		t.Error("apache2 should be in results")
	}
	if !containsName(est.Cookbooks, "iis") {
		t.Error("iis should be in results")
	}
	if containsName(est.Cookbooks, "nginx") {
		t.Error("nginx should not be in results")
	}
}

func TestResolveBatch_PreviousStatusFilterWithoutProvider(t *testing.T) {
	repos := []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
		{Name: "iis", GitRepoURL: "https://git.example.com/iis.git", HasTestSuite: true},
	}
	mock := &mockRepoLister{repos: repos}
	r := NewResolver(mock) // No result provider.

	est, err := r.ResolveBatch(context.Background(), Filters{PreviousStatus: "passed"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Filter ignored when provider is nil — all 3 returned.
	if est.TotalCookbooks != 3 {
		t.Errorf("expected 3 cookbooks, got %d: %v", est.TotalCookbooks, cookbookNames(est.Cookbooks))
	}
}

func TestResolveBatch_PlatformsPopulated(t *testing.T) {
	repos := []GitRepo{
		{Name: "apache2", GitRepoURL: "https://git.example.com/apache2.git", HasTestSuite: true},
		{Name: "nginx", GitRepoURL: "https://git.example.com/nginx.git", HasTestSuite: true},
	}
	mock := &mockRepoLister{repos: repos}
	analysis := &mockAnalysisProvider{
		platforms: map[string][]string{
			"apache2": {"ubuntu-22.04", "centos-8", "debian-11"},
			"nginx":   {"ubuntu-22.04"},
		},
	}
	r := NewResolver(mock, WithAnalysisProvider(analysis))

	est, err := r.ResolveBatch(context.Background(), Filters{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCookbooks != 2 {
		t.Errorf("expected 2 cookbooks, got %d", est.TotalCookbooks)
	}

	for _, c := range est.Cookbooks {
		expected := analysis.platforms[c.Name]
		if len(c.Platforms) != len(expected) {
			t.Errorf("cookbook %s: expected %d platforms, got %d", c.Name, len(expected), len(c.Platforms))
		}
		if c.EstimatedVMs != len(expected) {
			t.Errorf("cookbook %s: expected EstimatedVMs=%d, got %d", c.Name, len(expected), c.EstimatedVMs)
		}
	}

	// Total estimated VMs = 3 + 1 = 4
	if est.TotalEstimatedVMs != 4 {
		t.Errorf("expected TotalEstimatedVMs=4, got %d", est.TotalEstimatedVMs)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*", "anything", true},
		{"nginx", "nginx", true},
		{"nginx", "apache", false},
		{"b_win_*", "b_win_iis", true},
		{"b_win_*", "nginx", false},
		{"", "anything", false},
		{"aett_*", "aett_fx_base", true},
		{"?edis", "redis", true},
		{"java", "java", true},
		{"java", "javascript", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.name, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.name)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}
