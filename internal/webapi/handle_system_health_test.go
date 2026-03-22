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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — method checks
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /admin/system-health status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAdminSystemHealth_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /admin/system-health status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAdminSystemHealth_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /admin/system-health status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAdminSystemHealth_MethodNotAllowed_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — happy path
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_HappyPath(t *testing.T) {
	store := &mockStore{
		DatabaseSizeFn: func(ctx context.Context) (int64, error) {
			return 104857600, nil // 100 MB
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/system-health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify required top-level keys exist.
	requiredKeys := []string{
		"timestamp", "uptime", "disks",
		"cpu_count", "load_avg_1", "load_per_cpu",
		"mem_total_bytes", "mem_avail_bytes", "mem_used_percent",
		"go_heap_bytes", "go_goroutines",
		"database_size_bytes",
		"alerts", "collection_paused", "thresholds",
	}
	for _, key := range requiredKeys {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}

	// Verify database_size_bytes has the expected value.
	dbSize, ok := body["database_size_bytes"].(float64)
	if !ok {
		t.Fatalf("database_size_bytes is not a number: %T", body["database_size_bytes"])
	}
	if int64(dbSize) != 104857600 {
		t.Errorf("database_size_bytes = %v, want 104857600", dbSize)
	}

	// Verify disks is an array (never null).
	if _, ok := body["disks"].([]any); !ok {
		t.Errorf("disks should be an array, got %T", body["disks"])
	}

	// Verify alerts is an array (never null).
	if _, ok := body["alerts"].([]any); !ok {
		t.Errorf("alerts should be an array, got %T", body["alerts"])
	}

	// Verify collection_paused is a boolean.
	if _, ok := body["collection_paused"].(bool); !ok {
		t.Errorf("collection_paused should be a bool, got %T", body["collection_paused"])
	}

	// Verify thresholds is an object with expected keys.
	thresholds, ok := body["thresholds"].(map[string]any)
	if !ok {
		t.Fatalf("thresholds should be an object, got %T", body["thresholds"])
	}
	thresholdKeys := []string{
		"disk_used_warning_percent", "disk_used_critical_percent",
		"cpu_load_warning_per_cpu", "cpu_load_critical_per_cpu",
		"mem_used_warning_percent", "mem_used_critical_percent",
	}
	for _, key := range thresholdKeys {
		if _, ok := thresholds[key]; !ok {
			t.Errorf("missing threshold key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — database size error fallback
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_DatabaseSizeError_ReturnsZero(t *testing.T) {
	store := &mockStore{
		DatabaseSizeFn: func(ctx context.Context) (int64, error) {
			return 0, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/system-health status = %d, want %d (DB error should not fail endpoint)", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// database_size_bytes should gracefully fall back to 0 on error.
	dbSize, ok := body["database_size_bytes"].(float64)
	if !ok {
		t.Fatalf("database_size_bytes is not a number: %T", body["database_size_bytes"])
	}
	if int64(dbSize) != 0 {
		t.Errorf("database_size_bytes = %v, want 0 on DB error", dbSize)
	}
}

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — nil DB (no store configured)
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_NilDB_ReturnsZeroDatabaseSize(t *testing.T) {
	// testRouter() passes nil for the DataStore.
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/system-health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	dbSize, ok := body["database_size_bytes"].(float64)
	if !ok {
		t.Fatalf("database_size_bytes is not a number: %T", body["database_size_bytes"])
	}
	if int64(dbSize) != 0 {
		t.Errorf("database_size_bytes = %v, want 0 when DB is nil", dbSize)
	}
}

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — configured thresholds propagate
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_ConfiguredThresholds(t *testing.T) {
	store := &mockStore{
		DatabaseSizeFn: func(ctx context.Context) (int64, error) {
			return 0, nil
		},
	}
	cfg := testConfig()
	cfg.SystemHealth = config.SystemHealthConfig{
		DiskPaths:               []string{"/"},
		DiskUsedWarningPercent:  75,
		DiskUsedCriticalPercent: 85,
		CPULoadWarningPerCPU:    1.5,
		CPULoadCriticalPerCPU:   3.0,
		MemUsedWarningPercent:   70,
		MemUsedCriticalPercent:  88,
	}
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/system-health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	thresholds, ok := body["thresholds"].(map[string]any)
	if !ok {
		t.Fatalf("thresholds should be an object, got %T", body["thresholds"])
	}

	checks := map[string]float64{
		"disk_used_warning_percent":  75,
		"disk_used_critical_percent": 85,
		"cpu_load_warning_per_cpu":   1.5,
		"cpu_load_critical_per_cpu":  3.0,
		"mem_used_warning_percent":   70,
		"mem_used_critical_percent":  88,
	}
	for key, want := range checks {
		got, ok := thresholds[key].(float64)
		if !ok {
			t.Errorf("thresholds[%q] is not a number", key)
			continue
		}
		if got != want {
			t.Errorf("thresholds[%q] = %v, want %v", key, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// handleAdminSystemHealth — large database size
// ---------------------------------------------------------------------------

func TestHandleAdminSystemHealth_LargeDatabaseSize(t *testing.T) {
	// 50 GB — verify large int64 values survive JSON round-trip.
	const fiftyGB int64 = 50 * 1024 * 1024 * 1024

	store := &mockStore{
		DatabaseSizeFn: func(ctx context.Context) (int64, error) {
			return fiftyGB, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/system-health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	dbSize, ok := body["database_size_bytes"].(float64)
	if !ok {
		t.Fatalf("database_size_bytes is not a number: %T", body["database_size_bytes"])
	}
	if int64(dbSize) != fiftyGB {
		t.Errorf("database_size_bytes = %v, want %d", dbSize, fiftyGB)
	}
}
