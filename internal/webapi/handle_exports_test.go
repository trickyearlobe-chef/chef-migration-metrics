// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// POST /api/v1/exports — method checks
// ---------------------------------------------------------------------------

func TestHandleExports_MethodNotAllowed_GET(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /exports status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleExports_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/exports", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /exports status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleExports_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/exports", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /exports status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleExports_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/exports — validation (query-param API)
// ---------------------------------------------------------------------------

func TestHandleExports_InvalidExportType(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=bogus&format=csv", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	assertBodyContains(t, w, "nodes, cookbooks, roles, git_repos")
}

func TestHandleExports_InvalidFormat(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=nodes&format=xml", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertBodyContains(t, w, "Invalid format")
}

func TestHandleExports_ChefSearchQueryNonNodes(t *testing.T) {
	// chef_search_query is nodes-only; the guard fires before any store call.
	r := newTestRouterWithMockAndConfig(&mockStore{}, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=cookbooks&format=chef_search_query", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	assertBodyContains(t, w, "only supported for the nodes export")
}

// ---------------------------------------------------------------------------
// POST /api/v1/exports — synchronous streaming per list view
// ---------------------------------------------------------------------------

func oneOrgStore() *mockStore {
	return &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "production"}}, nil
		},
	}
}

func TestHandleExports_Sync_NodesCSV(t *testing.T) {
	free := 5000
	store := oneOrgStore()
	store.CountNodeSnapshotsFilteredFn = func(ctx context.Context, f datastore.NodeSnapshotFilter) (int, error) { return 1, nil }
	store.ListNodeSnapshotsForExportFn = func(ctx context.Context, f datastore.NodeSnapshotFilter, after datastore.NodeSnapshotCursor, limit int) ([]datastore.NodeSnapshot, error) {
		if after.Valid {
			return nil, nil
		}
		return []datastore.NodeSnapshot{
			{OrganisationName: "production", NodeName: "web1", ChefEnvironment: "prod", ChefVersion: "17.10.0", Platform: "ubuntu", PlatformVersion: "22.04", AvailableDiskMB: &free, OhaiTime: 1719400000, CollectedAt: time.Now().UTC()},
		}, nil
	}

	r := newTestRouterWithMockAndConfig(store, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=nodes&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if rc := w.Header().Get("X-Export-Row-Count"); rc != "1" {
		t.Errorf("X-Export-Row-Count = %q, want 1", rc)
	}
	body := w.Body.String()
	// ohai_time renders as a datetime (like collected_at), not a unix epoch.
	wantOhai := time.Unix(1719400000, 0).UTC().Format("2006-01-02T15:04:05Z")
	for _, want := range []string{"node_name", "web1", "available_disk_mb", "5000", "install_path", "ohai_time", wantOhai} {
		if !strings.Contains(body, want) {
			t.Errorf("CSV missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "1719400000") {
		t.Errorf("CSV should not contain the raw ohai epoch:\n%s", body)
	}
}

func TestHandleExports_Sync_NodesChefSearchQuery(t *testing.T) {
	store := oneOrgStore()
	store.CountNodeSnapshotsFilteredFn = func(ctx context.Context, f datastore.NodeSnapshotFilter) (int, error) { return 2, nil }
	store.ListNodeSnapshotsForExportFn = func(ctx context.Context, f datastore.NodeSnapshotFilter, after datastore.NodeSnapshotCursor, limit int) ([]datastore.NodeSnapshot, error) {
		if after.Valid {
			return nil, nil
		}
		return []datastore.NodeSnapshot{
			{OrganisationName: "production", NodeName: "web1", CollectedAt: time.Now().UTC()},
			{OrganisationName: "production", NodeName: "db1", CollectedAt: time.Now().UTC()},
		}, nil
	}

	r := newTestRouterWithMockAndConfig(store, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=nodes&format=chef_search_query", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "name:web1 OR name:db1" {
		t.Errorf("chef search = %q, want %q", got, "name:web1 OR name:db1")
	}
}

func TestHandleExports_Sync_Cookbooks(t *testing.T) {
	store := oneOrgStore()
	store.ListCookbooksFilteredFn = func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
		return []datastore.CookbookFilterRow{
			{OrganisationName: "production", Name: "apt", Version: "7.4.0", CookstyleStatus: "ready", License: "Apache-2.0"},
		}, 1, nil
	}
	r := newTestRouterWithMockAndConfig(store, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=cookbooks&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"cookstyle_status", "apt", "license", "Apache-2.0"} {
		if !strings.Contains(body, want) {
			t.Errorf("cookbook CSV missing %q:\n%s", want, body)
		}
	}
}

func TestHandleExports_Sync_Roles(t *testing.T) {
	store := oneOrgStore()
	store.ListRolesFilteredFn = func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
		return []datastore.RoleFilterRow{
			{RoleName: "base", Organisations: []string{"production"}, CompatibilityStatus: "compatible"},
		}, 1, datastore.RoleFilterSummary{}, nil
	}
	r := newTestRouterWithMockAndConfig(store, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=roles&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"role_name", "base", "compatibility_status", "compatible"} {
		if !strings.Contains(body, want) {
			t.Errorf("role CSV missing %q:\n%s", want, body)
		}
	}
}

func TestHandleExports_Sync_GitRepos(t *testing.T) {
	store := &mockStore{
		ListGitReposFilteredFn: func(ctx context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
			return []datastore.GitRepo{
				{Name: "cookbook-apt", CloneStatus: "ok", CookstyleStatus: "ready", TKStatus: "passed"},
			}, 1, nil
		},
	}
	r := newTestRouterWithMockAndConfig(store, exportTestConfig())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=git_repos&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"name", "cookbook-apt", "cookstyle_status", "tk_status"} {
		if !strings.Contains(body, want) {
			t.Errorf("git repo CSV missing %q:\n%s", want, body)
		}
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/exports — async dispatch
// ---------------------------------------------------------------------------

func TestHandleExports_Async_LargeEstimate(t *testing.T) {
	insertCalled := false
	store := oneOrgStore()
	store.CountNodeSnapshotsFilteredFn = func(ctx context.Context, f datastore.NodeSnapshotFilter) (int, error) { return 50000, nil }
	store.InsertExportJobFn = func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error) {
		insertCalled = true
		if p.ExportType != "nodes" {
			t.Errorf("InsertExportJob export_type = %q, want nodes", p.ExportType)
		}
		return &datastore.ExportJob{ID: "job-async-001", ExportType: p.ExportType, Format: p.Format, Status: datastore.ExportStatusPending, RequestedAt: time.Now().UTC()}, nil
	}
	store.UpdateExportJobStatusFn = func(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error {
		return nil
	}
	store.ListNodeSnapshotsForExportFn = func(ctx context.Context, f datastore.NodeSnapshotFilter, after datastore.NodeSnapshotCursor, limit int) ([]datastore.NodeSnapshot, error) {
		return nil, nil
	}

	cfg := exportTestConfig()
	cfg.Exports.AsyncThreshold = 100
	cfg.Exports.OutputDirectory = t.TempDir()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=nodes&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
	if !insertCalled {
		t.Error("InsertExportJob was not called for async export")
	}
	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JobID != "job-async-001" {
		t.Errorf("job_id = %q, want job-async-001", resp.JobID)
	}
}

func TestHandleExports_Async_InsertJobError(t *testing.T) {
	store := oneOrgStore()
	store.CountNodeSnapshotsFilteredFn = func(ctx context.Context, f datastore.NodeSnapshotFilter) (int, error) { return 50000, nil }
	store.InsertExportJobFn = func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error) {
		return nil, fmt.Errorf("database connection lost")
	}

	cfg := exportTestConfig()
	cfg.Exports.AsyncThreshold = 100

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports?export_type=nodes&format=csv", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/exports/:id — job status
// ---------------------------------------------------------------------------

func TestHandleExportStatus_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports/some-id", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /exports/:id status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleExportStatus_NotFound(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return nil, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/nonexistent-id", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleExportStatus_NoJobID(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()

	// Path /api/v1/exports/ with trailing slash but no ID segment.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleExportStatus_Pending(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			if id != "job-001" {
				return nil, datastore.ErrNotFound
			}
			return &datastore.ExportJob{
				ID:          "job-001",
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusPending,
				RequestedAt: now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.JobID != "job-001" {
		t.Errorf("job_id = %q, want %q", resp.JobID, "job-001")
	}
	if resp.Status != "pending" {
		t.Errorf("status = %q, want pending", resp.Status)
	}
	if resp.DownloadURL != "" {
		t.Errorf("download_url should be empty for pending jobs, got %q", resp.DownloadURL)
	}
}

func TestHandleExportStatus_Processing(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "blocked_nodes",
				Format:      "json",
				Status:      datastore.ExportStatusProcessing,
				RequestedAt: now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-002", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Status != "processing" {
		t.Errorf("status = %q, want processing", resp.Status)
	}
	if resp.DownloadURL != "" {
		t.Errorf("download_url should be empty for processing jobs, got %q", resp.DownloadURL)
	}
}

func TestHandleExportStatus_Completed(t *testing.T) {
	now := time.Now().UTC()
	completedAt := now.Add(10 * time.Second)
	expiresAt := now.Add(24 * time.Hour)

	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:            id,
				ExportType:    "ready_nodes",
				Format:        "csv",
				Status:        datastore.ExportStatusCompleted,
				RowCount:      500,
				FileSizeBytes: 12345,
				RequestedAt:   now,
				CompletedAt:   completedAt,
				ExpiresAt:     expiresAt,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-003", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Status != "completed" {
		t.Errorf("status = %q, want completed", resp.Status)
	}
	if resp.RowCount != 500 {
		t.Errorf("row_count = %d, want 500", resp.RowCount)
	}
	if resp.FileSizeBytes != 12345 {
		t.Errorf("file_size_bytes = %d, want 12345", resp.FileSizeBytes)
	}
	expectedURL := "/api/v1/exports/job-003/download"
	if resp.DownloadURL != expectedURL {
		t.Errorf("download_url = %q, want %q", resp.DownloadURL, expectedURL)
	}
	if resp.CompletedAt == "" {
		t.Error("completed_at should be set for completed jobs")
	}
	if resp.ExpiresAt == "" {
		t.Error("expires_at should be set when the job has an expiry")
	}
}

func TestHandleExportStatus_Failed(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:           id,
				ExportType:   "ready_nodes",
				Format:       "csv",
				Status:       datastore.ExportStatusFailed,
				ErrorMessage: "database timeout during export generation",
				RequestedAt:  now,
				CompletedAt:  now.Add(5 * time.Second),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-004", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Status != "failed" {
		t.Errorf("status = %q, want failed", resp.Status)
	}
	if resp.ErrorMessage == "" {
		t.Error("error_message should be set for failed jobs")
	}
	if resp.DownloadURL != "" {
		t.Errorf("download_url should be empty for failed jobs, got %q", resp.DownloadURL)
	}
}

func TestHandleExportStatus_DBError(t *testing.T) {
	// When GetExportJob returns a non-ErrNotFound error with a nil job,
	// the handler's defensive nil-job check returns 404 before the error
	// branch is reached. This is by design — a nil job is always "not found"
	// from the caller's perspective.
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-err", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (nil job treated as not-found)", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/exports/:id/download — file download
// ---------------------------------------------------------------------------

func TestHandleExportDownload_NotFound(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return nil, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/nonexistent/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleExportDownload_Pending_Conflict(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusPending,
				RequestedAt: time.Now().UTC(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-pend/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (409 Conflict for pending job)", w.Code, http.StatusConflict)
	}
}

func TestHandleExportDownload_Processing_Conflict(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusProcessing,
				RequestedAt: time.Now().UTC(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-proc/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (409 Conflict for processing job)", w.Code, http.StatusConflict)
	}
}

func TestHandleExportDownload_Failed_Conflict(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:           id,
				ExportType:   "ready_nodes",
				Format:       "csv",
				Status:       datastore.ExportStatusFailed,
				ErrorMessage: "generation failed",
				RequestedAt:  time.Now().UTC(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-fail/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (409 Conflict for failed job)", w.Code, http.StatusConflict)
	}
	assertBodyContains(t, w, "failed")
}

func TestHandleExportDownload_Expired_Gone(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusExpired,
				RequestedAt: time.Now().UTC(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-exp/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d (410 Gone for expired job)", w.Code, http.StatusGone)
	}
}

func TestHandleExportDownload_ExpiredByTime_Gone(t *testing.T) {
	// Status is still "completed" but expires_at is in the past.
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusCompleted,
				FilePath:    "/tmp/nonexistent-export.csv",
				RequestedAt: time.Now().UTC().Add(-48 * time.Hour),
				ExpiresAt:   time.Now().UTC().Add(-1 * time.Hour), // expired 1 hour ago
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-time-exp/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d (410 Gone for time-expired job)", w.Code, http.StatusGone)
	}
}

func TestHandleExportDownload_EmptyFilePath(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusCompleted,
				FilePath:    "", // no file path set
				RequestedAt: time.Now().UTC(),
				ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-nopath/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleExportDownload_MissingFile(t *testing.T) {
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:          id,
				ExportType:  "ready_nodes",
				Format:      "csv",
				Status:      datastore.ExportStatusCompleted,
				FilePath:    "/tmp/definitely-does-not-exist-export-test.csv",
				RequestedAt: time.Now().UTC(),
				ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-missingfile/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (file removed from disk)", w.Code, http.StatusNotFound)
	}
}

func TestHandleExportDownload_Success_CSV(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "export.csv")
	content := "node_name,organisation,environment\nweb1,prod,production\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:            id,
				ExportType:    "ready_nodes",
				Format:        "csv",
				Status:        datastore.ExportStatusCompleted,
				FilePath:      filePath,
				FileSizeBytes: int64(len(content)),
				RowCount:      1,
				RequestedAt:   time.Now().UTC(),
				ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-dl-csv/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv prefix", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}

	if w.Body.String() != content {
		t.Errorf("body = %q, want %q", w.Body.String(), content)
	}
}

func TestHandleExportDownload_Success_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "export.json")
	content := `[{"node_name":"web1"}]`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:            id,
				ExportType:    "ready_nodes",
				Format:        "json",
				Status:        datastore.ExportStatusCompleted,
				FilePath:      filePath,
				FileSizeBytes: int64(len(content)),
				RowCount:      1,
				RequestedAt:   time.Now().UTC(),
				ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-dl-json/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
}

func TestHandleExportDownload_Success_ChefSearchQuery(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "export.txt")
	content := "name:web1 OR name:web2"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return &datastore.ExportJob{
				ID:            id,
				ExportType:    "ready_nodes",
				Format:        "chef_search_query",
				Status:        datastore.ExportStatusCompleted,
				FilePath:      filePath,
				FileSizeBytes: int64(len(content)),
				RowCount:      2,
				RequestedAt:   time.Now().UTC(),
				ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-dl-search/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
}

func TestHandleExportDownload_DBError(t *testing.T) {
	// Same as the status endpoint: nil job + non-ErrNotFound error triggers
	// the defensive nil-check path, returning 404.
	store := &mockStore{
		GetExportJobFn: func(ctx context.Context, id string) (*datastore.ExportJob, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/job-dberr/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (nil job treated as not-found)", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Helper: contentTypeForFormat
// ---------------------------------------------------------------------------

func TestContentTypeForFormat(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{"csv", "text/csv; charset=utf-8"},
		{"json", "application/json; charset=utf-8"},
		{"chef_search_query", "text/plain; charset=utf-8"},
		{"unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		got := contentTypeForFormat(tt.format)
		if got != tt.want {
			t.Errorf("contentTypeForFormat(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper: downloadFilename
// ---------------------------------------------------------------------------

func TestDownloadFilename(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		exportType string
		format     string
		want       string
	}{
		{"ready_nodes", "csv", "ready_nodes_2025-06-15.csv"},
		{"ready_nodes", "json", "ready_nodes_2025-06-15.json"},
		{"ready_nodes", "chef_search_query", "ready_nodes_2025-06-15.txt"},
		{"blocked_nodes", "csv", "blocked_nodes_2025-06-15.csv"},
		{"cookbook_remediation", "json", "cookbook_remediation_2025-06-15.json"},
	}

	for _, tt := range tests {
		got := downloadFilename(tt.exportType, tt.format, ts)
		if got != tt.want {
			t.Errorf("downloadFilename(%q, %q) = %q, want %q", tt.exportType, tt.format, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test fixtures and helpers
// ---------------------------------------------------------------------------

// exportTestConfig returns a config suitable for export handler tests.
// The async threshold is set high so exports run synchronously by default.
func exportTestConfig() *config.Config {
	wsEnabled := true
	cfg := &config.Config{}
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersion = "18.0.0"
	cfg.Exports.MaxRows = 100000
	cfg.Exports.AsyncThreshold = 100000 // high threshold → sync by default
	cfg.Exports.RetentionHours = 24
	return cfg
}

// assertBodyContains is a test helper that checks the response body contains
// the given substring.
func assertBodyContains(t *testing.T, w *httptest.ResponseRecorder, substr string) {
	t.Helper()
	if !strings.Contains(w.Body.String(), substr) {
		t.Errorf("response body %q does not contain %q", w.Body.String(), substr)
	}
}
