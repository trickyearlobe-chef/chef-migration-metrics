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

// Ensure unused imports don't cause errors.
var _ = datastore.NodeSnapshotFilter{}

// ---------------------------------------------------------------------------
// Target Chef Versions filter (config-driven, no DB needed)
// ---------------------------------------------------------------------------

func TestHandleFilterTargetChefVersions_OK(t *testing.T) {
	wsEnabled := true
	r := testRouterWithTargetVersion("18.0.0", &wsEnabled)
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
	// A single active target now yields a one-element array.
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	if body.Data[0] != "18.0.0" {
		t.Errorf("data = %v, want [18.0.0]", body.Data)
	}
}

func TestHandleFilterTargetChefVersions_Empty(t *testing.T) {
	wsEnabled := true
	r := testRouterWithTargetVersion("", &wsEnabled)
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
// filterOrgIDs helper tests
// ---------------------------------------------------------------------------

func TestFilterOrgIDs_NoOrgs(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	f, err := r.filterOrgIDs(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.OrganisationNames) != 0 {
		t.Errorf("expected 0 org IDs, got %d", len(f.OrganisationNames))
	}
}

func TestFilterOrgIDs_WithOrgs(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{Name: "prod"},
				{Name: "staging"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	f, err := r.filterOrgIDs(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.OrganisationNames) != 2 {
		t.Errorf("expected 2 org IDs, got %d", len(f.OrganisationNames))
	}
}

func TestFilterOrgIDs_DBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/environments", nil)
	_, err := r.filterOrgIDs(req)
	if err == nil {
		t.Error("expected error, got nil")
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
	r := testRouterWithTargetVersion("18.0.0", &wsEnabled)

	// First request.
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/filters/target-chef-versions", nil)
	r.ServeHTTP(w1, req1)

	// The handler must not mutate the configured scalar target.
	if r.cfg.TargetChefVersion != "18.0.0" {
		t.Errorf("cfg.TargetChefVersion = %q, want %q — config was mutated", r.cfg.TargetChefVersion, "18.0.0")
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
// Test helper — build a Router with the target Chef version configured
// ---------------------------------------------------------------------------

func testRouterWithTargetVersion(version string, wsEnabled *bool) *Router {
	cfg := testConfig()
	cfg.TargetChefVersion = version
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
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			return []string{"production", "staging"}, nil
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
	// Should be sorted (returned by SQL ORDER BY).
	if body.Data[0] != "production" || body.Data[1] != "staging" {
		t.Errorf("data = %v, want [production staging]", body.Data)
	}
}

func TestHandleFilterEnvironments_HappyPath_Empty(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			return nil, nil
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
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

func TestHandleFilterRoles_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeRolesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
			return []string{"base", "db", "web"}, nil
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
	// Sorted: base, db, web (returned by SQL ORDER BY).
	if body.Data[0] != "base" || body.Data[1] != "db" || body.Data[2] != "web" {
		t.Errorf("data = %v, want [base db web]", body.Data)
	}
}

func TestHandleFilterPlatforms_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			return []string{"windows 10.0.22631", "ubuntu 22.04"}, nil
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
		Data []struct {
			Value       string  `json:"value"`
			DisplayName *string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(body.Data))
	}
	// Windows should get a display name from defaults.
	if body.Data[0].Value != "windows 10.0.22631" {
		t.Errorf("data[0].value = %q, want %q", body.Data[0].Value, "windows 10.0.22631")
	}
	if body.Data[0].DisplayName == nil || *body.Data[0].DisplayName != "Win11 23H2" {
		t.Errorf("data[0].display_name = %v, want %q", body.Data[0].DisplayName, "Win11 23H2")
	}
	// Ubuntu should also get a display name.
	if body.Data[1].Value != "ubuntu 22.04" {
		t.Errorf("data[1].value = %q, want %q", body.Data[1].Value, "ubuntu 22.04")
	}
	if body.Data[1].DisplayName == nil || *body.Data[1].DisplayName != "Ubuntu 22.04 LTS (Jammy)" {
		t.Errorf("data[1].display_name = %v, want %q", body.Data[1].DisplayName, "Ubuntu 22.04 LTS (Jammy)")
	}
}

func TestHandleFilterPolicyNames_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			// SQL DISTINCT already excludes empty strings.
			return []string{"database", "webserver"}, nil
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
	if len(body.Data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(body.Data))
	}
}

func TestHandleFilterPolicyGroups_HappyPath(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			return []string{"prod-eu-west", "prod-us-east"}, nil
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
// Filter endpoints — ListDistinctNodeValues DB error is fatal (500)
// ---------------------------------------------------------------------------

func TestHandleFilterEnvironments_DistinctDBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeValuesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
			return nil, errors.New("partial failure")
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

func TestHandleFilterTags_HappyPath(t *testing.T) {
	var gotOpts datastore.DistinctValueOpts
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeTagsFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
			gotOpts = opts
			// Returned in count-ranked order by the datastore; the handler
			// passes the order through unchanged.
			return []string{"upgrade", "prepare", "eu-west"}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/tags", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// The facet is always bounded — a cap is applied even without a prefix.
	if gotOpts.Limit != 50 {
		t.Errorf("opts.Limit = %d, want 50 (always bounded)", gotOpts.Limit)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 3 || body.Data[0] != "upgrade" {
		t.Errorf("data = %v, want count-ranked [upgrade prepare eu-west]", body.Data)
	}
}

func TestHandleFilterTags_PrefixPassed(t *testing.T) {
	var gotOpts datastore.DistinctValueOpts
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeTagsFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
			gotOpts = opts
			return []string{"eu-west"}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/tags?q=eu", nil)
	r.ServeHTTP(w, req)

	if gotOpts.SearchPrefix != "eu" {
		t.Errorf("opts.SearchPrefix = %q, want %q", gotOpts.SearchPrefix, "eu")
	}
	if gotOpts.Limit != 50 {
		t.Errorf("opts.Limit = %d, want 50", gotOpts.Limit)
	}
}

func TestHandleFilterTags_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filters/tags", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /filters/tags status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFilterTags_DistinctDBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeTagsFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
			return nil, errors.New("partial failure")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/tags", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleFilterRoles_DistinctDBError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListDistinctNodeRolesFn: func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
			return nil, errors.New("partial failure")
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
