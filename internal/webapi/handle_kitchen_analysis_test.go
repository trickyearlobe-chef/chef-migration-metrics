// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func TestHandleKitchenAnalysisSummary(t *testing.T) {
	mock := &mockStore{
		GetKitchenAnalysisSummaryFn: func(ctx context.Context) (*datastore.KitchenAnalysisSummary, error) {
			return &datastore.KitchenAnalysisSummary{
				TotalScanned:           42,
				TotalWithoutKitchen:    5,
				TotalWithLocalOverride: 3,
				TotalWithConflicts:     1,
				DriverCounts:           map[string]int{"vagrant": 20, "dokken": 17},
				TransportCounts:        map[string]int{"ssh": 30, "winrm": 7},
				ProvisionerCounts:      map[string]int{"chef_zero": 37},
				PlatformCount:          12,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/summary", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp datastore.KitchenAnalysisSummary
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.TotalScanned != 42 {
		t.Errorf("expected total_scanned 42, got %d", resp.TotalScanned)
	}
	if resp.PlatformCount != 12 {
		t.Errorf("expected platform_count 12, got %d", resp.PlatformCount)
	}
}

func TestHandleKitchenAnalysisSummary_DBError(t *testing.T) {
	mock := &mockStore{
		GetKitchenAnalysisSummaryFn: func(ctx context.Context) (*datastore.KitchenAnalysisSummary, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/summary", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleKitchenAnalysisPlatforms(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		ListDiscoveredPlatformsFn: func(ctx context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{
					PlatformName:   "ubuntu-22.04",
					NormalisedName: "ubuntu-2204",
					OSFamily:       "debian",
					CookbookCount:  15,
					UpdatedAt:      now,
				},
				{
					PlatformName:   "centos-7",
					NormalisedName: "centos-7",
					OSFamily:       "rhel",
					CookbookCount:  10,
					UpdatedAt:      now,
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/platforms", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []datastore.KitchenDiscoveredPlatform
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(resp))
	}
	if resp[0].PlatformName != "ubuntu-22.04" {
		t.Errorf("expected first platform ubuntu-22.04, got %q", resp[0].PlatformName)
	}
}

func TestHandleKitchenAnalysisPlatforms_WithFilters(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		ListDiscoveredPlatformsFilteredFn: func(ctx context.Context, osFamily string, minCount int) ([]datastore.KitchenDiscoveredPlatform, error) {
			if osFamily != "rhel" {
				t.Errorf("expected os_family rhel, got %q", osFamily)
			}
			if minCount != 5 {
				t.Errorf("expected min_count 5, got %d", minCount)
			}
			return []datastore.KitchenDiscoveredPlatform{
				{
					PlatformName:   "centos-7",
					NormalisedName: "centos-7",
					OSFamily:       "rhel",
					CookbookCount:  10,
					UpdatedAt:      now,
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/platforms?os_family=rhel&min_count=5", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []datastore.KitchenDiscoveredPlatform
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(resp))
	}
}

func TestHandleKitchenAnalysisPlatforms_BadMinCount(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/platforms?min_count=abc", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleKitchenAnalysisPlatforms_NegativeMinCount(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/platforms?min_count=-1", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleKitchenAnalysisPlatforms_EmptyList(t *testing.T) {
	mock := &mockStore{
		ListDiscoveredPlatformsFn: func(ctx context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/platforms", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Must serialise as [] not null.
	body := rec.Body.String()
	if body == "null\n" || body == "null" {
		t.Error("expected [] not null for empty platform list")
	}
}

func TestHandleKitchenAnalysisCookbooks(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		ListKitchenAnalysisResultsFn: func(ctx context.Context) ([]datastore.KitchenAnalysisResult, error) {
			return []datastore.KitchenAnalysisResult{
				{
					GitRepoName:      "my-cookbook",
					GitRepoURL:       "https://example.com/my-cookbook.git",
					AnalysedAt:       now,
					HasLocalOverride: false,
					DriverName:       "vagrant",
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []datastore.KitchenAnalysisResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}
	if resp[0].GitRepoName != "my-cookbook" {
		t.Errorf("expected git_repo_name my-cookbook, got %q", resp[0].GitRepoName)
	}
}

func TestHandleKitchenAnalysisCookbooks_WithDriverFilter(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		ListKitchenAnalysisResultsFilteredFn: func(ctx context.Context, driverName string, hasLocalOverride *bool) ([]datastore.KitchenAnalysisResult, error) {
			if driverName != "vagrant" {
				t.Errorf("expected driver vagrant, got %q", driverName)
			}
			if hasLocalOverride != nil {
				t.Errorf("expected nil hasLocalOverride, got %v", *hasLocalOverride)
			}
			return []datastore.KitchenAnalysisResult{
				{
					GitRepoName: "vagrant-cookbook",
					DriverName:  "vagrant",
					AnalysedAt:  now,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks?driver=vagrant", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []datastore.KitchenAnalysisResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}
}

func TestHandleKitchenAnalysisCookbooks_WithHasLocalOverrideFilter(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		ListKitchenAnalysisResultsFilteredFn: func(ctx context.Context, driverName string, hasLocalOverride *bool) ([]datastore.KitchenAnalysisResult, error) {
			if driverName != "" {
				t.Errorf("expected empty driver, got %q", driverName)
			}
			if hasLocalOverride == nil || !*hasLocalOverride {
				t.Errorf("expected hasLocalOverride=true")
			}
			return []datastore.KitchenAnalysisResult{
				{
					GitRepoName:      "override-cookbook",
					HasLocalOverride: true,
					AnalysedAt:       now,
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks?has_local_override=true", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleKitchenAnalysisCookbooks_EmptyList(t *testing.T) {
	mock := &mockStore{
		ListKitchenAnalysisResultsFn: func(ctx context.Context) ([]datastore.KitchenAnalysisResult, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if body == "null\n" || body == "null" {
		t.Error("expected [] not null for empty cookbook list")
	}
}

func TestHandleKitchenAnalysisCookbookDetail(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(ctx context.Context, gitRepoName string) (*datastore.KitchenAnalysisResult, error) {
			if gitRepoName != "my-cookbook" {
				t.Errorf("expected name my-cookbook, got %q", gitRepoName)
			}
			return &datastore.KitchenAnalysisResult{
				GitRepoName:      "my-cookbook",
				GitRepoURL:       "https://example.com/my-cookbook.git",
				AnalysedAt:       now,
				HasLocalOverride: false,
				DriverName:       "dokken",
				CreatedAt:        now,
				UpdatedAt:        now,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks/my-cookbook", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp datastore.KitchenAnalysisResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.GitRepoName != "my-cookbook" {
		t.Errorf("expected git_repo_name my-cookbook, got %q", resp.GitRepoName)
	}
	if resp.DriverName != "dokken" {
		t.Errorf("expected driver_name dokken, got %q", resp.DriverName)
	}
}

func TestHandleKitchenAnalysisCookbookDetail_NotFound(t *testing.T) {
	mock := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(ctx context.Context, gitRepoName string) (*datastore.KitchenAnalysisResult, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/cookbooks/nonexistent", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleKitchenAnalysisTrigger(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/analysis/trigger", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "accepted" {
		t.Errorf("expected status accepted, got %q", resp["status"])
	}
}

func TestHandleKitchenAnalysisTrigger_WrongMethod(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/analysis/trigger", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}
