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
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server", IsActive: true},
		{Name: "nginx", Source: "git", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(result))
	}
}

func TestFilterCookbooks_BySource(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server"},
		{Name: "nginx", Source: "git"},
		{Name: "mysql", Source: "chef_server"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=chef_server", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(result))
	}
	for _, cb := range result {
		if cb.Source != "chef_server" {
			t.Errorf("expected source=chef_server, got %q", cb.Source)
		}
	}
}

func TestFilterCookbooks_BySourceGit(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server"},
		{Name: "nginx", Source: "git"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=git", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_ByActiveTrue(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
		{Name: "mysql", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true", nil)
	result := filterCookbooks(req, cookbooks)
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
	cookbooks := []datastore.Cookbook{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=false", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 inactive cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_ByName(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server"},
		{Name: "nginx", Source: "git"},
		{Name: "apt", Source: "git"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apt", nil)
	result := filterCookbooks(req, cookbooks)
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
	cookbooks := []datastore.Cookbook{
		{Name: "apache2"},
		{Name: "apt"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=ap", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks matching 'ap', got %d", len(result))
	}
}

func TestFilterCookbooks_ByNameCaseInsensitive(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "Apache2"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apache", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook matching 'apache', got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "Apache2" {
		t.Errorf("expected Apache2, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_MultipleFilters(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server", IsActive: true},
		{Name: "apt", Source: "git", IsActive: true},
		{Name: "apt", Source: "chef_server", IsActive: false},
		{Name: "nginx", Source: "chef_server", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=chef_server&active=true&name=apt", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook, got %d", len(result))
	}
	if len(result) > 0 {
		cb := result[0]
		if cb.Name != "apt" || cb.Source != "chef_server" || !cb.IsActive {
			t.Errorf("unexpected cookbook: name=%q source=%q active=%v", cb.Name, cb.Source, cb.IsActive)
		}
	}
}

func TestFilterCookbooks_EmptyInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=git", nil)
	result := filterCookbooks(req, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(result))
	}
}

func TestFilterCookbooks_NoMatch(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{Name: "apt", Source: "chef_server"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=git", nil)
	result := filterCookbooks(req, cookbooks)
	if len(result) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// collapseChefServerCookbooks tests
// ---------------------------------------------------------------------------

func TestCollapseChefServerCookbooks_MultipleVersions(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{ID: "cb-1", Name: "apt", Version: "1.0.0", Source: "chef_server"},
		{ID: "cb-2", Name: "apt", Version: "2.0.0", Source: "chef_server"},
		{ID: "cb-3", Name: "apt", Version: "3.0.0", Source: "chef_server"},
		{ID: "cb-4", Name: "nginx", Version: "1.0.0", Source: "chef_server"},
	}
	collapsed, counts := collapseChefServerCookbooks(cookbooks)
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

func TestCollapseChefServerCookbooks_GitPassedThrough(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{ID: "cb-1", Name: "myapp", Version: "1.0.0", Source: "git"},
		{ID: "cb-2", Name: "myapp", Version: "2.0.0", Source: "chef_server"},
		{ID: "cb-3", Name: "myapp", Version: "3.0.0", Source: "chef_server"},
	}
	collapsed, counts := collapseChefServerCookbooks(cookbooks)
	// Git cookbook passes through unchanged; chef_server versions collapse to one.
	if len(collapsed) != 2 {
		t.Fatalf("expected 2 collapsed cookbooks, got %d", len(collapsed))
	}
	if collapsed[0].Source != "git" {
		t.Errorf("first entry source = %q, want git", collapsed[0].Source)
	}
	if collapsed[1].Source != "chef_server" || collapsed[1].ID != "cb-2" {
		t.Errorf("second entry: source=%q id=%q, want chef_server/cb-2", collapsed[1].Source, collapsed[1].ID)
	}
	if counts["myapp"] != 2 {
		t.Errorf("myapp version count = %d, want 2", counts["myapp"])
	}
}

func TestCollapseChefServerCookbooks_Empty(t *testing.T) {
	collapsed, counts := collapseChefServerCookbooks(nil)
	if len(collapsed) != 0 {
		t.Errorf("expected 0 collapsed cookbooks, got %d", len(collapsed))
	}
	if len(counts) != 0 {
		t.Errorf("expected 0 counts, got %d", len(counts))
	}
}

func TestCollapseChefServerCookbooks_AllGit(t *testing.T) {
	cookbooks := []datastore.Cookbook{
		{ID: "cb-1", Name: "app1", Source: "git"},
		{ID: "cb-2", Name: "app2", Source: "git"},
	}
	collapsed, counts := collapseChefServerCookbooks(cookbooks)
	if len(collapsed) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(collapsed))
	}
	if len(counts) != 0 {
		t.Errorf("expected 0 version counts for all-git, got %d", len(counts))
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
		ListCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Source: "chef_server", IsActive: true, DownloadStatus: "ok"},
			}, nil
		},
		ListGitCookbooksFn: func(ctx context.Context) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-2", Name: "nginx", Source: "git", IsActive: true, DownloadStatus: "ok"},
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
		ListCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0", Source: "chef_server", IsActive: true},
				{ID: "cb-2", Name: "apt", Version: "2.0.0", Source: "chef_server", IsActive: true},
				{ID: "cb-3", Name: "apt", Version: "3.0.0", Source: "chef_server", IsActive: true},
				{ID: "cb-4", Name: "nginx", Version: "1.0.0", Source: "chef_server", IsActive: true},
			}, nil
		},
		ListGitCookbooksFn: func(ctx context.Context) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-5", Name: "myapp", Version: "0.1.0", Source: "git", IsActive: true},
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
			Version      string `json:"version"`
			VersionCount int    `json:"version_count"`
			Source       string `json:"source"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 4 chef_server versions of apt collapse to 1 + 1 nginx + 1 git myapp = 3 rows.
	if body.Pagination.TotalItems != 3 {
		t.Fatalf("total_items = %d, want 3", body.Pagination.TotalItems)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}

	// Find the apt entry and check version_count.
	found := false
	for _, cb := range body.Data {
		if cb.Name == "apt" && cb.Source == "chef_server" {
			found = true
			if cb.VersionCount != 3 {
				t.Errorf("apt version_count = %d, want 3", cb.VersionCount)
			}
		}
		if cb.Name == "nginx" && cb.Source == "chef_server" {
			if cb.VersionCount != 1 {
				t.Errorf("nginx version_count = %d, want 1", cb.VersionCount)
			}
		}
		// Git cookbooks should have version_count = 1.
		if cb.Source == "git" && cb.VersionCount != 1 {
			t.Errorf("git cookbook %q has version_count = %d, want 1", cb.Name, cb.VersionCount)
		}
	}
	if !found {
		t.Error("did not find collapsed apt entry in response")
	}
}

func TestHandleCookbooks_HappyPath_FilterBySource(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Source: "chef_server", IsActive: true},
			}, nil
		},
		ListGitCookbooksFn: func(ctx context.Context) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-2", Name: "nginx", Source: "git", IsActive: true},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?source=git", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (only git)", body.Pagination.TotalItems)
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
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			if name == "apt" {
				return []datastore.Cookbook{
					{ID: "cb-1", Name: "apt", Version: "7.4.0", Source: "chef_server"},
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
		Name string `json:"name"`
		Data []struct {
			Cookbook struct {
				Name string `json:"name"`
			} `json:"cookbook"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Name != "apt" {
		t.Errorf("name = %q, want %q", body.Name, "apt")
	}
	if len(body.Data) != 1 {
		t.Errorf("len(data) = %d, want 1", len(body.Data))
	}
}

func TestHandleCookbookDetail_NotFound(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
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
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			// Return chef_server entries first to verify the handler re-sorts.
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "myapp", Version: "1.0.0", Source: "chef_server"},
				{ID: "cb-2", Name: "myapp", Version: "2.0.0", Source: "chef_server"},
				{ID: "cb-3", Name: "myapp", Version: "1.0.0", Source: "git"},
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
		Data []struct {
			Cookbook struct {
				ID     string `json:"id"`
				Source string `json:"source"`
			} `json:"cookbook"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}
	// The git-sourced cookbook (cb-3) must appear first.
	if body.Data[0].Cookbook.Source != "git" {
		t.Errorf("data[0].source = %q, want %q", body.Data[0].Cookbook.Source, "git")
	}
	if body.Data[0].Cookbook.ID != "cb-3" {
		t.Errorf("data[0].id = %q, want %q", body.Data[0].Cookbook.ID, "cb-3")
	}
	// The chef_server entries should follow in their original relative order.
	if body.Data[1].Cookbook.Source != "chef_server" {
		t.Errorf("data[1].source = %q, want %q", body.Data[1].Cookbook.Source, "chef_server")
	}
	if body.Data[1].Cookbook.ID != "cb-1" {
		t.Errorf("data[1].id = %q, want %q (stable sort preserves original order)", body.Data[1].Cookbook.ID, "cb-1")
	}
	if body.Data[2].Cookbook.Source != "chef_server" {
		t.Errorf("data[2].source = %q, want %q", body.Data[2].Cookbook.Source, "chef_server")
	}
	if body.Data[2].Cookbook.ID != "cb-2" {
		t.Errorf("data[2].id = %q, want %q (stable sort preserves original order)", body.Data[2].Cookbook.ID, "cb-2")
	}
}

func TestHandleCookbookDetail_DBError(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
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
