// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
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
