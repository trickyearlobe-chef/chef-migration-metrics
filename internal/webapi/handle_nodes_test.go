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

// testConfig builds a minimal config suitable for testing.
func testConfig() *config.Config {
	wsEnabled := true
	cfg := &config.Config{}
	cfg.Server.WebSocket.Enabled = &wsEnabled
	return cfg
}

// testRouter builds a Router suitable for testing. The db is nil so handlers
// that touch the database will fail — use this for route-wiring and
// method-check tests only.
func testRouter() *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(nil, cfg, hub)
	return r
}

// ---------------------------------------------------------------------------
// nodeUsesCookbook tests
// ---------------------------------------------------------------------------

func TestNodeUsesCookbook_Match(t *testing.T) {
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`{"apt":{"version":"7.4.0"},"nginx":{"version":"2.0.0"}}`),
	}
	if !nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return true for 'apt'")
	}
	if !nodeUsesCookbook(n, "nginx") {
		t.Error("expected nodeUsesCookbook to return true for 'nginx'")
	}
}

func TestNodeUsesCookbook_NoMatch(t *testing.T) {
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`{"apt":{"version":"7.4.0"}}`),
	}
	if nodeUsesCookbook(n, "nginx") {
		t.Error("expected nodeUsesCookbook to return false for 'nginx'")
	}
}

func TestNodeUsesCookbook_EmptyCookbooks(t *testing.T) {
	n := datastore.NodeSnapshot{}
	if nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return false for empty cookbooks")
	}
}

func TestNodeUsesCookbook_NullCookbooks(t *testing.T) {
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`null`),
	}
	// json.RawMessage(`null`) has length 4, not 0.
	// The substring check should not match.
	if nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return false for null cookbooks")
	}
}

func TestNodeUsesCookbook_PartialNameNoFalsePositive(t *testing.T) {
	// "apt-repo" contains "apt" as a substring but the JSON key check
	// uses the quoted form "apt" which should NOT match "apt-repo".
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`{"apt-repo":{"version":"1.0.0"}}`),
	}
	if nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return false — 'apt' != 'apt-repo'")
	}
}

func TestNodeUsesCookbook_NameInValueNoFalsePositive(t *testing.T) {
	// The cookbook name "apt" appears as a value (inside "source") but is
	// NOT a top-level key. A substring-based check would false-positive;
	// the JSON-parse approach should not.
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`{"some_cookbook":{"version":"1.0.0","source":"apt"}}`),
	}
	if nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return false — 'apt' is a value, not a key")
	}
}

func TestNodeUsesCookbook_InvalidJSON(t *testing.T) {
	n := datastore.NodeSnapshot{
		Cookbooks: json.RawMessage(`{not valid json`),
	}
	if nodeUsesCookbook(n, "apt") {
		t.Error("expected nodeUsesCookbook to return false for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// Route wiring tests — verify method checks and 404s
// ---------------------------------------------------------------------------

func TestHandleNodes_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/nodes status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodesByVersion_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/by-version/17.0.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /nodes/by-version status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodesByCookbook_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/by-cookbook/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /nodes/by-cookbook status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodeDetail_NotEnoughSegments(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	// Only one segment — should 404 with a helpful message.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/someorg", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/nodes/someorg status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

func TestHandleNodeDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/myorg/mynode", nil)
	r.ServeHTTP(w, req)

	// Should return 405 because POST is not allowed even with valid segments.
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /nodes/myorg/mynode status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodesByVersion_MissingVersion(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-version/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /nodes/by-version/ (no version) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleNodesByCookbook_MissingName(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-cookbook/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /nodes/by-cookbook/ (no name) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleNodes — happy path with mock DB
// ---------------------------------------------------------------------------

func TestHandleNodes_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
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

func TestHandleNodes_HappyPath_WithNodes(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListNodeSnapshotsFilteredFn: func(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
			return []datastore.NodeSnapshot{
				{OrganisationName: "org-1", NodeName: "web1", ChefVersion: "18.0.0", CollectedAt: now},
				{OrganisationName: "org-1", NodeName: "web2", ChefVersion: "17.0.0", CollectedAt: now},
			}, 2, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", body.Pagination.TotalItems)
	}
}

func TestHandleNodes_DBError_ListOrganisations(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleNodeDetail — happy path, not-found, DB errors
// ---------------------------------------------------------------------------

func TestHandleNodeDetail_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{NodeName: "web1", OrganisationName: "org-1", CollectedAt: now}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/prod/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Node struct {
			NodeName string `json:"node_name"`
		} `json:"node"`
		OrganisationName string `json:"organisation_name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Node.NodeName != "web1" {
		t.Errorf("node_name = %q, want %q", body.Node.NodeName, "web1")
	}
	if body.OrganisationName != "prod" {
		t.Errorf("organisation_name = %q, want %q", body.OrganisationName, "prod")
	}
}

func TestHandleNodeDetail_OrgNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/nope/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleNodeDetail_NodeNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/prod/missing", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleNodeDetail_OrgDBError(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, errors.New("db down")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/prod/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleNodesByVersion — happy path, DB error
// ---------------------------------------------------------------------------

func TestHandleNodesByVersion_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListNodeSnapshotsFilteredFn: func(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
			// The handler sets ChefVersionExact, so only matching nodes returned.
			if f.ChefVersionExact == "18.0.0" {
				return []datastore.NodeSnapshot{
					{NodeName: "web1", ChefVersion: "18.0.0", CollectedAt: now},
				}, 1, nil
			}
			return nil, 0, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-version/18.0.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Total != 1 {
		t.Errorf("total = %d, want 1", body.Total)
	}
}

func TestHandleNodesByVersion_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-version/18.0.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleNodesByCookbook — happy path, DB error
// ---------------------------------------------------------------------------

func TestHandleNodesByCookbook_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListNodeSnapshotsFilteredFn: func(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
			return []datastore.NodeSnapshot{
				{OrganisationName: "org-1", NodeName: "web1", Cookbooks: json.RawMessage(`{"apt":{"version":"7.0"}}`), CollectedAt: now},
				{OrganisationName: "org-1", NodeName: "web2", Cookbooks: json.RawMessage(`{"nginx":{"version":"2.0"}}`), CollectedAt: now},
			}, 2, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-cookbook/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Total != 1 {
		t.Errorf("total = %d, want 1", body.Total)
	}
}

func TestHandleNodesByCookbook_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/by-cookbook/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// nodeSnapshotFilterFromRequest tests
// ---------------------------------------------------------------------------

func TestNodeSnapshotFilterFromRequest_SingleValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nodes?environment=prod&platform=centos&chef_version=18.5.0&policy_name=base&policy_group=default", nil)
	f := nodeSnapshotFilterFromRequest(req, []string{"org-1"}, 24, 7)

	if f.Environment != "prod" {
		t.Errorf("Environment = %q, want %q", f.Environment, "prod")
	}
	if len(f.Environments) != 0 {
		t.Errorf("Environments should be empty for single value, got %v", f.Environments)
	}
	if f.Platform != "centos" {
		t.Errorf("Platform = %q, want %q", f.Platform, "centos")
	}
	if f.ChefVersion != "18.5.0" {
		t.Errorf("ChefVersion = %q, want %q", f.ChefVersion, "18.5.0")
	}
	if f.PolicyName != "base" {
		t.Errorf("PolicyName = %q, want %q", f.PolicyName, "base")
	}
	if f.PolicyGroup != "default" {
		t.Errorf("PolicyGroup = %q, want %q", f.PolicyGroup, "default")
	}
}

func TestNodeSnapshotFilterFromRequest_CommaSeparatedValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nodes?environment=prod,staging&platform=centos+7,ubuntu+20.04&chef_version=18.5.0,17.10.0&policy_name=base,web&policy_group=prod,staging", nil)
	f := nodeSnapshotFilterFromRequest(req, []string{"org-1"}, 24, 7)

	// Comma-separated values should populate the slice fields.
	if len(f.Environments) != 2 || f.Environments[0] != "prod" || f.Environments[1] != "staging" {
		t.Errorf("Environments = %v, want [prod staging]", f.Environments)
	}
	if f.Environment != "" {
		t.Errorf("Environment should be empty when comma-separated, got %q", f.Environment)
	}
	if len(f.Platforms) != 2 || f.Platforms[0] != "centos 7" || f.Platforms[1] != "ubuntu 20.04" {
		t.Errorf("Platforms = %v, want [centos 7 ubuntu 20.04]", f.Platforms)
	}
	if len(f.ChefVersions) != 2 || f.ChefVersions[0] != "18.5.0" || f.ChefVersions[1] != "17.10.0" {
		t.Errorf("ChefVersions = %v, want [18.5.0 17.10.0]", f.ChefVersions)
	}
	if len(f.PolicyNames) != 2 || f.PolicyNames[0] != "base" || f.PolicyNames[1] != "web" {
		t.Errorf("PolicyNames = %v, want [base web]", f.PolicyNames)
	}
	if len(f.PolicyGroups) != 2 || f.PolicyGroups[0] != "prod" || f.PolicyGroups[1] != "staging" {
		t.Errorf("PolicyGroups = %v, want [prod staging]", f.PolicyGroups)
	}
}

func TestNodeSnapshotFilterFromRequest_ReadinessParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nodes?target_chef_version=18.5.0&readiness_filter=ready", nil)
	f := nodeSnapshotFilterFromRequest(req, []string{"org-1"}, 24, 7)

	if f.TargetChefVersion != "18.5.0" {
		t.Errorf("TargetChefVersion = %q, want %q", f.TargetChefVersion, "18.5.0")
	}
	if f.ReadinessFilter != "ready" {
		t.Errorf("ReadinessFilter = %q, want %q", f.ReadinessFilter, "ready")
	}
}

func TestNodeSnapshotFilterFromRequest_EmptyParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	f := nodeSnapshotFilterFromRequest(req, []string{"org-1"}, 24, 7)

	if f.Environment != "" || len(f.Environments) != 0 {
		t.Errorf("expected empty environment filters, got Environment=%q Environments=%v", f.Environment, f.Environments)
	}
	if f.ReadinessFilter != "" {
		t.Errorf("ReadinessFilter = %q, want empty", f.ReadinessFilter)
	}
	if f.TargetChefVersion != "" {
		t.Errorf("TargetChefVersion = %q, want empty", f.TargetChefVersion)
	}
}

// ---------------------------------------------------------------------------
// bulkLoadReadiness tests
// ---------------------------------------------------------------------------

func TestBulkLoadReadiness_SingleOrg(t *testing.T) {
	called := false
	store := &mockStore{
		BulkListNodeReadinessByNodeNamesFn: func(ctx context.Context, organisationID string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
			called = true
			if organisationID != "org-1" {
				t.Errorf("organisationID = %q, want %q", organisationID, "org-1")
			}
			if len(nodeNames) != 2 {
				t.Errorf("nodeNames len = %d, want 2", len(nodeNames))
			}
			return map[string][]datastore.NodeReadiness{
				"web1": {{NodeName: "web1", TargetChefVersion: "18.0.0", IsReady: true}},
				"web2": {{NodeName: "web2", TargetChefVersion: "18.0.0", IsReady: false, BlockingCookbooks: json.RawMessage(`["apt"]`)}},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	nodes := []datastore.NodeSnapshot{
		{OrganisationName: "org-1", NodeName: "web1"},
		{OrganisationName: "org-1", NodeName: "web2"},
	}

	result := bulkLoadReadiness(context.Background(), store, nodes, r)

	if !called {
		t.Fatal("BulkListNodeReadinessByNodeNames was not called")
	}
	if len(result) != 2 {
		t.Fatalf("result len = %d, want 2", len(result))
	}
	if len(result["web1"]) != 1 || !result["web1"][0].IsReady {
		t.Errorf("web1 readiness: got %+v", result["web1"])
	}
	if len(result["web2"]) != 1 || result["web2"][0].IsReady {
		t.Errorf("web2 readiness: got %+v", result["web2"])
	}
	if result["web2"][0].BlockingCookbookCount != 1 {
		t.Errorf("web2 blocking count = %d, want 1", result["web2"][0].BlockingCookbookCount)
	}
}

func TestBulkLoadReadiness_MultipleOrgs(t *testing.T) {
	callsByOrg := make(map[string][]string)
	store := &mockStore{
		BulkListNodeReadinessByNodeNamesFn: func(ctx context.Context, organisationID string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
			callsByOrg[organisationID] = nodeNames
			result := make(map[string][]datastore.NodeReadiness)
			for _, name := range nodeNames {
				result[name] = []datastore.NodeReadiness{
					{NodeName: name, TargetChefVersion: "18.0.0", IsReady: true},
				}
			}
			return result, nil
		},
	}

	r := newTestRouterWithMock(store)
	nodes := []datastore.NodeSnapshot{
		{OrganisationName: "org-1", NodeName: "web1"},
		{OrganisationName: "org-2", NodeName: "db1"},
		{OrganisationName: "org-1", NodeName: "web2"},
	}

	result := bulkLoadReadiness(context.Background(), store, nodes, r)

	// Should have been called once per org (2 calls total, not 3).
	if len(callsByOrg) != 2 {
		t.Fatalf("expected 2 bulk calls (one per org), got %d", len(callsByOrg))
	}
	if len(callsByOrg["org-1"]) != 2 {
		t.Errorf("org-1 node names len = %d, want 2", len(callsByOrg["org-1"]))
	}
	if len(callsByOrg["org-2"]) != 1 {
		t.Errorf("org-2 node names len = %d, want 1", len(callsByOrg["org-2"]))
	}

	// All three nodes should have readiness entries.
	if len(result) != 3 {
		t.Fatalf("result len = %d, want 3", len(result))
	}
	for _, name := range []string{"web1", "web2", "db1"} {
		if len(result[name]) != 1 {
			t.Errorf("result[%q] len = %d, want 1", name, len(result[name]))
		}
	}
}

func TestBulkLoadReadiness_EmptyNodes(t *testing.T) {
	store := &mockStore{
		BulkListNodeReadinessByNodeNamesFn: func(ctx context.Context, organisationID string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
			t.Fatal("BulkListNodeReadinessByNodeNames should not be called for empty nodes")
			return nil, nil
		},
	}

	r := newTestRouterWithMock(store)
	result := bulkLoadReadiness(context.Background(), store, nil, r)

	if len(result) != 0 {
		t.Errorf("result len = %d, want 0", len(result))
	}
}

func TestBulkLoadReadiness_DBErrorNonFatal(t *testing.T) {
	store := &mockStore{
		BulkListNodeReadinessByNodeNamesFn: func(ctx context.Context, organisationID string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
			return nil, errors.New("connection timeout")
		},
	}

	r := newTestRouterWithMock(store)
	nodes := []datastore.NodeSnapshot{
		{OrganisationName: "org-1", NodeName: "web1"},
	}

	// Should not panic — DB errors are non-fatal.
	result := bulkLoadReadiness(context.Background(), store, nodes, r)

	if len(result) != 0 {
		t.Errorf("result len = %d, want 0 (error should suppress readiness)", len(result))
	}
}

// ---------------------------------------------------------------------------
// Compile-time import usage guards
// ---------------------------------------------------------------------------

var (
	_ = time.Now
	_ = datastore.NodeSnapshot{}
	_ = json.RawMessage{}
)
