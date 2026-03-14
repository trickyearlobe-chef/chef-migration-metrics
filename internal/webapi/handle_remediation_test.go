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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// handleRemediationPriority — method checks
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /remediation/priority status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationPriority_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /remediation/priority status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationPriority_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /remediation/priority status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationPriority_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — no target version configured
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_NoTargetVersion(t *testing.T) {
	store := &mockStore{}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	// No TargetChefVersions set

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — happy path empty
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return nil, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TargetChefVersion    string `json:"target_chef_version"`
		TotalCookbooks       int    `json:"total_cookbooks"`
		TotalAutoCorrectable int    `json:"total_auto_correctable"`
		Data                 []any  `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TargetChefVersion != "18.0.0" {
		t.Errorf("target_chef_version = %q, want %q", resp.TargetChefVersion, "18.0.0")
	}
	if resp.TotalCookbooks != 0 {
		t.Errorf("total_cookbooks = %d, want 0", resp.TotalCookbooks)
	}
	if len(resp.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(resp.Data))
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — happy path with data
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_HappyPath_WithData(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0"},
				{ID: "cb-2", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{
					ID:                   "cc-1",
					ServerCookbookID:    "cb-1",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      42,
					ComplexityLabel:      "high",
					AffectedNodeCount:    10,
					AffectedRoleCount:    3,
					AutoCorrectableCount: 5,
					ManualFixCount:       2,
					DeprecationCount:     4,
					ErrorCount:           1,
					EvaluatedAt:          now,
				},
				{
					ID:                   "cc-2",
					ServerCookbookID:    "cb-2",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      10,
					ComplexityLabel:      "low",
					AffectedNodeCount:    20,
					AffectedRoleCount:    5,
					AutoCorrectableCount: 8,
					ManualFixCount:       0,
					DeprecationCount:     2,
					ErrorCount:           0,
					EvaluatedAt:          now,
				},
				{
					// Different target version — should be filtered out.
					ID:                "cc-3",
					ServerCookbookID: "cb-1",
					TargetChefVersion: "17.0.0",
					ComplexityScore:   99,
				},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TargetChefVersion    string `json:"target_chef_version"`
		TotalCookbooks       int    `json:"total_cookbooks"`
		TotalAutoCorrectable int    `json:"total_auto_correctable"`
		TotalManualFix       int    `json:"total_manual_fix"`
		TotalDeprecations    int    `json:"total_deprecations"`
		TotalErrors          int    `json:"total_errors"`
		Data                 []struct {
			CookbookName         string `json:"cookbook_name"`
			PriorityScore        int    `json:"priority_score"`
			ComplexityScore      int    `json:"complexity_score"`
			AffectedNodeCount    int    `json:"affected_node_count"`
			AutoCorrectableCount int    `json:"auto_correctable_count"`
			ManualFixCount       int    `json:"manual_fix_count"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalCookbooks != 2 {
		t.Errorf("total_cookbooks = %d, want 2", resp.TotalCookbooks)
	}
	if resp.TotalAutoCorrectable != 13 { // 5 + 8
		t.Errorf("total_auto_correctable = %d, want 13", resp.TotalAutoCorrectable)
	}
	if resp.TotalManualFix != 2 {
		t.Errorf("total_manual_fix = %d, want 2", resp.TotalManualFix)
	}
	if resp.TotalDeprecations != 6 { // 4 + 2
		t.Errorf("total_deprecations = %d, want 6", resp.TotalDeprecations)
	}
	if resp.TotalErrors != 1 {
		t.Errorf("total_errors = %d, want 1", resp.TotalErrors)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}

	// Default sort is by priority_score desc.
	// apache2: 42 * 10 = 420
	// nginx:   10 * 20 = 200
	if resp.Data[0].CookbookName != "apache2" {
		t.Errorf("data[0].cookbook_name = %q, want %q", resp.Data[0].CookbookName, "apache2")
	}
	if resp.Data[0].PriorityScore != 420 {
		t.Errorf("data[0].priority_score = %d, want 420", resp.Data[0].PriorityScore)
	}
	if resp.Data[1].CookbookName != "nginx" {
		t.Errorf("data[1].cookbook_name = %q, want %q", resp.Data[1].CookbookName, "nginx")
	}
	if resp.Data[1].PriorityScore != 200 {
		t.Errorf("data[1].priority_score = %d, want 200", resp.Data[1].PriorityScore)
	}

	if resp.Pagination.TotalItems != 2 {
		t.Errorf("pagination.total_items = %d, want 2", resp.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — sorting
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_SortByName(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "zookeeper", Version: "1.0.0"},
				{ID: "cb-2", Name: "apache2", Version: "1.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 5, AffectedNodeCount: 1, EvaluatedAt: now},
				{ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ComplexityScore: 50, AffectedNodeCount: 10, EvaluatedAt: now},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?sort=name&order=asc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			CookbookName string `json:"cookbook_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].CookbookName != "apache2" {
		t.Errorf("data[0].cookbook_name = %q, want %q", resp.Data[0].CookbookName, "apache2")
	}
	if resp.Data[1].CookbookName != "zookeeper" {
		t.Errorf("data[1].cookbook_name = %q, want %q", resp.Data[1].CookbookName, "zookeeper")
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — explicit target version
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_ExplicitTargetVersion(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "test", Version: "1.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{ServerCookbookID: "cb-1", TargetChefVersion: "17.0.0", ComplexityScore: 10, AffectedNodeCount: 5, EvaluatedAt: now},
				{ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 20, AffectedNodeCount: 3, EvaluatedAt: now},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0", "17.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?target_chef_version=17.0.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		TargetChefVersion string `json:"target_chef_version"`
		TotalCookbooks    int    `json:"total_cookbooks"`
		Data              []struct {
			ComplexityScore int `json:"complexity_score"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TargetChefVersion != "17.0.0" {
		t.Errorf("target_chef_version = %q, want %q", resp.TargetChefVersion, "17.0.0")
	}
	if resp.TotalCookbooks != 1 {
		t.Errorf("total_cookbooks = %d, want 1", resp.TotalCookbooks)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].ComplexityScore != 10 {
		t.Errorf("data[0].complexity_score = %d, want 10", resp.Data[0].ComplexityScore)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — organisation filter
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_OrganisationFilter(t *testing.T) {
	now := time.Now()

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
					{ID: "cb-1", Name: "prod-cookbook", Version: "1.0.0"},
				}, nil
			}
			return []datastore.ServerCookbook{
				{ID: "cb-2", Name: "staging-cookbook", Version: "1.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			if organisationID == "org-1" {
				return []datastore.ServerCookbookComplexity{
					{ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 10, AffectedNodeCount: 1, EvaluatedAt: now},
				}, nil
			}
			return []datastore.ServerCookbookComplexity{
				{ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ComplexityScore: 20, AffectedNodeCount: 2, EvaluatedAt: now},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		TotalCookbooks int `json:"total_cookbooks"`
		Data           []struct {
			CookbookName string `json:"cookbook_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalCookbooks != 1 {
		t.Errorf("total_cookbooks = %d, want 1", resp.TotalCookbooks)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].CookbookName != "prod-cookbook" {
		t.Errorf("data[0].cookbook_name = %q, want %q", resp.Data[0].CookbookName, "prod-cookbook")
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — DB error
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("db connection lost")
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — zero affected nodes uses blastRadius=1
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_ZeroAffectedNodesUsesOne(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", Name: "unused", Version: "1.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 25, AffectedNodeCount: 0, EvaluatedAt: now},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			PriorityScore int `json:"priority_score"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(resp.Data))
	}
	// priority_score = 25 * 1 (clamped from 0)
	if resp.Data[0].PriorityScore != 25 {
		t.Errorf("data[0].priority_score = %d, want 25", resp.Data[0].PriorityScore)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — method checks
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /remediation/summary status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationSummary_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /remediation/summary status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationSummary_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /remediation/summary status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRemediationSummary_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — no target version configured
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_NoTargetVersion(t *testing.T) {
	store := &mockStore{}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — happy path empty
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return nil, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			return 0, 0, 0, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TargetChefVersion        string `json:"target_chef_version"`
		TotalCookbooksEvaluated  int    `json:"total_cookbooks_evaluated"`
		TotalNeedingRemediation  int    `json:"total_needing_remediation"`
		QuickWins                int    `json:"quick_wins"`
		ManualFixes              int    `json:"manual_fixes"`
		BlockedNodesByComplexity int    `json:"blocked_nodes_by_complexity"`
		BlockedNodesByReadiness  int    `json:"blocked_nodes_by_readiness"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TargetChefVersion != "18.0.0" {
		t.Errorf("target_chef_version = %q, want %q", resp.TargetChefVersion, "18.0.0")
	}
	if resp.TotalCookbooksEvaluated != 0 {
		t.Errorf("total_cookbooks_evaluated = %d, want 0", resp.TotalCookbooksEvaluated)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — happy path with data
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_HappyPath_WithData(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{
					ServerCookbookID:    "cb-1",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      42,
					AutoCorrectableCount: 5,
					ManualFixCount:       2,
					AffectedNodeCount:    10,
					EvaluatedAt:          now,
				},
				{
					// Quick win: auto-correctable only, no manual fixes.
					ServerCookbookID:    "cb-2",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      5,
					AutoCorrectableCount: 3,
					ManualFixCount:       0,
					AffectedNodeCount:    5,
					EvaluatedAt:          now,
				},
				{
					// Zero complexity — not needing remediation.
					ServerCookbookID: "cb-3",
					TargetChefVersion: "18.0.0",
					ComplexityScore:   0,
					EvaluatedAt:       now,
				},
			}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			return 20, 12, 8, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TargetChefVersion        string `json:"target_chef_version"`
		TotalCookbooksEvaluated  int    `json:"total_cookbooks_evaluated"`
		TotalNeedingRemediation  int    `json:"total_needing_remediation"`
		QuickWins                int    `json:"quick_wins"`
		ManualFixes              int    `json:"manual_fixes"`
		BlockedNodesByComplexity int    `json:"blocked_nodes_by_complexity"`
		BlockedNodesByReadiness  int    `json:"blocked_nodes_by_readiness"`
		TotalAutoCorrectable     int    `json:"total_auto_correctable"`
		TotalManualFix           int    `json:"total_manual_fix"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalCookbooksEvaluated != 3 {
		t.Errorf("total_cookbooks_evaluated = %d, want 3", resp.TotalCookbooksEvaluated)
	}
	if resp.TotalNeedingRemediation != 2 {
		t.Errorf("total_needing_remediation = %d, want 2", resp.TotalNeedingRemediation)
	}
	if resp.QuickWins != 1 {
		t.Errorf("quick_wins = %d, want 1", resp.QuickWins)
	}
	if resp.ManualFixes != 1 {
		t.Errorf("manual_fixes = %d, want 1", resp.ManualFixes)
	}
	if resp.BlockedNodesByComplexity != 15 { // 10 + 5
		t.Errorf("blocked_nodes_by_complexity = %d, want 15", resp.BlockedNodesByComplexity)
	}
	if resp.BlockedNodesByReadiness != 8 {
		t.Errorf("blocked_nodes_by_readiness = %d, want 8", resp.BlockedNodesByReadiness)
	}
	if resp.TotalAutoCorrectable != 8 { // 5 + 3
		t.Errorf("total_auto_correctable = %d, want 8", resp.TotalAutoCorrectable)
	}
	if resp.TotalManualFix != 2 {
		t.Errorf("total_manual_fix = %d, want 2", resp.TotalManualFix)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — DB error
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("db connection lost")
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleRemediationSummary — organisation filter
// ---------------------------------------------------------------------------

func TestHandleRemediationSummary_OrganisationFilter(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			if organisationID == "org-1" {
				return []datastore.ServerCookbookComplexity{
					{ServerCookbookID: "cb-1", TargetChefVersion: "18.0.0", ComplexityScore: 10, AffectedNodeCount: 5, ManualFixCount: 1, EvaluatedAt: now},
				}, nil
			}
			return []datastore.ServerCookbookComplexity{
				{ServerCookbookID: "cb-2", TargetChefVersion: "18.0.0", ComplexityScore: 20, AffectedNodeCount: 10, ManualFixCount: 3, EvaluatedAt: now},
			}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			if organisationID == "org-2" {
				return 10, 3, 7, nil
			}
			return 5, 4, 1, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary?organisation=staging", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		TotalCookbooksEvaluated int `json:"total_cookbooks_evaluated"`
		TotalNeedingRemediation int `json:"total_needing_remediation"`
		ManualFixes             int `json:"manual_fixes"`
		BlockedNodesByReadiness int `json:"blocked_nodes_by_readiness"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// Only staging org data should be included.
	if resp.TotalCookbooksEvaluated != 1 {
		t.Errorf("total_cookbooks_evaluated = %d, want 1", resp.TotalCookbooksEvaluated)
	}
	if resp.TotalNeedingRemediation != 1 {
		t.Errorf("total_needing_remediation = %d, want 1", resp.TotalNeedingRemediation)
	}
	if resp.ManualFixes != 1 {
		t.Errorf("manual_fixes = %d, want 1", resp.ManualFixes)
	}
	if resp.BlockedNodesByReadiness != 7 {
		t.Errorf("blocked_nodes_by_readiness = %d, want 7", resp.BlockedNodesByReadiness)
	}
}

// ---------------------------------------------------------------------------
// Route registration — remediation routes do not shadow each other
// ---------------------------------------------------------------------------

func TestRemediationRoutes_NotFallingThrough(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			return 0, 0, 0, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	paths := []string{
		"/api/v1/remediation/priority",
		"/api/v1/remediation/summary",
	}

	for _, path := range paths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)

		// Both should succeed (200) or return a proper API status —
		// they should NOT return 404 or 501.
		if w.Code == http.StatusNotFound || w.Code == http.StatusNotImplemented {
			t.Errorf("GET %s returned %d — route not properly registered", path, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// handleRemediationPriority — pagination
// ---------------------------------------------------------------------------

func TestHandleRemediationPriority_Pagination(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			cbs := make([]datastore.ServerCookbook, 5)
			for i := range cbs {
				id := string(rune('a' + i))
				cbs[i] = datastore.ServerCookbook{ID: "cb-" + id, Name: "cookbook-" + id, Version: "1.0.0"}
			}
			return cbs, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			ccs := make([]datastore.ServerCookbookComplexity, 5)
			for i := range ccs {
				id := string(rune('a' + i))
				ccs[i] = datastore.ServerCookbookComplexity{
					ServerCookbookID: "cb-" + id,
					TargetChefVersion: "18.0.0",
					ComplexityScore:   (5 - i) * 10,
					AffectedNodeCount: 1,
					EvaluatedAt:       now,
				}
			}
			return ccs, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?page=2&per_page=2", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data       []any `json:"data"`
		Pagination struct {
			Page       int `json:"page"`
			PerPage    int `json:"per_page"`
			TotalItems int `json:"total_items"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Pagination.Page != 2 {
		t.Errorf("pagination.page = %d, want 2", resp.Pagination.Page)
	}
	if resp.Pagination.TotalItems != 5 {
		t.Errorf("pagination.total_items = %d, want 5", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 3 {
		t.Errorf("pagination.total_pages = %d, want 3", resp.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// resolveOrganisationFilter — helper tests
// ---------------------------------------------------------------------------

func TestResolveOrganisationFilter_NoFilter_ReturnsAll(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary", nil)
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		t.Fatalf("resolveOrganisationFilter error: %v", err)
	}
	if len(orgs) != 2 {
		t.Errorf("len(orgs) = %d, want 2", len(orgs))
	}
}

func TestResolveOrganisationFilter_WithFilter_ReturnsMatch(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
				{ID: "org-3", Name: "dev"},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/summary?organisation=prod&organisation=dev", nil)
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		t.Fatalf("resolveOrganisationFilter error: %v", err)
	}
	if len(orgs) != 2 {
		t.Errorf("len(orgs) = %d, want 2", len(orgs))
	}

	names := make(map[string]bool)
	for _, org := range orgs {
		names[org.Name] = true
	}
	if !names["prod"] {
		t.Error("expected prod in filtered orgs")
	}
	if !names["dev"] {
		t.Error("expected dev in filtered orgs")
	}
	if names["staging"] {
		t.Error("did not expect staging in filtered orgs")
	}
}

func TestHandleRemediationPriority_ComplexityLabelFilter(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0"},
				{ID: "cb-2", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0"},
				{ID: "cb-3", OrganisationID: "org-1", Name: "mysql", Version: "8.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{
					ID:                   "cc-1",
					ServerCookbookID:    "cb-1",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      42,
					ComplexityLabel:      "high",
					AffectedNodeCount:    10,
					AutoCorrectableCount: 5,
					ManualFixCount:       2,
					DeprecationCount:     4,
					ErrorCount:           1,
					EvaluatedAt:          now,
				},
				{
					ID:                   "cc-2",
					ServerCookbookID:    "cb-2",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      10,
					ComplexityLabel:      "low",
					AffectedNodeCount:    20,
					AutoCorrectableCount: 8,
					ManualFixCount:       0,
					DeprecationCount:     2,
					ErrorCount:           0,
					EvaluatedAt:          now,
				},
				{
					ID:                   "cc-3",
					ServerCookbookID:    "cb-3",
					TargetChefVersion:    "18.0.0",
					ComplexityScore:      80,
					ComplexityLabel:      "high",
					AffectedNodeCount:    5,
					AutoCorrectableCount: 1,
					ManualFixCount:       10,
					DeprecationCount:     6,
					ErrorCount:           3,
					EvaluatedAt:          now,
				},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)

	// Filter to only "high" complexity cookbooks.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?complexity_label=high", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TotalCookbooks int `json:"total_cookbooks"`
		Data           []struct {
			CookbookName    string `json:"cookbook_name"`
			ComplexityLabel string `json:"complexity_label"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// Only the two "high" complexity cookbooks should be returned.
	if resp.TotalCookbooks != 2 {
		t.Errorf("total_cookbooks = %d, want 2", resp.TotalCookbooks)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	for i, item := range resp.Data {
		if item.ComplexityLabel != "high" {
			t.Errorf("data[%d].complexity_label = %q, want %q", i, item.ComplexityLabel, "high")
		}
	}
	if resp.Pagination.TotalItems != 2 {
		t.Errorf("pagination.total_items = %d, want 2", resp.Pagination.TotalItems)
	}
}

func TestHandleRemediationPriority_ComplexityLabelFilter_NoMatch(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{
					ID:                "cc-1",
					ServerCookbookID: "cb-1",
					TargetChefVersion: "18.0.0",
					ComplexityScore:   10,
					ComplexityLabel:   "low",
					AffectedNodeCount: 5,
					EvaluatedAt:       now,
				},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)

	// Filter to "critical" — no cookbooks match.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority?complexity_label=critical", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TotalCookbooks int           `json:"total_cookbooks"`
		Data           []interface{} `json:"data"`
		Pagination     struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalCookbooks != 0 {
		t.Errorf("total_cookbooks = %d, want 0", resp.TotalCookbooks)
	}
	if len(resp.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(resp.Data))
	}
	if resp.Pagination.TotalItems != 0 {
		t.Errorf("pagination.total_items = %d, want 0", resp.Pagination.TotalItems)
	}
}

func TestHandleRemediationPriority_ComplexityLabelFilter_OmittedReturnsAll(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
			}, nil
		},
		ListServerCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "cb-1", OrganisationID: "org-1", Name: "apache2", Version: "5.0.0"},
				{ID: "cb-2", OrganisationID: "org-1", Name: "nginx", Version: "3.0.0"},
			}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{
					ID:                "cc-1",
					ServerCookbookID: "cb-1",
					TargetChefVersion: "18.0.0",
					ComplexityScore:   42,
					ComplexityLabel:   "high",
					AffectedNodeCount: 10,
					EvaluatedAt:       now,
				},
				{
					ID:                "cc-2",
					ServerCookbookID: "cb-2",
					TargetChefVersion: "18.0.0",
					ComplexityScore:   10,
					ComplexityLabel:   "low",
					AffectedNodeCount: 20,
					EvaluatedAt:       now,
				},
			}, nil
		},
	}

	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)

	// No complexity_label param — should return all.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation/priority", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		TotalCookbooks int           `json:"total_cookbooks"`
		Data           []interface{} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalCookbooks != 2 {
		t.Errorf("total_cookbooks = %d, want 2 (no filter should return all)", resp.TotalCookbooks)
	}
	if len(resp.Data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(resp.Data))
	}
}
