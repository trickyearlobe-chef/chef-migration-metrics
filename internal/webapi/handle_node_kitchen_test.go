// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/node-runs
// ---------------------------------------------------------------------------

func TestHandleNodeKitchenRuns_WithNodeFilter(t *testing.T) {
	store := &mockStore{
		ListNodeKitchenRunsByNodeFn: func(_ context.Context, org, node string) ([]datastore.NodeKitchenRun, error) {
			if org != "test-org" || node != "web1" {
				t.Errorf("unexpected args: org=%q node=%q", org, node)
			}
			return []datastore.NodeKitchenRun{
				{ID: "run-1", NodeName: "web1", OrganisationName: "test-org"},
				{ID: "run-2", NodeName: "web1", OrganisationName: "test-org"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/node-runs?org=test-org&node=web1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var runs []datastore.NodeKitchenRun
	if err := json.NewDecoder(w.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
}

func TestHandleNodeKitchenRuns_OrgOnly(t *testing.T) {
	store := &mockStore{
		ListNodeKitchenRunsFn: func(_ context.Context, org string) ([]datastore.NodeKitchenRun, error) {
			if org != "test-org" {
				t.Errorf("unexpected org=%q", org)
			}
			return []datastore.NodeKitchenRun{
				{ID: "run-1", NodeName: "db1", OrganisationName: "test-org"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/node-runs?org=test-org", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var runs []datastore.NodeKitchenRun
	if err := json.NewDecoder(w.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len = %d, want 1", len(runs))
	}
}

func TestHandleNodeKitchenRuns_MissingOrg(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/node-runs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}

func TestHandleNodeKitchenRuns_WrongMethod(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/node-runs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/node-runs/<id>
// ---------------------------------------------------------------------------

func TestHandleGetNodeKitchenRun_Found(t *testing.T) {
	store := &mockStore{
		GetNodeKitchenRunFn: func(_ context.Context, id string) (*datastore.NodeKitchenRun, error) {
			if id != "test-uuid" {
				t.Errorf("unexpected id=%q", id)
			}
			return &datastore.NodeKitchenRun{
				ID:                "test-uuid",
				NodeName:          "web1",
				OrganisationName:  "test-org",
				TargetChefVersion: "18.4.2",
				CookbookSource:    "server",
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/node-runs/test-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var run datastore.NodeKitchenRun
	if err := json.NewDecoder(w.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.ID != "test-uuid" {
		t.Errorf("id = %q, want %q", run.ID, "test-uuid")
	}
	if run.NodeName != "web1" {
		t.Errorf("node_name = %q, want %q", run.NodeName, "web1")
	}
	if run.OrganisationName != "test-org" {
		t.Errorf("organisation_name = %q, want %q", run.OrganisationName, "test-org")
	}
}

func TestHandleGetNodeKitchenRun_NotFound(t *testing.T) {
	store := &mockStore{
		GetNodeKitchenRunFn: func(_ context.Context, _ string) (*datastore.NodeKitchenRun, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/node-runs/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/kitchen/node-runs/<id>
// ---------------------------------------------------------------------------

func TestHandleDeleteNodeKitchenRun_Success(t *testing.T) {
	store := &mockStore{
		DeleteNodeKitchenRunFn: func(_ context.Context, id string) error {
			if id != "test-uuid" {
				t.Errorf("unexpected id=%q", id)
			}
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kitchen/node-runs/test-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestHandleDeleteNodeKitchenRun_NotFound(t *testing.T) {
	store := &mockStore{
		DeleteNodeKitchenRunFn: func(_ context.Context, _ string) error {
			return datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kitchen/node-runs/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/node-run
// ---------------------------------------------------------------------------

func TestHandleNodeKitchenTrigger_RunnerNotConfigured(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"node_name":"web1","organisation_name":"test-org","target_chef_version":"18.4.2","cookbook_source":"server"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/node-run", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeServiceUnavailable {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeServiceUnavailable)
	}
}

func TestHandleNodeKitchenTrigger_InvalidBody(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	// Missing required fields.
	body := `{"node_name":"web1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/node-run", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}
