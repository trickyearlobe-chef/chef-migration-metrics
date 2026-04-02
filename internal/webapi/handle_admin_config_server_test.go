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

const validServerBody = `{"tls":{"mode":"off"}}`

const validServerWithWSBody = `{"tls":{"mode":"off"},"websocket":{"max_connections":50,"send_buffer_size":32,"write_timeout_seconds":5,"ping_interval_seconds":20,"pong_timeout_seconds":45},"graceful_shutdown_seconds":30}`

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/server
// ---------------------------------------------------------------------------

func TestAdminConfigServer_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Server.GracefulShutdownSeconds = 45
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["graceful_shutdown_seconds"] != float64(45) {
		t.Errorf("graceful_shutdown_seconds = %v, want 45", got["graceful_shutdown_seconds"])
	}
}

func TestAdminConfigServer_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(testConfig(), nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigServer_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.Server.GracefulShutdownSeconds = 60
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["graceful_shutdown_seconds"] != float64(60) {
		t.Errorf("graceful_shutdown_seconds = %v, want 60", got["graceful_shutdown_seconds"])
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/server
// ---------------------------------------------------------------------------

func TestAdminConfigServer_PUT_Success_TLSOff(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(validServerBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	tls, ok := got["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls field missing or wrong type: %v", got["tls"])
	}
	if tls["mode"] != "off" {
		t.Errorf("tls.mode = %v, want %q", tls["mode"], "off")
	}

	// Verify all three sub-keys were persisted.
	for _, key := range []string{configstore.KeyServerTLS, configstore.KeyServerWebSocket, configstore.KeyServerGracefulShutdown} {
		stored, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("store.Get %s: %v", key, err)
		}
		if stored == nil {
			t.Errorf("key %s not stored", key)
		}
	}
}

func TestAdminConfigServer_PUT_Success_TLSStatic(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"static","cert_path":"/etc/certs/server.crt","key_path":"/etc/certs/server.key","min_version":"1.2"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigServer_PUT_Success_TLSAcme(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","min_version":"1.3","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":true,"challenge":"tls-alpn-01"}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigServer_PUT_Success_WithWebSocket(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(validServerWithWSBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigServer_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(validServerBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigServer_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigServer_PUT_422_UnknownTLSMode(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"unknown"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_StaticMissingCertPath(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"static","key_path":"/etc/certs/server.key"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_StaticMissingKeyPath(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"static","cert_path":"/etc/certs/server.crt"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_StaticBadMinVersion(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"static","cert_path":"/etc/certs/server.crt","key_path":"/etc/certs/server.key","min_version":"1.1"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_AcmeMissingDomains(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","acme":{"email":"admin@example.com","agree_to_tos":true}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_AcmeMissingEmail(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","acme":{"domains":["app.example.com"],"agree_to_tos":true}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_AcmeAgreeToTOSFalse(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":false}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_AcmeBadChallenge(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":true,"challenge":"invalid"}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_NegativeWebSocketField(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"off"},"websocket":{"max_connections":-1}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_PongNotGreaterThanPing(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"off"},"websocket":{"ping_interval_seconds":30,"pong_timeout_seconds":30}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}
