// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
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
		{ID: "repo-1", Name: "cookbook-a", GitRepoURL: "https://github.com/org/cookbook-a.git", HasTestSuite: true},
		{ID: "repo-2", Name: "cookbook-b", GitRepoURL: "https://github.com/org/cookbook-b.git", HasTestSuite: false},
		{ID: "repo-3", Name: "cookbook-c", GitRepoURL: "https://github.com/org/cookbook-c.git", HasTestSuite: true},
		{ID: "repo-4", Name: "cookbook-d", GitRepoURL: "https://github.com/org/cookbook-d.git", HasTestSuite: true},
	}
}

func sampleTKResults() []datastore.GitRepoTestKitchenResult {
	now := time.Now()
	return []datastore.GitRepoTestKitchenResult{
		{ID: "tk-1", GitRepoID: "repo-1", TargetChefVersion: "18.0.0", Compatible: true, TimedOut: false, StartedAt: now},
		{ID: "tk-2", GitRepoID: "repo-2", TargetChefVersion: "18.0.0", Compatible: false, TimedOut: false, StartedAt: now},
		{ID: "tk-3", GitRepoID: "repo-3", TargetChefVersion: "18.0.0", Compatible: false, TimedOut: true, StartedAt: now},
		// repo-4 has no TK result → "untested"
	}
}

func sampleComplexities() []datastore.GitRepoComplexity {
	return []datastore.GitRepoComplexity{
		{ID: "cx-1", GitRepoID: "repo-1", TargetChefVersion: "18.0.0", ErrorCount: 0},
		{ID: "cx-2", GitRepoID: "repo-2", TargetChefVersion: "18.0.0", ErrorCount: 5},
		// repo-3 and repo-4 have no complexity → "untested"
	}
}

func defaultGitRepoMockStore() *mockStore {
	return &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return sampleGitRepos(), nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return sampleComplexities(), nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			return sampleTKResults(), nil
		},
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status is returned in response
// ---------------------------------------------------------------------------

func TestHandleGitRepos_TKStatusInResponse(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 4 {
		t.Fatalf("expected 4 repos, got %d", len(resp.Data))
	}

	// Build a name→tk_status map for easy assertion.
	tkMap := make(map[string]string)
	for _, d := range resp.Data {
		tkMap[d.Name] = d.TKStatus
	}

	tests := []struct {
		name     string
		expected string
	}{
		{"cookbook-a", "passed"},
		{"cookbook-b", "failed"},
		{"cookbook-c", "timed_out"},
		{"cookbook-d", "untested"},
	}
	for _, tc := range tests {
		got := tkMap[tc.name]
		if got != tc.expected {
			t.Errorf("tk_status for %q: expected %q, got %q", tc.name, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status filter — passed
// ---------------------------------------------------------------------------

func TestHandleGitRepos_FilterTKStatus_Passed(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=passed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo with tk_status=passed, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-a" {
		t.Errorf("expected cookbook-a, got %q", resp.Data[0].Name)
	}
	if resp.Data[0].TKStatus != "passed" {
		t.Errorf("expected tk_status=passed, got %q", resp.Data[0].TKStatus)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status filter — failed
// ---------------------------------------------------------------------------

func TestHandleGitRepos_FilterTKStatus_Failed(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=failed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo with tk_status=failed, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-b" {
		t.Errorf("expected cookbook-b, got %q", resp.Data[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status filter — timed_out
// ---------------------------------------------------------------------------

func TestHandleGitRepos_FilterTKStatus_TimedOut(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=timed_out", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo with tk_status=timed_out, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-c" {
		t.Errorf("expected cookbook-c, got %q", resp.Data[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status filter — untested
// ---------------------------------------------------------------------------

func TestHandleGitRepos_FilterTKStatus_Untested(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=untested", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo with tk_status=untested, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-d" {
		t.Errorf("expected cookbook-d, got %q", resp.Data[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK status filter with no match
// ---------------------------------------------------------------------------

func TestHandleGitRepos_FilterTKStatus_NoMatch(t *testing.T) {
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "only-repo"},
			}, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			return nil, nil
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=passed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 0 {
		t.Errorf("expected 0 repos, got %d", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// Tests: Combined filters — tk_status + compatibility
// ---------------------------------------------------------------------------

func TestHandleGitRepos_CombinedFilter_TKStatus_And_Compatibility(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// cookbook-a: compatibility=compatible, tk_status=passed → should match
	// cookbook-b: compatibility=incompatible, tk_status=failed → no match
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?compatibility=compatible&tk_status=passed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo matching both filters, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "cookbook-a" {
		t.Errorf("expected cookbook-a, got %q", resp.Data[0].Name)
	}
}

func TestHandleGitRepos_CombinedFilter_TKStatus_And_Compatibility_NoOverlap(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// cookbook-a: compatibility=compatible, tk_status=passed
	// Asking for compatible + failed → no overlap
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?compatibility=compatible&tk_status=failed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 0 {
		t.Errorf("expected 0 repos, got %d", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// Tests: TK results for a different target version are ignored
// ---------------------------------------------------------------------------

func TestHandleGitRepos_TKStatus_DifferentTargetVersion(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "cookbook-a"},
			}, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			return []datastore.GitRepoTestKitchenResult{
				// This result is for version 19.0.0, not 18.0.0.
				{ID: "tk-1", GitRepoID: "repo-1", TargetChefVersion: "19.0.0", Compatible: true, TimedOut: false, StartedAt: now},
			}, nil
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	// Should be "untested" because the TK result is for a different version.
	if resp.Data[0].TKStatus != "untested" {
		t.Errorf("expected tk_status=untested (wrong version), got %q", resp.Data[0].TKStatus)
	}
}

// ---------------------------------------------------------------------------
// Tests: TK results DB error is handled gracefully
// ---------------------------------------------------------------------------

func TestHandleGitRepos_TKResults_DBError(t *testing.T) {
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "cookbook-a"},
			}, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	// Should still succeed — TK errors are non-fatal (logged as WARN).
	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	// All repos default to "untested" when TK lookup fails.
	if resp.Data[0].TKStatus != "untested" {
		t.Errorf("expected tk_status=untested (DB error fallback), got %q", resp.Data[0].TKStatus)
	}
}

// ---------------------------------------------------------------------------
// Tests: ListGitRepos DB error returns 500
// ---------------------------------------------------------------------------

func TestHandleGitRepos_ListGitRepos_DBError(t *testing.T) {
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
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
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
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

func TestHandleGitRepos_NameFilter_WithTKStatus_NoMatch(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// Filter by name=cookbook-a AND tk_status=failed — cookbook-a is passed, not failed.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?name=cookbook-a&tk_status=failed", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 0 {
		t.Errorf("expected 0 repos, got %d", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// Tests: Target chef version from query string is used
// ---------------------------------------------------------------------------

func TestHandleGitRepos_ExplicitTargetVersion(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "cookbook-a"},
			}, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			return []datastore.GitRepoTestKitchenResult{
				{ID: "tk-1", GitRepoID: "repo-1", TargetChefVersion: "19.0.0", Compatible: true, TimedOut: false, StartedAt: now},
			}, nil
		},
	}
	r := newGitRepoTestRouter(store)

	// Use target_chef_version=19.0.0 explicitly — should now see "passed".
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?target_chef_version=19.0.0", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	if resp.Data[0].TKStatus != "passed" {
		t.Errorf("expected tk_status=passed with explicit version, got %q", resp.Data[0].TKStatus)
	}
	if resp.Data[0].TargetChefVersion != "19.0.0" {
		t.Errorf("expected target_chef_version=19.0.0, got %q", resp.Data[0].TargetChefVersion)
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
// Tests: Pagination with TK filter
// ---------------------------------------------------------------------------

func TestHandleGitRepos_Pagination_WithTKFilter(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	// Request page 1 with per_page=2, no filter → should get 2 of 4.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?per_page=2&page=1", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 repos on page 1, got %d", len(resp.Data))
	}
	if resp.Pagination.TotalItems != 4 {
		t.Errorf("expected total_items=4, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 2 {
		t.Errorf("expected total_pages=2, got %d", resp.Pagination.TotalPages)
	}

	// With tk_status=untested → only 1 repo, pagination should reflect that.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?tk_status=untested&per_page=2&page=1", nil)
	r.ServeHTTP(w2, req2)

	resp2 := decodeGitRepoListResponse(t, w2)

	if resp2.Pagination.TotalItems != 1 {
		t.Errorf("expected total_items=1 with tk_status=untested, got %d", resp2.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// Tests: No target versions configured — everything is untested
// ---------------------------------------------------------------------------

func TestHandleGitRepos_NoTargetVersions(t *testing.T) {
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "cookbook-a"},
			}, nil
		},
		// These should not be called since no target version is configured.
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			t.Error("ListAllGitRepoComplexities should not be called when no target version")
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			t.Error("ListAllGitRepoTestKitchenResults should not be called when no target version")
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = nil // No target versions.
	r := newGitRepoTestRouterWithConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	if resp.Data[0].TKStatus != "untested" {
		t.Errorf("expected tk_status=untested when no target versions, got %q", resp.Data[0].TKStatus)
	}
	if resp.Data[0].Compatibility != "untested" {
		t.Errorf("expected compatibility=untested when no target versions, got %q", resp.Data[0].Compatibility)
	}
}

// ---------------------------------------------------------------------------
// Tests: TimedOut takes precedence over Compatible in TK status derivation
// ---------------------------------------------------------------------------

func TestHandleGitRepos_TKStatus_TimedOutPrecedence(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "repo-1", Name: "cookbook-a"},
			}, nil
		},
		ListAllGitRepoComplexitiesFn: func(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
			return nil, nil
		},
		ListAllGitRepoTestKitchenResultsFn: func(ctx context.Context) ([]datastore.GitRepoTestKitchenResult, error) {
			// Both TimedOut=true AND Compatible=true — TimedOut should win.
			return []datastore.GitRepoTestKitchenResult{
				{ID: "tk-1", GitRepoID: "repo-1", TargetChefVersion: "18.0.0", Compatible: true, TimedOut: true, StartedAt: now},
			}, nil
		},
	}
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(resp.Data))
	}
	if resp.Data[0].TKStatus != "timed_out" {
		t.Errorf("expected tk_status=timed_out (takes precedence over compatible), got %q", resp.Data[0].TKStatus)
	}
}
