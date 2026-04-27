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
// filterCookbookRows tests
// ---------------------------------------------------------------------------

func TestFilterCookbooks_NoFilters(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(result))
	}
}

func TestFilterCookbooks_ByActiveTrue(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
		{Name: "mysql", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true", nil)
	result := filterCookbookRows(req, rows)
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
	rows := []cookbookRow{
		{Name: "apt", IsActive: true},
		{Name: "nginx", IsActive: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=false", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 1 {
		t.Errorf("expected 1 inactive cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_ByName(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", Version: "1.0.0"},
		{Name: "nginx", Version: "1.0.0"},
		{Name: "apt", Version: "2.0.0"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apt", nil)
	result := filterCookbookRows(req, rows)
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
	rows := []cookbookRow{
		{Name: "apache2"},
		{Name: "apt"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=ap", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 2 {
		t.Errorf("expected 2 cookbooks matching 'ap', got %d", len(result))
	}
}

func TestFilterCookbooks_ByNameCaseInsensitive(t *testing.T) {
	rows := []cookbookRow{
		{Name: "Apache2"},
		{Name: "nginx"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=apache", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "Apache2" {
		t.Errorf("expected Apache2, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_MultipleFilters(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", IsActive: true},
		{Name: "apt-mirror", IsActive: false},
		{Name: "nginx", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?active=true&name=apt", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 1 {
		t.Errorf("expected 1 cookbook, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "apt" {
		t.Errorf("expected apt, got %q", result[0].Name)
	}
}

func TestFilterCookbooks_EmptyInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	result := filterCookbookRows(req, nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestFilterCookbooks_NoMatch(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", IsActive: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?name=zzz", nil)
	result := filterCookbookRows(req, rows)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// sortCookbookRows tests
// ---------------------------------------------------------------------------

func TestSortCookbookRows_ByNameAsc(t *testing.T) {
	rows := []cookbookRow{
		{Name: "nginx", Version: "1.0.0"},
		{Name: "apt", Version: "2.0.0"},
		{Name: "apt", Version: "1.0.0"},
	}
	sortCookbookRows(rows, "name", "asc")
	if rows[0].Name != "apt" || rows[0].Version != "1.0.0" {
		t.Errorf("first = %s/%s, want apt/1.0.0", rows[0].Name, rows[0].Version)
	}
	if rows[1].Name != "apt" || rows[1].Version != "2.0.0" {
		t.Errorf("second = %s/%s, want apt/2.0.0", rows[1].Name, rows[1].Version)
	}
	if rows[2].Name != "nginx" {
		t.Errorf("third = %s, want nginx", rows[2].Name)
	}
}

func TestSortCookbookRows_ByNameDesc(t *testing.T) {
	rows := []cookbookRow{
		{Name: "apt", Version: "1.0.0"},
		{Name: "nginx", Version: "1.0.0"},
	}
	sortCookbookRows(rows, "name", "desc")
	if rows[0].Name != "nginx" {
		t.Errorf("first = %s, want nginx", rows[0].Name)
	}
}

func TestSortCookbookRows_ByDownloadStatus(t *testing.T) {
	rows := []cookbookRow{
		{Name: "a", DownloadStatus: "pending"},
		{Name: "b", DownloadStatus: "failed"},
		{Name: "c", DownloadStatus: "ok"},
	}
	sortCookbookRows(rows, "download_status", "asc")
	if rows[0].DownloadStatus != "failed" {
		t.Errorf("first = %s, want failed", rows[0].DownloadStatus)
	}
	if rows[1].DownloadStatus != "ok" {
		t.Errorf("second = %s, want ok", rows[1].DownloadStatus)
	}
	if rows[2].DownloadStatus != "pending" {
		t.Errorf("third = %s, want pending", rows[2].DownloadStatus)
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
		t.Errorf("POST /cookbooks status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /cookbooks/apt status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookDetail_MissingName(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleCookbooks — happy path
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

func TestHandleCookbooks_HappyPath_EachVersionIsARow(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "1.0.0", IsActive: true, DownloadStatus: "ok"},
				{OrganisationName: "prod", Name: "apt", Version: "2.0.0", IsActive: true, DownloadStatus: "pending"},
				{OrganisationName: "prod", Name: "nginx", Version: "1.0.0", IsActive: true, DownloadStatus: "ok"},
			}
			return rows, len(rows), nil
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
			Name           string `json:"name"`
			Version        string `json:"version"`
			DownloadStatus string `json:"download_status"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Each version is its own row — no collapsing.
	if body.Pagination.TotalItems != 3 {
		t.Fatalf("total_items = %d, want 3", body.Pagination.TotalItems)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}

	// Verify each row has its own version and download status.
	byVersion := make(map[string]string) // version → download_status
	for _, cb := range body.Data {
		byVersion[cb.Version] = cb.DownloadStatus
	}
	if byVersion["1.0.0"] != "ok" {
		t.Errorf("1.0.0 download_status = %q, want ok", byVersion["1.0.0"])
	}
	if byVersion["2.0.0"] != "pending" {
		t.Errorf("2.0.0 download_status = %q, want pending", byVersion["2.0.0"])
	}
}

func TestHandleCookbooks_HappyPath_MultiOrg_EachRowDistinct(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{Name: "prod"},
				{Name: "staging"},
			}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "7.2.0", IsActive: true, DownloadStatus: "ok"},
				{OrganisationName: "staging", Name: "apt", Version: "7.2.0", IsActive: true, DownloadStatus: "pending"},
			}
			return rows, len(rows), nil
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
			ID               string `json:"id"`
			Name             string `json:"name"`
			Version          string `json:"version"`
			OrganisationName string `json:"organisation_name"`
			DownloadStatus   string `json:"download_status"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Same cookbook name+version in two orgs → two distinct rows.
	if body.Pagination.TotalItems != 2 {
		t.Fatalf("total_items = %d, want 2", body.Pagination.TotalItems)
	}

	// Verify each row retains its own org and download status.
	statusByOrg := make(map[string]string)
	for _, cb := range body.Data {
		statusByOrg[cb.OrganisationName] = cb.DownloadStatus
	}
	if statusByOrg["prod"] != "ok" {
		t.Errorf("prod download_status = %q, want ok", statusByOrg["prod"])
	}
	if statusByOrg["staging"] != "pending" {
		t.Errorf("staging download_status = %q, want pending", statusByOrg["staging"])
	}
}

func TestHandleCookbooks_HappyPath_VersionFieldInResponse(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "7.2.0", IsActive: true},
			}
			return rows, len(rows), nil
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
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].Version != "7.2.0" {
		t.Errorf("version = %q, want 7.2.0", body.Data[0].Version)
	}
}

func TestHandleCookbooks_HappyPath_NoVersionCountField(t *testing.T) {
	// The response must not include the old version_count field.
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "1.0.0", IsActive: true},
				{OrganisationName: "prod", Name: "apt", Version: "2.0.0", IsActive: true},
			}
			return rows, len(rows), nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &items); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	for _, item := range items {
		if _, ok := item["version_count"]; ok {
			t.Error("response should not contain version_count field")
		}
	}
}

// ---------------------------------------------------------------------------
// handleCookbooks — download_status filter
// ---------------------------------------------------------------------------

func TestHandleCookbooks_FilterByDownloadStatus(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			all := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "1.0.0", IsActive: true, DownloadStatus: "ok"},
				{OrganisationName: "prod", Name: "nginx", Version: "1.0.0", IsActive: true, DownloadStatus: "pending"},
				{OrganisationName: "prod", Name: "mysql", Version: "1.0.0", IsActive: true, DownloadStatus: "failed"},
			}
			// Simulate SQL filtering by download_status.
			if f.DownloadStatus != "" {
				var filtered []datastore.CookbookFilterRow
				for _, r := range all {
					if r.DownloadStatus == f.DownloadStatus {
						filtered = append(filtered, r)
					}
				}
				return filtered, len(filtered), nil
			}
			return all, len(all), nil
		},
	}
	r := newTestRouterWithMock(store)

	tests := []struct {
		filter string
		want   int
	}{
		{"ok", 1},
		{"pending", 1},
		{"failed", 1},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?download_status="+tt.filter, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var body PaginatedResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.Pagination.TotalItems != tt.want {
			t.Errorf("download_status=%s: total_items = %d, want %d", tt.filter, body.Pagination.TotalItems, tt.want)
		}
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
			return []datastore.ServerCookbook{
				{Name: "apt", Version: "7.2.0"},
			}, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
		ListServerCookbookCookstyleResultsFn: func(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
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
		t.Errorf("name = %q, want apt", body.Name)
	}
	if len(body.ServerCookbooks) != 1 {
		t.Errorf("server_cookbooks count = %d, want 1", len(body.ServerCookbooks))
	}
}

func TestHandleCookbookDetail_NotFound(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCookbookDetail_GitBeforeChefServer(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{Name: "myapp", Version: "1.0.0"},
			}, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{Name: "myapp"},
			}, nil
		},
		ListServerCookbookCookstyleResultsFn: func(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return nil, nil
		},
		ListGitRepoCookstyleResultsFn: func(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoCookstyleResult, error) {
			return nil, nil
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
				Name string `json:"name"`
			} `json:"cookbook"`
		} `json:"server_cookbooks"`
		GitRepos []struct {
			GitRepo struct {
				Name string `json:"name"`
			} `json:"git_repo"`
		} `json:"git_repos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.ServerCookbooks) != 1 || body.ServerCookbooks[0].Cookbook.Name != "myapp" {
		t.Errorf("expected 1 server cookbook with name myapp")
	}
	if len(body.GitRepos) != 1 || body.GitRepos[0].GitRepo.Name != "myapp" {
		t.Errorf("expected 1 git repo with name myapp")
	}
}

// ---------------------------------------------------------------------------
// handleCookbooks — compatibility assignment
// ---------------------------------------------------------------------------

func TestHandleCookbooks_UnscannedCookbooks_ShowUntested(t *testing.T) {
	// When no cookstyle results exist, every cookbook must show as "untested".
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "1.0.0", IsActive: true, DownloadStatus: "ok", Compatibility: "untested"},
				{OrganisationName: "prod", Name: "nginx", Version: "1.0.0", IsActive: true, DownloadStatus: "pending", Compatibility: "untested"},
			}
			return rows, len(rows), nil
		},
	}

	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Data []struct {
			Name          string `json:"name"`
			Compatibility string `json:"compatibility"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, cb := range body.Data {
		if cb.Compatibility != "untested" {
			t.Errorf("cookbook %q compatibility = %q, want %q (no scan results exist)",
				cb.Name, cb.Compatibility, "untested")
		}
	}
}

func TestHandleCookbooks_ScannedCookbooks_CompatibilityPerID(t *testing.T) {
	// Compatibility is now per cookbook ID (version), not per name. Each
	// version gets its own compatibility status from its cookstyle result.
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListCookbooksFilteredFn: func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
			rows := []datastore.CookbookFilterRow{
				{OrganisationName: "prod", Name: "apt", Version: "1.0.0", IsActive: true, DownloadStatus: "ok", Compatibility: "compatible"},
				{OrganisationName: "prod", Name: "apt", Version: "2.0.0", IsActive: true, DownloadStatus: "ok", Compatibility: "incompatible"},
				{OrganisationName: "prod", Name: "nginx", Version: "1.0.0", IsActive: true, DownloadStatus: "ok", Compatibility: "incompatible"},
				{OrganisationName: "prod", Name: "mysql", Version: "1.0.0", IsActive: true, DownloadStatus: "pending", Compatibility: "untested"},
			}
			return rows, len(rows), nil
		},
	}

	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Data []struct {
			Name          string `json:"name"`
			Version       string `json:"version"`
			Compatibility string `json:"compatibility"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]string{
		"apt:1.0.0":   "compatible",   // apt 1.0.0 passed
		"apt:2.0.0":   "incompatible", // apt 2.0.0 failed
		"nginx:1.0.0": "incompatible", // nginx failed
		"mysql:1.0.0": "untested",     // mysql no result
	}
	for _, cb := range body.Data {
		key := cb.Name + ":" + cb.Version
		expected, ok := want[key]
		if !ok {
			continue
		}
		if cb.Compatibility != expected {
			t.Errorf("cookbook %q version %s compatibility = %q, want %q",
				cb.Name, cb.Version, cb.Compatibility, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// handleCookbookDetail — DB error
// ---------------------------------------------------------------------------

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
