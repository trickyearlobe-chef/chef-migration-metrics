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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
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
		CancelPendingBatchInstancesFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
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

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/batches/:id/run — IsEnabled gate
// ---------------------------------------------------------------------------

func TestHandleRunKitchenBatch_DisabledReturns409(t *testing.T) {
	disabled := false
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "My batch",
				Status: datastore.BatchStatusDraft,
				DryRun: true,
			}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.Enabled = &disabled

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Batch execution wiring tests
// ---------------------------------------------------------------------------

func TestHandleRunKitchenBatch_NonDryRun_Returns202(t *testing.T) {
	statusCalls := []string{}
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Real batch",
				Status: datastore.BatchStatusDraft,
				DryRun: false,
				Filters: datastore.BatchFilters{
					CookbookNames:      []string{"my-cookbook"},
					TargetChefVersions: []string{"18.5.0"},
				},
				CreatedAt: time.Now(),
			}, nil
		},
		UpdateKitchenBatchStatusFn: func(_ context.Context, id string, status string, _ time.Time) (datastore.KitchenBatch, error) {
			statusCalls = append(statusCalls, status)
			return datastore.KitchenBatch{
				ID:        id,
				Name:      "Real batch",
				Status:    status,
				CreatedAt: time.Now(),
			}, nil
		},
		ListGitReposFn: func(_ context.Context) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{Name: "my-cookbook", GitRepoURL: "https://git.example.com/my-cookbook.git", HasTestSuite: true},
			}, nil
		},
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, name string) (*datastore.KitchenAnalysisResult, error) {
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   name,
				GitRepoURL:    "https://git.example.com/" + name + ".git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"}]`),
			}, nil
		},
		CreateBatchInstancesFn: func(_ context.Context, params []datastore.CreateBatchInstanceParams) ([]datastore.KitchenBatchInstance, error) {
			result := make([]datastore.KitchenBatchInstance, len(params))
			for i, p := range params {
				result[i] = datastore.KitchenBatchInstance{
					ID:           "inst-" + p.InstanceName,
					BatchID:      p.BatchID,
					GitRepoName:  p.GitRepoName,
					InstanceName: p.InstanceName,
					Status:       "pending",
				}
			}
			return result, nil
		},
		UpdateBatchInstanceStatusFn: func(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
			return nil
		},
		UpdateKitchenBatchStatusIfCurrentFn: func(_ context.Context, _ string, _, _ string, _ time.Time) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{Status: "completed"}, nil
		},
		CancelPendingBatchInstancesFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		CountBatchInstancesByStatusFn: func(_ context.Context, _ string) (map[string]int, error) {
			return map[string]int{"passed": 1}, nil
		},
		CancelKitchenRunsByBatchFn: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
		ListKitchenQueueFn: func(_ context.Context, _ datastore.KitchenQueueFilter) ([]datastore.KitchenQueueItem, error) {
			return nil, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-template"},
	}

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Cancel the background goroutine and wait for it to exit.
	t.Cleanup(func() {
		r.batchMu.Lock()
		if fn, ok := r.runningBatch["test-uuid-1"]; ok {
			fn()
		}
		r.batchMu.Unlock()
		// Wait for the goroutine to remove itself from runningBatch.
		for i := 0; i < 100; i++ {
			time.Sleep(50 * time.Millisecond)
			r.batchMu.Lock()
			_, running := r.runningBatch["test-uuid-1"]
			r.batchMu.Unlock()
			if !running {
				return
			}
		}
		t.Log("WARN: background batch goroutine did not exit within 5s")
	})

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	// First status transition should be to "preparing".
	if len(statusCalls) == 0 || statusCalls[0] != "preparing" {
		t.Errorf("first status transition = %v, want [preparing, ...]", statusCalls)
	}
}

func TestHandleRunKitchenBatch_SingleRunningGuard(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Batch 2",
				Status: datastore.BatchStatusDraft,
				DryRun: false,
			}, nil
		},
	}

	cfg := testConfig()

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	// Simulate an already-running batch in the map.
	r.batchMu.Lock()
	r.runningBatch["existing-batch"] = func() {}
	r.batchMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-2/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestHandleRunKitchenBatch_NoQueue(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Batch",
				Status: datastore.BatchStatusDraft,
				DryRun: false,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

func TestHandleCancelKitchenBatch_CallsCancelFunc(t *testing.T) {
	cancelCalled := false
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Running batch",
				Status: datastore.BatchStatusRunning,
			}, nil
		},
		UpdateKitchenBatchStatusFn: func(_ context.Context, id string, status string, _ time.Time) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{ID: id, Status: status}, nil
		},
		CancelPendingBatchInstancesFn: func(_ context.Context, _ string) (int, error) {
			return 3, nil
		},
	}

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, testConfig(), hub)

	// Register a fake cancel function.
	r.batchMu.Lock()
	r.runningBatch["test-uuid-1"] = func() { cancelCalled = true }
	r.batchMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !cancelCalled {
		t.Error("expected cancel function to be called")
	}

	// Cancel should NOT remove from runningBatch (goroutine does that on exit).
	r.batchMu.Lock()
	_, stillInMap := r.runningBatch["test-uuid-1"]
	r.batchMu.Unlock()
	if !stillInMap {
		t.Error("batch should remain in runningBatch until goroutine exits")
	}
}

func TestHandleCancelKitchenBatch_PreparingStatus(t *testing.T) {
	store := &mockStore{
		GetKitchenBatchFn: func(_ context.Context, id string) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{
				ID:     id,
				Name:   "Preparing batch",
				Status: "preparing",
			}, nil
		},
		UpdateKitchenBatchStatusFn: func(_ context.Context, id string, status string, _ time.Time) (datastore.KitchenBatch, error) {
			return datastore.KitchenBatch{ID: id, Status: status}, nil
		},
		CancelPendingBatchInstancesFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/progress
// ---------------------------------------------------------------------------

func TestHandleBatchProgress_Success(t *testing.T) {
	store := &mockStore{
		CountBatchInstancesByStatusFn: func(_ context.Context, _ string) (map[string]int, error) {
			return map[string]int{
				"pending": 5,
				"running": 2,
				"passed":  10,
				"failed":  1,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1/progress", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result map[string]int
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["total"] != 18 {
		t.Errorf("total = %d, want 18", result["total"])
	}
	if result["passed"] != 10 {
		t.Errorf("passed = %d, want 10", result["passed"])
	}
	if result["pending"] != 5 {
		t.Errorf("pending = %d, want 5", result["pending"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/instances
// ---------------------------------------------------------------------------

func TestHandleListBatchInstances_Success(t *testing.T) {
	started := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	var gotBatchID string
	store := &mockStore{
		ListBatchInstancesFn: func(_ context.Context, batchID string) ([]datastore.KitchenBatchInstance, error) {
			gotBatchID = batchID
			return []datastore.KitchenBatchInstance{
				{
					ID:           "inst-1",
					BatchID:      batchID,
					GitRepoName:  "nginx",
					InstanceName: "default-ubuntu-2204",
					PlatformName: "ubuntu-22.04",
					SuiteName:    "default",
					Status:       "passed",
					StartedAt:    &started,
				},
				{
					ID:           "inst-2",
					BatchID:      batchID,
					GitRepoName:  "apache",
					InstanceName: "default-centos-8",
					PlatformName: "centos-8",
					SuiteName:    "default",
					Status:       "failed",
					ErrorMessage: "converge failed",
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if gotBatchID != "test-uuid-1" {
		t.Errorf("ListBatchInstances called with %q, want %q", gotBatchID, "test-uuid-1")
	}

	var result []datastore.KitchenBatchInstance
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].GitRepoName != "nginx" || result[0].Status != "passed" {
		t.Errorf("result[0] = %+v, want nginx/passed", result[0])
	}
	if result[1].ErrorMessage != "converge failed" {
		t.Errorf("result[1].ErrorMessage = %q, want %q", result[1].ErrorMessage, "converge failed")
	}
}

func TestHandleListBatchInstances_Empty(t *testing.T) {
	store := &mockStore{
		ListBatchInstancesFn: func(_ context.Context, _ string) ([]datastore.KitchenBatchInstance, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Empty list must serialise as [] not null.
	if got := w.Body.String(); got != "[]\n" && got != "[]" {
		t.Errorf("body = %q, want empty JSON array", got)
	}
}

func TestHandleListBatchInstances_Error(t *testing.T) {
	store := &mockStore{
		ListBatchInstancesFn: func(_ context.Context, _ string) ([]datastore.KitchenBatchInstance, error) {
			return nil, context.DeadlineExceeded
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleListBatchInstances_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/batches/test-uuid-1/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleBatchProgress_Empty(t *testing.T) {
	store := &mockStore{
		CountBatchInstancesByStatusFn: func(_ context.Context, _ string) (map[string]int, error) {
			return map[string]int{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/test-uuid-1/progress", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result map[string]int
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["total"] != 0 {
		t.Errorf("total = %d, want 0", result["total"])
	}
}
