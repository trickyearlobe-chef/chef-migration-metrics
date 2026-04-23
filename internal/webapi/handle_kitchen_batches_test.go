// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/batches
// ---------------------------------------------------------------------------

func TestHandleCreateKitchenBatch_Success(t *testing.T) {
	store := &mockStore{
		CreateKitchenBatchFn: func(_ context.Context, p datastore.CreateKitchenBatchParams) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        "test-uuid-1",
				Name:      p.Name,
				Status:    datastore.BatchStatusDraft,
				DryRun:    p.DryRun,
				Filters:   p.Filters,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"RHEL 7 first pass","filters":{"cookbook_names":["b_win_*"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var batch datastore.KitchenBatch
	if err := json.NewDecoder(w.Body).Decode(&batch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if batch.Name != "RHEL 7 first pass" {
		t.Errorf("name = %q, want %q", batch.Name, "RHEL 7 first pass")
	}
	if batch.Status != datastore.BatchStatusDraft {
		t.Errorf("status = %q, want %q", batch.Status, datastore.BatchStatusDraft)
	}
}

func TestHandleCreateKitchenBatch_EmptyName(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"name":"","filters":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches", bytes.NewBufferString(body))
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

func TestHandleCreateKitchenBatch_InvalidJSON(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches", bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches
// ---------------------------------------------------------------------------

func TestHandleListKitchenBatches_Success(t *testing.T) {
	store := &mockStore{
		ListKitchenBatchesFn: func(_ context.Context) ([]datastore.KitchenBatch, error) {
			return []datastore.KitchenBatch{
				{ID: "batch-1", Name: "Batch A", Status: datastore.BatchStatusDraft, CreatedAt: time.Now()},
				{ID: "batch-2", Name: "Batch B", Status: datastore.BatchStatusRunning, CreatedAt: time.Now()},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var batches []datastore.KitchenBatch
	if err := json.NewDecoder(w.Body).Decode(&batches); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("len = %d, want 2", len(batches))
	}
}

func TestHandleListKitchenBatches_Empty(t *testing.T) {
	store := &mockStore{
		ListKitchenBatchesFn: func(_ context.Context) ([]datastore.KitchenBatch, error) {
			return []datastore.KitchenBatch{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var batches []datastore.KitchenBatch
	if err := json.NewDecoder(w.Body).Decode(&batches); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("len = %d, want 0", len(batches))
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

func TestHandleGetKitchenBatch_Success(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			if id != "test-uuid-1" {
				t.Errorf("unexpected id=%q", id)
			}
			return datastore.KitchenBatch{
				ID:     "test-uuid-1",
				Name:   "RHEL 7 first pass",
				Status: datastore.BatchStatusDraft,
				Filters: datastore.BatchFilters{
					CookbookNames: []string{"b_win_*"},
				},
				CreatedAt: time.Now(),
			}, nil
		},
		ListGitReposFn: func(_ context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["name"]; !ok {
		t.Error("response missing 'name' field")
	}
	if _, ok := resp["estimate"]; !ok {
		t.Error("response missing 'estimate' field")
	}
}

func TestHandleGetKitchenBatch_NotFound(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, _ string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/nonexistent", nil)
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
// PUT /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

func TestHandleUpdateKitchenBatch_Success(t *testing.T) {
	store := &mockStore{
		UpdateKitchenBatchFn: func(_ context.Context, id string, p datastore.UpdateKitchenBatchParams) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        id,
				Name:      p.Name,
				Status:    datastore.BatchStatusDraft,
				Filters:   p.Filters,
				DryRun:    p.DryRun,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"Updated batch","filters":{"cookbook_names":["app_*"]}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kitchen/batches/test-uuid-1", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var batch datastore.KitchenBatch
	if err := json.NewDecoder(w.Body).Decode(&batch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if batch.Name != "Updated batch" {
		t.Errorf("name = %q, want %q", batch.Name, "Updated batch")
	}
}

func TestHandleUpdateKitchenBatch_NotDraft(t *testing.T) {
	store := &mockStore{
		UpdateKitchenBatchFn: func(_ context.Context, _ string, _ datastore.UpdateKitchenBatchParams) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"Updated batch","filters":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kitchen/batches/test-uuid-1", bytes.NewBufferString(body))
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
// DELETE /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

func TestHandleDeleteKitchenBatch_Success(t *testing.T) {
	store := &mockStore{
		DeleteKitchenBatchFn: func(_ context.Context, id string) error {
			if id != "test-uuid-1" {
				t.Errorf("unexpected id=%q", id)
			}
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kitchen/batches/test-uuid-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestHandleDeleteKitchenBatch_NotFound(t *testing.T) {
	store := &mockStore{
		DeleteKitchenBatchFn: func(_ context.Context, _ string) error {
			return datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kitchen/batches/nonexistent", nil)
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
// POST /api/v1/kitchen/batches/:id/run
// ---------------------------------------------------------------------------

func TestHandleRunKitchenBatch_DryRun(t *testing.T) {
	statusCalls := 0
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Dry run batch",
				Status: datastore.BatchStatusDraft,
				DryRun: true,
				Filters: datastore.BatchFilters{
					CookbookNames: []string{"b_win_*"},
				},
				CreatedAt: time.Now(),
			}, nil
		},
		UpdateKitchenBatchStatusFn: func(_ context.Context, id string, status string, _ time.Time) (datastore.KitchenBatch, error) {
			statusCalls++
			return datastore.KitchenBatch{
				ID:        id,
				Name:      "Dry run batch",
				Status:    status,
				DryRun:    true,
				CreatedAt: time.Now(),
			}, nil
		},
		ListGitReposFn: func(_ context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// UpdateKitchenBatchStatus should be called twice: once for previewing, once for completed.
	if statusCalls != 2 {
		t.Errorf("UpdateKitchenBatchStatus called %d times, want 2", statusCalls)
	}
}

func TestHandleRunKitchenBatch_NotDraft(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, _ string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        "test-uuid-1",
				Name:      "Running batch",
				Status:    datastore.BatchStatusRunning,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/batches/:id/cancel
// ---------------------------------------------------------------------------

func TestHandleCancelKitchenBatch_Success(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        id,
				Name:      "Running batch",
				Status:    datastore.BatchStatusRunning,
				CreatedAt: time.Now(),
			}, nil
		},
		UpdateKitchenBatchStatusFn: func(_ context.Context, id string, status string, _ time.Time) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        id,
				Name:      "Running batch",
				Status:    status,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var batch datastore.KitchenBatch
	if err := json.NewDecoder(w.Body).Decode(&batch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if batch.Status != datastore.BatchStatusCancelled {
		t.Errorf("status = %q, want %q", batch.Status, datastore.BatchStatusCancelled)
	}
}

func TestHandleCancelKitchenBatch_NotRunning(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, _ string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:        "test-uuid-1",
				Name:      "Completed batch",
				Status:    datastore.BatchStatusCompleted,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

func TestHandleGitRepoExclude_Success(t *testing.T) {
	store := &mockStore{
		SetGitRepoKitchenExclusionFn: func(_ context.Context, name string, reason string, excludedBy string) error {
			if name != "my-cookbook" {
				t.Errorf("unexpected name=%q", name)
			}
			if reason != "deprecated" {
				t.Errorf("unexpected reason=%q", reason)
			}
			if excludedBy != "admin" {
				t.Errorf("unexpected excluded_by=%q", excludedBy)
			}
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"reason":"deprecated","excluded_by":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/my-cookbook/exclude", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleGitRepoExclude_NotFound(t *testing.T) {
	store := &mockStore{
		SetGitRepoKitchenExclusionFn: func(_ context.Context, _ string, _ string, _ string) error {
			return datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"reason":"deprecated","excluded_by":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/nonexistent/exclude", bytes.NewBufferString(body))
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
// DELETE /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

func TestHandleGitRepoClearExclusion_Success(t *testing.T) {
	store := &mockStore{
		ClearGitRepoKitchenExclusionFn: func(_ context.Context, name string) error {
			if name != "my-cookbook" {
				t.Errorf("unexpected name=%q", name)
			}
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/git-repos/my-cookbook/exclude", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/git-repos/excluded
// ---------------------------------------------------------------------------

func TestHandleListExcludedGitRepos_Success(t *testing.T) {
	store := &mockStore{
		ListExcludedGitReposFn: func(_ context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{Name: "old-cookbook", KitchenExcluded: true, KitchenExcludeReason: "deprecated"},
				{Name: "broken-cookbook", KitchenExcluded: true, KitchenExcludeReason: "failing tests"},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/excluded", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var repos []datastore.GitRepo
	if err := json.NewDecoder(w.Body).Decode(&repos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("len = %d, want 2", len(repos))
	}
}
