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

func TestHandleCookbookPlatformCoverage_Found(t *testing.T) {
	coverageData := map[string]any{
		"kitchen_platforms":      []string{"ubuntu-22.04", "centos-7"},
		"gap_count":              1,
		"coverage_percentage":    88.1,
		"total_production_nodes": 67,
		"covered_node_count":     59,
		"tested_and_in_production": []map[string]any{
			{
				"kitchen_name":     "ubuntu-22.04",
				"platform":         "ubuntu",
				"platform_version": "22.04",
				"node_count":       42,
			},
		},
		"tested_not_in_production": []string{"centos-7"},
		"in_production_not_tested": []map[string]any{
			{
				"platform":         "redhat",
				"platform_version": "8.9",
				"platform_family":  "rhel",
				"node_count":       8,
			},
		},
		"production_platforms": []map[string]any{
			{
				"platform":         "ubuntu",
				"platform_version": "22.04",
				"platform_family":  "debian",
				"node_count":       42,
			},
			{
				"platform":         "redhat",
				"platform_version": "8.9",
				"platform_family":  "rhel",
				"node_count":       8,
			},
		},
	}

	now := time.Now().UTC()
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			if cookbookName != "my-cookbook" {
				t.Errorf("unexpected cookbook name: %q", cookbookName)
			}
			return &datastore.CookbookPlatformCoverage{
				CookbookName: "my-cookbook",
				CoverageData: coverageData,
				EvaluatedAt:  now,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/my-cookbook/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp coverageAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.CookbookName != "my-cookbook" {
		t.Errorf("expected cookbook name my-cookbook, got %q", resp.CookbookName)
	}
	if resp.EvaluatedAt == "" {
		t.Error("expected evaluated_at to be set")
	}

	// Verify integer fields are properly typed (not float64 artefacts).
	if resp.GapCount != 1 {
		t.Errorf("expected gap_count 1, got %d", resp.GapCount)
	}
	if resp.TotalProductionNodes != 67 {
		t.Errorf("expected total_production_nodes 67, got %d", resp.TotalProductionNodes)
	}
	if resp.CoveredNodeCount != 59 {
		t.Errorf("expected covered_node_count 59, got %d", resp.CoveredNodeCount)
	}
	if resp.CoveragePercentage != 88.1 {
		t.Errorf("expected coverage_percentage 88.1, got %f", resp.CoveragePercentage)
	}

	// Verify kitchen platforms.
	if len(resp.KitchenPlatforms) != 2 {
		t.Fatalf("expected 2 kitchen platforms, got %d", len(resp.KitchenPlatforms))
	}

	// Verify production platforms have integer node_count.
	if len(resp.ProductionPlatforms) != 2 {
		t.Fatalf("expected 2 production platforms, got %d", len(resp.ProductionPlatforms))
	}
	if resp.ProductionPlatforms[0].NodeCount != 42 {
		t.Errorf("expected production platform node_count 42, got %d", resp.ProductionPlatforms[0].NodeCount)
	}

	// Verify tested_and_in_production has integer node_count.
	if len(resp.TestedAndInProd) != 1 {
		t.Fatalf("expected 1 tested_and_in_production entry, got %d", len(resp.TestedAndInProd))
	}
	if resp.TestedAndInProd[0].NodeCount != 42 {
		t.Errorf("expected tested match node_count 42, got %d", resp.TestedAndInProd[0].NodeCount)
	}
	if resp.TestedAndInProd[0].KitchenName != "ubuntu-22.04" {
		t.Errorf("expected kitchen_name ubuntu-22.04, got %q", resp.TestedAndInProd[0].KitchenName)
	}

	// Verify in_production_not_tested has integer node_count.
	if len(resp.InProdNotTested) != 1 {
		t.Fatalf("expected 1 in_production_not_tested entry, got %d", len(resp.InProdNotTested))
	}
	if resp.InProdNotTested[0].NodeCount != 8 {
		t.Errorf("expected gap platform node_count 8, got %d", resp.InProdNotTested[0].NodeCount)
	}
	if resp.InProdNotTested[0].PlatformFamily != "rhel" {
		t.Errorf("expected platform_family rhel, got %q", resp.InProdNotTested[0].PlatformFamily)
	}

	// Verify tested_not_in_production.
	if len(resp.TestedNotInProd) != 1 || resp.TestedNotInProd[0] != "centos-7" {
		t.Errorf("expected tested_not_in_production [centos-7], got %v", resp.TestedNotInProd)
	}
}

func TestHandleCookbookPlatformCoverage_NoInternalFields(t *testing.T) {
	// Verify that internal DB fields (id, created_at, updated_at, git_repo_id)
	// are not present in the JSON response.
	now := time.Now().UTC()
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			return &datastore.CookbookPlatformCoverage{
				CookbookName: cookbookName,
				CoverageData: map[string]any{
					"kitchen_platforms":        []string{},
					"production_platforms":     []any{},
					"tested_and_in_production": []any{},
					"tested_not_in_production": []string{},
					"in_production_not_tested": []any{},
					"gap_count":                0,
					"total_production_nodes":   0,
					"covered_node_count":       0,
					"coverage_percentage":      0.0,
				},
				EvaluatedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/test-cb/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}

	for _, field := range []string{"id", "git_repo_id", "created_at", "updated_at"} {
		if _, ok := raw[field]; ok {
			t.Errorf("internal field %q should not be in API response", field)
		}
	}

	// Confirm expected fields are present.
	for _, field := range []string{"cookbook_name", "evaluated_at", "gap_count", "coverage_percentage"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("expected field %q to be in API response", field)
		}
	}
}

func TestHandleCookbookPlatformCoverage_IntegerRoundTrip(t *testing.T) {
	// Simulate the JSONB round-trip where integers become float64 via
	// json.Unmarshal into any. The handler must still produce integer
	// values in the JSON output.
	now := time.Now().UTC()

	// Build coverage data that mimics what comes out of the DB: all
	// numbers are float64 because json.Unmarshal into any does that.
	coverageData := map[string]any{
		"kitchen_platforms": []any{"ubuntu-22.04"},
		"production_platforms": []any{
			map[string]any{
				"platform":         "ubuntu",
				"platform_version": "22.04",
				"platform_family":  "debian",
				"node_count":       float64(12), // float64 from json.Unmarshal
			},
		},
		"tested_and_in_production": []any{
			map[string]any{
				"kitchen_name":     "ubuntu-22.04",
				"platform":         "ubuntu",
				"platform_version": "22.04",
				"node_count":       float64(12),
			},
		},
		"tested_not_in_production": []any{},
		"in_production_not_tested": []any{},
		"gap_count":                float64(0),
		"total_production_nodes":   float64(12),
		"covered_node_count":       float64(12),
		"coverage_percentage":      float64(100),
	}

	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			return &datastore.CookbookPlatformCoverage{
				CookbookName: cookbookName,
				CoverageData: coverageData,
				EvaluatedAt:  now,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/roundtrip-cb/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Decode into raw JSON and check that integer fields are actual
	// integers (no decimal point) in the serialised output.
	body := rec.Body.String()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("failed to unmarshal raw response: %v", err)
	}

	// gap_count should be "0" not "0.0"
	if string(raw["gap_count"]) != "0" {
		t.Errorf("gap_count should be integer 0, got %s", string(raw["gap_count"]))
	}
	if string(raw["total_production_nodes"]) != "12" {
		t.Errorf("total_production_nodes should be integer 12, got %s", string(raw["total_production_nodes"]))
	}
	if string(raw["covered_node_count"]) != "12" {
		t.Errorf("covered_node_count should be integer 12, got %s", string(raw["covered_node_count"]))
	}

	// Also verify via typed decode.
	var resp coverageAPIResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode typed response: %v", err)
	}
	if resp.TotalProductionNodes != 12 {
		t.Errorf("expected total_production_nodes 12, got %d", resp.TotalProductionNodes)
	}
	if len(resp.ProductionPlatforms) != 1 {
		t.Fatalf("expected 1 production platform, got %d", len(resp.ProductionPlatforms))
	}
	if resp.ProductionPlatforms[0].NodeCount != 12 {
		t.Errorf("expected node_count 12, got %d", resp.ProductionPlatforms[0].NodeCount)
	}
}

func TestHandleCookbookPlatformCoverage_NotFound(t *testing.T) {
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nonexistent/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCookbookPlatformCoverage_InternalError(t *testing.T) {
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			return nil, fmt.Errorf("database connection lost")
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/my-cookbook/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCookbookPlatformCoverage_EmptyCoverage(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			return &datastore.CookbookPlatformCoverage{
				CookbookName: cookbookName,
				CoverageData: map[string]any{
					"kitchen_platforms":        []string{},
					"production_platforms":     []any{},
					"tested_and_in_production": []any{},
					"tested_not_in_production": []string{},
					"in_production_not_tested": []any{},
					"gap_count":                0,
					"total_production_nodes":   0,
					"covered_node_count":       0,
					"coverage_percentage":      0.0,
				},
				EvaluatedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/empty-cookbook/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp coverageAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// All slices should be non-nil (serialise to [] not null).
	if resp.KitchenPlatforms == nil {
		t.Error("kitchen_platforms should not be nil")
	}
	if resp.ProductionPlatforms == nil {
		t.Error("production_platforms should not be nil")
	}
	if resp.TestedAndInProd == nil {
		t.Error("tested_and_in_production should not be nil")
	}
	if resp.TestedNotInProd == nil {
		t.Error("tested_not_in_production should not be nil")
	}
	if resp.InProdNotTested == nil {
		t.Error("in_production_not_tested should not be nil")
	}
}

func TestHandleCookbookPlatformCoverage_MethodNotAllowed(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/my-cookbook/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed for POST request, got %d", rec.Code)
	}
}
