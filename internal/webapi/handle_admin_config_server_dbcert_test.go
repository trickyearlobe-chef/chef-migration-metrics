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
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// genCertKeyPEM generates a self-signed ECDSA cert/key pair as in-memory PEM
// strings, for exercising the cert_source: db admin save path.
func genCertKeyPEM(t *testing.T, cn string, dnsNames ...string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling key: %v", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

// dbCertBody builds a PUT body for static + cert_source: db with the given
// cert/key PEM (omit either by passing "").
func dbCertBody(t *testing.T, certPEM, keyPEM string) string {
	t.Helper()
	tlsObj := map[string]any{"mode": "static", "cert_source": "db", "min_version": "1.2"}
	if certPEM != "" {
		tlsObj["certificate"] = certPEM
	}
	if keyPEM != "" {
		tlsObj["private_key"] = keyPEM
	}
	body, err := json.Marshal(map[string]any{"tls": tlsObj})
	if err != nil {
		t.Fatalf("marshalling body: %v", err)
	}
	return string(body)
}

func putServer(r *Router, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/server", strings.NewReader(body))
	r.ServeHTTP(w, req)
	return w
}

// A valid cert/key pair for cert_source: db is accepted, the certificate is
// stored non-secret, the private key is stored secret, and neither leaks into
// the server.tls config section.
func TestAdminConfigServer_PUT_DBCertSource_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, keyPEM := genCertKeyPEM(t, "db.example.com")
	w := putServer(r, dbCertBody(t, certPEM, keyPEM))
	assertStatus(t, w, http.StatusOK)

	ctx := context.Background()

	// Certificate stored non-secret and readable.
	storedCert, err := store.Get(ctx, configstore.KeyServerTLSCertificate)
	if err != nil {
		t.Fatalf("store.Get certificate: %v", err)
	}
	var gotCert string
	if err := json.Unmarshal(storedCert, &gotCert); err != nil {
		t.Fatalf("unmarshal stored cert: %v", err)
	}
	if gotCert != certPEM {
		t.Errorf("stored certificate mismatch")
	}

	// Private key stored WITH the secret flag (GetSecret succeeds).
	storedKey, err := store.GetSecret(ctx, configstore.KeyServerTLSPrivateKey)
	if err != nil {
		t.Fatalf("store.GetSecret private_key: %v", err)
	}
	var gotKey string
	if err := json.Unmarshal(storedKey, &gotKey); err != nil {
		t.Fatalf("unmarshal stored key: %v", err)
	}
	if gotKey != keyPEM {
		t.Errorf("stored private key mismatch")
	}

	// The server.tls section must NOT contain any PEM material.
	section, err := store.Get(ctx, configstore.KeyServerTLS)
	if err != nil {
		t.Fatalf("store.Get server.tls: %v", err)
	}
	if strings.Contains(string(section), "PRIVATE KEY") || strings.Contains(string(section), "BEGIN CERTIFICATE") {
		t.Errorf("server.tls section leaked PEM material: %s", section)
	}
}

// cert_source: db does not require cert_path/key_path (the file-source fields).
func TestAdminConfigServer_PUT_DBCertSource_NoPathRequired(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, keyPEM := genCertKeyPEM(t, "db.example.com")
	w := putServer(r, dbCertBody(t, certPEM, keyPEM))
	assertStatus(t, w, http.StatusOK)
}

// A mismatched cert/key pair is rejected before anything is persisted.
func TestAdminConfigServer_PUT_DBCertSource_MismatchedPair(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, _ := genCertKeyPEM(t, "db.example.com")
	_, otherKey := genCertKeyPEM(t, "other.example.com")
	w := putServer(r, dbCertBody(t, certPEM, otherKey))
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)

	if _, err := store.Get(context.Background(), configstore.KeyServerTLSCertificate); err == nil {
		t.Error("certificate should not be persisted on a bad pair")
	}
}

// Submitting only the certificate (no key) is rejected.
func TestAdminConfigServer_PUT_DBCertSource_PartialSubmission(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, _ := genCertKeyPEM(t, "db.example.com")
	w := putServer(r, dbCertBody(t, certPEM, ""))
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

// Activating cert_source: db with no pair submitted and none already stored is
// rejected (cannot serve TLS with no certificate).
func TestAdminConfigServer_PUT_DBCertSource_NoneStored(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := putServer(r, dbCertBody(t, "", ""))
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

// Re-saving cert_source: db without resubmitting the pair is allowed when a
// pair is already stored (operator is changing some other field).
func TestAdminConfigServer_PUT_DBCertSource_KeepsExistingPair(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, keyPEM := genCertKeyPEM(t, "db.example.com")
	assertStatus(t, putServer(r, dbCertBody(t, certPEM, keyPEM)), http.StatusOK)

	// Second save without the pair — should succeed because one is stored.
	assertStatus(t, putServer(r, dbCertBody(t, "", "")), http.StatusOK)
}

// GET returns certificate metadata (subject/SANs/expiry) for a db source and
// never returns the private key.
func TestAdminConfigServer_GET_DBCertSource_ReturnsMetadataNotKey(t *testing.T) {
	store := newTestConfigStore(t)
	cfg := testConfig()
	cfg.Server.TLS = config.TLSConfig{Mode: "static", CertSource: "db"}
	r := newTestRouterForAdminConfig(cfg, store, nil)

	certPEM, keyPEM := genCertKeyPEM(t, "leaf.example.com", "leaf.example.com")
	assertStatus(t, putServer(r, dbCertBody(t, certPEM, keyPEM)), http.StatusOK)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server", nil)
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	if strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatalf("GET response leaked private key material: %s", w.Body.String())
	}

	var got map[string]any
	decodeBody(t, w, &got)
	info, ok := got["tls_certificate_info"].(map[string]any)
	if !ok {
		t.Fatalf("tls_certificate_info missing or wrong type: %v", got["tls_certificate_info"])
	}
	if info["subject"] != "leaf.example.com" {
		t.Errorf("subject = %v, want leaf.example.com", info["subject"])
	}
}

// fakeReloader records the last PEM it was asked to reload.
type fakeReloader struct {
	certPEM, keyPEM []byte
	err             error
}

func (f *fakeReloader) ReloadFromPEM(certPEM, keyPEM []byte) error {
	f.certPEM = certPEM
	f.keyPEM = keyPEM
	return f.err
}

// Saving a db pair triggers an in-place reload of the running listener.
func TestAdminConfigServer_PUT_DBCertSource_TriggersReload(t *testing.T) {
	store := newTestConfigStore(t)
	holder := NewTLSReloadHolder()
	fr := &fakeReloader{}
	holder.Set(fr)

	ms := &mockStore{}
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(ms, testConfig(), hub, WithConfigStore(store, nil), WithTLSReload(holder))

	certPEM, keyPEM := genCertKeyPEM(t, "db.example.com")
	assertStatus(t, putServer(r, dbCertBody(t, certPEM, keyPEM)), http.StatusOK)

	if string(fr.certPEM) != certPEM || string(fr.keyPEM) != keyPEM {
		t.Errorf("in-place reload not invoked with the saved pair")
	}
}
