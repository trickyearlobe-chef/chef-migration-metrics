// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Helper: build a router with ownership enabled
// ---------------------------------------------------------------------------

func ownershipTestConfig() *Router {
	cfg := testConfig()
	cfg.Ownership.Enabled = true
	cfg.TargetChefVersions = []string{"18.5.0"}
	store := &mockStore{}
	return newTestRouterWithMockAndConfig(store, cfg)
}

func ownershipRouter(store *mockStore) *Router {
	cfg := testConfig()
	cfg.Ownership.Enabled = true
	cfg.TargetChefVersions = []string{"18.5.0"}
	return newTestRouterWithMockAndConfig(store, cfg)
}

func ownershipDisabledRouter(store *mockStore) *Router {
	cfg := testConfig()
	cfg.Ownership.Enabled = false
	return newTestRouterWithMockAndConfig(store, cfg)
}

// ---------------------------------------------------------------------------
// Ownership disabled — all endpoints return 404
// ---------------------------------------------------------------------------

func TestOwnership_Disabled_ListOwners(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_CreateOwner(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"name":"test","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_GetOwner(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/platform-team", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_Reassign(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"from_owner":"a","to_owner":"b"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_Lookup(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node&entity_key=web-01", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_AuditLog(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/audit-log", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnership_Disabled_Import(t *testing.T) {
	r := ownershipDisabledRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/owners — List Owners
// ---------------------------------------------------------------------------

func TestListOwners_MethodNotAllowed(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/owners", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH /api/v1/owners status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestListOwners_Empty(t *testing.T) {
	store := &mockStore{
		ListOwnersWithSummaryFn: func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error) {
			return []datastore.OwnerWithSummary{}, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners", nil)
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

func TestListOwners_WithOwners(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		ListOwnersWithSummaryFn: func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error) {
			return []datastore.OwnerWithSummary{
				{Owner: datastore.Owner{ID: "id-1", Name: "web-platform", DisplayName: "Web Platform", OwnerType: "team", CreatedAt: now, UpdatedAt: now}, NodeCount: 10, CookbookCount: 3},
				{Owner: datastore.Owner{ID: "id-2", Name: "payments-team", DisplayName: "Payments", OwnerType: "team", CreatedAt: now, UpdatedAt: now}},
			}, 2, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []struct {
			Name             string         `json:"name"`
			DisplayName      string         `json:"display_name"`
			OwnerType        string         `json:"owner_type"`
			AssignmentCounts map[string]int `json:"assignment_counts"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", body.Pagination.TotalItems)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(body.Data))
	}
	if body.Data[0].Name != "web-platform" {
		t.Errorf("data[0].name = %q, want %q", body.Data[0].Name, "web-platform")
	}
	if body.Data[0].AssignmentCounts["node"] != 10 {
		t.Errorf("data[0].assignment_counts.node = %d, want 10", body.Data[0].AssignmentCounts["node"])
	}
	// Ensure zero-filled entity types.
	if body.Data[0].AssignmentCounts["role"] != 0 {
		t.Errorf("data[0].assignment_counts.role = %d, want 0", body.Data[0].AssignmentCounts["role"])
	}
}

func TestListOwners_DBError(t *testing.T) {
	store := &mockStore{
		ListOwnersWithSummaryFn: func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error) {
			return nil, 0, errors.New("connection refused")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestListOwners_Filters(t *testing.T) {
	var capturedFilter datastore.OwnerListFilter
	store := &mockStore{
		ListOwnersWithSummaryFn: func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error) {
			capturedFilter = f
			return []datastore.OwnerWithSummary{}, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners?owner_type=team&search=web&page=2&per_page=10", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedFilter.OwnerType != "team" {
		t.Errorf("filter.OwnerType = %q, want %q", capturedFilter.OwnerType, "team")
	}
	if capturedFilter.Search != "web" {
		t.Errorf("filter.Search = %q, want %q", capturedFilter.Search, "web")
	}
	if capturedFilter.Offset != 10 {
		t.Errorf("filter.Offset = %d, want 10", capturedFilter.Offset)
	}
	if capturedFilter.Limit != 10 {
		t.Errorf("filter.Limit = %d, want 10", capturedFilter.Limit)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/owners — Create Owner
// ---------------------------------------------------------------------------

func TestCreateOwner_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		InsertOwnerFn: func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{
				ID:           "new-id",
				Name:         p.Name,
				DisplayName:  p.DisplayName,
				ContactEmail: p.ContactEmail,
				OwnerType:    p.OwnerType,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"name":"new-team","display_name":"New Team","contact_email":"new@example.com","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp datastore.Owner
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Name != "new-team" {
		t.Errorf("name = %q, want %q", resp.Name, "new-team")
	}
}

func TestCreateOwner_MissingName(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateOwner_InvalidNameFormat(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"name":"UPPER-CASE","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateOwner_InvalidNameStartsWithDash(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"name":"-starts-with-dash","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateOwner_MissingOwnerType(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"name":"my-team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateOwner_InvalidOwnerType(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"name":"my-team","owner_type":"department"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateOwner_Conflict(t *testing.T) {
	store := &mockStore{
		InsertOwnerFn: func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrAlreadyExists
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"name":"existing-team","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCreateOwner_InvalidJSON(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader("not json"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateOwner_ValidOwnerTypes(t *testing.T) {
	for _, ot := range []string{"team", "individual", "business_unit", "cost_centre", "custom"} {
		now := time.Now().Truncate(time.Second)
		store := &mockStore{
			InsertOwnerFn: func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
				return datastore.Owner{ID: "id", Name: p.Name, OwnerType: p.OwnerType, CreatedAt: now, UpdatedAt: now}, nil
			},
			InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
				return nil
			},
		}
		r := ownershipRouter(store)
		w := httptest.NewRecorder()
		body := `{"name":"team-` + ot + `","owner_type":"` + ot + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("owner_type=%q: status = %d, want %d", ot, w.Code, http.StatusCreated)
		}
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/owners/:name — Get Owner Detail
// ---------------------------------------------------------------------------

func TestGetOwner_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{
				ID:          "id-1",
				Name:        "web-platform",
				DisplayName: "Web Platform Team",
				OwnerType:   "team",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		CountAssignmentsByOwnerFn: func(ctx context.Context, ownerName string) (map[string]int, error) {
			return map[string]int{"node": 10, "cookbook": 5}, nil
		},
		GetOwnerReadinessSummaryFn: func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error) {
			return datastore.OwnerReadinessSummary{
				TargetChefVersion: targetChefVersion,
				TotalNodes:        10,
				Ready:             7,
				Blocked:           2,
				Stale:             1,
				BlockingCookbooks: []datastore.BlockingCookbookSummary{},
			}, nil
		},
		GetOwnerCookbookSummaryFn: func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error) {
			return datastore.OwnerCookbookSummary{Total: 5, Compatible: 3, Incompatible: 1, Untested: 1}, nil
		},
		GetOwnerGitRepoSummaryFn: func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error) {
			return datastore.OwnerGitRepoSummary{Total: 2, Compatible: 1, Incompatible: 1}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["readiness_summary"]; !ok {
		t.Error("expected readiness_summary in response")
	}
	if _, ok := resp["cookbook_summary"]; !ok {
		t.Error("expected cookbook_summary in response")
	}
	if _, ok := resp["git_repo_summary"]; !ok {
		t.Error("expected git_repo_summary in response")
	}
	if _, ok := resp["assignment_counts"]; !ok {
		t.Error("expected assignment_counts in response")
	}
}

func TestGetOwner_NotFound(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetOwner_DBError(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{}, errors.New("timeout")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/owners/:name — Update Owner
// ---------------------------------------------------------------------------

func TestUpdateOwner_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		UpdateOwnerFn: func(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error) {
			dn := "Updated Name"
			if p.DisplayName != nil {
				dn = *p.DisplayName
			}
			return datastore.Owner{
				ID:          "id-1",
				Name:        name,
				DisplayName: dn,
				OwnerType:   "team",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"display_name":"Updated Name"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/owners/web-platform", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp datastore.Owner
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DisplayName != "Updated Name" {
		t.Errorf("display_name = %q, want %q", resp.DisplayName, "Updated Name")
	}
}

func TestUpdateOwner_NotFound(t *testing.T) {
	store := &mockStore{
		UpdateOwnerFn: func(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"display_name":"X"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/owners/nonexistent", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUpdateOwner_InvalidOwnerType(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"owner_type":"invalid"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/owners/web-platform", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestUpdateOwner_InvalidJSON(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/owners/web-platform", strings.NewReader("{bad"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/owners/:name — Delete Owner
// ---------------------------------------------------------------------------

func TestDeleteOwner_HappyPath(t *testing.T) {
	store := &mockStore{
		DeleteOwnerFn: func(ctx context.Context, name string) (int, error) {
			return 5, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/old-team", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestDeleteOwner_NotFound(t *testing.T) {
	store := &mockStore{
		DeleteOwnerFn: func(ctx context.Context, name string) (int, error) {
			return 0, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteOwner_DBError(t *testing.T) {
	store := &mockStore{
		DeleteOwnerFn: func(ctx context.Context, name string) (int, error) {
			return 0, errors.New("database error")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/web-platform", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// Owner sub-path method not allowed
// ---------------------------------------------------------------------------

func TestOwnerSubpath_MethodNotAllowed(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/owners/web-platform", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestOwnerSubpath_MissingName(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestOwnerSubpath_UnknownPath(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform/unknown", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/owners/:name/assignments — List Assignments
// ---------------------------------------------------------------------------

func TestListAssignments_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		ListAssignmentsByOwnerFn: func(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error) {
			if f.OwnerName != "web-platform" {
				t.Errorf("filter.OwnerName = %q, want %q", f.OwnerName, "web-platform")
			}
			return []datastore.OwnershipAssignment{
				{
					ID:               "a1",
					OwnerID:          "id-1",
					EntityType:       "node",
					EntityKey:        "web-01",
					AssignmentSource: "manual",
					Confidence:       "definitive",
					CreatedAt:        now,
				},
			}, 1, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform/assignments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1", body.Pagination.TotalItems)
	}
}

func TestListAssignments_WithFilters(t *testing.T) {
	var capturedFilter datastore.AssignmentListFilter
	store := &mockStore{
		ListAssignmentsByOwnerFn: func(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error) {
			capturedFilter = f
			return []datastore.OwnershipAssignment{}, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform/assignments?entity_type=cookbook&assignment_source=manual&organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedFilter.EntityType != "cookbook" {
		t.Errorf("filter.EntityType = %q, want %q", capturedFilter.EntityType, "cookbook")
	}
	if capturedFilter.AssignmentSource != "manual" {
		t.Errorf("filter.AssignmentSource = %q, want %q", capturedFilter.AssignmentSource, "manual")
	}
	if capturedFilter.OrganisationName != "prod" {
		t.Errorf("filter.OrganisationName = %q, want %q", capturedFilter.OrganisationName, "prod")
	}
}

func TestListAssignments_MethodNotAllowed(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/owners/web-platform/assignments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestListAssignments_DBError(t *testing.T) {
	store := &mockStore{
		ListAssignmentsByOwnerFn: func(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error) {
			return nil, 0, errors.New("db error")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform/assignments", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/owners/:name/assignments — Create Assignments
// ---------------------------------------------------------------------------

func TestCreateAssignments_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{
				ID:               "a-new",
				OwnerID:          p.OwnerID,
				EntityType:       p.EntityType,
				EntityKey:        p.EntityKey,
				AssignmentSource: p.AssignmentSource,
				Confidence:       p.Confidence,
				CreatedAt:        now,
			}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"cookbook","entity_key":"nginx"},{"entity_type":"node","entity_key":"web-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Created     int                             `json:"created"`
		Assignments []datastore.OwnershipAssignment `json:"assignments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Created != 2 {
		t.Errorf("created = %d, want 2", resp.Created)
	}
	if len(resp.Assignments) != 2 {
		t.Errorf("assignments len = %d, want 2", len(resp.Assignments))
	}
}

func TestCreateAssignments_OwnerNotFound(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"node","entity_key":"web-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/nonexistent/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCreateAssignments_InvalidEntityType(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"database","entity_key":"db-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateAssignments_EmptyEntityKey(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"node","entity_key":""}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateAssignments_EmptyArray(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateAssignments_Duplicate(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{}, datastore.ErrAlreadyExists
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"node","entity_key":"web-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCreateAssignments_WithOrganisation(t *testing.T) {
	var capturedOrgID string
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: name}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			capturedOrgID = p.OrganisationID
			return datastore.OwnershipAssignment{
				ID:               "a-new",
				OwnerID:          p.OwnerID,
				EntityType:       p.EntityType,
				EntityKey:        p.EntityKey,
				OrganisationID:   p.OrganisationID,
				AssignmentSource: "manual",
				Confidence:       "definitive",
				CreatedAt:        time.Now(),
			}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"node","entity_key":"web-01","organisation":"prod"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if capturedOrgID != "org-1" {
		t.Errorf("organisation_id = %q, want %q", capturedOrgID, "org-1")
	}
}

func TestCreateAssignments_OrganisationNotFound(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"assignments":[{"entity_type":"node","entity_key":"web-01","organisation":"nonexistent"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateAssignments_InvalidJSON(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners/web-platform/assignments", strings.NewReader("{bad"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/owners/:name/assignments/:id — Delete Assignment
// ---------------------------------------------------------------------------

func TestDeleteAssignment_HappyPath(t *testing.T) {
	store := &mockStore{
		GetAssignmentFn: func(ctx context.Context, id string) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{
				ID:               id,
				EntityType:       "node",
				EntityKey:        "web-01",
				AssignmentSource: "manual",
				Confidence:       "definitive",
			}, nil
		},
		DeleteAssignmentFn: func(ctx context.Context, id string) error {
			return nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/web-platform/assignments/a-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestDeleteAssignment_NotFound(t *testing.T) {
	store := &mockStore{
		GetAssignmentFn: func(ctx context.Context, id string) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/web-platform/assignments/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteAssignment_MethodNotAllowed(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owners/web-platform/assignments/a-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/reassign — Bulk Reassign
// ---------------------------------------------------------------------------

func TestReassign_HappyPath(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			return 10, 2, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old-team","to_owner":"new-team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Reassigned         int    `json:"reassigned"`
		Skipped            int    `json:"skipped"`
		FromOwner          string `json:"from_owner"`
		ToOwner            string `json:"to_owner"`
		SourceOwnerDeleted bool   `json:"source_owner_deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Reassigned != 10 {
		t.Errorf("reassigned = %d, want 10", resp.Reassigned)
	}
	if resp.Skipped != 2 {
		t.Errorf("skipped = %d, want 2", resp.Skipped)
	}
	if resp.SourceOwnerDeleted {
		t.Error("source_owner_deleted should be false")
	}
}

func TestReassign_SameOwner(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"from_owner":"same","to_owner":"same"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReassign_MissingFields(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	body := `{"from_owner":"old-team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReassign_SourceNotFound(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			if name == "missing" {
				return datastore.Owner{}, datastore.ErrNotFound
			}
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"missing","to_owner":"new-team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestReassign_TargetNotFound(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			if name == "missing" {
				return datastore.Owner{}, datastore.ErrNotFound
			}
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old-team","to_owner":"missing"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestReassign_WithEntityTypeFilter(t *testing.T) {
	var capturedEntityType string
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			capturedEntityType = entityType
			return 5, 0, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old","to_owner":"new","entity_type":"cookbook"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedEntityType != "cookbook" {
		t.Errorf("entity_type = %q, want %q", capturedEntityType, "cookbook")
	}
}

func TestReassign_WithDeleteSourceOwner(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			return 5, 0, nil
		},
		CountAssignmentsByOwnerFn: func(ctx context.Context, ownerName string) (map[string]int, error) {
			// No remaining assignments — allow deletion.
			return map[string]int{}, nil
		},
		DeleteOwnerFn: func(ctx context.Context, name string) (int, error) {
			return 0, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old","to_owner":"new","delete_source_owner":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		SourceOwnerDeleted bool `json:"source_owner_deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.SourceOwnerDeleted {
		t.Error("source_owner_deleted should be true")
	}
}

func TestReassign_DeleteSourceOwnerNotAllowedWhenRemainingAssignments(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			return 3, 0, nil
		},
		CountAssignmentsByOwnerFn: func(ctx context.Context, ownerName string) (map[string]int, error) {
			// Still has remaining assignments.
			return map[string]int{"role": 2}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old","to_owner":"new","delete_source_owner":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		SourceOwnerDeleted bool `json:"source_owner_deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.SourceOwnerDeleted {
		t.Error("source_owner_deleted should be false when remaining assignments exist")
	}
}

func TestReassign_MethodNotAllowed(t *testing.T) {
	r := ownershipTestConfig()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/reassign", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestReassign_InvalidJSON(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader("not-json"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReassign_DBError(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			return 0, 0, errors.New("transaction failed")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old","to_owner":"new"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestReassign_WithOrganisationFilter(t *testing.T) {
	var capturedOrgID string
	store := &mockStore{
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-" + name, Name: name}, nil
		},
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: name}, nil
		},
		ReassignOwnershipFn: func(ctx context.Context, fromOwnerID, toOwnerID, entityType, organisationID string) (int, int, error) {
			capturedOrgID = organisationID
			return 3, 0, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"from_owner":"old","to_owner":"new","organisation":"prod"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/reassign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedOrgID != "org-1" {
		t.Errorf("organisation_id = %q, want %q", capturedOrgID, "org-1")
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/lookup — Ownership Lookup
// ---------------------------------------------------------------------------

func TestLookup_HappyPath(t *testing.T) {
	store := &mockStore{
		LookupOwnershipFn: func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error) {
			return []datastore.OwnershipLookupResult{
				{
					OwnerName:        "web-platform",
					DisplayName:      "Web Platform Team",
					AssignmentSource: "manual",
					Confidence:       "definitive",
					Resolution:       "direct",
				},
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node&entity_key=web-01", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		EntityType string                            `json:"entity_type"`
		EntityKey  string                            `json:"entity_key"`
		Owners     []datastore.OwnershipLookupResult `json:"owners"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EntityType != "node" {
		t.Errorf("entity_type = %q, want %q", resp.EntityType, "node")
	}
	if len(resp.Owners) != 1 {
		t.Fatalf("owners len = %d, want 1", len(resp.Owners))
	}
	if resp.Owners[0].OwnerName != "web-platform" {
		t.Errorf("owners[0].name = %q, want %q", resp.Owners[0].OwnerName, "web-platform")
	}
}

func TestLookup_NoOwners(t *testing.T) {
	store := &mockStore{
		LookupOwnershipFn: func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=cookbook&entity_key=nginx", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Owners []datastore.OwnershipLookupResult `json:"owners"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should be an empty array, not null.
	if resp.Owners == nil {
		t.Error("owners should be empty array, not nil")
	}
}

func TestLookup_MissingParams(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLookup_MissingEntityType(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_key=web-01", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLookup_WithOrganisation(t *testing.T) {
	var capturedOrgID string
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{ID: "org-1", Name: name}, nil
		},
		LookupOwnershipFn: func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error) {
			capturedOrgID = organisationID
			return []datastore.OwnershipLookupResult{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node&entity_key=web-01&organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedOrgID != "org-1" {
		t.Errorf("organisation_id = %q, want %q", capturedOrgID, "org-1")
	}
}

func TestLookup_OrganisationNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node&entity_key=web-01&organisation=missing", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestLookup_MethodNotAllowed(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/lookup", strings.NewReader("{}"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestLookup_DBError(t *testing.T) {
	store := &mockStore{
		LookupOwnershipFn: func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error) {
			return nil, errors.New("db error")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/lookup?entity_type=node&entity_key=web-01", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/audit-log — Audit Log
// ---------------------------------------------------------------------------

func TestAuditLog_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		ListAuditLogFn: func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			return []datastore.OwnershipAuditEntry{
				{
					ID:        "log-1",
					Timestamp: now,
					Action:    "owner_created",
					Actor:     "admin@example.com",
					OwnerName: "web-platform",
				},
			}, 1, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/audit-log", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1", body.Pagination.TotalItems)
	}
}

func TestAuditLog_WithFilters(t *testing.T) {
	var capturedFilter datastore.AuditLogFilter
	store := &mockStore{
		ListAuditLogFn: func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			capturedFilter = f
			return []datastore.OwnershipAuditEntry{}, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ownership/audit-log?action=owner_created&actor=admin&owner_name=web&entity_type=node&since=2024-01-01T00:00:00Z&until=2024-12-31T23:59:59Z",
		nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedFilter.Action != "owner_created" {
		t.Errorf("filter.Action = %q, want %q", capturedFilter.Action, "owner_created")
	}
	if capturedFilter.Actor != "admin" {
		t.Errorf("filter.Actor = %q, want %q", capturedFilter.Actor, "admin")
	}
	if capturedFilter.OwnerName != "web" {
		t.Errorf("filter.OwnerName = %q, want %q", capturedFilter.OwnerName, "web")
	}
	if capturedFilter.EntityType != "node" {
		t.Errorf("filter.EntityType = %q, want %q", capturedFilter.EntityType, "node")
	}
	if capturedFilter.Since.IsZero() {
		t.Error("filter.Since should not be zero")
	}
	if capturedFilter.Until.IsZero() {
		t.Error("filter.Until should not be zero")
	}
}

func TestAuditLog_MethodNotAllowed(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/audit-log", strings.NewReader("{}"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAuditLog_DBError(t *testing.T) {
	store := &mockStore{
		ListAuditLogFn: func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			return nil, 0, errors.New("db error")
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/audit-log", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// Unknown ownership endpoint
// ---------------------------------------------------------------------------

func TestOwnershipEndpoints_Unknown(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ownership/unknown", nil)
	// This would not match any registered route, so it falls through to
	// the default handler. We just verify it doesn't panic.
	r.ServeHTTP(w, req)
	// The expected status depends on routing; it should not be 200.
	if w.Code == http.StatusOK {
		t.Error("unexpected 200 for unknown ownership endpoint")
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookbooks/:name/committers — Cookbook Committers
// ---------------------------------------------------------------------------

func TestCookbookCommitters_HappyPath(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListCommittersByRepoFn: func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			return []datastore.GitRepoCommitter{
				{
					ID:            "c-1",
					GitRepoURL:    "https://gitlab.example.com/cookbooks/nginx.git",
					AuthorName:    "Jane Smith",
					AuthorEmail:   "jsmith@example.com",
					CommitCount:   47,
					FirstCommitAt: now.Add(-365 * 24 * time.Hour),
					LastCommitAt:  now,
				},
			}, 1, nil
		},
		// The cookbook detail handler may also call these, so provide defaults.
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nginx/committers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		CookbookName string                       `json:"cookbook_name"`
		GitRepoURL   string                       `json:"git_repo_url"`
		Data         []datastore.GitRepoCommitter `json:"data"`
		Pagination   struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.CookbookName != "nginx" {
		t.Errorf("cookbook_name = %q, want %q", resp.CookbookName, "nginx")
	}
	if resp.GitRepoURL != "https://gitlab.example.com/cookbooks/nginx.git" {
		t.Errorf("git_repo_url = %q", resp.GitRepoURL)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("data len = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].AuthorName != "Jane Smith" {
		t.Errorf("data[0].author_name = %q", resp.Data[0].AuthorName)
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1", resp.Pagination.TotalItems)
	}
}

func TestCookbookCommitters_NotGitSourced(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "", datastore.ErrNotFound
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/committers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCookbookCommitters_EmptyResults(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/test.git", nil
		},
		ListCommittersByRepoFn: func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			return nil, 0, nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/test/committers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []datastore.GitRepoCommitter `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should be an empty array, not null.
	if resp.Data == nil {
		t.Error("data should be empty array, not nil")
	}
}

func TestCookbookCommitters_WithSortAndSince(t *testing.T) {
	var capturedFilter datastore.CommitterListFilter
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListCommittersByRepoFn: func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			capturedFilter = f
			return []datastore.GitRepoCommitter{}, 0, nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nginx/committers?sort=commit_count&order=asc&since=2024-01-01T00:00:00Z", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedFilter.Sort != "commit_count" {
		t.Errorf("sort = %q, want %q", capturedFilter.Sort, "commit_count")
	}
	if capturedFilter.Order != "asc" {
		t.Errorf("order = %q, want %q", capturedFilter.Order, "asc")
	}
	if capturedFilter.Since.IsZero() {
		t.Error("since should not be zero")
	}
}

func TestCookbookCommitters_InvalidSince(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nginx/committers?since=not-a-date", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCookbookCommitters_DBError(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListCommittersByRepoFn: func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			return nil, 0, errors.New("timeout")
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nginx/committers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCookbookCommitters_MethodNotAllowed(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers", strings.NewReader("{}"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/cookbooks/:name/committers/assign — Assign Committers
// ---------------------------------------------------------------------------

func TestCookbookCommittersAssign_HappyPath(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		InsertOwnerFn: func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{ID: "new-" + p.Name, Name: p.Name, OwnerType: "individual"}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{ID: "a-new"}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"jsmith@example.com","owner_name":"jsmith","display_name":"Jane Smith"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		OwnersCreated      int `json:"owners_created"`
		AssignmentsCreated int `json:"assignments_created"`
		Skipped            int `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OwnersCreated != 1 {
		t.Errorf("owners_created = %d, want 1", resp.OwnersCreated)
	}
	if resp.AssignmentsCreated != 1 {
		t.Errorf("assignments_created = %d, want 1", resp.AssignmentsCreated)
	}
}

func TestCookbookCommittersAssign_ExistingOwner(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "existing-id", Name: name, OwnerType: "individual"}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{ID: "a-new"}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			return nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"jsmith@example.com","owner_name":"jsmith","display_name":"Jane Smith"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		OwnersCreated      int `json:"owners_created"`
		AssignmentsCreated int `json:"assignments_created"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OwnersCreated != 0 {
		t.Errorf("owners_created = %d, want 0 (existing owner)", resp.OwnersCreated)
	}
	if resp.AssignmentsCreated != 1 {
		t.Errorf("assignments_created = %d, want 1", resp.AssignmentsCreated)
	}
}

func TestCookbookCommittersAssign_DuplicateAssignment(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		GetOwnerByNameFn: func(ctx context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name}, nil
		},
		InsertAssignmentFn: func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{}, datastore.ErrAlreadyExists
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"jsmith@example.com","owner_name":"jsmith","display_name":"Jane Smith"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", resp.Skipped)
	}
}

func TestCookbookCommittersAssign_NotGitSourced(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "", datastore.ErrNotFound
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"a@b.com","owner_name":"a"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCookbookCommittersAssign_MissingAuthorEmail(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"owner_name":"jsmith"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCookbookCommittersAssign_MissingOwnerName(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"a@b.com"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCookbookCommittersAssign_EmptyCommitters(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCookbookCommittersAssign_OwnershipDisabled(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(ctx context.Context, cookbookName string) (string, error) {
			return "https://gitlab.example.com/cookbooks/nginx.git", nil
		},
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipDisabledRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"a@b.com","owner_name":"jsmith"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Owner filter helpers
// ---------------------------------------------------------------------------

func TestParseOwnerFilter_NoParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	f := parseOwnerFilter(req)

	if f.Active {
		t.Error("expected Active=false with no owner params")
	}
	if len(f.OwnerNames) != 0 {
		t.Errorf("expected empty OwnerNames, got %v", f.OwnerNames)
	}
	if f.Unowned {
		t.Error("expected Unowned=false")
	}
}

func TestParseOwnerFilter_WithOwner(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?owner=web-platform,payments-team", nil)
	f := parseOwnerFilter(req)

	if !f.Active {
		t.Error("expected Active=true")
	}
	if len(f.OwnerNames) != 2 {
		t.Errorf("expected 2 owner names, got %d", len(f.OwnerNames))
	}
	if f.OwnerNames[0] != "web-platform" {
		t.Errorf("OwnerNames[0] = %q, want %q", f.OwnerNames[0], "web-platform")
	}
}

func TestParseOwnerFilter_WithUnowned(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?unowned=true", nil)
	f := parseOwnerFilter(req)

	if !f.Active {
		t.Error("expected Active=true")
	}
	if !f.Unowned {
		t.Error("expected Unowned=true")
	}
}

func TestValidateOwnerFilter_MutuallyExclusive(t *testing.T) {
	w := httptest.NewRecorder()
	f := ownerFilter{
		OwnerNames: []string{"web"},
		Unowned:    true,
		Active:     true,
	}
	ok := validateOwnerFilter(w, f)
	if ok {
		t.Error("expected false when both owner and unowned are set")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestValidateOwnerFilter_OwnerOnly(t *testing.T) {
	w := httptest.NewRecorder()
	f := ownerFilter{
		OwnerNames: []string{"web"},
		Active:     true,
	}
	ok := validateOwnerFilter(w, f)
	if !ok {
		t.Error("expected true when only owner is set")
	}
}

func TestValidateOwnerFilter_UnownedOnly(t *testing.T) {
	w := httptest.NewRecorder()
	f := ownerFilter{
		Unowned: true,
		Active:  true,
	}
	ok := validateOwnerFilter(w, f)
	if !ok {
		t.Error("expected true when only unowned is set")
	}
}

func TestValidateOwnerFilter_Neither(t *testing.T) {
	w := httptest.NewRecorder()
	f := ownerFilter{}
	ok := validateOwnerFilter(w, f)
	if !ok {
		t.Error("expected true when neither is set")
	}
}

// ---------------------------------------------------------------------------
// Owner name validation regex
// ---------------------------------------------------------------------------

func TestOwnerNameRegex_Valid(t *testing.T) {
	valid := []string{"team-a", "web.platform", "sre_emea", "a", "a1b2c3", "0start"}
	for _, name := range valid {
		if !ownerNameRe.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
}

func TestOwnerNameRegex_Invalid(t *testing.T) {
	invalid := []string{"", "UPPERCASE", "-starts-with-dash", ".starts-with-dot", "_starts-with-underscore", "has space", "has@special"}
	for _, name := range invalid {
		if ownerNameRe.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: audit logging is called
// ---------------------------------------------------------------------------

func TestCreateOwner_AuditLog(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	var auditCalled bool
	store := &mockStore{
		InsertOwnerFn: func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{ID: "new-id", Name: p.Name, OwnerType: p.OwnerType, CreatedAt: now, UpdatedAt: now}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			auditCalled = true
			if p.Action != "owner_created" {
				t.Errorf("audit action = %q, want %q", p.Action, "owner_created")
			}
			if p.OwnerName != "audited-team" {
				t.Errorf("audit owner_name = %q, want %q", p.OwnerName, "audited-team")
			}
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"name":"audited-team","owner_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if !auditCalled {
		t.Error("audit log entry was not created")
	}
}

func TestDeleteOwner_AuditLog(t *testing.T) {
	var auditCalled bool
	store := &mockStore{
		DeleteOwnerFn: func(ctx context.Context, name string) (int, error) {
			return 3, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			auditCalled = true
			if p.Action != "owner_deleted" {
				t.Errorf("audit action = %q, want %q", p.Action, "owner_deleted")
			}
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/owners/delete-me", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if !auditCalled {
		t.Error("audit log entry was not created")
	}
}

func TestUpdateOwner_AuditLog(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	var auditCalled bool
	var auditDetails string
	store := &mockStore{
		UpdateOwnerFn: func(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{ID: "id-1", Name: name, OwnerType: "team", CreatedAt: now, UpdatedAt: now}, nil
		},
		InsertAuditEntryFn: func(ctx context.Context, p datastore.InsertAuditEntryParams) error {
			auditCalled = true
			if p.Action != "owner_updated" {
				t.Errorf("audit action = %q, want %q", p.Action, "owner_updated")
			}
			auditDetails = string(p.Details)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"display_name":"New Name","contact_email":"new@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/owners/my-team", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !auditCalled {
		t.Error("audit log entry was not created")
	}
	if !strings.Contains(auditDetails, "display_name") {
		t.Errorf("audit details should contain display_name, got %s", auditDetails)
	}
	if !strings.Contains(auditDetails, "contact_email") {
		t.Errorf("audit details should contain contact_email, got %s", auditDetails)
	}
}
