// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Helper: build a Router with target chef versions configured.
// ---------------------------------------------------------------------------

func newGitRepoTestRouter(store *mockStore) *Router {
	cfg := testConfig()
	cfg.TargetChefVersion = "18.0.0"
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
		CookstyleStatus   string `json:"cookstyle_status"`
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
// Fixtures — repos now have materialised status columns populated
// ---------------------------------------------------------------------------

func sampleGitRepos() []datastore.GitRepo {
	return []datastore.GitRepo{
		{Name: "cookbook-a", GitRepoURL: "https://github.com/org/cookbook-a.git", HasTestSuite: true, CompatibilityStatus: "compatible", CookstyleStatus: "ready", TKStatus: "passed", TKPassed: 2, TKTotal: 2},
		{Name: "cookbook-b", GitRepoURL: "https://github.com/org/cookbook-b.git", HasTestSuite: false, CompatibilityStatus: "incompatible", CookstyleStatus: "blocked", TKStatus: "partial", TKPassed: 1, TKTotal: 2},
		{Name: "cookbook-c", GitRepoURL: "https://github.com/org/cookbook-c.git", HasTestSuite: true, CompatibilityStatus: "untested", CookstyleStatus: "needs_review", TKStatus: "untested", TKPassed: 0, TKTotal: 0},
		{Name: "cookbook-d", GitRepoURL: "https://github.com/org/cookbook-d.git", HasTestSuite: true, CompatibilityStatus: "untested", CookstyleStatus: "untested", TKStatus: "untested", TKPassed: 0, TKTotal: 0},
	}
}

func sampleCookstyleResults() []datastore.GitRepoCookstyleResult {
	now := time.Now()
	return []datastore.GitRepoCookstyleResult{
		{GitRepoName: "cookbook-a", GitRepoURL: "https://github.com/org/cookbook-a.git", TargetChefVersion: "18.0.0", Passed: true, ScannedAt: now},
		{GitRepoName: "cookbook-b", GitRepoURL: "https://github.com/org/cookbook-b.git", TargetChefVersion: "18.0.0", Passed: false, OffenceCount: 5, ScannedAt: now},
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
	}
}

func defaultGitRepoMockStore() *mockStore {
	allRepos := sampleGitRepos()
	return &mockStore{
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return allRepos, nil
		},
		ListGitReposFilteredFn: func(ctx context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			// Simple mock: apply name and status filters against sample data.
			var result []datastore.GitRepo
			for _, r := range allRepos {
				if f.Name != "" && !containsFold(r.Name, f.Name) {
					continue
				}
				compat := r.CompatibilityStatus
				if compat == "" {
					compat = "untested"
				}
				if f.CompatibilityStatus != "" && compat != f.CompatibilityStatus {
					continue
				}
				tkStatus := r.TKStatus
				if tkStatus == "" {
					tkStatus = "untested"
				}
				if f.TKStatus != "" && tkStatus != f.TKStatus {
					continue
				}
				if f.CloneStatus != "" && r.CloneStatus != f.CloneStatus {
					continue
				}
				if f.HasTestSuite != nil {
					if *f.HasTestSuite && !r.HasTestSuite {
						continue
					}
					if !*f.HasTestSuite && r.HasTestSuite {
						continue
					}
				}
				result = append(result, r)
			}
			total := len(result)
			// Apply offset/limit.
			if f.Offset > 0 && f.Offset < len(result) {
				result = result[f.Offset:]
			} else if f.Offset >= len(result) {
				result = nil
			}
			if f.Limit > 0 && f.Limit < len(result) {
				result = result[:f.Limit]
			}
			return result, total, nil
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
// Tests: Compatibility is returned correctly from materialised column
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

func TestHandleGitRepos_CookstyleStatusInResponse(t *testing.T) {
	store := defaultGitRepoMockStore()
	r := newGitRepoTestRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	statusMap := make(map[string]string)
	for _, d := range resp.Data {
		statusMap[d.Name] = d.CookstyleStatus
	}
	want := map[string]string{
		"cookbook-a": "ready",
		"cookbook-b": "blocked",
		"cookbook-c": "needs_review",
		"cookbook-d": "untested",
	}
	for name, expected := range want {
		if statusMap[name] != expected {
			t.Errorf("cookstyle_status for %q: expected %q, got %q", name, expected, statusMap[name])
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: Empty repos list returns empty data
// ---------------------------------------------------------------------------

func TestHandleGitRepos_EmptyList(t *testing.T) {
	store := &mockStore{
		ListGitReposFilteredFn: func(ctx context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			return nil, 0, nil
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

	if tkStatusMap["cookbook-a"] != "passed" {
		t.Errorf("cookbook-a: expected tk_status=passed, got %q", tkStatusMap["cookbook-a"])
	}
	if tkStatusMap["cookbook-b"] != "partial" {
		t.Errorf("cookbook-b: expected tk_status=partial, got %q", tkStatusMap["cookbook-b"])
	}
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

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?has_test_suite=yes,no", nil)
	r.ServeHTTP(w, req)

	resp := decodeGitRepoListResponse(t, w)

	if len(resp.Data) != 4 {
		t.Fatalf("expected all 4 repos when both selected, got %d", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// The failure register on the git repo list
//
// The materialised cookstyle and tk columns report what each tool said and are
// deliberately not rewritten when a person overrules them. Without the
// standing verdict on the row, the list would go on showing a repo as blocked
// that the register says is fine — the two views contradicting each other in
// public, which is the credibility problem the register exists to prevent.
// ---------------------------------------------------------------------------

func TestGitRepos_MarksARepoAPersonHasOverruled(t *testing.T) {
	store := &mockStore{
		ListGitReposFilteredFn: func(_ context.Context, _ datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			return []datastore.GitRepo{
				{Name: "acme-apache", GitRepoURL: "https://git.example.com/acme-apache.git", CookstyleStatus: "blocked"},
				{Name: "acme-nginx", GitRepoURL: "https://git.example.com/acme-nginx.git", CookstyleStatus: "ready"},
			}, 2, nil
		},
		ListOpenFailureVerdictsFn: func(_ context.Context) (map[string]datastore.StandingVerdict, error) {
			return map[string]datastore.StandingVerdict{
				"acme-apache": {
					SubjectName: "acme-apache", CookbookName: "apache",
					Verdict: datastore.VerdictNotBroken,
					Reason:  "kitchen never converged; this runs on 4000 nodes",
				},
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			Name            string `json:"name"`
			CookstyleStatus string `json:"cookstyle_status"`
			HumanVerdict    string `json:"human_verdict,omitempty"`
			HumanReason     string `json:"human_verdict_reason,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d rows, want 2", len(resp.Data))
	}

	byName := map[string]string{}
	reasons := map[string]string{}
	statuses := map[string]string{}
	for _, row := range resp.Data {
		byName[row.Name] = row.HumanVerdict
		reasons[row.Name] = row.HumanReason
		statuses[row.Name] = row.CookstyleStatus
	}

	if byName["acme-apache"] != "not_broken" {
		t.Errorf("acme-apache human verdict = %q, want not_broken", byName["acme-apache"])
	}
	if reasons["acme-apache"] == "" {
		t.Error("the reason did not reach the row; the marker is unreadable without it")
	}
	// The scan's own verdict is retained exactly as it was — the register
	// overrules it, it does not rewrite it.
	if statuses["acme-apache"] != "blocked" {
		t.Errorf("cookstyle status = %q, want it left as blocked", statuses["acme-apache"])
	}
	// A repo nobody has an opinion about carries no marker.
	if byName["acme-nginx"] != "" {
		t.Errorf("acme-nginx carries a verdict %q; nobody recorded one", byName["acme-nginx"])
	}
}

// The register failing must not take the repo list with it.
func TestGitRepos_SurvivesTheRegisterBeingUnreadable(t *testing.T) {
	store := &mockStore{
		ListGitReposFilteredFn: func(_ context.Context, _ datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			return []datastore.GitRepo{{Name: "acme-apache", CookstyleStatus: "blocked"}}, 1, nil
		},
		ListOpenFailureVerdictsFn: func(_ context.Context) (map[string]datastore.StandingVerdict, error) {
			return nil, context.DeadlineExceeded
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — the list is still worth having: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "acme-apache") {
		t.Error("the list was dropped because the register was unreadable")
	}
}

// The filter reaches the datastore, so pagination counts the filtered set
// rather than the page being filtered after the fact.
func TestGitRepos_HumanVerdictFilterReachesTheStore(t *testing.T) {
	var got datastore.GitRepoFilter
	store := &mockStore{
		ListGitReposFilteredFn: func(_ context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			got = f
			return nil, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/api/v1/git-repos?human_verdict=not_broken&cookstyle_status=blocked", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got.HumanVerdict != "not_broken" {
		t.Errorf("human verdict filter = %q, want not_broken", got.HumanVerdict)
	}
	// It composes rather than replacing — this pair is the false-positive list.
	if got.CookstyleStatus != "blocked" {
		t.Errorf("the cookstyle filter was lost: %+v", got)
	}
}
