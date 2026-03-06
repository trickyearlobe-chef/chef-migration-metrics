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
// handleLogs — method checks and parameter validation
// ---------------------------------------------------------------------------

func TestHandleLogs_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/logs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLogs_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/v1/logs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLogs_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/v1/logs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLogs_InvalidSinceParam(t *testing.T) {
	// The handler calls r.db which is nil, but the 'since' validation happens
	// before the DB call. A bad 'since' value should return 400.
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since=not-a-date", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/v1/logs?since=not-a-date status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}

func TestHandleLogs_InvalidUntilParam(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?until=yesterday", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/v1/logs?until=yesterday status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}

func TestHandleLogs_ValidSinceParamAccepted(t *testing.T) {
	store := &mockStore{
		CountLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
			// Verify the since filter was parsed correctly.
			expected := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			if !filter.Since.Equal(expected) {
				t.Errorf("filter.Since = %v, want %v", filter.Since, expected)
			}
			return 0, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since=2025-01-01T00:00:00Z", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("valid 'since' param returned 400 — parameter validation rejected a valid RFC3339 timestamp")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// handleLogDetail — method checks, missing ID, sub-path handling
// ---------------------------------------------------------------------------

func TestHandleLogDetail_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/some-uuid", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/logs/some-uuid status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLogDetail_MissingID(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/logs/ (no ID) status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

func TestHandleLogDetail_SubPathNotFound(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	// Two segments — should be rejected as not a valid single-ID path.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/some-uuid/extra", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/logs/some-uuid/extra status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleCollectionRuns — method checks
// ---------------------------------------------------------------------------

func TestHandleCollectionRuns_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/logs/collection-runs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCollectionRuns_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/v1/logs/collection-runs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCollectionRuns_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/v1/logs/collection-runs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Route wiring — ensure collection-runs is matched before log detail
// ---------------------------------------------------------------------------

func TestCollectionRunsRouteTakesPriorityOverLogDetail(t *testing.T) {
	// /api/v1/logs/collection-runs should be matched by handleCollectionRuns,
	// NOT by handleLogDetail with id = "collection-runs". Since db is nil,
	// handleCollectionRuns will panic on ListOrganisations, while
	// handleLogDetail would panic on GetLogEntry. We verify via the method
	// check: POST should give 405 from handleCollectionRuns's requireGET,
	// not a 404 from handleLogDetail's segment check.
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/logs/collection-runs status = %d, want %d (route priority test)", w.Code, http.StatusMethodNotAllowed)
	}

	// Also verify the error message mentions GET requirement, not "not found".
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error code = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Content-Type verification
// ---------------------------------------------------------------------------

func TestHandleLogs_InvalidSince_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since=bad", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandleLogDetail_MissingID_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandleCollectionRuns_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

// ---------------------------------------------------------------------------
// Error response structure validation
// ---------------------------------------------------------------------------

func TestHandleLogs_InvalidParam_ErrorStructure(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since=xyz", nil)
	r.ServeHTTP(w, req)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error code")
	}
	if resp.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// handleLogs — happy paths with mock DB
// ---------------------------------------------------------------------------

func TestHandleLogs_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
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

func TestHandleLogs_HappyPath_WithEntries(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		CountLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
			return 2, nil
		},
		ListLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
			return []datastore.LogEntry{
				{ID: "log-1", Severity: "INFO", Scope: "collection", Message: "started", Timestamp: now},
				{ID: "log-2", Severity: "WARN", Scope: "collection", Message: "slow", Timestamp: now},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
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

func TestHandleLogs_HappyPath_FilterParams(t *testing.T) {
	var capturedFilter datastore.LogEntryFilter
	store := &mockStore{
		CountLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
			capturedFilter = filter
			return 0, nil
		},
		ListLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?scope=collection&severity=ERROR&organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedFilter.Scope != "collection" {
		t.Errorf("filter.Scope = %q, want %q", capturedFilter.Scope, "collection")
	}
	if capturedFilter.Severity != "ERROR" {
		t.Errorf("filter.Severity = %q, want %q", capturedFilter.Severity, "ERROR")
	}
	if capturedFilter.Organisation != "prod" {
		t.Errorf("filter.Organisation = %q, want %q", capturedFilter.Organisation, "prod")
	}
}

// ---------------------------------------------------------------------------
// handleLogs — DB errors
// ---------------------------------------------------------------------------

func TestHandleLogs_DBError_Count(t *testing.T) {
	store := &mockStore{
		CountLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
			return 0, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleLogs_DBError_List(t *testing.T) {
	store := &mockStore{
		CountLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
			return 5, nil
		},
		ListLogEntriesFn: func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
			return nil, errors.New("timeout")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleLogDetail — happy path, not-found, DB error
// ---------------------------------------------------------------------------

func TestHandleLogDetail_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetLogEntryFn: func(ctx context.Context, id string) (datastore.LogEntry, error) {
			if id == "abc-123" {
				return datastore.LogEntry{ID: "abc-123", Severity: "INFO", Message: "hello", Timestamp: now}, nil
			}
			return datastore.LogEntry{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/abc-123", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var entry struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.ID != "abc-123" {
		t.Errorf("id = %q, want %q", entry.ID, "abc-123")
	}
	if entry.Message != "hello" {
		t.Errorf("message = %q, want %q", entry.Message, "hello")
	}
}

func TestHandleLogDetail_NotFound(t *testing.T) {
	store := &mockStore{
		GetLogEntryFn: func(ctx context.Context, id string) (datastore.LogEntry, error) {
			return datastore.LogEntry{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/no-such-id", nil)
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

func TestHandleLogDetail_DBError(t *testing.T) {
	store := &mockStore{
		GetLogEntryFn: func(ctx context.Context, id string) (datastore.LogEntry, error) {
			return datastore.LogEntry{}, errors.New("disk I/O error")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/abc-123", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleCollectionRuns — happy path, DB error
// ---------------------------------------------------------------------------

func TestHandleCollectionRuns_HappyPath_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/collection-runs", nil)
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

func TestHandleCollectionRuns_HappyPath_WithRuns(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{ID: "org-1", Name: "prod"}}, nil
		},
		ListCollectionRunsFn: func(ctx context.Context, orgID string, limit int) ([]datastore.CollectionRun, error) {
			return []datastore.CollectionRun{
				{ID: "run-1", OrganisationID: "org-1", Status: "completed", StartedAt: now, CompletedAt: now},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/collection-runs", nil)
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

func TestHandleCollectionRuns_HappyPath_FilterByOrg(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "prod"},
				{ID: "org-2", Name: "staging"},
			}, nil
		},
		ListCollectionRunsFn: func(ctx context.Context, orgID string, limit int) ([]datastore.CollectionRun, error) {
			return []datastore.CollectionRun{
				{ID: "run-" + orgID, OrganisationID: orgID, Status: "completed", StartedAt: now},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/collection-runs?organisation=prod", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (filtered to prod only)", body.Pagination.TotalItems)
	}
}

func TestHandleCollectionRuns_DBError_ListOrganisations(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/collection-runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
