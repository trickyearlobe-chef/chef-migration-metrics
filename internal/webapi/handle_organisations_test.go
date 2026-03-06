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
// handleOrganisations — method checks
// ---------------------------------------------------------------------------

func TestHandleOrganisations_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleOrganisations_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleOrganisations_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// handleOrganisations — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleOrganisations_HappyPath_EmptyList(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations", nil)
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

func TestHandleOrganisations_HappyPath_WithOrgs(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod", ChefServerURL: "https://chef.example.com", OrgName: "production", ClientName: "admin", Source: "config"},
				{ID: "org-2", Name: "staging", ChefServerURL: "https://chef-stg.example.com", OrgName: "staging", ClientName: "admin", ClientKeyCredentialID: "cred-123", Source: "config"},
			}, nil
		},
		GetLatestCollectionRunFn: func(ctx context.Context, orgID string) (datastore.CollectionRun, error) {
			if orgID == "org-1" {
				return datastore.CollectionRun{Status: "completed", NodesCollected: 42, CompletedAt: now}, nil
			}
			return datastore.CollectionRun{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			Name                 string `json:"name"`
			CredentialSource     string `json:"credential_source"`
			NodeCount            int    `json:"node_count"`
			LastCollectedAt      string `json:"last_collected_at"`
			LastCollectionStatus string `json:"last_collection_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(body.Data))
	}
	if body.Data[0].Name != "prod" {
		t.Errorf("data[0].name = %q, want %q", body.Data[0].Name, "prod")
	}
	if body.Data[0].CredentialSource != "config" {
		t.Errorf("data[0].credential_source = %q, want %q", body.Data[0].CredentialSource, "config")
	}
	if body.Data[0].NodeCount != 42 {
		t.Errorf("data[0].node_count = %d, want 42", body.Data[0].NodeCount)
	}
	if body.Data[0].LastCollectionStatus != "completed" {
		t.Errorf("data[0].last_collection_status = %q, want %q", body.Data[0].LastCollectionStatus, "completed")
	}
	if body.Data[1].CredentialSource != "secrets_store" {
		t.Errorf("data[1].credential_source = %q, want %q", body.Data[1].CredentialSource, "secrets_store")
	}
	if body.Data[1].LastCollectionStatus != "" {
		t.Errorf("data[1].last_collection_status = %q, want empty", body.Data[1].LastCollectionStatus)
	}
}

func TestHandleOrganisations_HappyPath_RunningUsesStartedAt(t *testing.T) {
	started := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "dev", Source: "config", ClientName: "c", ChefServerURL: "u", OrgName: "o"}}, nil
		},
		GetLatestCollectionRunFn: func(ctx context.Context, orgID string) (datastore.CollectionRun, error) {
			return datastore.CollectionRun{Status: "running", StartedAt: started}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []struct {
			LastCollectedAt string `json:"last_collected_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Data[0].LastCollectedAt != "2025-06-15T10:00:00Z" {
		t.Errorf("last_collected_at = %q, want %q", body.Data[0].LastCollectedAt, "2025-06-15T10:00:00Z")
	}
}

// ---------------------------------------------------------------------------
// handleOrganisations — DB errors
// ---------------------------------------------------------------------------

func TestHandleOrganisations_DBError_ListOrganisations(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleOrganisations_DBError_LatestRunNonFatal(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod", Source: "config", ClientName: "c", ChefServerURL: "u", OrgName: "o"}}, nil
		},
		GetLatestCollectionRunFn: func(ctx context.Context, orgID string) (datastore.CollectionRun, error) {
			return datastore.CollectionRun{}, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (non-fatal DB error)", w.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// handleOrganisationDetail — method checks & missing name
// ---------------------------------------------------------------------------

func TestHandleOrganisationDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organisations/prod", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleOrganisationDetail_MissingName(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleOrganisationDetail_TestSubpath(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations/prod/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

// ---------------------------------------------------------------------------
// handleOrganisationDetail — happy path
// ---------------------------------------------------------------------------

func TestHandleOrganisationDetail_HappyPath(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			if name == "prod" {
				return datastore.Organisation{ID: "org-1", Name: "prod", ChefServerURL: "https://chef.example.com", OrgName: "production", ClientName: "admin", Source: "config"}, nil
			}
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations/prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var org struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &org); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if org.Name != "prod" {
		t.Errorf("name = %q, want %q", org.Name, "prod")
	}
}

// ---------------------------------------------------------------------------
// handleOrganisationDetail — ErrNotFound
// ---------------------------------------------------------------------------

func TestHandleOrganisationDetail_NotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations/nope", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleOrganisationDetail — DB error
// ---------------------------------------------------------------------------

func TestHandleOrganisationDetail_DBError(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, errors.New("disk I/O error")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organisations/prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
