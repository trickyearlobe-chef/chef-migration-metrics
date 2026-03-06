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
	// All 5 dashboard endpoints should respond with 405 on POST (because
	// they use requireGET), NOT with 404 from the frontend fallback or
	// with 501 from handleNotImplemented — proving they are properly wired.
	endpoints := []string{
		"/api/v1/dashboard/version-distribution",
		"/api/v1/dashboard/version-distribution/trend",
		"/api/v1/dashboard/readiness",
		"/api/v1/dashboard/readiness/trend",
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
		ListCollectionRunsFn: func(ctx context.Context, orgID string, limit int) ([]datastore.CollectionRun, error) {
			return []datastore.CollectionRun{
				{ID: "run-1", OrganisationID: "org-1", Status: "completed", CompletedAt: now},
			}, nil
		},
		ListNodeSnapshotsByCollectionRunFn: func(ctx context.Context, runID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ID: "n1", ChefVersion: "18.0.0"},
				{ID: "n2", ChefVersion: "17.0.0"},
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
		ListCookbooksByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt"},
				{ID: "cb-2", Name: "nginx"},
			}, nil
		},
		GetLatestTestKitchenResultFn: func(ctx context.Context, cbID, tv string) (*datastore.TestKitchenResult, error) {
			if cbID == "cb-1" {
				return &datastore.TestKitchenResult{Compatible: true}, nil
			}
			return &datastore.TestKitchenResult{Compatible: false}, nil
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
