// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Helper: build a Router with target chef versions configured.
// ---------------------------------------------------------------------------

func newGitRepoTestRouter(store *mockStore) *Router {
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

func newGitRepoTestRouterWithConfig(store *mockStore, cfg *config.Config) *Router {
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

// ---------------------------------------------------------------------------
// parseGitRepoListResponse is a helper that decodes the paginated response.
// ---------------------------------------------------------------------------

type gitRepoListResponse struct {
	Data []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		GitRepoURL        string `json:"git_repo_url"`
		HeadCommitSHA     string `json:"head_commit_sha"`
		DefaultBranch     string `json:"default_branch"`
		HasTestSuite      bool   `json:"has_test_suite"`
		LastFetchedAt     string `json:"last_fetched_at"`
		Compatibility     string `json:"compatibility"`
		TKStatus          string `json:"tk_status"`
		TKPassed          int    `json:"tk_passed"`
		TKTotal           int    `json:"tk_total"`
		TargetChefVersion string `json:"target_chef_version"`
	} `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalItems int `json:"total_items"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

func decodeGitRepoListResponse(t *testing.T, w *httptest.ResponseRecorder) gitRepoListResponse {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp gitRepoListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func sampleGitRepos() []datastore.GitRepo {
	return []datastore.GitRepo{
		{Name: "cookbook-a", GitRepoURL: "https://github.com/org/cookbook-a.git", HasTestSuite: true},
		{Name: "cookbook-b", GitRepoURL: "https://github.com/org/cookbook-b.git", HasTestSuite: false},
		{Name: "cookbook-c", GitRepoURL: "https://github.com/org/cookbook-c.git", HasTestSuite: true},
		{Name: "cookbook-d", GitRepoURL: "https://github.com/org/cookbook-d.git", HasTestSuite: true},
	}
}

func sampleCookstyleResults() []datastore.GitRepoCookstyleResult {
	now := time.Now()
	return []datastore.GitRepoCookstyleResult{
		{GitRepoName: "cookbook-a", GitRepoURL: "https://github.com/org/cookbook-a.git", TargetChefVersion: "18.0.0", Passed: true, ScannedAt: now},
		{GitRepoName: "cookbook-b", GitRepoURL: "https://github.com/org/cookbook-b.git", TargetChefVersion: "18.0.0", Passed: false, OffenceCount: 5, ScannedAt: now},
		// cookbook-c and cookbook-d have no cookstyle result → "untested"
	}
}

func sampleKitchenResults() []datastore.GitKitchenResult {
	passed := true
	failed := false
	return []datastore.GitKitchenResult{
		{GitRepoName: "cookbook-a", InstanceName: "default-ubuntu-2404", Passed: &passed},
		{GitRepoName: "cookbook-a", InstanceName: "default-rocky-9", Passed: &passed},
		{GitRepoName: "cookbook-b", InstanceName: "default-ubuntu-2404", Passed: &passed},
		{GitRepoName: "cookbook-b", InstanceName: "default-rocky-9", Passed: &failed},
		// cookbook-c and cookbook-d have no kitchen results → "untested"
	}
}

func defaultGitRepoMockStore() *mockStore {
	return &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return sampleGitRepos(), nil
		},
		ListAllGitRepoCookstyleResultsFn: func(ctx context.Context) ([]datastore.GitRepoCookstyleResult, error) {
			return sampleCookstyleResults(), nil
		},
		ListGitKitchenResultsFn: func(ctx context.Context) ([]datastore.GitKitchenResult, error) {
			return sampleKitchenResults(), nil
		},
		ListActiveGitKitchenResultsFn: func(ctx context.Context) ([]datastore.GitKitchenResult, error) {
			return sampleKitchenResults(), nil
		},
	}
}

// ---------------------------------------------------------------------------
// Tests: Compatibility is also returned correctly
// ---------------------------------------------------------------------------

func TestHandleGitRepos_CompatibilityInResponse(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	compatMap := make(map[string]string)
	for _, d := range resp.Data {
		compatMap[d.Name] = d.Compatibility
	}

	tests := []struct {
		name     string
		expected string
	}{
		{"cookbook-a", "compatible"},
		{"cookbook-b", "incompatible"},
		{"cookbook-c", "untested"},
		{"cookbook-d", "untested"},
	}
	for _, tc := range tests {
		got := compatMap[tc.name]
		if got != tc.expected {
			t.Errorf("compatibility for %q: expected %q, got %q", tc.name, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: Empty repos list returns empty data
// ---------------------------------------------------------------------------

func TestHandleGitRepos_EmptyList(t *testing.T) {
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return nil, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 0 {
		t.Errorf("expected 0 repos, got %d", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// Tests: Name filter still works alongside TK status filter
// ---------------------------------------------------------------------------

func TestHandleGitRepos_NameFilter_WithTKStatus(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// Filter by name=cookbook-a AND tk_status=passed — should match.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?name=cookbook-a&tk_status=passed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-a" {
		t.Errorf("expected cookbook-a, got %q", resp.Data[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Tests: POST method is rejected
// ---------------------------------------------------------------------------

func TestHandleGitRepos_PostMethodRejected(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK partial status — repos with mixed pass/fail are "partial"
// ---------------------------------------------------------------------------

func TestHandleGitRepos_TKPartialStatus(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	tkStatusMap := make(map[string]string)
	for _, d := range resp.Data {
		tkStatusMap[d.Name] = d.TKStatus
	}

	// cookbook-a: 2 passed, 0 failed → "passed"
	if tkStatusMap["cookbook-a"] != "passed" {
		t.Errorf("cookbook-a: expected tk_status=passed, got %q", tkStatusMap["cookbook-a"])
	}
	// cookbook-b: 1 passed, 1 failed → "partial"
	if tkStatusMap["cookbook-b"] != "partial" {
		t.Errorf("cookbook-b: expected tk_status=partial, got %q", tkStatusMap["cookbook-b"])
	}
	// cookbook-c: no results → "untested"
	if tkStatusMap["cookbook-c"] != "untested" {
		t.Errorf("cookbook-c: expected tk_status=untested, got %q", tkStatusMap["cookbook-c"])
	}
}

func TestHandleGitRepos_TKStatusFilter_Partial(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=partial", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo with partial status, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-b" {
		t.Errorf("expected cookbook-b, got %q", resp.Data[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Tests: has_test_suite filter
// ---------------------------------------------------------------------------

func TestHandleGitRepos_HasTestSuiteFilter_Yes(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?has_test_suite=yes", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	// cookbook-a, cookbook-c, cookbook-d have test suites
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 repos with test suite, got %d", len(resp.Data))
	}
	for _, d := range resp.Data {
		if !d.HasTestSuite {
			t.Errorf("repo %q should have test suite", d.Name)
		}
	}
}

func TestHandleGitRepos_HasTestSuiteFilter_No(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?has_test_suite=no", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	// Only cookbook-b has no test suite
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo without test suite, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-b" {
		t.Errorf("expected cookbook-b, got %q", resp.Data[0].Name)
	}
}

func TestHandleGitRepos_HasTestSuiteFilter_Both(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// Both yes and no selected = no filter applied
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?has_test_suite=yes,no", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 4 {
		t.Fatalf("expected all 4 repos when both selected, got %d", len(resp.Data))
	}
}

