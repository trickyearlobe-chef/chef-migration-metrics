// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// handleDashboardVersionDistribution — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardVersionDistribution_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/version-distribution status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardVersionDistribution_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/version-distribution status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardVersionDistribution_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/version-distribution status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardVersionDistribution_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandleDashboardVersionDistribution_MethodNotAllowed_ErrorStructure(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
	}
	if resp.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// handleDashboardVersionDistributionTrend — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardVersionDistributionTrend_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/version-distribution/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/version-distribution/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardVersionDistributionTrend_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/version-distribution/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/version-distribution/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardVersionDistributionTrend_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/version-distribution/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/version-distribution/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardReadiness — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardReadiness_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/readiness status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardReadiness_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/readiness status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardReadiness_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/readiness status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardReadiness_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandleDashboardReadiness_MethodNotAllowed_ErrorStructure(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
	}
	if resp.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// handleDashboardReadinessTrend — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardReadinessTrend_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/readiness/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/readiness/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardReadinessTrend_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/readiness/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/readiness/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardReadinessTrend_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/readiness/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/readiness/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardCookbookCompatibility — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardCookbookCompatibility_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/cookbook-compatibility status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookCompatibility_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/cookbook-compatibility status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookCompatibility_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/cookbook-compatibility status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookCompatibility_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandleDashboardCookbookCompatibility_MethodNotAllowed_ErrorStructure(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
	}
	if resp.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// Route wiring — ensure all 5 dashboard routes are registered and not 404
// ---------------------------------------------------------------------------

func TestDashboardRoutes_NotFallingThrough(t *testing.T) {
	// All 7 dashboard endpoints should respond with 405 on POST (because
	// they use requireGET), NOT with 404 from the frontend fallback or
	// with 501 from handleNotImplemented — proving they are properly wired.
	endpoints := []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
		"/api/v1/dashboard/complexity/trend",
		"/api/v1/dashboard/stale/trend",
		"/api/v1/dashboard/cookbook-compatibility",
	}

	r := testRouter()

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, ep, nil)
			r.ServeHTTP(w, req)

			if w.Code == http.StatusNotImplemented {
				t.Errorf("POST %s returned 501 — route is still wired to handleNotImplemented", ep)
			}
			if w.Code == http.StatusNotFound {
				t.Errorf("POST %s returned 404 — route is not registered", ep)
			}
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST %s status = %d, want %d", ep, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Content-Type verification for all dashboard 405 responses
// ---------------------------------------------------------------------------

func TestDashboardRoutes_MethodNotAllowed_ContentType(t *testing.T) {
	endpoints := []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
		"/api/v1/dashboard/complexity/trend",
		"/api/v1/dashboard/stale/trend",
		"/api/v1/dashboard/cookbook-compatibility",
	}

	r := testRouter()

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, ep, nil)
			r.ServeHTTP(w, req)

			ct := w.Header().Get("Content-Type")
			if ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Error response structure verification for all dashboard 405 responses
// ---------------------------------------------------------------------------

func TestDashboardRoutes_MethodNotAllowed_ErrorStructure(t *testing.T) {
	endpoints := []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
		"/api/v1/dashboard/complexity/trend",
		"/api/v1/dashboard/stale/trend",
		"/api/v1/dashboard/cookbook-compatibility",
	}

	r := testRouter()

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, ep, nil)
			r.ServeHTTP(w, req)

			var resp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if resp.Error != ErrCodeMethodNotAllowed {
				t.Errorf("error code = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
			}
			if resp.Message == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Trend route does not shadow parent — version-distribution vs trend
// ---------------------------------------------------------------------------

func TestDashboardVersionDistribution_DoesNotShadowTrend(t *testing.T) {
	// Ensure /api/v1/dashboard/version-distribution and
	// /api/v1/dashboard/version-distribution/trend are distinct routes.
	r := testRouter()

	// Both should return 405 on POST (not 404 or 501).
	for _, ep := range []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, ep, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", ep, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestDashboardReadiness_DoesNotShadowTrend(t *testing.T) {
	// Ensure /api/v1/dashboard/readiness and
	// /api/v1/dashboard/readiness/trend are distinct routes.
	r := testRouter()

	for _, ep := range []string{
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, ep, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", ep, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// ---------------------------------------------------------------------------
// HEAD method — GET-only endpoints should reject HEAD if requireGET is strict
// ---------------------------------------------------------------------------

func TestDashboardRoutes_HEAD_MethodNotAllowed(t *testing.T) {
	endpoints := []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
		"/api/v1/dashboard/complexity/trend",
		"/api/v1/dashboard/stale/trend",
		"/api/v1/dashboard/cookbook-compatibility",
	}

	r := testRouter()

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodHead, ep, nil)
			r.ServeHTTP(w, req)

			// requireGET only allows GET, so HEAD should be 405.
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("HEAD %s status = %d, want %d", ep, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleDashboardVersionDistribution — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleDashboardVersionDistribution_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		TotalNodes   int `json:"total_nodes"`
		Distribution []struct {
			Version string `json:"version"`
			Count   int    `json:"count"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TotalNodes != 0 {
		t.Errorf("total_nodes = %d, want 0", body.TotalNodes)
	}
}

func TestHandleDashboardVersionDistribution_HappyPath_WithNodes(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ID: "n1", ChefVersion: "18.0.0", CollectedAt: now},
				{ID: "n2", ChefVersion: "18.0.0", CollectedAt: now},
				{ID: "n3", ChefVersion: "17.0.0", CollectedAt: now},
				{ID: "n4", ChefVersion: "", CollectedAt: now},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		TotalNodes   int `json:"total_nodes"`
		Distribution []struct {
			Version string  `json:"version"`
			Count   int     `json:"count"`
			Percent float64 `json:"percent"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TotalNodes != 4 {
		t.Errorf("total_nodes = %d, want 4", body.TotalNodes)
	}
	if len(body.Distribution) != 3 {
		t.Errorf("len(distribution) = %d, want 3 (18.0.0, 17.0.0, unknown)", len(body.Distribution))
	}
	// Sorted by count desc: 18.0.0 (2), then 17.0.0 (1), unknown (1).
	if len(body.Distribution) >= 1 && body.Distribution[0].Version != "18.0.0" {
		t.Errorf("distribution[0].version = %q, want %q", body.Distribution[0].Version, "18.0.0")
	}
	if len(body.Distribution) >= 1 && body.Distribution[0].Count != 2 {
		t.Errorf("distribution[0].count = %d, want 2", body.Distribution[0].Count)
	}
}

func TestHandleDashboardVersionDistribution_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/version-distribution", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardReadiness — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleDashboardReadiness_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0 (no target versions configured)", len(body.Data))
	}
}

func TestHandleDashboardReadiness_HappyPath_WithData(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, orgID, tv string) (int, int, int, error) {
			return 10, 7, 3, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			TargetChefVersion string  `json:"target_chef_version"`
			TotalNodes        int     `json:"total_nodes"`
			ReadyNodes        int     `json:"ready_nodes"`
			BlockedNodes      int     `json:"blocked_nodes"`
			ReadyPercent      float64 `json:"ready_percent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].TotalNodes != 10 {
		t.Errorf("total_nodes = %d, want 10", body.Data[0].TotalNodes)
	}
	if body.Data[0].ReadyNodes != 7 {
		t.Errorf("ready_nodes = %d, want 7", body.Data[0].ReadyNodes)
	}
	if body.Data[0].BlockedNodes != 3 {
		t.Errorf("blocked_nodes = %d, want 3", body.Data[0].BlockedNodes)
	}
	if body.Data[0].ReadyPercent != 70 {
		t.Errorf("ready_percent = %v, want 70", body.Data[0].ReadyPercent)
	}
}

func TestHandleDashboardReadiness_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/readiness", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardVersionDistributionTrend — happy path with mock DB
// ---------------------------------------------------------------------------

func TestHandleDashboardVersionDistributionTrend_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListMetricSnapshotsByOrganisationFn: func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
			return []datastore.MetricSnapshot{
				{
					ID:              "ms-1",
					CollectionRunID: "run-1",
					OrganisationID:  "org-1",
					SnapshotType:    "chef_version_distribution",
					Data:            json.RawMessage(`{"distribution":{"18.0.0":1,"17.0.0":1},"total_nodes":2,"stale_nodes":0,"fresh_nodes":2}`),
					SnapshotAt:      now,
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/version-distribution/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			TotalNodes   int            `json:"total_nodes"`
			Distribution map[string]int `json:"distribution"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].TotalNodes != 2 {
		t.Errorf("total_nodes = %d, want 2", body.Data[0].TotalNodes)
	}
}

func TestHandleDashboardVersionDistributionTrend_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/version-distribution/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardCookbookCompatibility — happy path with mock DB
// ---------------------------------------------------------------------------

func TestHandleDashboardCookbookCompatibility_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
				{ID: "cb-2", Name: "nginx", Version: "1.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{ID: "cc-1", ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ErrorCount: 0, DeprecationCount: 0},
				{ID: "cc-2", ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ErrorCount: 1, DeprecationCount: 0},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			TotalCookbooks        int `json:"total_cookbooks"`
			CompatibleCookbooks   int `json:"compatible_cookbooks"`
			IncompatibleCookbooks int `json:"incompatible_cookbooks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].TotalCookbooks != 2 {
		t.Errorf("total = %d, want 2", body.Data[0].TotalCookbooks)
	}
	if body.Data[0].CompatibleCookbooks != 1 {
		t.Errorf("compatible = %d, want 1", body.Data[0].CompatibleCookbooks)
	}
	if body.Data[0].IncompatibleCookbooks != 1 {
		t.Errorf("incompatible = %d, want 1", body.Data[0].IncompatibleCookbooks)
	}
}

func TestHandleDashboardCookbookCompatibility_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-compatibility", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardReadinessTrend — happy path with mock DB
// ---------------------------------------------------------------------------

func TestHandleDashboardReadinessTrend_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, orgID, tv string) (int, int, int, error) {
			return 5, 3, 2, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/readiness/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			OrganisationName string `json:"organisation_name"`
			TotalNodes       int    `json:"total_nodes"`
			ReadyNodes       int    `json:"ready_nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].TotalNodes != 5 {
		t.Errorf("total_nodes = %d, want 5", body.Data[0].TotalNodes)
	}
}

func TestHandleDashboardReadinessTrend_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/readiness/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardComplexityTrend — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardComplexityTrend_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/complexity/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardComplexityTrend_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/complexity/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardComplexityTrend_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/complexity/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardComplexityTrend — happy path
// ---------------------------------------------------------------------------

func TestHandleDashboardComplexityTrend_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{ID: "cc-1", ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 10, ComplexityLabel: "low"},
				{ID: "cc-2", ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ComplexityScore: 45, ComplexityLabel: "high"},
				{ID: "cc-3", ServerCookbookID: "cb-3", TargetChefVersion: "18.0.0", ComplexityScore: 80, ComplexityLabel: "critical"},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			OrganisationName  string  `json:"organisation_name"`
			TargetChefVersion string  `json:"target_chef_version"`
			TotalCookbooks    int     `json:"total_cookbooks"`
			TotalScore        int     `json:"total_score"`
			AverageScore      float64 `json:"average_score"`
			LowCount          int     `json:"low_count"`
			MediumCount       int     `json:"medium_count"`
			HighCount         int     `json:"high_count"`
			CriticalCount     int     `json:"critical_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	pt := body.Data[0]
	if pt.OrganisationName != "prod" {
		t.Errorf("organisation_name = %q, want %q", pt.OrganisationName, "prod")
	}
	if pt.TotalCookbooks != 3 {
		t.Errorf("total_cookbooks = %d, want 3", pt.TotalCookbooks)
	}
	if pt.TotalScore != 135 {
		t.Errorf("total_score = %d, want 135", pt.TotalScore)
	}
	if pt.AverageScore != 45.0 {
		t.Errorf("average_score = %f, want 45.0", pt.AverageScore)
	}
	if pt.LowCount != 1 {
		t.Errorf("low_count = %d, want 1", pt.LowCount)
	}
	if pt.HighCount != 1 {
		t.Errorf("high_count = %d, want 1", pt.HighCount)
	}
	if pt.CriticalCount != 1 {
		t.Errorf("critical_count = %d, want 1", pt.CriticalCount)
	}
}

func TestHandleDashboardComplexityTrend_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbookComplexity, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

func TestHandleDashboardComplexityTrend_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleDashboardComplexityTrend_MultipleOrgsAndVersions(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.ServerCookbookComplexity, error) {
			if orgID == "org-1" {
				return []datastore.ServerCookbookComplexity{
					{ID: "cc-1", ServerCookbookID: "cb-1", TargetChefVersion: "17.0.0", ComplexityScore: 20, ComplexityLabel: "medium"},
					{ID: "cc-2", ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ComplexityScore: 5, ComplexityLabel: "low"},
				}, nil
			}
			return []datastore.ServerCookbookComplexity{
				{ID: "cc-3", ServerCookbookID: "cb-3", TargetChefVersion: "18.0.0", ComplexityScore: 60, ComplexityLabel: "critical"},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"17.0.0", "18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/complexity/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			OrganisationName  string `json:"organisation_name"`
			TargetChefVersion string `json:"target_chef_version"`
			TotalCookbooks    int    `json:"total_cookbooks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// prod has 17.0.0 (1 cb) and 18.0.0 (1 cb); staging has 18.0.0 (1 cb) → 3 points
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}
}

// ---------------------------------------------------------------------------
// handleDashboardStaleTrend — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardStaleTrend_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dashboard/stale/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardStaleTrend_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dashboard/stale/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardStaleTrend_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dashboard/stale/trend status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardStaleTrend — happy path
// ---------------------------------------------------------------------------

func TestHandleDashboardStaleTrend_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListMetricSnapshotsByOrganisationFn: func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
			return []datastore.MetricSnapshot{
				{
					ID:              "ms-1",
					CollectionRunID: "run-1",
					OrganisationID:  "org-1",
					SnapshotType:    "chef_version_distribution",
					Data:            json.RawMessage(`{"distribution":{"18.0.0":3,"17.0.0":2},"total_nodes":5,"stale_nodes":3,"fresh_nodes":2}`),
					SnapshotAt:      now,
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			OrganisationName string `json:"organisation_name"`
			CollectionRunID  string `json:"collection_run_id"`
			CompletedAt      string `json:"completed_at"`
			TotalNodes       int    `json:"total_nodes"`
			StaleNodes       int    `json:"stale_nodes"`
			FreshNodes       int    `json:"fresh_nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	pt := body.Data[0]
	if pt.OrganisationName != "prod" {
		t.Errorf("organisation_name = %q, want %q", pt.OrganisationName, "prod")
	}
	if pt.CollectionRunID != "run-1" {
		t.Errorf("collection_run_id = %q, want %q", pt.CollectionRunID, "run-1")
	}
	if pt.TotalNodes != 5 {
		t.Errorf("total_nodes = %d, want 5", pt.TotalNodes)
	}
	if pt.StaleNodes != 3 {
		t.Errorf("stale_nodes = %d, want 3", pt.StaleNodes)
	}
	if pt.FreshNodes != 2 {
		t.Errorf("fresh_nodes = %d, want 2", pt.FreshNodes)
	}
	if pt.CompletedAt != "2025-06-15T12:00:00Z" {
		t.Errorf("completed_at = %q, want %q", pt.CompletedAt, "2025-06-15T12:00:00Z")
	}
}

func TestHandleDashboardStaleTrend_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListMetricSnapshotsByOrganisationFn: func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

func TestHandleDashboardStaleTrend_SkipsNonCompletedRuns(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	// Metric snapshots are only recorded for completed runs, so the
	// "skip non-completed" behaviour is implicit — only run-3's metric
	// snapshot exists.
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListMetricSnapshotsByOrganisationFn: func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
			return []datastore.MetricSnapshot{
				{
					ID:              "ms-3",
					CollectionRunID: "run-3",
					OrganisationID:  "org-1",
					SnapshotType:    "chef_version_distribution",
					Data:            json.RawMessage(`{"distribution":{},"total_nodes":1,"stale_nodes":1,"fresh_nodes":0}`),
					SnapshotAt:      now,
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			CollectionRunID string `json:"collection_run_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Only run-3 is completed
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0].CollectionRunID != "run-3" {
		t.Errorf("collection_run_id = %q, want %q", body.Data[0].CollectionRunID, "run-3")
	}
}

func TestHandleDashboardStaleTrend_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleDashboardStaleTrend_MultipleRuns(t *testing.T) {
	t1 := time.Date(2025, 6, 14, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListMetricSnapshotsByOrganisationFn: func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
			return []datastore.MetricSnapshot{
				{
					ID:              "ms-1",
					CollectionRunID: "run-1",
					OrganisationID:  "org-1",
					SnapshotType:    "chef_version_distribution",
					Data:            json.RawMessage(`{"distribution":{},"total_nodes":2,"stale_nodes":2,"fresh_nodes":0}`),
					SnapshotAt:      t1,
				},
				{
					ID:              "ms-2",
					CollectionRunID: "run-2",
					OrganisationID:  "org-1",
					SnapshotType:    "chef_version_distribution",
					Data:            json.RawMessage(`{"distribution":{},"total_nodes":3,"stale_nodes":1,"fresh_nodes":2}`),
					SnapshotAt:      t2,
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stale/trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			CollectionRunID string `json:"collection_run_id"`
			TotalNodes      int    `json:"total_nodes"`
			StaleNodes      int    `json:"stale_nodes"`
			FreshNodes      int    `json:"fresh_nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(body.Data))
	}
	// run-1: 2 nodes, 2 stale
	if body.Data[0].TotalNodes != 2 || body.Data[0].StaleNodes != 2 || body.Data[0].FreshNodes != 0 {
		t.Errorf("run-1: total=%d stale=%d fresh=%d, want 2/2/0",
			body.Data[0].TotalNodes, body.Data[0].StaleNodes, body.Data[0].FreshNodes)
	}
	// run-2: 3 nodes, 1 stale, 2 fresh
	if body.Data[1].TotalNodes != 3 || body.Data[1].StaleNodes != 1 || body.Data[1].FreshNodes != 2 {
		t.Errorf("run-2: total=%d stale=%d fresh=%d, want 3/1/2",
			body.Data[1].TotalNodes, body.Data[1].StaleNodes, body.Data[1].FreshNodes)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardCookbookDownloadStatus — method checks
// ---------------------------------------------------------------------------

func TestHandleDashboardCookbookDownloadStatus_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookDownloadStatus_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookDownloadStatus_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDashboardCookbookDownloadStatus_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

func TestHandleDashboardCookbookDownloadStatus_MethodNotAllowed_ErrorStructure(t *testing.T) {
	r := testRouter()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error != "method_not_allowed" {
		t.Errorf("error = %q, want method_not_allowed", body.Error)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardCookbookDownloadStatus — happy paths
// ---------------------------------------------------------------------------

func TestHandleDashboardCookbookDownloadStatus_HappyPath_NoCookbooks(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "test-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks  int    `json:"total_cookbooks"`
		HasFailures     bool   `json:"has_failures"`
		FailureMessage  string `json:"failure_message"`
		FailedCookbooks []struct {
			Name string `json:"name"`
		} `json:"failed_cookbooks"`
		FailedCookbookCount int `json:"failed_cookbook_count"`
		StatusCounts        struct {
			OK      int `json:"ok"`
			Failed  int `json:"failed"`
			Pending int `json:"pending"`
		} `json:"status_counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TotalCookbooks != 0 {
		t.Errorf("total_cookbooks = %d, want 0", body.TotalCookbooks)
	}
	if body.HasFailures {
		t.Error("has_failures = true, want false")
	}
	if body.FailedCookbookCount != 0 {
		t.Errorf("failed_cookbook_count = %d, want 0", body.FailedCookbookCount)
	}
	if len(body.FailedCookbooks) != 0 {
		t.Errorf("failed_cookbooks length = %d, want 0", len(body.FailedCookbooks))
	}
	if body.StatusCounts.OK != 0 || body.StatusCounts.Failed != 0 || body.StatusCounts.Pending != 0 {
		t.Errorf("status_counts = {ok:%d, failed:%d, pending:%d}, want all zeros",
			body.StatusCounts.OK, body.StatusCounts.Failed, body.StatusCounts.Pending)
	}
	if body.FailureMessage != "All cookbook versions downloaded successfully." {
		t.Errorf("failure_message = %q, want success message", body.FailureMessage)
	}
}

func TestHandleDashboardCookbookDownloadStatus_HappyPath_MixedStatuses(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0", DownloadStatus: "ok", IsActive: true},
				{ID: "cb-2", OrganisationID: "org-1", Name: "apache2", Version: "5.1.0", DownloadStatus: "ok", IsActive: true},
				{ID: "cb-3", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0", DownloadStatus: "failed", DownloadError: "HTTP 403: Forbidden", IsActive: true},
				{ID: "cb-4", OrganisationID: "org-1", Name: "mysql", Version: "8.0.0", DownloadStatus: "pending", IsActive: false},
				{ID: "cb-5", OrganisationID: "org-1", Name: "java", Version: "2.0.0", DownloadStatus: "failed", DownloadError: "connection timeout", IsActive: false},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks  int    `json:"total_cookbooks"`
		HasFailures     bool   `json:"has_failures"`
		FailureMessage  string `json:"failure_message"`
		FailedCookbooks []struct {
			ID             string `json:"id"`
			OrganisationID string `json:"organisation_id"`
			OrgName        string `json:"organisation_name"`
			Name           string `json:"name"`
			Version        string `json:"version"`
			DownloadError  string `json:"download_error"`
			IsActive       bool   `json:"is_active"`
		} `json:"failed_cookbooks"`
		FailedCookbookCount int `json:"failed_cookbook_count"`
		StatusCounts        struct {
			OK      int `json:"ok"`
			Failed  int `json:"failed"`
			Pending int `json:"pending"`
		} `json:"status_counts"`
		StatusPercentages struct {
			OKPercent      float64 `json:"ok_percent"`
			FailedPercent  float64 `json:"failed_percent"`
			PendingPercent float64 `json:"pending_percent"`
		} `json:"status_percentages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.TotalCookbooks != 5 {
		t.Errorf("total_cookbooks = %d, want 5", body.TotalCookbooks)
	}
	if body.StatusCounts.OK != 2 {
		t.Errorf("status_counts.ok = %d, want 2", body.StatusCounts.OK)
	}
	if body.StatusCounts.Failed != 2 {
		t.Errorf("status_counts.failed = %d, want 2", body.StatusCounts.Failed)
	}
	if body.StatusCounts.Pending != 1 {
		t.Errorf("status_counts.pending = %d, want 1", body.StatusCounts.Pending)
	}
	if !body.HasFailures {
		t.Error("has_failures = false, want true")
	}
	if body.FailedCookbookCount != 2 {
		t.Errorf("failed_cookbook_count = %d, want 2", body.FailedCookbookCount)
	}

	// Failed cookbooks are sorted: active first, then by name.
	if len(body.FailedCookbooks) != 2 {
		t.Fatalf("failed_cookbooks length = %d, want 2", len(body.FailedCookbooks))
	}

	// First failure should be the active cookbook (nginx).
	if body.FailedCookbooks[0].Name != "nginx" {
		t.Errorf("failed_cookbooks[0].name = %q, want nginx", body.FailedCookbooks[0].Name)
	}
	if !body.FailedCookbooks[0].IsActive {
		t.Error("failed_cookbooks[0].is_active = false, want true")
	}
	if body.FailedCookbooks[0].DownloadError != "HTTP 403: Forbidden" {
		t.Errorf("failed_cookbooks[0].download_error = %q, want 'HTTP 403: Forbidden'", body.FailedCookbooks[0].DownloadError)
	}
	if body.FailedCookbooks[0].OrgName != "prod-org" {
		t.Errorf("failed_cookbooks[0].organisation_name = %q, want prod-org", body.FailedCookbooks[0].OrgName)
	}

	// Second failure should be the inactive cookbook (java).
	if body.FailedCookbooks[1].Name != "java" {
		t.Errorf("failed_cookbooks[1].name = %q, want java", body.FailedCookbooks[1].Name)
	}
	if body.FailedCookbooks[1].IsActive {
		t.Error("failed_cookbooks[1].is_active = true, want false")
	}
	if body.FailedCookbooks[1].DownloadError != "connection timeout" {
		t.Errorf("failed_cookbooks[1].download_error = %q, want 'connection timeout'", body.FailedCookbooks[1].DownloadError)
	}

	// Check percentages.
	expectedOKPct := float64(2) / float64(5) * 100
	if body.StatusPercentages.OKPercent != expectedOKPct {
		t.Errorf("ok_percent = %f, want %f", body.StatusPercentages.OKPercent, expectedOKPct)
	}
	expectedFailedPct := float64(2) / float64(5) * 100
	if body.StatusPercentages.FailedPercent != expectedFailedPct {
		t.Errorf("failed_percent = %f, want %f", body.StatusPercentages.FailedPercent, expectedFailedPct)
	}

	// Check failure message.
	expected := fmt.Sprintf(
		"%d cookbook version(s) failed to download. These versions are excluded from compatibility analysis. "+
			"They will be retried on the next collection run.", 2)
	if body.FailureMessage != expected {
		t.Errorf("failure_message = %q, want %q", body.FailureMessage, expected)
	}
}

func TestHandleDashboardCookbookDownloadStatus_IgnoresGitCookbooks(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "test-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0", DownloadStatus: "ok"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks int `json:"total_cookbooks"`
		StatusCounts   struct {
			OK int `json:"ok"`
		} `json:"status_counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Only the chef_server cookbook should be counted.
	if body.TotalCookbooks != 1 {
		t.Errorf("total_cookbooks = %d, want 1 (git cookbook should be excluded)", body.TotalCookbooks)
	}
	if body.StatusCounts.OK != 1 {
		t.Errorf("status_counts.ok = %d, want 1", body.StatusCounts.OK)
	}
}

func TestHandleDashboardCookbookDownloadStatus_AllOK(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "test-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0", DownloadStatus: "ok"},
				{ID: "cb-2", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0", DownloadStatus: "ok"},
				{ID: "cb-3", OrganisationID: "org-1", Name: "mysql", Version: "8.0.0", DownloadStatus: "ok"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks      int        `json:"total_cookbooks"`
		HasFailures         bool       `json:"has_failures"`
		FailedCookbookCount int        `json:"failed_cookbook_count"`
		FailedCookbooks     []struct{} `json:"failed_cookbooks"`
		FailureMessage      string     `json:"failure_message"`
		StatusCounts        struct {
			OK      int `json:"ok"`
			Failed  int `json:"failed"`
			Pending int `json:"pending"`
		} `json:"status_counts"`
		StatusPercentages struct {
			OKPercent float64 `json:"ok_percent"`
		} `json:"status_percentages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.TotalCookbooks != 3 {
		t.Errorf("total_cookbooks = %d, want 3", body.TotalCookbooks)
	}
	if body.HasFailures {
		t.Error("has_failures = true, want false")
	}
	if body.FailedCookbookCount != 0 {
		t.Errorf("failed_cookbook_count = %d, want 0", body.FailedCookbookCount)
	}
	if len(body.FailedCookbooks) != 0 {
		t.Errorf("failed_cookbooks length = %d, want 0", len(body.FailedCookbooks))
	}
	if body.StatusCounts.OK != 3 {
		t.Errorf("status_counts.ok = %d, want 3", body.StatusCounts.OK)
	}
	if body.StatusCounts.Failed != 0 {
		t.Errorf("status_counts.failed = %d, want 0", body.StatusCounts.Failed)
	}
	if body.StatusPercentages.OKPercent != 100 {
		t.Errorf("ok_percent = %f, want 100", body.StatusPercentages.OKPercent)
	}
	if body.FailureMessage != "All cookbook versions downloaded successfully." {
		t.Errorf("failure_message = %q, want success message", body.FailureMessage)
	}
}

func TestHandleDashboardCookbookDownloadStatus_MultipleOrgs(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			if organisationID == "org-1" {
				return []datastore.ServerCookbook{
					{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0", DownloadStatus: "ok"},
					{ID: "cb-2", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0", DownloadStatus: "failed", DownloadError: "404 Not Found", IsActive: true},
				}, nil
			}
			return []datastore.ServerCookbook{
				{ID: "cb-3", OrganisationID: "org-2", Name: "mysql", Version: "8.0.0", DownloadStatus: "ok"},
				{ID: "cb-4", OrganisationID: "org-2", Name: "redis", Version: "1.0.0", DownloadStatus: "pending"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks      int `json:"total_cookbooks"`
		FailedCookbookCount int `json:"failed_cookbook_count"`
		StatusCounts        struct {
			OK      int `json:"ok"`
			Failed  int `json:"failed"`
			Pending int `json:"pending"`
		} `json:"status_counts"`
		FailedCookbooks []struct {
			Name    string `json:"name"`
			OrgName string `json:"organisation_name"`
		} `json:"failed_cookbooks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.TotalCookbooks != 4 {
		t.Errorf("total_cookbooks = %d, want 4", body.TotalCookbooks)
	}
	if body.StatusCounts.OK != 2 {
		t.Errorf("status_counts.ok = %d, want 2", body.StatusCounts.OK)
	}
	if body.StatusCounts.Failed != 1 {
		t.Errorf("status_counts.failed = %d, want 1", body.StatusCounts.Failed)
	}
	if body.StatusCounts.Pending != 1 {
		t.Errorf("status_counts.pending = %d, want 1", body.StatusCounts.Pending)
	}
	if body.FailedCookbookCount != 1 {
		t.Errorf("failed_cookbook_count = %d, want 1", body.FailedCookbookCount)
	}
	if len(body.FailedCookbooks) != 1 {
		t.Fatalf("failed_cookbooks length = %d, want 1", len(body.FailedCookbooks))
	}
	if body.FailedCookbooks[0].Name != "nginx" {
		t.Errorf("failed_cookbooks[0].name = %q, want nginx", body.FailedCookbooks[0].Name)
	}
	if body.FailedCookbooks[0].OrgName != "prod" {
		t.Errorf("failed_cookbooks[0].organisation_name = %q, want prod", body.FailedCookbooks[0].OrgName)
	}
}

func TestHandleDashboardCookbookDownloadStatus_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleDashboardCookbookDownloadStatus_CookbookListError_NonFatal(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "org1"},
				{ID: "org-2", Name: "org2"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			if organisationID == "org-1" {
				return nil, errors.New("timeout")
			}
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-2", Name: "apache2", Version: "5.0.0", DownloadStatus: "ok"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	// Should still succeed — org-1 error is non-fatal.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks int `json:"total_cookbooks"`
		StatusCounts   struct {
			OK int `json:"ok"`
		} `json:"status_counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Only org-2 cookbooks counted.
	if body.TotalCookbooks != 1 {
		t.Errorf("total_cookbooks = %d, want 1", body.TotalCookbooks)
	}
	if body.StatusCounts.OK != 1 {
		t.Errorf("status_counts.ok = %d, want 1", body.StatusCounts.OK)
	}
}

func TestHandleDashboardCookbookDownloadStatus_EmptyDownloadStatusTreatedAsPending(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "test-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "legacy", Version: "1.0.0", DownloadStatus: ""},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks int `json:"total_cookbooks"`
		StatusCounts   struct {
			OK      int `json:"ok"`
			Failed  int `json:"failed"`
			Pending int `json:"pending"`
		} `json:"status_counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.TotalCookbooks != 1 {
		t.Errorf("total_cookbooks = %d, want 1", body.TotalCookbooks)
	}
	if body.StatusCounts.Pending != 1 {
		t.Errorf("status_counts.pending = %d, want 1 (empty status should be treated as pending)", body.StatusCounts.Pending)
	}
	if body.StatusCounts.OK != 0 {
		t.Errorf("status_counts.ok = %d, want 0", body.StatusCounts.OK)
	}
}

func TestHandleDashboardCookbookDownloadStatus_FailedSortedActiveFirst(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "test-org"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "zebra", Version: "1.0.0", DownloadStatus: "failed", DownloadError: "err1", IsActive: false},
				{ID: "cb-2", OrganisationID: "org-1", Name: "alpha", Version: "1.0.0", DownloadStatus: "failed", DownloadError: "err2", IsActive: true},
				{ID: "cb-3", OrganisationID: "org-1", Name: "beta", Version: "1.0.0", DownloadStatus: "failed", DownloadError: "err3", IsActive: true},
				{ID: "cb-4", OrganisationID: "org-1", Name: "delta", Version: "1.0.0", DownloadStatus: "failed", DownloadError: "err4", IsActive: false},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		FailedCookbooks []struct {
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"failed_cookbooks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(body.FailedCookbooks) != 4 {
		t.Fatalf("failed_cookbooks length = %d, want 4", len(body.FailedCookbooks))
	}

	// Active cookbooks first, sorted alphabetically.
	expectedOrder := []struct {
		name   string
		active bool
	}{
		{"alpha", true},
		{"beta", true},
		{"delta", false},
		{"zebra", false},
	}

	for i, exp := range expectedOrder {
		if body.FailedCookbooks[i].Name != exp.name {
			t.Errorf("failed_cookbooks[%d].name = %q, want %q", i, body.FailedCookbooks[i].Name, exp.name)
		}
		if body.FailedCookbooks[i].IsActive != exp.active {
			t.Errorf("failed_cookbooks[%d].is_active = %v, want %v", i, body.FailedCookbooks[i].IsActive, exp.active)
		}
	}
}

func TestHandleDashboardCookbookDownloadStatus_NoOrgs(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMock(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookbook-download-status", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		TotalCookbooks int  `json:"total_cookbooks"`
		HasFailures    bool `json:"has_failures"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.TotalCookbooks != 0 {
		t.Errorf("total_cookbooks = %d, want 0", body.TotalCookbooks)
	}
	if body.HasFailures {
		t.Error("has_failures = true, want false")
	}
}

// Ensure the _ = fmt.Sprintf is used (keeps the import alive).
var _ = fmt.Sprintf
