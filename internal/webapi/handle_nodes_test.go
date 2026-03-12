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
// filterNodes tests
// ---------------------------------------------------------------------------

func TestFilterNodes_NoFilters(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "prod", ChefVersion: "17.0.0", Platform: "ubuntu"},
		{NodeName: "web2", ChefEnvironment: "staging", ChefVersion: "18.0.0", Platform: "centos"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestFilterNodes_ByEnvironment(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "prod"},
		{NodeName: "web2", ChefEnvironment: "staging"},
		{NodeName: "web3", ChefEnvironment: "prod"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?environment=prod", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
	for _, n := range result {
		if n.ChefEnvironment != "prod" {
			t.Errorf("expected environment=prod, got %q", n.ChefEnvironment)
		}
	}
}

func TestFilterNodes_ByPlatform(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", Platform: "ubuntu"},
		{NodeName: "web2", Platform: "centos"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?platform=ubuntu", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web1" {
		t.Errorf("expected web1, got %q", result[0].NodeName)
	}
}

func TestFilterNodes_ByChefVersion(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefVersion: "17.0.0"},
		{NodeName: "web2", ChefVersion: "18.0.0"},
		{NodeName: "web3", ChefVersion: "17.0.0"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?chef_version=17.0.0", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestFilterNodes_ByPolicyName(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", PolicyName: "webserver"},
		{NodeName: "web2", PolicyName: "database"},
		{NodeName: "web3", PolicyName: "webserver"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_name=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestFilterNodes_ByPolicyGroup(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", PolicyGroup: "prod"},
		{NodeName: "web2", PolicyGroup: "staging"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_group=staging", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
}

func TestFilterNodes_ByStaleTrue(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", IsStale: true},
		{NodeName: "web2", IsStale: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?stale=true", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 stale node, got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web1" {
		t.Errorf("expected web1 (stale), got %q", result[0].NodeName)
	}
}

func TestFilterNodes_ByStaleFalse(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", IsStale: true},
		{NodeName: "web2", IsStale: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?stale=false", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 non-stale node, got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web2" {
		t.Errorf("expected web2 (non-stale), got %q", result[0].NodeName)
	}
}

func TestFilterNodes_MultipleFilters(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "prod", Platform: "ubuntu", ChefVersion: "17.0.0"},
		{NodeName: "web2", ChefEnvironment: "prod", Platform: "centos", ChefVersion: "17.0.0"},
		{NodeName: "web3", ChefEnvironment: "staging", Platform: "ubuntu", ChefVersion: "17.0.0"},
		{NodeName: "web4", ChefEnvironment: "prod", Platform: "ubuntu", ChefVersion: "18.0.0"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?environment=prod&platform=ubuntu&chef_version=17.0.0", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web1" {
		t.Errorf("expected web1, got %q", result[0].NodeName)
	}
}

func TestFilterNodes_EmptyInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?environment=prod", nil)
	result := filterNodes(req, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

func TestFilterNodes_ByNodeName(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web-prod-01"},
		{NodeName: "web-staging-02"},
		{NodeName: "db-prod-01"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?node_name=web", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes matching 'web', got %d", len(result))
	}
}

func TestFilterNodes_ByNodeName_CaseInsensitive(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "WebServer-01"},
		{NodeName: "database-01"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?node_name=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node matching 'webserver', got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "WebServer-01" {
		t.Errorf("expected WebServer-01, got %q", result[0].NodeName)
	}
}

func TestFilterNodes_ByNodeName_NoMatch(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web-prod-01"},
		{NodeName: "db-prod-01"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?node_name=cache", nil)
	result := filterNodes(req, nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
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
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ID: "n1", OrganisationID: "org-1", NodeName: "web1", ChefVersion: "18.0.0", CollectedAt: now},
				{ID: "n2", OrganisationID: "org-1", NodeName: "web2", ChefVersion: "17.0.0", CollectedAt: now},
			}, nil
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
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{ID: "snap-1", NodeName: "web1", OrganisationID: "org-1", CollectedAt: now}, nil
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
			return datastore.Organisation{ID: "org-1", Name: "prod"}, nil
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
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ID: "n1", NodeName: "web1", ChefVersion: "18.0.0", CollectedAt: now},
				{ID: "n2", NodeName: "web2", ChefVersion: "17.0.0", CollectedAt: now},
			}, nil
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
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ID: "n1", NodeName: "web1", Cookbooks: json.RawMessage(`{"apt":{"version":"7.0"}}`), CollectedAt: now},
				{ID: "n2", NodeName: "web2", Cookbooks: json.RawMessage(`{"nginx":{"version":"2.0"}}`), CollectedAt: now},
			}, nil
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
// filterNodes — role filter
// ---------------------------------------------------------------------------

func TestFilterNodes_ByRole(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", Roles: json.RawMessage(`["base","webserver"]`)},
		{NodeName: "db1", Roles: json.RawMessage(`["base","database"]`)},
		{NodeName: "web2", Roles: json.RawMessage(`["base","webserver"]`)},
		{NodeName: "bare", Roles: nil},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes with role=webserver, got %d", len(result))
	}
	for _, n := range result {
		if n.NodeName != "web1" && n.NodeName != "web2" {
			t.Errorf("unexpected node %q in results", n.NodeName)
		}
	}
}

func TestFilterNodes_ByRole_NoMatch(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", Roles: json.RawMessage(`["base","webserver"]`)},
		{NodeName: "db1", Roles: json.RawMessage(`["base","database"]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=loadbalancer", nil)
	result := filterNodes(req, nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

func TestFilterNodes_ByRoleCombinedWithEnv(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "prod", Roles: json.RawMessage(`["base","webserver"]`)},
		{NodeName: "web2", ChefEnvironment: "staging", Roles: json.RawMessage(`["base","webserver"]`)},
		{NodeName: "db1", ChefEnvironment: "prod", Roles: json.RawMessage(`["base","database"]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver&environment=prod", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web1" {
		t.Errorf("expected web1, got %q", result[0].NodeName)
	}
}

// ---------------------------------------------------------------------------
// Role filtering via filterNodes (delegates to export.FilterNodes)
// ---------------------------------------------------------------------------

func TestFilterNodes_RoleMatch(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`["base","webserver","database"]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=webserver, got %d", len(result))
	}
}

func TestFilterNodes_RoleNoMatch(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`["base","webserver"]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=database", nil)
	result := filterNodes(req, nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes for role=database, got %d", len(result))
	}
}

func TestFilterNodes_RoleEmptyRoles(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`[]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes for role=webserver on empty roles, got %d", len(result))
	}
}

func TestFilterNodes_RoleNilRoles(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes for role=webserver on nil roles, got %d", len(result))
	}
}

func TestFilterNodes_RolePartialNameMatches(t *testing.T) {
	// With case-insensitive substring matching, "web" WILL match "webserver".
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`["webserver"]`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=web", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=web (substring match), got %d", len(result))
	}
}

func TestFilterNodes_RoleSubstringMatchAmongSimilar(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`["web","webserver","web-proxy"]`)},
	}

	// All of these should match because they are exact or substring matches.
	for _, role := range []string{"web", "webserver", "web-proxy"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role="+role, nil)
		result := filterNodes(req, nodes)
		if len(result) != 1 {
			t.Errorf("expected 1 node for role=%s, got %d", role, len(result))
		}
	}

	// "server" is a substring of "webserver", so it should also match.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=server", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=server (substring of webserver), got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// Partial-match and case-insensitive tests
// ---------------------------------------------------------------------------

func TestFilterNodes_PartialMatch(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "production", Platform: "ubuntu-20.04", ChefVersion: "17.0.0", PolicyName: "webserver-policy", PolicyGroup: "prod-us-east"},
		{NodeName: "web2", ChefEnvironment: "pre-prod", Platform: "ubuntu-22.04", ChefVersion: "17.1.0", PolicyName: "database-policy", PolicyGroup: "staging"},
		{NodeName: "web3", ChefEnvironment: "staging", Platform: "centos", ChefVersion: "18.0.0", PolicyName: "cache-policy", PolicyGroup: "dev"},
	}

	// "prod" matches "production" and "pre-prod"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?environment=prod", nil)
	result := filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes for environment=prod (partial), got %d", len(result))
	}

	// "ubuntu" matches "ubuntu-20.04" and "ubuntu-22.04"
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?platform=ubuntu", nil)
	result = filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes for platform=ubuntu (partial), got %d", len(result))
	}

	// "17" matches "17.0.0" and "17.1.0"
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?chef_version=17", nil)
	result = filterNodes(req, nodes)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes for chef_version=17 (partial), got %d", len(result))
	}

	// "policy" matches all three policy names
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_name=policy", nil)
	result = filterNodes(req, nodes)
	if len(result) != 3 {
		t.Errorf("expected 3 nodes for policy_name=policy (partial), got %d", len(result))
	}

	// "prod" matches "prod-us-east"
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_group=prod", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for policy_group=prod (partial), got %d", len(result))
	}
}

func TestFilterNodes_CaseInsensitive(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "web1", ChefEnvironment: "Production", Platform: "Ubuntu", ChefVersion: "17.0.0", PolicyName: "WebServer", PolicyGroup: "Prod-US"},
		{NodeName: "web2", ChefEnvironment: "staging", Platform: "centos", ChefVersion: "18.0.0", PolicyName: "database", PolicyGroup: "dev"},
	}

	// Upper-case filter matches lower/mixed-case field
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?environment=PRODUCTION", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for environment=PRODUCTION (case-insensitive), got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "web1" {
		t.Errorf("expected web1, got %q", result[0].NodeName)
	}

	// Lower-case filter matches mixed-case field
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?platform=ubuntu", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for platform=ubuntu (case-insensitive), got %d", len(result))
	}

	// Mixed-case partial filter
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_name=Web", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for policy_name=Web (case-insensitive partial), got %d", len(result))
	}

	// Upper-case partial filter on policy_group
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?policy_group=PROD", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for policy_group=PROD (case-insensitive partial), got %d", len(result))
	}
}

func TestFilterNodes_CaseInsensitiveRole(t *testing.T) {
	nodes := []datastore.NodeSnapshot{
		{NodeName: "n1", Roles: json.RawMessage(`["WebServer","Database"]`)},
		{NodeName: "n2", Roles: json.RawMessage(`["cache"]`)},
	}

	// "webserver" matches "WebServer" (case-insensitive)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=webserver", nil)
	result := filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=webserver (case-insensitive), got %d", len(result))
	}
	if len(result) > 0 && result[0].NodeName != "n1" {
		t.Errorf("expected n1, got %q", result[0].NodeName)
	}

	// "DATABASE" matches "Database" (case-insensitive)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=DATABASE", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=DATABASE (case-insensitive), got %d", len(result))
	}

	// "Server" is a substring of "WebServer" (case-insensitive partial)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=Server", nil)
	result = filterNodes(req, nodes)
	if len(result) != 1 {
		t.Errorf("expected 1 node for role=Server (case-insensitive partial), got %d", len(result))
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
