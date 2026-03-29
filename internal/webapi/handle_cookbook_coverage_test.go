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
		"kitchen_platforms":        []string{"ubuntu-22.04", "centos-7"},
		"gap_count":                1,
		"coverage_percentage":      88.1,
		"total_production_nodes":   67,
		"covered_node_count":       59,
		"tested_and_in_production": []map[string]any{},
		"tested_not_in_production": []string{},
		"in_production_not_tested": []map[string]any{},
		"production_platforms":     []map[string]any{},
	}

	now := time.Now().UTC()
	mock := &mockStore{
		GetCookbookPlatformCoverageFn: func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
			if cookbookName != "my-cookbook" {
				t.Errorf("unexpected cookbook name: %q", cookbookName)
			}
			return &datastore.CookbookPlatformCoverage{
				ID:           "cov-1",
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

	var resp datastore.CookbookPlatformCoverage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.CookbookName != "my-cookbook" {
		t.Errorf("expected cookbook name my-cookbook, got %q", resp.CookbookName)
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
				ID:           "cov-2",
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
}

func TestHandleCookbookPlatformCoverage_MethodNotAllowed(t *testing.T) {
	mock := &mockStore{}

	r := newTestRouterWithMock(mock)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/my-cookbook/platform-coverage", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	// POST should be rejected — either 405 or some error status.
	if rec.Code == http.StatusOK {
		t.Fatal("expected non-200 for POST request")
	}
}
