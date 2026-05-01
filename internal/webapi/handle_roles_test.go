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
// GET /api/v1/roles — list
// ---------------------------------------------------------------------------

func TestHandleRoles_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /roles status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRoles_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := body["data"]; !ok {
		t.Error("response missing 'data' key")
	}
	if _, ok := body["summary"]; !ok {
		t.Error("response missing 'summary' key")
	}
	if _, ok := body["pagination"]; !ok {
		t.Error("response missing 'pagination' key")
	}
}

func TestHandleRoles_HappyPath_WithRows(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			rows := []datastore.RoleFilterRow{
				{
					RoleName:            "webserver",
					Organisations:       []string{"prod"},
					NodeCount:           100,
					DirectCookbookCount: 3,
					TotalCookbookCount:  7,
					CompatibilityStatus: "incompatible",
					CompatibleCount:     5,
					IncompatibleCount:   2,
				},
				{
					RoleName:            "base",
					Organisations:       []string{"prod"},
					NodeCount:           200,
					DirectCookbookCount: 2,
					TotalCookbookCount:  2,
					CompatibilityStatus: "compatible",
					CompatibleCount:     2,
				},
			}
			summary := datastore.RoleFilterSummary{
				TargetChefVersion: "18.5.0",
				CompatibleRoles:   1,
				IncompatibleRoles: 1,
				TotalRoles:        2,
			}
			return rows, 2, summary, nil
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Data []struct {
			RoleName            string   `json:"role_name"`
			Organisations       []string `json:"organisations"`
			NodeCount           int      `json:"node_count"`
			CompatibilityStatus string   `json:"compatibility_status"`
			TotalCookbookCount  int      `json:"total_cookbook_count"`
		} `json:"data"`
		Summary struct {
			CompatibleRoles   int `json:"compatible_roles"`
			IncompatibleRoles int `json:"incompatible_roles"`
			TotalRoles        int `json:"total_roles"`
		} `json:"summary"`
		Pagination PaginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(body.Data) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(body.Data))
	}
	if body.Data[0].RoleName != "webserver" {
		t.Errorf("expected first role = webserver, got %q", body.Data[0].RoleName)
	}
	if body.Data[0].NodeCount != 100 {
		t.Errorf("expected node_count = 100, got %d", body.Data[0].NodeCount)
	}
	if body.Data[0].CompatibilityStatus != "incompatible" {
		t.Errorf("expected compatibility_status = incompatible, got %q", body.Data[0].CompatibilityStatus)
	}
	if body.Data[0].TotalCookbookCount != 7 {
		t.Errorf("expected total_cookbook_count = 7, got %d", body.Data[0].TotalCookbookCount)
	}
	if body.Summary.TotalRoles != 2 {
		t.Errorf("expected summary total_roles = 2, got %d", body.Summary.TotalRoles)
	}
	if body.Summary.CompatibleRoles != 1 {
		t.Errorf("expected summary compatible_roles = 1, got %d", body.Summary.CompatibleRoles)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("expected pagination total_items = 2, got %d", body.Pagination.TotalItems)
	}
}

func TestHandleRoles_FiltersPassedToDatastore(t *testing.T) {
	var capturedFilter datastore.RoleFilter

	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			capturedFilter = f
			return nil, 0, datastore.RoleFilterSummary{}, nil
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/roles?name=web&compatibility_status=incompatible&sort=node_count&order=desc&page=2&per_page=10",
		nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if capturedFilter.Name != "web" {
		t.Errorf("expected name filter = 'web', got %q", capturedFilter.Name)
	}
	if capturedFilter.CompatibilityStatus != "incompatible" {
		t.Errorf("expected compatibility_status = 'incompatible', got %q", capturedFilter.CompatibilityStatus)
	}
	if capturedFilter.Sort != "node_count" {
		t.Errorf("expected sort = 'node_count', got %q", capturedFilter.Sort)
	}
	if capturedFilter.SortOrder != "desc" {
		t.Errorf("expected order = 'desc', got %q", capturedFilter.SortOrder)
	}
	if capturedFilter.Offset != 10 {
		t.Errorf("expected offset = 10 (page 2 * per_page 10), got %d", capturedFilter.Offset)
	}
	if capturedFilter.Limit != 10 {
		t.Errorf("expected limit = 10, got %d", capturedFilter.Limit)
	}
}

func TestHandleRoles_DBError_ListOrganisations(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleRoles_DBError_ListRolesFiltered(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			return nil, 0, datastore.RoleFilterSummary{}, errors.New("query timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleRoles_NilDataReturnedAsEmptyArray(t *testing.T) {
	store := &mockStore{
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			return nil, 0, datastore.RoleFilterSummary{}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Data == nil {
		t.Error("data should be empty array, not null")
	}
}

// ---------------------------------------------------------------------------
// Role compat summary cache tests
// ---------------------------------------------------------------------------

func TestHandleRoles_Cache_FetchesOnFirstRequest(t *testing.T) {
	calls := 0
	expectedSummary := datastore.RoleFilterSummary{
		TargetChefVersion: "",
		CompatibleRoles:   3,
		IncompatibleRoles: 1,
		UntestedRoles:     2,
		TotalRoles:        6,
	}
	store := &mockStore{
		GetRoleCompatSummaryFn: func(ctx context.Context, f datastore.RoleFilter) (datastore.RoleFilterSummary, map[string]string, error) {
			calls++
			return expectedSummary, map[string]string{"role-a": "compatible"}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if calls != 1 {
		t.Errorf("GetRoleCompatSummaryFn called %d times on first request, want 1", calls)
	}

	var body struct {
		Summary datastore.RoleFilterSummary `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Summary.CompatibleRoles != expectedSummary.CompatibleRoles {
		t.Errorf("summary.compatible_roles = %d, want %d", body.Summary.CompatibleRoles, expectedSummary.CompatibleRoles)
	}
	if body.Summary.TotalRoles != expectedSummary.TotalRoles {
		t.Errorf("summary.total_roles = %d, want %d", body.Summary.TotalRoles, expectedSummary.TotalRoles)
	}
}

func TestHandleRoles_Cache_DoesNotFetchOnSecondRequestWithinTTL(t *testing.T) {
	calls := 0
	expectedSummary := datastore.RoleFilterSummary{
		CompatibleRoles:   5,
		IncompatibleRoles: 2,
		TotalRoles:        7,
	}
	store := &mockStore{
		GetRoleCompatSummaryFn: func(ctx context.Context, f datastore.RoleFilter) (datastore.RoleFilterSummary, map[string]string, error) {
			calls++
			return expectedSummary, map[string]string{"role-b": "incompatible"}, nil
		},
	}
	r := newTestRouterWithMock(store)

	// First request — populates cache.
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", w1.Code, http.StatusOK)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call after first request, got %d", calls)
	}

	// Second request within TTL — must NOT call GetRoleCompatSummaryFn again.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want %d", w2.Code, http.StatusOK)
	}
	if calls != 1 {
		t.Errorf("GetRoleCompatSummaryFn called %d times after second request within TTL, want 1", calls)
	}

	// Summary should be served from cache on the second request.
	var body struct {
		Summary datastore.RoleFilterSummary `json:"summary"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Summary.CompatibleRoles != expectedSummary.CompatibleRoles {
		t.Errorf("cached summary.compatible_roles = %d, want %d", body.Summary.CompatibleRoles, expectedSummary.CompatibleRoles)
	}
	if body.Summary.TotalRoles != expectedSummary.TotalRoles {
		t.Errorf("cached summary.total_roles = %d, want %d", body.Summary.TotalRoles, expectedSummary.TotalRoles)
	}
}

func TestHandleRoles_Cache_PrecomputedCompatMapPassedToListRolesFiltered(t *testing.T) {
	compatMap := map[string]string{"role-a": "compatible", "role-b": "incompatible"}
	var capturedFilter datastore.RoleFilter
	store := &mockStore{
		GetRoleCompatSummaryFn: func(ctx context.Context, f datastore.RoleFilter) (datastore.RoleFilterSummary, map[string]string, error) {
			return datastore.RoleFilterSummary{TotalRoles: 2}, compatMap, nil
		},
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			capturedFilter = f
			return nil, 0, datastore.RoleFilterSummary{}, nil
		},
	}
	r := newTestRouterWithMock(store)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))

	if len(capturedFilter.PrecomputedCompatMap) != len(compatMap) {
		t.Errorf("PrecomputedCompatMap len = %d, want %d", len(capturedFilter.PrecomputedCompatMap), len(compatMap))
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/roles/:name — detail
// ---------------------------------------------------------------------------

func TestHandleRoleDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roles/webserver", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /roles/webserver status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRoleDetail_NotFound(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return nil, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleRoleDetail_HappyPath(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			if roleName != "webserver" {
				return nil, datastore.ErrNotFound
			}
			return &datastore.RoleDetail{
				RoleName:            "webserver",
				Organisations:       []string{"prod", "staging"},
				NodeCount:           500,
				DirectCookbooks:     []string{"nginx", "ssl"},
				DirectRoles:         []string{"base"},
				TransitiveCookbooks: []string{"nginx", "ssl", "apt", "base-cookbook"},
				BlockingCookbooks: []datastore.BlockingCookbook{
					{
						CookbookName:      "nginx",
						CookbookVersion:   "5.1.0",
						TargetChefVersion: "19.0.0",
						ComplexityScore:   30,
						ComplexityLabel:   "medium",
						AutoCorrectable:   4,
						ManualFix:         3,
						DependencyPath:    []string{"role:webserver", "cookbook:nginx"},
					},
				},
				NestedRoleChain: &datastore.RoleChainNode{
					Name: "webserver",
					Type: "role",
					Children: []*datastore.RoleChainNode{
						{Name: "base", Type: "role", Children: []*datastore.RoleChainNode{
							{Name: "apt", Type: "cookbook", CompatibilityStatus: "compatible"},
						}},
						{Name: "nginx", Type: "cookbook", CompatibilityStatus: "incompatible"},
					},
				},
				NodesByOrganisation: []datastore.OrgCount{
					{Organisation: "prod", Count: 480},
					{Organisation: "staging", Count: 20},
				},
				NodesByEnvironment: []datastore.EnvCount{
					{Environment: "production", Count: 450},
					{Environment: "staging", Count: 50},
				},
				NodesByPlatform: []datastore.PlatformCount{
					{Platform: "ubuntu", PlatformVersion: "22.04", Count: 400},
					{Platform: "centos", PlatformVersion: "7", Count: 100},
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		RoleName            string   `json:"role_name"`
		Organisations       []string `json:"organisations"`
		NodeCount           int      `json:"node_count"`
		DirectCookbooks     []string `json:"direct_cookbooks"`
		DirectRoles         []string `json:"direct_roles"`
		TransitiveCookbooks []string `json:"transitive_cookbooks"`
		BlockingCookbooks   []struct {
			CookbookName    string   `json:"cookbook_name"`
			ComplexityScore int      `json:"complexity_score"`
			DependencyPath  []string `json:"dependency_path"`
		} `json:"blocking_cookbooks"`
		NestedRoleChain struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Children []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"children"`
		} `json:"nested_role_chain"`
		NodesByOrganisation []struct {
			Organisation string `json:"organisation"`
			Count        int    `json:"count"`
		} `json:"nodes_by_organisation"`
		NodesByEnvironment []struct {
			Environment string `json:"environment"`
			Count       int    `json:"count"`
		} `json:"nodes_by_environment"`
		NodesByPlatform []struct {
			Platform        string `json:"platform"`
			PlatformVersion string `json:"platform_version"`
			Count           int    `json:"count"`
		} `json:"nodes_by_platform"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body.RoleName != "webserver" {
		t.Errorf("role_name = %q, want webserver", body.RoleName)
	}
	if len(body.Organisations) != 2 {
		t.Errorf("expected 2 organisations, got %d", len(body.Organisations))
	}
	if body.NodeCount != 500 {
		t.Errorf("node_count = %d, want 500", body.NodeCount)
	}
	if len(body.DirectCookbooks) != 2 {
		t.Errorf("expected 2 direct_cookbooks, got %d", len(body.DirectCookbooks))
	}
	if len(body.DirectRoles) != 1 {
		t.Errorf("expected 1 direct_roles, got %d", len(body.DirectRoles))
	}
	if len(body.TransitiveCookbooks) != 4 {
		t.Errorf("expected 4 transitive_cookbooks, got %d", len(body.TransitiveCookbooks))
	}
	if len(body.BlockingCookbooks) != 1 {
		t.Fatalf("expected 1 blocking_cookbooks, got %d", len(body.BlockingCookbooks))
	}
	if body.BlockingCookbooks[0].CookbookName != "nginx" {
		t.Errorf("blocking cookbook name = %q, want nginx", body.BlockingCookbooks[0].CookbookName)
	}
	if body.BlockingCookbooks[0].ComplexityScore != 30 {
		t.Errorf("blocking cookbook complexity_score = %d, want 30", body.BlockingCookbooks[0].ComplexityScore)
	}
	if len(body.BlockingCookbooks[0].DependencyPath) != 2 {
		t.Errorf("expected 2 path segments, got %d", len(body.BlockingCookbooks[0].DependencyPath))
	}
	if body.NestedRoleChain.Name != "webserver" {
		t.Errorf("nested_role_chain root = %q, want webserver", body.NestedRoleChain.Name)
	}
	if len(body.NestedRoleChain.Children) != 2 {
		t.Errorf("expected 2 chain children, got %d", len(body.NestedRoleChain.Children))
	}
	if len(body.NodesByOrganisation) != 2 {
		t.Errorf("expected 2 nodes_by_organisation entries, got %d", len(body.NodesByOrganisation))
	}
	if len(body.NodesByEnvironment) != 2 {
		t.Errorf("expected 2 nodes_by_environment entries, got %d", len(body.NodesByEnvironment))
	}
	if len(body.NodesByPlatform) != 2 {
		t.Errorf("expected 2 nodes_by_platform entries, got %d", len(body.NodesByPlatform))
	}
}

func TestHandleRoleDetail_TargetVersionPassedThrough(t *testing.T) {
	var capturedVersion string
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			capturedVersion = targetChefVersion
			return &datastore.RoleDetail{
				RoleName:            roleName,
				Organisations:       []string{},
				DirectCookbooks:     []string{},
				DirectRoles:         []string{},
				TransitiveCookbooks: []string{},
				BlockingCookbooks:   []datastore.BlockingCookbook{},
				NodesByOrganisation: []datastore.OrgCount{},
				NodesByEnvironment:  []datastore.EnvCount{},
				NodesByPlatform:     []datastore.PlatformCount{},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver?target_chef_version=19.0.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedVersion != "19.0.0" {
		t.Errorf("target_chef_version = %q, want 19.0.0", capturedVersion)
	}
}

func TestHandleRoleDetail_DBError(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return nil, errors.New("database exploded")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/roles/:name/dependency-graph
// ---------------------------------------------------------------------------

func TestHandleRoleDependencyGraph_NotFound(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return nil, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/nonexistent/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleRoleDependencyGraph_HappyPath(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            "webserver",
				Organisations:       []string{"prod"},
				DirectCookbooks:     []string{},
				DirectRoles:         []string{},
				TransitiveCookbooks: []string{},
				BlockingCookbooks:   []datastore.BlockingCookbook{},
				NestedRoleChain: &datastore.RoleChainNode{
					Name: "webserver",
					Type: "role",
					Children: []*datastore.RoleChainNode{
						{Name: "nginx", Type: "cookbook", CompatibilityStatus: "incompatible"},
					},
				},
				NodesByOrganisation: []datastore.OrgCount{},
				NodesByEnvironment:  []datastore.EnvCount{},
				NodesByPlatform:     []datastore.PlatformCount{},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{OrganisationName: "prod", RoleName: "webserver", DependencyType: "role", DependencyName: "base"},
				{OrganisationName: "prod", RoleName: "webserver", DependencyType: "cookbook", DependencyName: "nginx"},
				{OrganisationName: "prod", RoleName: "base", DependencyType: "cookbook", DependencyName: "apt"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Nodes []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
		Metadata struct {
			TotalRoles     int `json:"total_roles"`
			TotalCookbooks int `json:"total_cookbooks"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have 2 roles (webserver, base) and 2 cookbooks (nginx, apt).
	if body.Metadata.TotalRoles != 2 {
		t.Errorf("total_roles = %d, want 2", body.Metadata.TotalRoles)
	}
	if body.Metadata.TotalCookbooks != 2 {
		t.Errorf("total_cookbooks = %d, want 2", body.Metadata.TotalCookbooks)
	}
	if len(body.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(body.Nodes))
	}
	if len(body.Edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(body.Edges))
	}

	// Verify nodes are sorted (cookbook < role, then by name).
	if len(body.Nodes) >= 4 {
		if body.Nodes[0].Type != "cookbook" {
			t.Errorf("first node type = %q, want cookbook (sorted)", body.Nodes[0].Type)
		}
		if body.Nodes[2].Type != "role" {
			t.Errorf("third node type = %q, want role (sorted)", body.Nodes[2].Type)
		}
	}
}

func TestHandleRoleDependencyGraph_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roles/webserver/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST dependency-graph status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRoleDependencyGraph_DBError(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            "webserver",
				Organisations:       []string{"prod"},
				DirectCookbooks:     []string{},
				DirectRoles:         []string{},
				TransitiveCookbooks: []string{},
				BlockingCookbooks:   []datastore.BlockingCookbook{},
				NodesByOrganisation: []datastore.OrgCount{},
				NodesByEnvironment:  []datastore.EnvCount{},
				NodesByPlatform:     []datastore.PlatformCount{},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return nil, errors.New("disk full")
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleRoleDependencyGraph_CookbookTransitiveDeps(t *testing.T) {
	// role:webserver → cookbook:nginx; nginx → apt (via ListCookbookDependenciesByOrg).
	// The graph should include an apt node and an edge nginx→apt.
	store := &mockStore{
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            "webserver",
				Organisations:       []string{"prod"},
				DirectCookbooks:     []string{},
				DirectRoles:         []string{},
				TransitiveCookbooks: []string{},
				BlockingCookbooks:   []datastore.BlockingCookbook{},
				NodesByOrganisation: []datastore.OrgCount{},
				NodesByEnvironment:  []datastore.EnvCount{},
				NodesByPlatform:     []datastore.PlatformCount{},
			}, nil
		},
		ListRoleDependenciesByOrgFn: func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
			return []datastore.RoleDependency{
				{OrganisationName: "prod", RoleName: "webserver", DependencyType: "cookbook", DependencyName: "nginx"},
			}, nil
		},
		ListCookbookDependenciesByOrgFn: func(ctx context.Context, orgName string) (map[string][]string, error) {
			return map[string][]string{
				"nginx": {"apt"},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles/webserver/dependency-graph", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Nodes []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
		Metadata struct {
			TotalRoles     int `json:"total_roles"`
			TotalCookbooks int `json:"total_cookbooks"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Expect: 1 role (webserver), 2 cookbooks (nginx, apt).
	if body.Metadata.TotalRoles != 1 {
		t.Errorf("total_roles = %d, want 1", body.Metadata.TotalRoles)
	}
	if body.Metadata.TotalCookbooks != 2 {
		t.Errorf("total_cookbooks = %d, want 2; nodes = %v", body.Metadata.TotalCookbooks, body.Nodes)
	}

	// Find the apt node.
	nodeNames := make(map[string]bool)
	for _, n := range body.Nodes {
		nodeNames[n.Name] = true
	}
	if !nodeNames["apt"] {
		t.Errorf("expected apt node in graph, got nodes: %v", body.Nodes)
	}

	// Find the nginx→apt edge.
	foundEdge := false
	for _, e := range body.Edges {
		if e.From == "cookbook:nginx" && e.To == "cookbook:apt" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Errorf("expected nginx→apt edge, got edges: %v", body.Edges)
	}
}

// ---------------------------------------------------------------------------
// Route registration — verify role routes exist in the mux
// ---------------------------------------------------------------------------

func TestRoleRoutesRegistered(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListRolesFilteredFn: func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			return nil, 0, datastore.RoleFilterSummary{}, nil
		},
		GetRoleDetailFn: func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
			return nil, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	tests := []struct {
		path       string
		wantStatus int // expected status with mock data
	}{
		{"/api/v1/roles", http.StatusOK},
		{"/api/v1/roles/webserver", http.StatusNotFound},                  // mock returns ErrNotFound
		{"/api/v1/roles/webserver/dependency-graph", http.StatusNotFound}, // mock returns ErrNotFound
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GET %s status = %d, want %d; body = %s",
					tt.path, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
