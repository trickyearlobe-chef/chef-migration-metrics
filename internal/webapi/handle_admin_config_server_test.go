// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// writeTestKeyPair generates a self-signed ECDSA certificate and key in a temp
// directory and returns their paths. Used to exercise static-TLS preflight
// validation, which loads the pair the same way the listener does at startup.
func writeTestKeyPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling key: %v", err)
	}

	certPath = filepath.Join(dir, "server.crt")
	keyPath = filepath.Join(dir, "server.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("writing cert: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("writing key: %v", err)
	}
	return certPath, keyPath
}

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
	restartRequired := decodePutValue(t, w, &got)
	tls, ok := got["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls field missing or wrong type: %v", got["tls"])
	}
	if tls["mode"] != "off" {
		t.Errorf("tls.mode = %v, want %q", tls["mode"], "off")
	}
	if !restartRequired {
		t.Error("server PUT should set restart_required = true")
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

	certPath, keyPath := writeTestKeyPair(t)
	body := fmt.Sprintf(`{"tls":{"mode":"static","cert_path":%q,"key_path":%q,"min_version":"1.2"}}`, certPath, keyPath)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigServer_PUT_422_StaticCertNotFound(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"static","cert_path":"/no/such/server.crt","key_path":"/no/such/server.key"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_RedirectPortEqualsListenPort(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Port = 8443
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(cfg, store, nil)

	certPath, keyPath := writeTestKeyPair(t)
	body := fmt.Sprintf(`{"tls":{"mode":"static","cert_path":%q,"key_path":%q,"http_redirect_port":8443}}`, certPath, keyPath)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigServer_PUT_422_RedirectPortEquals443(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Port = 8080
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(cfg, store, nil)

	certPath, keyPath := writeTestKeyPair(t)
	body := fmt.Sprintf(`{"tls":{"mode":"static","cert_path":%q,"key_path":%q,"http_redirect_port":443}}`, certPath, keyPath)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
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

// A save that changes only graceful_shutdown_seconds applies live (it is read at
// shutdown time), so the response reports applied / restart_required=false even
// though the bundled PUT also rewrites the listener/TLS/websocket keys with
// unchanged values (config live-reload listener-rebind H1).
func TestAdminConfigServer_PUT_GracefulOnlyChange_Applied(t *testing.T) {
	cfg := testConfig()
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 8080
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	cfg.Server.GracefulShutdownSeconds = 30

	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(cfg, store, nil)

	// Body identical to the live server config except graceful_shutdown_seconds,
	// so only that sub-key differs in the diff.
	liveJSON, err := configstore.SerializeValue(cfg.Server)
	if err != nil {
		t.Fatalf("serialise live server: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(liveJSON, &body); err != nil {
		t.Fatalf("unmarshal live server: %v", err)
	}
	body["graceful_shutdown_seconds"] = 45
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(bodyBytes))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)

	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if resp.RestartRequired {
		t.Error("graceful-only change must not require a restart")
	}
	if resp.Reload != "applied" {
		t.Errorf("reload = %q, want %q", resp.Reload, "applied")
	}

	stored, err := store.Get(context.Background(), configstore.KeyServerGracefulShutdown)
	if err != nil {
		t.Fatalf("store.Get graceful: %v", err)
	}
	if strings.TrimSpace(string(stored)) != "45" {
		t.Errorf("stored graceful = %s, want 45", stored)
	}
}

// A save that changes a non-graceful sub-key (here websocket limits) still
// reports the pessimistic process granularity until that key's in-place rebind
// lands (H2–H4), even when graceful is also touched in the same bundle.
func TestAdminConfigServer_PUT_WebSocketChange_Process(t *testing.T) {
	cfg := testConfig()
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 8080
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	cfg.Server.WebSocket.MaxConnections = 50

	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(cfg, store, nil)

	liveJSON, err := configstore.SerializeValue(cfg.Server)
	if err != nil {
		t.Fatalf("serialise live server: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(liveJSON, &body); err != nil {
		t.Fatalf("unmarshal live server: %v", err)
	}
	body["websocket"].(map[string]any)["max_connections"] = 99
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(bodyBytes))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)

	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if !resp.RestartRequired {
		t.Error("websocket change must still require a restart at H1")
	}
	if resp.Reload != "process" {
		t.Errorf("reload = %q, want %q", resp.Reload, "process")
	}
}

// recordingRebinder captures the addr/port a save asked to rebind and returns a
// configurable granularity/error, standing in for the server controller.
type recordingRebinder struct {
	gran   ReloadGranularity
	err    error
	calls  int
	gotAdr string
	gotPrt int
}

func (rb *recordingRebinder) fn(addr string, port int) (ReloadGranularity, error) {
	rb.calls++
	rb.gotAdr = addr
	rb.gotPrt = port
	return rb.gran, rb.err
}

// serverBodyWithPort serialises the live server config and overrides only the
// port, so the diff isolates a listen-section change.
func serverBodyWithPort(t *testing.T, cfg *config.Config, port int) []byte {
	t.Helper()
	liveJSON, err := configstore.SerializeValue(cfg.Server)
	if err != nil {
		t.Fatalf("serialise live server: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(liveJSON, &body); err != nil {
		t.Fatalf("unmarshal live server: %v", err)
	}
	body["port"] = port
	out, _ := json.Marshal(body)
	return out
}

func serverTestConfig(port int) *config.Config {
	cfg := testConfig()
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = port
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	return cfg
}

// A changed listen target with an in-place rebinder wired applies live: the
// rebinder is called with the new address/port and the save reports listener /
// restart_required=false.
func TestAdminConfigServer_PUT_ListenChange_RebindsLive(t *testing.T) {
	cfg := serverTestConfig(8080)
	store := newTestConfigStore(t)
	rb := &recordingRebinder{gran: ReloadListener}
	h := NewListenerRebindHolder()
	h.Set(rb.fn)
	r := newTestRouterForAdminConfig(cfg, store, nil, WithListenerRebinder(h))

	newPort := freeTestPort(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(serverBodyWithPort(t, cfg, newPort)))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if resp.RestartRequired {
		t.Error("live listener rebind must not require a restart")
	}
	if resp.Reload != "listener" {
		t.Errorf("reload = %q, want %q", resp.Reload, "listener")
	}
	if rb.calls != 1 {
		t.Fatalf("rebinder calls = %d, want 1", rb.calls)
	}
	if rb.gotAdr != "127.0.0.1" || rb.gotPrt != newPort {
		t.Errorf("rebinder called with %s:%d, want 127.0.0.1:%d", rb.gotAdr, rb.gotPrt, newPort)
	}
}

// With no rebinder wired, a changed listen target is persisted but reported
// restart-required (the no-rebinder fallback path).
func TestAdminConfigServer_PUT_ListenChange_NoRebinder_Process(t *testing.T) {
	cfg := serverTestConfig(8080)
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(cfg, store, nil)

	newPort := freeTestPort(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(serverBodyWithPort(t, cfg, newPort)))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if !resp.RestartRequired {
		t.Error("listen change with no rebinder must require a restart")
	}
	if resp.Reload != "process" {
		t.Errorf("reload = %q, want %q", resp.Reload, "process")
	}
}

// When the in-place rebind fails (the new port is held by another process), the
// save returns 500 — the old listener keeps serving.
func TestAdminConfigServer_PUT_ListenChange_RebindError_500(t *testing.T) {
	cfg := serverTestConfig(8080)
	store := newTestConfigStore(t)
	rb := &recordingRebinder{err: errors.New("listen tcp 127.0.0.1:9999: bind: address already in use")}
	h := NewListenerRebindHolder()
	h.Set(rb.fn)
	r := newTestRouterForAdminConfig(cfg, store, nil, WithListenerRebinder(h))

	newPort := freeTestPort(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(serverBodyWithPort(t, cfg, newPort)))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorCode(t, w, ErrCodeInternalError)
	if rb.calls != 1 {
		t.Errorf("rebinder calls = %d, want 1", rb.calls)
	}
}

// A save that does not touch the listen section must not invoke the rebinder,
// even with one wired (here only graceful_shutdown_seconds changes).
func TestAdminConfigServer_PUT_NonListenChange_NoRebind(t *testing.T) {
	cfg := serverTestConfig(8080)
	cfg.Server.GracefulShutdownSeconds = 30
	store := newTestConfigStore(t)
	rb := &recordingRebinder{gran: ReloadListener}
	h := NewListenerRebindHolder()
	h.Set(rb.fn)
	r := newTestRouterForAdminConfig(cfg, store, nil, WithListenerRebinder(h))

	liveJSON, _ := configstore.SerializeValue(cfg.Server)
	var body map[string]any
	_ = json.Unmarshal(liveJSON, &body)
	body["graceful_shutdown_seconds"] = 45
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", bytes.NewReader(bodyBytes))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if rb.calls != 0 {
		t.Errorf("rebinder called %d times on a non-listen change; want 0", rb.calls)
	}
	if resp.Reload != "applied" {
		t.Errorf("reload = %q, want %q", resp.Reload, "applied")
	}
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

// acmeCertPEM generates a self-signed certificate PEM with the given CN and
// expiry, for seeding the ACME issued-cert config-store entry.
func acmeCertPEM(t *testing.T, cn string, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// In ACME mode the GET response surfaces the issued certificate's metadata and
// the operator status object (tls-acme.md § 3.14).
func TestAdminConfigServer_GET_ACMEStatusAndCertInfo(t *testing.T) {
	store := newTestConfigStore(t)
	ctx := context.Background()
	certPEM := acmeCertPEM(t, "app.example.com", time.Now().Add(60*24*time.Hour))
	certJSON, _ := json.Marshal(string(certPEM))
	if err := store.Set(ctx, configstore.KeyServerTLSACMECert, certJSON, false, "test"); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	statusJSON := []byte(`{"last_renewal":"2026-06-01T00:00:00Z","hostname_error":"no IPv4 detectable"}`)
	if err := store.Set(ctx, configstore.KeyServerTLSACMEStatus, statusJSON, false, "test"); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	cfg := testConfig()
	cfg.Server.TLS = config.TLSConfig{Mode: "acme"}
	r := newTestRouterForAdminConfig(cfg, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	var got map[string]any
	decodeBody(t, w, &got)

	chain, ok := got["tls_certificate_info"].([]any)
	if !ok {
		t.Fatalf("tls_certificate_info missing for acme mode: %v", got["tls_certificate_info"])
	}
	if len(chain) == 0 {
		t.Fatalf("tls_certificate_info empty for acme mode")
	}
	info, ok := chain[0].(map[string]any)
	if !ok {
		t.Fatalf("chain[0] wrong type: %v", chain[0])
	}
	if info["subject"] == nil || !strings.Contains(fmt.Sprint(info["subject"]), "app.example.com") {
		t.Errorf("subject = %v, want it to contain app.example.com", info["subject"])
	}
	status, ok := got["acme_status"].(map[string]any)
	if !ok {
		t.Fatalf("acme_status missing: %v", got["acme_status"])
	}
	if status["last_renewal"] != "2026-06-01T00:00:00Z" {
		t.Errorf("acme_status.last_renewal = %v", status["last_renewal"])
	}
	if status["hostname_error"] != "no IPv4 detectable" {
		t.Errorf("acme_status.hostname_error = %v", status["hostname_error"])
	}
}

// Non-ACME modes must not carry an acme_status object.
func TestAdminConfigServer_GET_NoACMEStatusWhenOff(t *testing.T) {
	cfg := testConfig()
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	r := newTestRouterForAdminConfig(cfg, newTestConfigStore(t), nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	var got map[string]any
	decodeBody(t, w, &got)
	if _, present := got["acme_status"]; present {
		t.Error("acme_status should be absent when mode is not acme")
	}
}

// getStoredSecretString reads a secret config-store entry and decodes the
// JSON-encoded string value (the shape route53 creds are stored in).
func getStoredSecretString(t *testing.T, store *configstore.Store, key string) (string, error) {
	t.Helper()
	raw, err := store.GetSecret(context.Background(), key)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode secret %s: %v", key, err)
	}
	return s, nil
}

const acmeDNS01CredBody = `{"tls":{"mode":"acme","min_version":"1.3","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":true,"challenge":"dns-01","dns_provider":"route53","dns_provider_config":{"region":"us-east-1","hosted_zone_id":"Z123ABC"},"route53":{"access_key_id":"AKIAEXAMPLE","secret_access_key":"s3cr3t-key-value"}}}}`

// Route 53 DNS-01 credentials submitted under tls.acme.route53 are persisted as
// encrypted secrets (tls-acme.md § 3.4/§ 3.5), not into the server.tls config
// section.
func TestAdminConfigServer_PUT_PersistsRoute53Creds(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(acmeDNS01CredBody))
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	id, err := getStoredSecretString(t, store, configstore.KeyServerTLSACMERoute53AccessKeyID)
	if err != nil || id != "AKIAEXAMPLE" {
		t.Errorf("access_key_id = %q, err %v; want %q stored as secret", id, err, "AKIAEXAMPLE")
	}
	secret, err := getStoredSecretString(t, store, configstore.KeyServerTLSACMERoute53SecretAccessKey)
	if err != nil || secret != "s3cr3t-key-value" {
		t.Errorf("secret_access_key = %q, err %v; want %q stored as secret", secret, err, "s3cr3t-key-value")
	}
}

// The secret credential must never appear in the PUT response (write-only).
func TestAdminConfigServer_PUT_Route53CredsNotEchoed(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(acmeDNS01CredBody))
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	if strings.Contains(w.Body.String(), "s3cr3t-key-value") || strings.Contains(w.Body.String(), "AKIAEXAMPLE") {
		t.Errorf("PUT response echoed route53 credentials:\n%s", w.Body.String())
	}
}

// An ACME save that omits credentials must preserve previously stored creds
// (write-only: empty submission does not wipe them).
func TestAdminConfigServer_PUT_EmptyRoute53CredsPreserved(t *testing.T) {
	store := newTestConfigStore(t)
	ctx := context.Background()
	seed, _ := json.Marshal("existing-secret")
	if err := store.Set(ctx, configstore.KeyServerTLSACMERoute53SecretAccessKey, seed, true, "admin"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"acme","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":true,"challenge":"dns-01","dns_provider":"route53"}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	got, err := getStoredSecretString(t, store, configstore.KeyServerTLSACMERoute53SecretAccessKey)
	if err != nil || got != "existing-secret" {
		t.Errorf("secret_access_key = %q, err %v; want preserved %q", got, err, "existing-secret")
	}
}

// An ACME config save fires the re-register trigger so the renewer re-asserts
// hostname registration immediately (tls-acme.md § 3.14); a non-ACME save does
// not.
func TestAdminConfigServer_PUT_ACMEFiresReRegister(t *testing.T) {
	store := newTestConfigStore(t)
	hub := NewEventHub()
	go hub.Run()
	fired := 0
	r := NewRouter(&mockStore{}, testConfig(), hub,
		WithConfigStore(store, nil),
		WithACMETrigger(func() { fired++ }))

	acmeBody := `{"tls":{"mode":"acme","acme":{"domains":["app.example.com"],"email":"admin@example.com","agree_to_tos":true,"challenge":"dns-01","dns_provider":"route53"}}}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(acmeBody)))
	assertStatus(t, w, http.StatusOK)
	if fired != 1 {
		t.Errorf("ACME save fired re-register %d times, want 1", fired)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(validServerBody)))
	assertStatus(t, w, http.StatusOK)
	if fired != 1 {
		t.Errorf("non-ACME save should not fire re-register; fired total = %d", fired)
	}
}

// freeTestPort returns a currently-free loopback TCP port.
func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeTestPort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// GET returns the DB-managed listen_address and port so the UI can edit them.
func TestAdminConfigServer_GET_ReturnsListen(t *testing.T) {
	cfg := testConfig()
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 9443
	cfg.Server.TLS = config.TLSConfig{Mode: "off"}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["listen_address"] != "127.0.0.1" {
		t.Errorf("listen_address = %v, want 127.0.0.1", got["listen_address"])
	}
	if got["port"] != float64(9443) {
		t.Errorf("port = %v, want 9443", got["port"])
	}
}

// A valid, bindable, changed listen target is persisted as the server.listen
// section and reported restart-required.
func TestAdminConfigServer_PUT_PersistsListen(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	port := freeTestPort(t)
	body := fmt.Sprintf(`{"listen_address":"127.0.0.1","port":%d,"tls":{"mode":"off"}}`, port)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)

	stored, err := store.Get(context.Background(), configstore.KeyServerListen)
	if err != nil {
		t.Fatalf("store.Get %s: %v", configstore.KeyServerListen, err)
	}
	var section configstore.ServerListenSection
	if err := json.Unmarshal(stored, &section); err != nil {
		t.Fatalf("unmarshal server.listen: %v", err)
	}
	if section.ListenAddress != "127.0.0.1" || section.Port != port {
		t.Errorf("stored server.listen = %+v, want {127.0.0.1 %d}", section, port)
	}
}

// The behind-proxy plain-HTTP deployment (tls.md § 9.1) sets server.trusted_proxy
// alongside tls.mode off. The handler must persist the dedicated trusted_proxy
// section, not just the TLS/listen/websocket ones, or the UI value is lost on
// reload/restart.
func TestAdminConfigServer_PUT_PersistsTrustedProxy(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"tls":{"mode":"off"},"trusted_proxy":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)

	stored, err := store.Get(context.Background(), configstore.KeyServerTrustedProxy)
	if err != nil {
		t.Fatalf("store.Get %s: %v", configstore.KeyServerTrustedProxy, err)
	}
	var tp bool
	if err := json.Unmarshal(stored, &tp); err != nil {
		t.Fatalf("unmarshal trusted_proxy: %v", err)
	}
	if !tp {
		t.Errorf("trusted_proxy = %v, want true", tp)
	}
}

// A port outside the valid range is rejected.
func TestAdminConfigServer_PUT_422_BadPort(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"port":70000,"tls":{"mode":"off"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

// A changed port that cannot be bound (already in use) is rejected by the
// save-time test-bind preflight, so it can never brick the next restart.
func TestAdminConfigServer_PUT_422_UnbindablePort(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	body := fmt.Sprintf(`{"listen_address":"127.0.0.1","port":%d,"tls":{"mode":"off"}}`, badPort)
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
