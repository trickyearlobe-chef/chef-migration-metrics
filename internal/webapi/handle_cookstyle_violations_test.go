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
// Method checks
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookstyle/violations?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Missing target_chef_version
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_NoTargetVersion(t *testing.T) {
	store := &mockStore{}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	// No TargetChefVersions configured

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// Server source — basic paginated response
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_ServerSource_Basic(t *testing.T) {
	offences := mustMarshal(t, []map[string]any{
		{
			"path": "recipes/default.rb",
			"offenses": []map[string]any{
				{
					"cop_name":    "Chef/Deprecations/NodeSet",
					"severity":    "warning",
					"message":     "Do not use node.set",
					"correctable": true,
				},
				{
					"cop_name":    "Chef/Correctness/InvalidDefaultAction",
					"severity":    "error",
					"message":     "Invalid default action",
					"correctable": false,
				},
			},
		},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			if tv != "18.0" {
				t.Errorf("target version = %q, want 18.0", tv)
			}
			return []datastore.ServerCookbookCookstyleResult{
				{
					OrganisationName:  "example-org",
					CookbookName:      "test-cookbook",
					CookbookVersion:   "1.0.0",
					TargetChefVersion: "18.0",
					Passed:            false,
					OffenceCount:      2,
					DeprecationCount:  1,
					CorrectnessCount:  1,
					Offences:          offences,
					ScannedAt:         time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&source=server", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1", resp.Pagination.TotalItems)
	}

	items, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", resp.Data)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	item := items[0].(map[string]any)
	if item["name"] != "test-cookbook" {
		t.Errorf("name = %v, want test-cookbook", item["name"])
	}
	if item["source"] != "server" {
		t.Errorf("source = %v, want server", item["source"])
	}
	if item["organisation"] != "example-org" {
		t.Errorf("organisation = %v, want example-org", item["organisation"])
	}

	// Check derived namespace_counts
	nsCounts, ok := item["namespace_counts"].(map[string]any)
	if !ok {
		t.Fatalf("namespace_counts not a map: %T", item["namespace_counts"])
	}
	if nsCounts["Chef/Deprecations/"] != float64(1) {
		t.Errorf("namespace_counts[Chef/Deprecations/] = %v, want 1", nsCounts["Chef/Deprecations/"])
	}
	if nsCounts["Chef/Correctness/"] != float64(1) {
		t.Errorf("namespace_counts[Chef/Correctness/] = %v, want 1", nsCounts["Chef/Correctness/"])
	}

	// Check derived severity_counts
	sevCounts, ok := item["severity_counts"].(map[string]any)
	if !ok {
		t.Fatalf("severity_counts not a map: %T", item["severity_counts"])
	}
	if sevCounts["warning"] != float64(1) {
		t.Errorf("severity_counts[warning] = %v, want 1", sevCounts["warning"])
	}
	if sevCounts["error"] != float64(1) {
		t.Errorf("severity_counts[error] = %v, want 1", sevCounts["error"])
	}

	// Check top_cops
	topCops, ok := item["top_cops"].([]any)
	if !ok {
		t.Fatalf("top_cops not an array: %T", item["top_cops"])
	}
	if len(topCops) != 2 {
		t.Errorf("len(top_cops) = %d, want 2", len(topCops))
	}
}

// ---------------------------------------------------------------------------
// Git source
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_GitSource(t *testing.T) {
	store := &mockStore{
		ListGitRepoCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.GitRepoCookstyleResult, error) {
			return []datastore.GitRepoCookstyleResult{
				{
					GitRepoName:       "git-cookbook",
					GitRepoURL:        "https://example.com/git-cookbook.git",
					TargetChefVersion: "18.0",
					Passed:            true,
					OffenceCount:      0,
					ScannedAt:         time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&source=git", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	items := resp.Data.([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	item := items[0].(map[string]any)
	if item["source"] != "git" {
		t.Errorf("source = %v, want git", item["source"])
	}
	if item["name"] != "git-cookbook" {
		t.Errorf("name = %v, want git-cookbook", item["name"])
	}
}

// ---------------------------------------------------------------------------
// Status filter
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_StatusFilter(t *testing.T) {
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{CookbookName: "passed-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: true, ScannedAt: time.Now()},
				{CookbookName: "failed-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, ScannedAt: time.Now()},
				{CookbookName: "error-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, ErrorMessage: "scan failed", ScannedAt: time.Now()},
			}, nil
		},
	}

	tests := []struct {
		status    string
		wantCount int
		wantName  string
	}{
		{"failed", 1, "failed-cb"},
		{"passed", 1, "passed-cb"},
		{"error", 1, "error-cb"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cfg := testConfigWithTargetVersions("18.0")
			r := newTestRouterWithMockAndConfig(store, cfg)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&status="+tt.status, nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
			}

			var resp PaginatedResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			items := resp.Data.([]any)
			if len(items) != tt.wantCount {
				t.Errorf("len(items) = %d, want %d", len(items), tt.wantCount)
			}
			if tt.wantCount > 0 {
				item := items[0].(map[string]any)
				if item["name"] != tt.wantName {
					t.Errorf("name = %v, want %s", item["name"], tt.wantName)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Namespace filter
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_NamespaceFilter(t *testing.T) {
	offencesWithDeprecation := mustMarshal(t, []map[string]any{
		{"path": "recipes/default.rb", "offenses": []map[string]any{
			{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning"},
		}},
	})
	offencesWithCorrectness := mustMarshal(t, []map[string]any{
		{"path": "recipes/default.rb", "offenses": []map[string]any{
			{"cop_name": "Chef/Correctness/InvalidAction", "severity": "error"},
		}},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{CookbookName: "dep-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 1, Offences: offencesWithDeprecation, ScannedAt: time.Now()},
				{CookbookName: "corr-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 1, Offences: offencesWithCorrectness, ScannedAt: time.Now()},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&namespace=Chef/Deprecations/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	items := resp.Data.([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].(map[string]any)["name"] != "dep-cb" {
		t.Errorf("expected dep-cb, got %v", items[0].(map[string]any)["name"])
	}
}

// ---------------------------------------------------------------------------
// Cop filter
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_CopFilter(t *testing.T) {
	offences := mustMarshal(t, []map[string]any{
		{"path": "recipes/default.rb", "offenses": []map[string]any{
			{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning"},
			{"cop_name": "Chef/Correctness/InvalidAction", "severity": "error"},
		}},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{CookbookName: "mixed-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 2, Offences: offences, ScannedAt: time.Now()},
				{CookbookName: "empty-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: true, OffenceCount: 0, ScannedAt: time.Now()},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&cop=Chef/Deprecations/NodeSet", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	items := resp.Data.([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].(map[string]any)["name"] != "mixed-cb" {
		t.Errorf("expected mixed-cb")
	}
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_Pagination(t *testing.T) {
	results := make([]datastore.ServerCookbookCookstyleResult, 5)
	for i := range results {
		results[i] = datastore.ServerCookbookCookstyleResult{
			CookbookName:      "cb-" + string(rune('a'+i)),
			CookbookVersion:   "1.0.0",
			TargetChefVersion: "18.0",
			Passed:            true,
			ScannedAt:         time.Now(),
		}
	}

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return results, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&page=2&per_page=2", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Pagination.TotalItems != 5 {
		t.Errorf("total_items = %d, want 5", resp.Pagination.TotalItems)
	}
	if resp.Pagination.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Pagination.Page)
	}
	if resp.Pagination.PerPage != 2 {
		t.Errorf("per_page = %d, want 2", resp.Pagination.PerPage)
	}

	items := resp.Data.([]any)
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

// ---------------------------------------------------------------------------
// Sort by offence_count desc
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_SortByOffenceCount(t *testing.T) {
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{CookbookName: "low-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 1, ScannedAt: time.Now()},
				{CookbookName: "high-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 10, ScannedAt: time.Now()},
				{CookbookName: "mid-cb", CookbookVersion: "1.0.0", TargetChefVersion: "18.0", Passed: false, OffenceCount: 5, ScannedAt: time.Now()},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0&sort=offence_count&order=desc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	items := resp.Data.([]any)
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}

	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.(map[string]any)["name"].(string)
	}

	if names[0] != "high-cb" || names[1] != "mid-cb" || names[2] != "low-cb" {
		t.Errorf("sort order = %v, want [high-cb, mid-cb, low-cb]", names)
	}
}

// ---------------------------------------------------------------------------
// Empty result set
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_EmptyResult(t *testing.T) {
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return nil, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Pagination.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", resp.Pagination.TotalItems)
	}

	items := resp.Data.([]any)
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

// ---------------------------------------------------------------------------
// Default target version from config
// ---------------------------------------------------------------------------

func TestHandleCookstyleViolations_DefaultTargetVersion(t *testing.T) {
	var capturedVersion string
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(ctx context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			capturedVersion = tv
			return nil, nil
		},
	}

	cfg := testConfigWithTargetVersions("19.1.164")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	// No target_chef_version param — should use default from config
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/violations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedVersion != "19.1.164" {
		t.Errorf("target version = %q, want 19.1.164", capturedVersion)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func testConfigWithTargetVersions(versions ...string) *config.Config {
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = versions
	return cfg
}
