// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/backup"
)

func newTestBackupService(t *testing.T) *backup.Service {
	t.Helper()
	dir := t.TempDir()
	conn := backup.ConnParams{Host: "localhost", Port: "5432", User: "test", DBName: "testdb"}
	exec := &testBackupExecutor{}
	svc, err := backup.NewService(dir, conn, exec, "1.0.0-test", 30, 3)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type testBackupExecutor struct{}

func (e *testBackupExecutor) PgDump(_ context.Context, _ backup.ConnParams, outputPath string) error {
	return os.WriteFile(outputPath, []byte("test-dump"), 0600)
}
func (e *testBackupExecutor) PgRestore(_ context.Context, _ backup.ConnParams, _ string) error {
	return nil
}
func (e *testBackupExecutor) PgDumpVersion(_ context.Context) (string, error) {
	return "pg_dump 17.2", nil
}
func (e *testBackupExecutor) PgServerVersion(_ context.Context, _ backup.ConnParams) (string, error) {
	return "17.2", nil
}

func newBackupRouter(t *testing.T) (*Router, *backup.Service) {
	t.Helper()
	svc := newTestBackupService(t)
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, cfg, hub, WithBackupService(svc))
	return r, svc
}

func TestHandleAdminBackups_NotConfigured(t *testing.T) {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, cfg, hub) // no backup service
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleAdminBackups_List_Empty(t *testing.T) {
	r, _ := newBackupRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	backups := resp["backups"].([]any)
	if len(backups) != 0 {
		t.Errorf("got %d backups, want 0", len(backups))
	}
}

func TestHandleAdminBackups_Create(t *testing.T) {
	r, svc := newBackupRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("response should contain backup id")
	}
	if resp["status"] != "running" {
		t.Errorf("status = %v, want running", resp["status"])
	}

	// Wait for background job
	for svc.IsActive() {
		// busy wait (test only)
	}
}

func TestHandleAdminBackups_Get(t *testing.T) {
	r, svc := newBackupRouter(t)
	ctx := context.Background()

	m, _ := svc.Create(ctx, "test")
	svc.RunCreate(ctx, &m)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+m.ID, nil)
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminBackups_Delete(t *testing.T) {
	r, svc := newBackupRouter(t)
	ctx := context.Background()

	m, _ := svc.Create(ctx, "test")
	svc.RunCreate(ctx, &m)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+m.ID, nil)
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminBackups_Restore_RequiresConfirmation(t *testing.T) {
	r, svc := newBackupRouter(t)
	ctx := context.Background()

	m, _ := svc.Create(ctx, "test")
	svc.RunCreate(ctx, &m)

	// No confirmation body
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/"+m.ID+"/restore", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminBackups_Restore_WithConfirmation(t *testing.T) {
	r, svc := newBackupRouter(t)
	ctx := context.Background()

	m, _ := svc.Create(ctx, "test")
	svc.RunCreate(ctx, &m)

	body := `{"confirm": "RESTORE"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/"+m.ID+"/restore", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.handleAdminBackups(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminBackupStatus_NoActiveJob(t *testing.T) {
	r, _ := newBackupRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/status", nil)
	w := httptest.NewRecorder()
	r.handleAdminBackupStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["active"] != false {
		t.Errorf("active = %v, want false", resp["active"])
	}
}

func TestMaintenanceMode_BlocksAPIRoutes(t *testing.T) {
	r, _ := newBackupRouter(t)

	// Enable maintenance mode
	r.maintenanceMode.Store(true)
	r.maintenanceMessage.Store("Restoring database")

	// API route should be blocked
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("API route: status = %d, want 503", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "maintenance" {
		t.Errorf("error = %v, want maintenance", resp["error"])
	}
}

func TestMaintenanceMode_AllowsHealthEndpoint(t *testing.T) {
	r, _ := newBackupRouter(t)

	r.maintenanceMode.Store(true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Health endpoint should still work (not 503)
	if w.Code == http.StatusServiceUnavailable {
		t.Error("health endpoint should not be blocked during maintenance")
	}
}

func TestMaintenanceMode_AllowsBackupStatus(t *testing.T) {
	r, _ := newBackupRouter(t)

	r.maintenanceMode.Store(true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusServiceUnavailable {
		t.Error("backup status endpoint should not be blocked during maintenance")
	}
}

func TestExecuteRestore_Success(t *testing.T) {
	r, svc := newBackupRouter(t)

	// Create a backup first
	ctx := context.Background()
	m, err := svc.Create(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RunCreate(ctx, &m)

	// Track whether exit was called
	var exitCode int
	exitCalled := make(chan struct{})
	r.exitFunc = func(code int) {
		exitCode = code
		close(exitCalled)
		select {} // block forever (simulates process exit)
	}

	hookCalled := false
	r.restoreHook = func() { hookCalled = true }

	go r.executeRestore(m.ID)

	<-exitCalled

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !hookCalled {
		t.Error("restore hook was not called")
	}
}

func TestExecuteRestore_Failure_ResumesNormalMode(t *testing.T) {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()

	dir := t.TempDir()
	conn := backup.ConnParams{Host: "localhost", Port: "5432", User: "test", DBName: "testdb"}
	failExec := &testBackupExecutor{}
	svc, err := backup.NewService(dir, conn, failExec, "1.0.0", 30, 3)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	r := NewRouter(&mockStore{}, cfg, hub, WithBackupService(svc))

	// Restore a non-existent backup — should fail
	done := make(chan struct{})
	r.exitFunc = func(code int) { t.Error("exit should not be called on failure") }
	go func() {
		r.executeRestore("nonexistent-id")
		close(done)
	}()

	<-done

	if r.maintenanceMode.Load() {
		t.Error("maintenance mode should be off after failed restore")
	}
}
