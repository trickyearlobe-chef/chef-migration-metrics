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
