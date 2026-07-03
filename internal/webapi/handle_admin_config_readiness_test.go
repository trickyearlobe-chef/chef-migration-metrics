// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

const validReadinessBody = `{
	"install_path_linux": "/hab",
	"install_path_windows": "C:\\hab",
	"install_size_mb_linux": 3072,
	"install_size_mb_windows": 6144,
	"min_remaining_free_percent": 20,
	"review_blocks_readiness": true
}`

func TestAdminConfigReadiness_GET_IncludesToggle(t *testing.T) {
	cfg := testConfig()
	cfg.Readiness = config.ReadinessConfig{
		InstallPathLinux:        "/hab",
		InstallPathWindows:      `C:\hab`,
		InstallSizeMBLinux:      3072,
		InstallSizeMBWindows:    6144,
		MinRemainingFreePercent: 20,
		ReviewBlocksReadiness:   true,
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/readiness", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	// The readiness GET writes the section object directly (no value envelope).
	var got map[string]any
	decodeBody(t, w, &got)
	if got["review_blocks_readiness"] != true {
		t.Errorf("review_blocks_readiness = %v, want true", got["review_blocks_readiness"])
	}
}

func TestAdminConfigReadiness_PUT_PersistsToggle(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/readiness", strings.NewReader(validReadinessBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodePutValue(t, w, &got)
	if got["review_blocks_readiness"] != true {
		t.Errorf("review_blocks_readiness = %v, want true", got["review_blocks_readiness"])
	}

	stored, err := store.Get(context.Background(), configstore.KeyReadiness)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !strings.Contains(string(stored), `"review_blocks_readiness":true`) {
		t.Errorf("stored readiness config missing review_blocks_readiness:true; got %s", stored)
	}
}

func TestAdminConfigReadiness_PUT_TriggersReconciler(t *testing.T) {
	store := newTestConfigStore(t)
	called := false
	r := newTestRouterForAdminConfig(nil, store, nil, WithReadinessReconciler(func() error {
		called = true
		return nil
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/readiness", strings.NewReader(validReadinessBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	// The reconciler runs as a subsystem applier after persist+reload, so a
	// readiness change applies without a restart.
	if restart := decodePutValue(t, w, nil); restart {
		t.Error("restart_required = true, want false (subsystem applier)")
	}
	if !called {
		t.Error("expected readiness reconciler to be invoked on a readiness PUT")
	}
}
