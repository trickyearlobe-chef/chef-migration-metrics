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
// handleDependencyGraph — method checks
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dependency-graph status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraph_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dependency-graph status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraph_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dependency-graph status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraph_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — missing organisation parameter
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_MissingOrganisation(t *testing.T) {
	store := &mockStore{}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — organisation not found
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_OrganisationNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — happy path empty
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Organisation string `json:"organisation"`
		Summary      struct {
			TotalNodes    int `json:"total_nodes"`
			TotalEdges    int `json:"total_edges"`
			RoleCount     int `json:"role_count"`
			CookbookCount int `json:"cookbook_count"`
		} `json:"summary"`
		Nodes []any `json:"nodes"`
		Edges []any `json:"edges"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Organisation != "prod" {
		t.Errorf("organisation = %q, want %q", resp.Organisation, "prod")
	}
	if resp.Summary.TotalNodes != 0 {
		t.Errorf("summary.total_nodes = %d, want 0", resp.Summary.TotalNodes)
	}
	if resp.Summary.TotalEdges != 0 {
		t.Errorf("summary.total_edges = %d, want 0", resp.Summary.TotalEdges)
	}
	if len(resp.Nodes) != 0 {
		t.Errorf("len(nodes) = %d, want 0", len(resp.Nodes))
	}
	if len(resp.Edges) != 0 {
		t.Errorf("len(edges) = %d, want 0", len(resp.Edges))
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — happy path with data
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_HappyPath_WithData(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{
					ID:             "dep-1",
					OrganisationID: "org-1",
					RoleName:       "webserver",
					DependencyType: "cookbook",
					DependencyName: "apache2",
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				{
					ID:             "dep-2",
					OrganisationID: "org-1",
					RoleName:       "webserver",
					DependencyType: "cookbook",
					DependencyName: "nginx",
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				{
					ID:             "dep-3",
					OrganisationID: "org-1",
					RoleName:       "webserver",
					DependencyType: "role",
					DependencyName: "base",
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				{
					ID:             "dep-4",
					OrganisationID: "org-1",
					RoleName:       "base",
					DependencyType: "cookbook",
					DependencyName: "ntp",
					CreatedAt:      now,
					UpdatedAt:      now,
				},
			}, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Organisation string `json:"organisation"`
		Summary      struct {
			TotalNodes    int `json:"total_nodes"`
			TotalEdges    int `json:"total_edges"`
			RoleCount     int `json:"role_count"`
			CookbookCount int `json:"cookbook_count"`
		} `json:"summary"`
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"nodes"`
		Edges []struct {
			Source         string `json:"source"`
			Target         string `json:"target"`
			DependencyType string `json:"dependency_type"`
		} `json:"edges"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// Expect 5 nodes: webserver (role), base (role), apache2 (cookbook),
	// nginx (cookbook), ntp (cookbook).
	if resp.Summary.TotalNodes != 5 {
		t.Errorf("summary.total_nodes = %d, want 5", resp.Summary.TotalNodes)
	}
	if resp.Summary.TotalEdges != 4 {
		t.Errorf("summary.total_edges = %d, want 4", resp.Summary.TotalEdges)
	}
	if resp.Summary.RoleCount != 2 {
		t.Errorf("summary.role_count = %d, want 2", resp.Summary.RoleCount)
	}
	if resp.Summary.CookbookCount != 3 {
		t.Errorf("summary.cookbook_count = %d, want 3", resp.Summary.CookbookCount)
	}

	if len(resp.Nodes) != 5 {
		t.Fatalf("len(nodes) = %d, want 5", len(resp.Nodes))
	}
	if len(resp.Edges) != 4 {
		t.Fatalf("len(edges) = %d, want 4", len(resp.Edges))
	}

	// Verify nodes are sorted: cookbooks first (alphabetical), then roles (alphabetical).
	nodeIDs := make([]string, len(resp.Nodes))
	for i, n := range resp.Nodes {
		nodeIDs[i] = n.ID
	}
	expectedNodeIDs := []string{
		"cookbook:apache2", "cookbook:nginx", "cookbook:ntp",
		"role:base", "role:webserver",
	}
	for i, expected := range expectedNodeIDs {
		if i >= len(nodeIDs) {
			t.Errorf("missing node at index %d, want %q", i, expected)
			continue
		}
		if nodeIDs[i] != expected {
			t.Errorf("nodes[%d].id = %q, want %q", i, nodeIDs[i], expected)
		}
	}

	// Verify edge structure.
	foundWebserverApache := false
	foundWebserverBase := false
	foundBaseNtp := false
	for _, e := range resp.Edges {
		if e.Source == "role:webserver" && e.Target == "cookbook:apache2" && e.DependencyType == "cookbook" {
			foundWebserverApache = true
		}
		if e.Source == "role:webserver" && e.Target == "role:base" && e.DependencyType == "role" {
			foundWebserverBase = true
		}
		if e.Source == "role:base" && e.Target == "cookbook:ntp" && e.DependencyType == "cookbook" {
			foundBaseNtp = true
		}
	}
	if !foundWebserverApache {
		t.Error("expected edge webserver -> apache2 not found")
	}
	if !foundWebserverBase {
		t.Error("expected edge webserver -> base not found")
	}
	if !foundBaseNtp {
		t.Error("expected edge base -> ntp not found")
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — DB error
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_DBError_GetOrg(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, errors.New("connection refused")
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleDependencyGraph_DBError_ListDeps(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, errors.New("query timeout")
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — duplicate edges are not created for same role→dep
// ---------------------------------------------------------------------------

func TestHandleDependencyGraph_NodeDeduplication(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			// Two different roles depend on the same cookbook — the cookbook
			// should appear as a single node in the graph.
			return []datastore.RoleDependency{
				{RoleName: "web", DependencyType: "cookbook", DependencyName: "apache2", CreatedAt: now, UpdatedAt: now},
				{RoleName: "proxy", DependencyType: "cookbook", DependencyName: "apache2", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Summary struct {
			TotalNodes    int `json:"total_nodes"`
			TotalEdges    int `json:"total_edges"`
			CookbookCount int `json:"cookbook_count"`
			RoleCount     int `json:"role_count"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// 3 nodes: web (role), proxy (role), apache2 (cookbook).
	if resp.Summary.TotalNodes != 3 {
		t.Errorf("summary.total_nodes = %d, want 3", resp.Summary.TotalNodes)
	}
	// 2 edges: web->apache2, proxy->apache2.
	if resp.Summary.TotalEdges != 2 {
		t.Errorf("summary.total_edges = %d, want 2", resp.Summary.TotalEdges)
	}
	if resp.Summary.CookbookCount != 1 {
		t.Errorf("summary.cookbook_count = %d, want 1", resp.Summary.CookbookCount)
	}
	if resp.Summary.RoleCount != 2 {
		t.Errorf("summary.role_count = %d, want 2", resp.Summary.RoleCount)
	}
}

// ===========================================================================
// handleDependencyGraphTable tests
// ===========================================================================

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — method checks
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /dependency-graph/table status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraphTable_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /dependency-graph/table status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraphTable_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /dependency-graph/table status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDependencyGraphTable_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — missing organisation parameter
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_MissingOrganisation(t *testing.T) {
	store := &mockStore{}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — organisation not found
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_OrganisationNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — happy path empty
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return nil, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Organisation    string `json:"organisation"`
		TotalRoles      int    `json:"total_roles"`
		SharedCookbooks []any  `json:"shared_cookbooks"`
		Data            []any  `json:"data"`
		Pagination      struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Organisation != "prod" {
		t.Errorf("organisation = %q, want %q", resp.Organisation, "prod")
	}
	if resp.TotalRoles != 0 {
		t.Errorf("total_roles = %d, want 0", resp.TotalRoles)
	}
	if len(resp.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(resp.Data))
	}
	if resp.Pagination.TotalItems != 0 {
		t.Errorf("pagination.total_items = %d, want 0", resp.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — happy path with data
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_HappyPath_WithData(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "webserver", CookbookCount: 2, RoleCount: 1, TotalDependency: 3},
				{RoleName: "base", CookbookCount: 1, RoleCount: 0, TotalDependency: 1},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "webserver", DependencyType: "cookbook", DependencyName: "apache2", CreatedAt: now, UpdatedAt: now},
				{RoleName: "webserver", DependencyType: "cookbook", DependencyName: "nginx", CreatedAt: now, UpdatedAt: now},
				{RoleName: "webserver", DependencyType: "role", DependencyName: "base", CreatedAt: now, UpdatedAt: now},
				{RoleName: "base", DependencyType: "cookbook", DependencyName: "ntp", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return []datastore.CookbookRoleCount{
				{CookbookName: "ntp", RoleCount: 3},
				{CookbookName: "apache2", RoleCount: 2},
				{CookbookName: "nginx", RoleCount: 1},
			}, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Organisation    string `json:"organisation"`
		TotalRoles      int    `json:"total_roles"`
		SharedCookbooks []struct {
			CookbookName string `json:"cookbook_name"`
			RoleCount    int    `json:"role_count"`
		} `json:"shared_cookbooks"`
		Data []struct {
			RoleName          string `json:"role_name"`
			CookbookCount     int    `json:"cookbook_count"`
			RoleCount         int    `json:"role_count"`
			TotalDependencies int    `json:"total_dependencies"`
			DependedOnBy      int    `json:"depended_on_by"`
			Dependencies      []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"dependencies"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.TotalRoles != 2 {
		t.Errorf("total_roles = %d, want 2", resp.TotalRoles)
	}

	// Default sort is total_dependencies desc: webserver (3) then base (1).
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].RoleName != "webserver" {
		t.Errorf("data[0].role_name = %q, want %q", resp.Data[0].RoleName, "webserver")
	}
	if resp.Data[0].CookbookCount != 2 {
		t.Errorf("data[0].cookbook_count = %d, want 2", resp.Data[0].CookbookCount)
	}
	if resp.Data[0].RoleCount != 1 {
		t.Errorf("data[0].role_count = %d, want 1", resp.Data[0].RoleCount)
	}
	if resp.Data[0].TotalDependencies != 3 {
		t.Errorf("data[0].total_dependencies = %d, want 3", resp.Data[0].TotalDependencies)
	}
	// base is depended on by webserver.
	if resp.Data[1].RoleName != "base" {
		t.Errorf("data[1].role_name = %q, want %q", resp.Data[1].RoleName, "base")
	}
	if resp.Data[1].DependedOnBy != 1 {
		t.Errorf("data[1].depended_on_by = %d, want 1", resp.Data[1].DependedOnBy)
	}

	// Dependencies should be sorted: cookbooks first then roles.
	if len(resp.Data[0].Dependencies) != 3 {
		t.Fatalf("len(data[0].dependencies) = %d, want 3", len(resp.Data[0].Dependencies))
	}
	if resp.Data[0].Dependencies[0].Type != "cookbook" {
		t.Errorf("data[0].dependencies[0].type = %q, want %q", resp.Data[0].Dependencies[0].Type, "cookbook")
	}
	if resp.Data[0].Dependencies[0].Name != "apache2" {
		t.Errorf("data[0].dependencies[0].name = %q, want %q", resp.Data[0].Dependencies[0].Name, "apache2")
	}

	// Shared cookbooks: ntp (3 roles), apache2 (2 roles). nginx only has 1 role
	// so it should be excluded.
	if len(resp.SharedCookbooks) != 2 {
		t.Fatalf("len(shared_cookbooks) = %d, want 2", len(resp.SharedCookbooks))
	}
	if resp.SharedCookbooks[0].CookbookName != "ntp" {
		t.Errorf("shared_cookbooks[0].cookbook_name = %q, want %q", resp.SharedCookbooks[0].CookbookName, "ntp")
	}
	if resp.SharedCookbooks[0].RoleCount != 3 {
		t.Errorf("shared_cookbooks[0].role_count = %d, want 3", resp.SharedCookbooks[0].RoleCount)
	}

	if resp.Pagination.TotalItems != 2 {
		t.Errorf("pagination.total_items = %d, want 2", resp.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — sorting
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_SortByRoleName(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "zookeeper", CookbookCount: 5, RoleCount: 0, TotalDependency: 5},
				{RoleName: "apache", CookbookCount: 1, RoleCount: 0, TotalDependency: 1},
				{RoleName: "mysql", CookbookCount: 3, RoleCount: 0, TotalDependency: 3},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "zookeeper", DependencyType: "cookbook", DependencyName: "zk", CreatedAt: now, UpdatedAt: now},
				{RoleName: "apache", DependencyType: "cookbook", DependencyName: "httpd", CreatedAt: now, UpdatedAt: now},
				{RoleName: "mysql", DependencyType: "cookbook", DependencyName: "mydb", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod&sort=role_name&order=asc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			RoleName string `json:"role_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(resp.Data))
	}
	if resp.Data[0].RoleName != "apache" {
		t.Errorf("data[0].role_name = %q, want %q", resp.Data[0].RoleName, "apache")
	}
	if resp.Data[1].RoleName != "mysql" {
		t.Errorf("data[1].role_name = %q, want %q", resp.Data[1].RoleName, "mysql")
	}
	if resp.Data[2].RoleName != "zookeeper" {
		t.Errorf("data[2].role_name = %q, want %q", resp.Data[2].RoleName, "zookeeper")
	}
}

func TestHandleDependencyGraphTable_SortByCookbookCountDesc(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "alpha", CookbookCount: 1, RoleCount: 0, TotalDependency: 1},
				{RoleName: "beta", CookbookCount: 5, RoleCount: 0, TotalDependency: 5},
				{RoleName: "gamma", CookbookCount: 3, RoleCount: 0, TotalDependency: 3},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "alpha", DependencyType: "cookbook", DependencyName: "a", CreatedAt: now, UpdatedAt: now},
				{RoleName: "beta", DependencyType: "cookbook", DependencyName: "b", CreatedAt: now, UpdatedAt: now},
				{RoleName: "gamma", DependencyType: "cookbook", DependencyName: "c", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod&sort=cookbook_count&order=desc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			RoleName      string `json:"role_name"`
			CookbookCount int    `json:"cookbook_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(resp.Data))
	}
	if resp.Data[0].RoleName != "beta" {
		t.Errorf("data[0].role_name = %q, want %q", resp.Data[0].RoleName, "beta")
	}
	if resp.Data[1].RoleName != "gamma" {
		t.Errorf("data[1].role_name = %q, want %q", resp.Data[1].RoleName, "gamma")
	}
	if resp.Data[2].RoleName != "alpha" {
		t.Errorf("data[2].role_name = %q, want %q", resp.Data[2].RoleName, "alpha")
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — pagination
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_Pagination(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			counts := make([]datastore.RoleDependencyCount, 7)
			for i := range counts {
				counts[i] = datastore.RoleDependencyCount{
					RoleName:        "role-" + string(rune('a'+i)),
					CookbookCount:   i + 1,
					TotalDependency: i + 1,
				}
			}
			return counts, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			var deps []datastore.RoleDependency
			for i := 0; i < 7; i++ {
				deps = append(deps, datastore.RoleDependency{
					RoleName:       "role-" + string(rune('a'+i)),
					DependencyType: "cookbook",
					DependencyName: "cb-" + string(rune('a'+i)),
					CreatedAt:      now,
					UpdatedAt:      now,
				})
			}
			return deps, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod&page=2&per_page=3", nil)
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

	if len(resp.Data) != 3 {
		t.Errorf("len(data) = %d, want 3", len(resp.Data))
	}
	if resp.Pagination.Page != 2 {
		t.Errorf("pagination.page = %d, want 2", resp.Pagination.Page)
	}
	if resp.Pagination.PerPage != 3 {
		t.Errorf("pagination.per_page = %d, want 3", resp.Pagination.PerPage)
	}
	if resp.Pagination.TotalItems != 7 {
		t.Errorf("pagination.total_items = %d, want 7", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 3 {
		t.Errorf("pagination.total_pages = %d, want 3", resp.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — DB errors
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_DBError_CountDeps(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return nil, errors.New("query timeout")
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleDependencyGraphTable_DBError_ListDeps(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "web", CookbookCount: 1, TotalDependency: 1},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, errors.New("connection refused")
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — shared cookbooks filtering
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_SharedCookbooks_OnlyIncludesMultiRole(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "web", CookbookCount: 2, TotalDependency: 2},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "web", DependencyType: "cookbook", DependencyName: "shared-cb", CreatedAt: now, UpdatedAt: now},
				{RoleName: "web", DependencyType: "cookbook", DependencyName: "solo-cb", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return []datastore.CookbookRoleCount{
				{CookbookName: "shared-cb", RoleCount: 5},
				{CookbookName: "solo-cb", RoleCount: 1},
			}, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		SharedCookbooks []struct {
			CookbookName string `json:"cookbook_name"`
			RoleCount    int    `json:"role_count"`
		} `json:"shared_cookbooks"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// Only shared-cb (role_count >= 2) should appear.
	if len(resp.SharedCookbooks) != 1 {
		t.Fatalf("len(shared_cookbooks) = %d, want 1", len(resp.SharedCookbooks))
	}
	if resp.SharedCookbooks[0].CookbookName != "shared-cb" {
		t.Errorf("shared_cookbooks[0].cookbook_name = %q, want %q", resp.SharedCookbooks[0].CookbookName, "shared-cb")
	}
	if resp.SharedCookbooks[0].RoleCount != 5 {
		t.Errorf("shared_cookbooks[0].role_count = %d, want 5", resp.SharedCookbooks[0].RoleCount)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — transitive (depended_on_by) counts
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_TransitiveCounts(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "app", CookbookCount: 0, RoleCount: 1, TotalDependency: 1},
				{RoleName: "web", CookbookCount: 0, RoleCount: 1, TotalDependency: 1},
				{RoleName: "base", CookbookCount: 1, RoleCount: 0, TotalDependency: 1},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				// Both app and web depend on base.
				{RoleName: "app", DependencyType: "role", DependencyName: "base", CreatedAt: now, UpdatedAt: now},
				{RoleName: "web", DependencyType: "role", DependencyName: "base", CreatedAt: now, UpdatedAt: now},
				{RoleName: "base", DependencyType: "cookbook", DependencyName: "ntp", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod&sort=role_name&order=asc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			RoleName     string `json:"role_name"`
			DependedOnBy int    `json:"depended_on_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(resp.Data))
	}

	// Sorted by role_name asc: app, base, web.
	// app: no roles depend on it → 0
	// base: both app and web depend on it → 2
	// web: no roles depend on it → 0
	for _, row := range resp.Data {
		switch row.RoleName {
		case "app":
			if row.DependedOnBy != 0 {
				t.Errorf("app.depended_on_by = %d, want 0", row.DependedOnBy)
			}
		case "base":
			if row.DependedOnBy != 2 {
				t.Errorf("base.depended_on_by = %d, want 2", row.DependedOnBy)
			}
		case "web":
			if row.DependedOnBy != 0 {
				t.Errorf("web.depended_on_by = %d, want 0", row.DependedOnBy)
			}
		default:
			t.Errorf("unexpected role_name: %q", row.RoleName)
		}
	}
}

// ---------------------------------------------------------------------------
// Route registration — dependency graph routes do not shadow each other
// ---------------------------------------------------------------------------

func TestDependencyGraphRoutes_NotFallingThrough(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return nil, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	paths := []string{
		"/api/v1/dependency-graph?organisation=prod",
		"/api/v1/dependency-graph/table?organisation=prod",
	}

	for _, path := range paths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)

		// Both should succeed (200) — they should NOT return 404 or 501.
		if w.Code == http.StatusNotFound || w.Code == http.StatusNotImplemented {
			t.Errorf("GET %s returned %d — route not properly registered", path, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — CountRolesPerCookbook error is non-fatal
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_CountRolesPerCookbook_ErrorNonFatal(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				{RoleName: "web", CookbookCount: 1, TotalDependency: 1},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "web", DependencyType: "cookbook", DependencyName: "apache2", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, errors.New("some transient error")
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	// Should still return 200 because CountRolesPerCookbook failure is non-fatal.
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — shared cookbooks limited to 20
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_SharedCookbooks_LimitedTo20(t *testing.T) {
	now := time.Now()

	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return nil, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{RoleName: "web", DependencyType: "cookbook", DependencyName: "cb", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			// 25 cookbooks, all shared by >= 2 roles.
			counts := make([]datastore.CookbookRoleCount, 25)
			for i := range counts {
				counts[i] = datastore.CookbookRoleCount{
					CookbookName: "cb-" + string(rune('a'+i)),
					RoleCount:    25 - i,
				}
			}
			return counts, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		SharedCookbooks []any `json:"shared_cookbooks"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.SharedCookbooks) != 20 {
		t.Errorf("len(shared_cookbooks) = %d, want 20 (capped)", len(resp.SharedCookbooks))
	}
}

// ---------------------------------------------------------------------------
// isNotFound helper
// ---------------------------------------------------------------------------

func TestIsNotFound(t *testing.T) {
	if !isNotFound(datastore.ErrNotFound) {
		t.Error("isNotFound(datastore.ErrNotFound) = false, want true")
	}

	if isNotFound(nil) {
		t.Error("isNotFound(nil) = true, want false")
	}

	if isNotFound(errors.New("some other error")) {
		t.Error("isNotFound(other error) = true, want false")
	}

	if !isNotFound(errors.New("not found")) {
		t.Error("isNotFound(errors.New(\"not found\")) = false, want true")
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraphTable — dependencies field is never null
// ---------------------------------------------------------------------------

func TestHandleDependencyGraphTable_DependenciesNeverNull(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return []datastore.RoleDependencyCount{
				// Role with dependencies returned by CountDependenciesByRole
				// but no matching records in ListRoleDependenciesByOrg.
				{RoleName: "orphan-role", CookbookCount: 0, RoleCount: 0, TotalDependency: 0},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Parse the raw JSON to check that dependencies is an array, not null.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding raw response: %v", err)
	}

	var dataItems []map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &dataItems); err != nil {
		t.Fatalf("decoding data items: %v", err)
	}

	if len(dataItems) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(dataItems))
	}

	depsRaw := string(dataItems[0]["dependencies"])
	if depsRaw == "null" {
		t.Error("dependencies should be [] not null")
	}
	if depsRaw != "[]" {
		t.Errorf("dependencies = %s, want []", depsRaw)
	}
}

// ---------------------------------------------------------------------------
// handleDependencyGraph — verify that /table does not shadow /
// ---------------------------------------------------------------------------

func TestDependencyGraph_TableDoesNotShadowBase(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, nil
		},
		CountDependenciesByRoleFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
			return nil, nil
		},
		CountRolesPerCookbookFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	r := newTestRouterWithMockAndConfig(store, cfg)

	// Graph endpoint should return nodes/edges structure.
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph?organisation=prod", nil)
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("GET /dependency-graph status = %d, want %d", w1.Code, http.StatusOK)
	}

	var graphResp map[string]json.RawMessage
	if err := json.Unmarshal(w1.Body.Bytes(), &graphResp); err != nil {
		t.Fatalf("decoding graph response: %v", err)
	}
	if _, ok := graphResp["nodes"]; !ok {
		t.Error("graph response should have 'nodes' field")
	}
	if _, ok := graphResp["edges"]; !ok {
		t.Error("graph response should have 'edges' field")
	}

	// Table endpoint should return data + pagination structure.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dependency-graph/table?organisation=prod", nil)
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("GET /dependency-graph/table status = %d, want %d", w2.Code, http.StatusOK)
	}

	var tableResp map[string]json.RawMessage
	if err := json.Unmarshal(w2.Body.Bytes(), &tableResp); err != nil {
		t.Fatalf("decoding table response: %v", err)
	}
	if _, ok := tableResp["data"]; !ok {
		t.Error("table response should have 'data' field")
	}
	if _, ok := tableResp["pagination"]; !ok {
		t.Error("table response should have 'pagination' field")
	}
	// table should NOT have nodes/edges
	if _, ok := tableResp["nodes"]; ok {
		t.Error("table response should NOT have 'nodes' field")
	}
}

// ---------------------------------------------------------------------------
// helpers for this test file
// ---------------------------------------------------------------------------

// newTestRouterWithMockAndConfigDG builds a Router with dependency graph
// test-friendly config. This uses the shared helpers from store_mock_test.go.
func newTestRouterWithMockAndConfigDG(store *mockStore, cfg *config.Config) *Router {
	return newTestRouterWithMockAndConfig(store, cfg)
}
