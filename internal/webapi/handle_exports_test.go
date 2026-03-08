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
// POST /api/v1/exports — request body validation
// ---------------------------------------------------------------------------

func TestHandleExports_InvalidJSON(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{bad json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "Invalid JSON")
}

func TestHandleExports_InvalidExportType(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"invalid_type","format":"csv"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "Invalid export_type")
}

func TestHandleExports_InvalidFormat(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"xml"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "Invalid format")
}

func TestHandleExports_ChefSearchQueryOnBlockedNodes(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"blocked_nodes","format":"chef_search_query","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "chef_search_query format is only supported for ready_nodes")
}

func TestHandleExports_ChefSearchQueryOnCookbookRemediation(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"cookbook_remediation","format":"chef_search_query"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "chef_search_query format is only supported for ready_nodes")
}

func TestHandleExports_MissingTargetVersionForReadyNodes_NoConfig(t *testing.T) {
	store := &mockStore{}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	// No TargetChefVersions configured.

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "target_chef_version is required")
}

func TestHandleExports_MissingTargetVersionForBlockedNodes_NoConfig(t *testing.T) {
	store := &mockStore{}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"blocked_nodes","format":"json"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	assertBodyContains(t, w, "target_chef_version is required")
}

func TestHandleExports_MissingTargetVersionDefaultsFromConfig(t *testing.T) {
	// When target_chef_version is omitted but the config has TargetChefVersions,
	// the handler defaults to the first one and proceeds successfully.
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil // no orgs → zero rows estimate → sync path → empty export
		},
	}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	cfg.TargetChefVersions = []string{"18.0.0", "17.0.0"}

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	// Should not be 400 — the handler defaulted the target version from config.
	if w.Code == http.StatusBadRequest {
		t.Errorf("expected non-400 when TargetChefVersions is configured, got %d: %s",
			w.Code, w.Body.String())
	}
}

func TestHandleExports_CookbookRemediationNoTargetVersionOK(t *testing.T) {
	// cookbook_remediation does not require a target_chef_version.
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil
		},
	}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"cookbook_remediation","format":"csv"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	// Should succeed (200 with empty data) or at least not be a 400.
	if w.Code == http.StatusBadRequest {
		t.Errorf("cookbook_remediation should not require target_chef_version, got 400: %s",
			w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/exports — synchronous export (small result set)
// ---------------------------------------------------------------------------

func TestHandleExports_Sync_ReadyNodesCSV(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
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
	if !strings.Contains(cd, "ready_nodes") {
		t.Errorf("Content-Disposition = %q, want filename containing ready_nodes", cd)
	}

	// Verify we got CSV content with a header row and at least one data row.
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 CSV lines (header + data), got %d", len(lines))
	}
}

func TestHandleExports_Sync_ReadyNodesJSON(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"json","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}

	// Verify the body is valid JSON.
	if !json.Valid(w.Body.Bytes()) {
		t.Errorf("response body is not valid JSON: %s", w.Body.String())
	}
}

func TestHandleExports_Sync_ReadyNodesChefSearchQuery(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"chef_search_query","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
}

func TestHandleExports_Sync_BlockedNodesCSV(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"blocked_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv prefix", ct)
	}
}

func TestHandleExports_Sync_BlockedNodesJSON(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"blocked_nodes","format":"json","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
}

func TestHandleExports_Sync_CookbookRemediationCSV(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"cookbook_remediation","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv prefix", ct)
	}
}

func TestHandleExports_Sync_CookbookRemediationJSON(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"cookbook_remediation","format":"json","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
}

func TestHandleExports_Sync_EmptyOrgs(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return nil, nil // no organisations
		},
	}
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Should still have a header line at minimum.
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) < 1 {
		t.Error("expected at least 1 CSV line (header), got 0")
	}
}

func TestHandleExports_Sync_RowCountHeader(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	rc := w.Header().Get("X-Export-Row-Count")
	if rc == "" {
		t.Error("expected X-Export-Row-Count header to be set")
	}
}

func TestHandleExports_Sync_WithFilters(t *testing.T) {
	store := newExportMockStore()
	cfg := exportTestConfig()

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{
		"export_type":"ready_nodes",
		"format":"csv",
		"target_chef_version":"18.0.0",
		"filters":{"environment":"prod","platform":"ubuntu"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	// Should succeed — filters are applied during generation.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/exports — asynchronous export (large result set)
// ---------------------------------------------------------------------------

func TestHandleExports_Async_LargeEstimate(t *testing.T) {
	// Set up a store that reports a large number of nodes to trigger async mode.
	insertCalled := false
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "production"},
			}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			// Return a count well above the async threshold.
			return 50000, 30000, 20000, nil
		},
		InsertExportJobFn: func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error) {
			insertCalled = true
			if p.ExportType != "ready_nodes" {
				t.Errorf("InsertExportJob export_type = %q, want ready_nodes", p.ExportType)
			}
			if p.Format != "csv" {
				t.Errorf("InsertExportJob format = %q, want csv", p.Format)
			}
			return &datastore.ExportJob{
				ID:          "job-async-001",
				ExportType:  p.ExportType,
				Format:      p.Format,
				Status:      datastore.ExportStatusPending,
				RequestedAt: time.Now().UTC(),
			}, nil
		},
		// The async goroutine will call these but we don't need to verify
		// them in this test — just prevent panics.
		UpdateExportJobStatusFn: func(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error {
			return nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error) {
			return nil, nil
		},
		ListNodeReadinessForSnapshotFn: func(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error) {
			return nil, nil
		},
	}

	cfg := exportTestConfig()
	cfg.Exports.AsyncThreshold = 100 // low threshold to trigger async

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	if !insertCalled {
		t.Error("InsertExportJob was not called for async export")
	}

	// Verify the response contains a job ID.
	var resp exportJobResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-async-001" {
		t.Errorf("job_id = %q, want %q", resp.JobID, "job-async-001")
	}
	if resp.Status != datastore.ExportStatusPending {
		t.Errorf("status = %q, want %q", resp.Status, datastore.ExportStatusPending)
	}
	if resp.Message == "" {
		t.Error("expected a message with poll instructions")
	}
}

func TestHandleExports_Async_InsertJobError(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{ID: "org-1", Name: "production"},
			}, nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			return 50000, 30000, 20000, nil
		},
		InsertExportJobFn: func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error) {
			return nil, fmt.Errorf("database connection lost")
		},
	}

	cfg := exportTestConfig()
	cfg.Exports.AsyncThreshold = 100

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()

	body := strings.NewReader(`{"export_type":"ready_nodes","format":"csv","target_chef_version":"18.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exports", body)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
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
	cfg.TargetChefVersions = []string{"18.0.0", "17.0.0"}
	cfg.Exports.MaxRows = 100000
	cfg.Exports.AsyncThreshold = 100000 // high threshold → sync by default
	cfg.Exports.RetentionHours = 24
	return cfg
}

// newExportMockStore creates a mockStore pre-populated with data suitable for
// sync export tests. It provides one organisation with two nodes (one ready,
// one blocked) and two cookbooks with complexity records.
func newExportMockStore() *mockStore {
	org := datastore.Organisation{ID: "org-1", Name: "production"}

	nodes := []datastore.NodeSnapshot{
		{
			ID:              "snap-1",
			OrganisationID:  "org-1",
			NodeName:        "web1",
			ChefEnvironment: "prod",
			ChefVersion:     "17.10.0",
			Platform:        "ubuntu",
			PlatformVersion: "22.04",
			PolicyName:      "webserver",
			PolicyGroup:     "prod",
			IsStale:         false,
			Cookbooks:       json.RawMessage(`{"apt":{"version":"7.4.0"}}`),
			Roles:           json.RawMessage(`["base","webserver"]`),
			CollectedAt:     time.Now().UTC(),
		},
		{
			ID:              "snap-2",
			OrganisationID:  "org-1",
			NodeName:        "db1",
			ChefEnvironment: "prod",
			ChefVersion:     "16.0.0",
			Platform:        "centos",
			PlatformVersion: "7.9",
			PolicyName:      "",
			PolicyGroup:     "",
			IsStale:         false,
			Cookbooks:       json.RawMessage(`{"mysql":{"version":"8.0.0"}}`),
			Roles:           json.RawMessage(`["base","database"]`),
			CollectedAt:     time.Now().UTC(),
		},
	}

	readiness := map[string][]datastore.NodeReadiness{
		"snap-1": {
			{
				ID:                "nr-1",
				NodeSnapshotID:    "snap-1",
				TargetChefVersion: "18.0.0",
				IsReady:           true,
			},
		},
		"snap-2": {
			{
				ID:                "nr-2",
				NodeSnapshotID:    "snap-2",
				TargetChefVersion: "18.0.0",
				IsReady:           false,
				BlockingCookbooks: json.RawMessage(`["mysql"]`),
			},
		},
	}

	cookbooks := []datastore.Cookbook{
		{
			ID:             "cb-1",
			OrganisationID: "org-1",
			Name:           "apt",
			Version:        "7.4.0",
		},
		{
			ID:             "cb-2",
			OrganisationID: "org-1",
			Name:           "mysql",
			Version:        "8.0.0",
		},
	}

	complexities := []datastore.CookbookComplexity{
		{
			ID:                "cc-1",
			CookbookID:        "cb-1",
			TargetChefVersion: "18.0.0",
			ComplexityScore:   5,
			ComplexityLabel:   "trivial",
		},
		{
			ID:                "cc-2",
			CookbookID:        "cb-2",
			TargetChefVersion: "18.0.0",
			ComplexityScore:   42,
			ComplexityLabel:   "moderate",
		},
	}

	return &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{org}, nil
		},
		ListNodeSnapshotsByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error) {
			if organisationID == "org-1" {
				return nodes, nil
			}
			return nil, nil
		},
		ListNodeReadinessForSnapshotFn: func(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error) {
			return readiness[nodeSnapshotID], nil
		},
		CountNodeReadinessFn: func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
			// 2 total, 1 ready, 1 blocked
			return 2, 1, 1, nil
		},
		ListCookbooksByOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.Cookbook, error) {
			if organisationID == "org-1" {
				return cookbooks, nil
			}
			return nil, nil
		},
		ListCookbookComplexitiesForOrganisationFn: func(ctx context.Context, organisationID string) ([]datastore.CookbookComplexity, error) {
			if organisationID == "org-1" {
				return complexities, nil
			}
			return nil, nil
		},
	}
}

// assertBodyContains is a test helper that checks the response body contains
// the given substring.
func assertBodyContains(t *testing.T, w *httptest.ResponseRecorder, substr string) {
	t.Helper()
	if !strings.Contains(w.Body.String(), substr) {
		t.Errorf("response body %q does not contain %q", w.Body.String(), substr)
	}
}
