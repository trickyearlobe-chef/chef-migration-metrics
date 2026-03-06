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
// Target Chef Versions filter (config-driven, no DB needed)
// ---------------------------------------------------------------------------

func TestHandleFilterTargetChefVersions_OK(t *testing.T) {
	wsEnabled := true
	r := testRouterWithTargetVersions([]string{"18.0.0", "17.0.0", "19.0.0"}, &wsEnabled)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/target-chef-versions", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}
	// Should be sorted.
	if body.Data[0] != "17.0.0" || body.Data[1] != "18.0.0" || body.Data[2] != "19.0.0" {
		t.Errorf("data = %v, want [17.0.0 18.0.0 19.0.0]", body.Data)
	}
}

func TestHandleFilterTargetChefVersions_Empty(t *testing.T) {
	wsEnabled := true
	r := testRouterWithTargetVersions(nil, &wsEnabled)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/target-chef-versions", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

// ---------------------------------------------------------------------------
// Complexity Labels filter (static list, no DB needed)
// ---------------------------------------------------------------------------

func TestHandleFilterComplexityLabels_OK(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/complexity-labels", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := []string{"trivial", "simple", "moderate", "complex", "very_complex"}
	if len(body.Data) != len(expected) {
		t.Fatalf("len(data) = %d, want %d", len(body.Data), len(expected))
	}
	for i, v := range expected {
		if body.Data[i] != v {
			t.Errorf("data[%d] = %q, want %q", i, body.Data[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// Method-not-allowed tests for all 7 filter endpoints
// ---------------------------------------------------------------------------

func TestHandleFilterEnvironments_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filters/environments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /filters/environments status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterRoles_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filters/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /filters/roles status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterPolicyNames_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/filters/policy-names", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /filters/policy-names status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterPolicyGroups_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/filters/policy-groups", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /filters/policy-groups status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterPlatforms_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/filters/platforms", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH /filters/platforms status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterTargetChefVersions_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filters/target-chef-versions", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /filters/target-chef-versions status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterComplexityLabels_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filters/complexity-labels", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /filters/complexity-labels status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// nodeFilterRecord extraction tests (unit tests for the helper structs)
// ---------------------------------------------------------------------------

func TestNodeFilterRecord_StringExtractors(t *testing.T) {
	rec := nodeFilterRecord{
		chefEnvironment: "production",
		platform:        "ubuntu",
		policyName:      "webserver",
		policyGroup:     "prod-us-east",
		roles:           []byte(`["base","web"]`),
	}

	tests := []struct {
		name string
		fn   func(nodeFilterRecord) string
		want string
	}{
		{"chefEnvironment", func(r nodeFilterRecord) string { return r.chefEnvironment }, "production"},
		{"platform", func(r nodeFilterRecord) string { return r.platform }, "ubuntu"},
		{"policyName", func(r nodeFilterRecord) string { return r.policyName }, "webserver"},
		{"policyGroup", func(r nodeFilterRecord) string { return r.policyGroup }, "prod-us-east"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(rec)
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestNodeFilterRecord_RolesJSONExtractor(t *testing.T) {
	rec := nodeFilterRecord{
		roles: []byte(`["base","web","monitoring"]`),
	}

	raw := rec.roles
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal roles: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(items))
	}
	want := []string{"base", "web", "monitoring"}
	for i, v := range want {
		if items[i] != v {
			t.Errorf("role[%d] = %q, want %q", i, items[i], v)
		}
	}
}

func TestNodeFilterRecord_EmptyRoles(t *testing.T) {
	rec := nodeFilterRecord{}
	if len(rec.roles) != 0 {
		t.Errorf("expected empty roles, got %q", string(rec.roles))
	}
}

func TestNodeFilterRecord_NullRoles(t *testing.T) {
	rec := nodeFilterRecord{
		roles: []byte(`null`),
	}
	var items []string
	if err := json.Unmarshal(rec.roles, &items); err != nil {
		t.Fatalf("unmarshal null roles: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items for null JSON, got %v", items)
	}
}

func TestNodeFilterRecord_EmptyStringFields(t *testing.T) {
	rec := nodeFilterRecord{}
	if rec.chefEnvironment != "" {
		t.Error("expected empty chefEnvironment")
	}
	if rec.platform != "" {
		t.Error("expected empty platform")
	}
	if rec.policyName != "" {
		t.Error("expected empty policyName")
	}
	if rec.policyGroup != "" {
		t.Error("expected empty policyGroup")
	}
}

// ---------------------------------------------------------------------------
// Complexity labels ordering test — the order is semantically meaningful
// ---------------------------------------------------------------------------

func TestHandleFilterComplexityLabels_Ordering(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/complexity-labels", nil)
	r.ServeHTTP(w, req)

	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify the specific ordering from trivial to most complex.
	if len(body.Data) < 5 {
		t.Fatalf("expected at least 5 labels, got %d", len(body.Data))
	}
	if body.Data[0] != "trivial" {
		t.Errorf("first label = %q, want %q", body.Data[0], "trivial")
	}
	if body.Data[len(body.Data)-1] != "very_complex" {
		t.Errorf("last label = %q, want %q", body.Data[len(body.Data)-1], "very_complex")
	}
}

// ---------------------------------------------------------------------------
// Target Chef Versions does not mutate the config
// ---------------------------------------------------------------------------

func TestHandleFilterTargetChefVersions_DoesNotMutateConfig(t *testing.T) {
	wsEnabled := true
	r := testRouterWithTargetVersions([]string{"18.0.0", "17.0.0"}, &wsEnabled)

	// First request.
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/filters/target-chef-versions", nil)
	r.ServeHTTP(w1, req1)

	// Check config is still in original order.
	if r.cfg.TargetChefVersions[0] != "18.0.0" {
		t.Errorf("cfg.TargetChefVersions[0] = %q, want %q — config was mutated", r.cfg.TargetChefVersions[0], "18.0.0")
	}
	if r.cfg.TargetChefVersions[1] != "17.0.0" {
		t.Errorf("cfg.TargetChefVersions[1] = %q, want %q — config was mutated", r.cfg.TargetChefVersions[1], "17.0.0")
	}
}

// ---------------------------------------------------------------------------
// Content-Type verification for filter responses
// ---------------------------------------------------------------------------

func TestHandleFilter_ContentType(t *testing.T) {
	endpoints := []string{
		"/api/v1/filters/target-chef-versions",
		"/api/v1/filters/complexity-labels",
	}
	r := testRouter()

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			r.ServeHTTP(w, req)

			ct := w.Header().Get("Content-Type")
			if ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helper — build a Router with TargetChefVersions configured
// ---------------------------------------------------------------------------

func testRouterWithTargetVersions(versions []string, wsEnabled *bool) *Router {
	cfg := testConfig()
	cfg.TargetChefVersions = versions
	if wsEnabled != nil {
		cfg.Server.WebSocket.Enabled = wsEnabled
	}
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(nil, cfg, hub)
}

// ---------------------------------------------------------------------------
// Filter endpoints — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleFilterEnvironments_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{ChefEnvironment: "production"},
				{ChefEnvironment: "staging"},
				{ChefEnvironment: "production"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2 (distinct)", len(body.Data))
	}
	// Should be sorted.
	if body.Data[0] != "production" || body.Data[1] != "staging" {
		t.Errorf("data = %v, want [production staging]", body.Data)
	}
}

func TestHandleFilterEnvironments_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

func TestHandleFilterRoles_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{Roles: json.RawMessage(`["base","web"]`)},
				{Roles: json.RawMessage(`["base","db"]`)},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3 (base, db, web)", len(body.Data))
	}
	// Sorted: base, db, web.
	if body.Data[0] != "base" || body.Data[1] != "db" || body.Data[2] != "web" {
		t.Errorf("data = %v, want [base db web]", body.Data)
	}
}

func TestHandleFilterPlatforms_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{Platform: "ubuntu"},
				{Platform: "centos"},
				{Platform: "ubuntu"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/platforms", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(body.Data))
	}
}

func TestHandleFilterPolicyNames_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{PolicyName: "webserver"},
				{PolicyName: "database"},
				{PolicyName: ""},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/policy-names", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Empty strings should be excluded.
	if len(body.Data) != 2 {
		t.Errorf("len(data) = %d, want 2 (empty policy_name excluded)", len(body.Data))
	}
}

func TestHandleFilterPolicyGroups_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return []datastore.NodeSnapshot{
				{PolicyGroup: "prod-us-east"},
				{PolicyGroup: "prod-eu-west"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/policy-groups", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(body.Data))
	}
}

// ---------------------------------------------------------------------------
// Filter endpoints — DB errors
// ---------------------------------------------------------------------------

func TestHandleFilterEnvironments_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleFilterRoles_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/roles", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleFilterPlatforms_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/platforms", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleFilterPolicyNames_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/policy-names", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleFilterPolicyGroups_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/policy-groups", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// Filter endpoints — node listing errors are non-fatal (WARN, skip org)
// ---------------------------------------------------------------------------

func TestHandleFilterEnvironments_NodeListError_NonFatal(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
			return nil, errors.New("partial failure")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	r.ServeHTTP(w, req)

	// Should still be 200 — the handler logs WARN but returns empty results.
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (non-fatal node list error)", w.Code, http.StatusOK)
	}
}
