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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// filterCookbooks tests
// ---------------------------------------------------------------------------

func TestFilterCookbooks_NoFilters(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", Source: "chef_server", IsActive: true},
		{Name: "nginx", Source: "git", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(result))
	}
}

func TestFilterCookbooks_ByActiveTrue(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
		{Name: "mysql", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 active cookbooks, got %d", len(result))
	}
	for _, cb := range result {
		if !cb.IsActive {
			t.Errorf("expected IsActive=true for %q", cb.Name)
		}
	}
}

func TestFilterCookbooks_ByActiveFalse(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=false", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 inactive cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_ByName(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", Source: "chef_server"},
		{Name: "nginx", Source: "git"},
		{Name: "apt", Source: "git"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apt", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks named apt, got %d", len(result))
	}
	for _, cb := range result {
		if cb.Name != "apt" {
			t.Errorf("expected name=apt, got %q", cb.Name)
		}
	}
}

func TestFilterCookbooks_ByNamePartialMatch(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apache2"},
		{Name: "apt"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=ap", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks matching 'ap', got %d", len(result))
	}
}

func TestFilterCookbooks_ByNameCaseInsensitive(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "Apache2"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apache", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook matching 'apache', got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "Apache2" {
		t.Errorf("expected Apache2, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_MultipleFilters(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", IsActive: true},
		{Name: "apt", IsActive: false},
		{Name: "nginx", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true&name=apt", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook, got %d", len(result))
	}
	if len(result) > 0 {
		cb := result[0]
		if cb.Name != "apt" || !cb.IsActive {
			t.Errorf("unexpected cookbook: name=%q active=%v", cb.Name, cb.IsActive)
		}
	}
}

func TestFilterCookbooks_EmptyInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true", nil)
	result := filterCookbookSummaries(req, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(result))
	}
}

func TestFilterCookbooks_NoMatch(t *testing.T) {
	cookbooks := []cookbookSummary{
		{Name: "apt", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=false", nil)
	result := filterCookbookSummaries(req, cookbooks)
	if len(result) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// collapseCookbooks tests
// ---------------------------------------------------------------------------

func TestCollapseCookbooks_MultipleVersions(t *testing.T) {
	cookbooks := []cookbookSummary{
		{ID: "cb-1", Name: "apt", Version: "1.0.0", Source: "chef_server"},
		{ID: "cb-2", Name: "apt", Version: "2.0.0", Source: "chef_server"},
		{ID: "cb-3", Name: "apt", Version: "3.0.0", Source: "chef_server"},
		{ID: "cb-4", Name: "nginx", Version: "1.0.0", Source: "chef_server"},
	}
	collapsed, counts := collapseCookbookSummaries(cookbooks)
	if len(collapsed) != 2 {
		t.Fatalf("expected 2 collapsed cookbooks, got %d", len(collapsed))
	}
	if collapsed[0].Name != "apt" || collapsed[0].ID != "cb-1" {
		t.Errorf("first entry: name=%q id=%q, want apt/cb-1", collapsed[0].Name, collapsed[0].ID)
	}
	if collapsed[1].Name != "nginx" || collapsed[1].ID != "cb-4" {
		t.Errorf("second entry: name=%q id=%q, want nginx/cb-4", collapsed[1].Name, collapsed[1].ID)
	}
	if counts["apt"] != 3 {
		t.Errorf("apt version count = %d, want 3", counts["apt"])
	}
	if counts["nginx"] != 1 {
		t.Errorf("nginx version count = %d, want 1", counts["nginx"])
	}
}

func TestCollapseCookbooks_GitAndChefServerMerged(t *testing.T) {
	// Git entry comes first so it becomes the representative.
	cookbooks := []cookbookSummary{
		{ID: "cb-1", Name: "myapp", Version: "1.0.0", Source: "git"},
		{ID: "cb-2", Name: "myapp", Version: "2.0.0", Source: "chef_server"},
		{ID: "cb-3", Name: "myapp", Version: "3.0.0", Source: "chef_server"},
	}
	collapsed, counts := collapseCookbookSummaries(cookbooks)
	// All three entries of myapp collapse into one row.
	if len(collapsed) != 1 {
		t.Fatalf("expected 1 collapsed cookbook, got %d", len(collapsed))
	}
	if collapsed[0].Source != "git" || collapsed[0].ID != "cb-1" {
		t.Errorf("representative: source=%q id=%q, want git/cb-1", collapsed[0].Source, collapsed[0].ID)
	}
	// Version count only includes chef_server entries (2), not the git entry.
	if counts["myapp"] != 2 {
		t.Errorf("myapp version count = %d, want 2", counts["myapp"])
	}
}

func TestCollapseCookbooks_Empty(t *testing.T) {
	collapsed, counts := collapseCookbookSummaries(nil)
	if len(collapsed) != 0 {
		t.Errorf("expected 0 collapsed cookbooks, got %d", len(collapsed))
	}
	if len(counts) != 0 {
		t.Errorf("expected 0 counts, got %d", len(counts))
	}
}

func TestCollapseCookbooks_AllGit(t *testing.T) {
	cookbooks := []cookbookSummary{
		{ID: "cb-1", Name: "app1", Source: "git"},
		{ID: "cb-2", Name: "app2", Source: "git"},
	}
	collapsed, counts := collapseCookbookSummaries(cookbooks)
	if len(collapsed) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(collapsed))
	}
	// Git-only repos have no server cookbook versions, so count is 0.
	if counts["app1"] != 0 {
		t.Errorf("app1 version count = %d, want 0", counts["app1"])
	}
	if counts["app2"] != 0 {
		t.Errorf("app2 version count = %d, want 0", counts["app2"])
	}
}

// ---------------------------------------------------------------------------
// Route wiring tests — verify method checks and 404s
// ---------------------------------------------------------------------------

func TestHandleCookbooks_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/cookbooks status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/v1/cookbooks/apt status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookDetail_MissingName(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/cookbooks/ status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleCookbooks — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleCookbooks_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", body.Pagination.TotalItems)
	}
}

func TestHandleCookbooks_HappyPath_WithCookbooks(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "apt", IsActive: true, DownloadStatus: "ok"},
			}, nil
		},
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "cb-2", Name: "nginx", HasTestSuite: false},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", body.Pagination.TotalItems)
	}
}

func TestHandleCookbooks_HappyPath_VersionCountCollapsed(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0", IsActive: true},
				{ID: "cb-2", Name: "apt", Version: "2.0.0", IsActive: true},
				{ID: "cb-3", Name: "apt", Version: "3.0.0", IsActive: true},
				{ID: "cb-4", Name: "nginx", Version: "1.0.0", IsActive: true},
			}, nil
		},
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "cb-5", Name: "myapp"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			VersionCount int    `json:"version_count"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 3 chef_server versions of apt + 1 nginx + 1 git myapp = 3 unique names.
	if body.Pagination.TotalItems != 3 {
		t.Fatalf("total_items = %d, want 3", body.Pagination.TotalItems)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}

	// Check version counts by name.
	counts := make(map[string]int)
	for _, cb := range body.Data {
		counts[cb.Name] = cb.VersionCount
	}
	if counts["apt"] != 3 {
		t.Errorf("apt version_count = %d, want 3", counts["apt"])
	}
	if counts["nginx"] != 1 {
		t.Errorf("nginx version_count = %d, want 1", counts["nginx"])
	}
	// myapp is git-only, so server cookbook version count is 0.
	if counts["myapp"] != 0 {
		t.Errorf("myapp version_count = %d, want 0", counts["myapp"])
	}
}

func TestHandleCookbooks_HappyPath_GitAndChefServerMerged(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-2", Name: "myapp", Version: "1.0.0", IsActive: true},
				{ID: "cb-3", Name: "myapp", Version: "2.0.0", IsActive: true},
			}, nil
		},
		ListGitReposFn: func(ctx context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "cb-1", Name: "myapp"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			VersionCount int    `json:"version_count"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// All entries (1 git + 2 chef_server) of myapp collapse to 1 row.
	if body.Pagination.TotalItems != 1 {
		t.Fatalf("total_items = %d, want 1", body.Pagination.TotalItems)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	// The git entry (cb-1) should be the representative since it's first.
	if body.Data[0].ID != "cb-1" {
		t.Errorf("representative id = %q, want cb-1 (git preferred)", body.Data[0].ID)
	}
	// Version count only includes chef_server entries (2), not the git entry.
	if body.Data[0].VersionCount != 2 {
		t.Errorf("myapp version_count = %d, want 2", body.Data[0].VersionCount)
	}
}

// ---------------------------------------------------------------------------
// handleCookbooks — DB errors
// ---------------------------------------------------------------------------

func TestHandleCookbooks_DBError_ListOrganisations(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookDetail — happy path, not-found, DB error
// ---------------------------------------------------------------------------

func TestHandleCookbookDetail_HappyPath(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			if name == "apt" {
				return []datastore.ServerCookbook{
					{ID: "cb-1", Name: "apt", Version: "7.4.0"},
				}, nil
			}
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Name            string `json:"name"`
		ServerCookbooks []struct {
			Cookbook struct {
				Name string `json:"name"`
			} `json:"cookbook"`
		} `json:"server_cookbooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Name != "apt" {
		t.Errorf("name = %q, want %q", body.Name, "apt")
	}
	if len(body.ServerCookbooks) != 1 {
		t.Errorf("len(server_cookbooks) = %d, want 1", len(body.ServerCookbooks))
	}
}

func TestHandleCookbookDetail_NotFound(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nope", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCookbookDetail_GitBeforeChefServer(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "myapp", Version: "1.0.0"},
				{ID: "cb-2", Name: "myapp", Version: "2.0.0"},
			}, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "cb-3", Name: "myapp"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/myapp", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		ServerCookbooks []struct {
			Cookbook struct {
				ID string `json:"id"`
			} `json:"cookbook"`
		} `json:"server_cookbooks"`
		GitRepos []struct {
			GitRepo struct {
				ID string `json:"id"`
			} `json:"git_repo"`
		} `json:"git_repos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Git repos should be present.
	if len(body.GitRepos) != 1 {
		t.Fatalf("len(git_repos) = %d, want 1", len(body.GitRepos))
	}
	if body.GitRepos[0].GitRepo.ID != "cb-3" {
		t.Errorf("git_repos[0].id = %q, want %q", body.GitRepos[0].GitRepo.ID, "cb-3")
	}
	// Server cookbooks should follow.
	if len(body.ServerCookbooks) != 2 {
		t.Fatalf("len(server_cookbooks) = %d, want 2", len(body.ServerCookbooks))
	}
	if body.ServerCookbooks[0].Cookbook.ID != "cb-1" {
		t.Errorf("server_cookbooks[0].id = %q, want %q", body.ServerCookbooks[0].Cookbook.ID, "cb-1")
	}
	if body.ServerCookbooks[1].Cookbook.ID != "cb-2" {
		t.Errorf("server_cookbooks[1].id = %q, want %q", body.ServerCookbooks[1].Cookbook.ID, "cb-2")
	}
}

func TestHandleCookbookDetail_DBError(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, errors.New("disk I/O error")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
