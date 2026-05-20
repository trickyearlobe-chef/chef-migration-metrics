// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

func TestSAMLGenerateKeypair_Success(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		CertificatePEM    string `json:"certificate_pem"`
		FingerprintSHA256 string `json:"fingerprint_sha256"`
		NotAfter          string `json:"not_after"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.CertificatePEM == "" {
		t.Error("certificate_pem should not be empty")
	}
	if resp.FingerprintSHA256 == "" {
		t.Error("fingerprint_sha256 should not be empty")
	}
	if resp.NotAfter == "" {
		t.Error("not_after should not be empty")
	}

	// Verify it's valid PEM/X.509.
	block, _ := pem.Decode([]byte(resp.CertificatePEM))
	if block == nil {
		t.Fatal("certificate_pem is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if cert.Subject.CommonName != "chef-migration-metrics-sp" {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, "chef-migration-metrics-sp")
	}

	// Verify credentials were stored.
	certCred, err := cs.Get(context.Background(), "saml-sp-cert")
	if err != nil {
		t.Fatalf("sp cert not stored: %v", err)
	}
	defer secrets.ZeroBytes(certCred.Plaintext)

	keyCred, err := cs.Get(context.Background(), "saml-sp-key")
	if err != nil {
		t.Fatalf("sp key not stored: %v", err)
	}
	defer secrets.ZeroBytes(keyCred.Plaintext)

	// Private key should be valid PEM.
	keyBlock, _ := pem.Decode(keyCred.Plaintext)
	if keyBlock == nil {
		t.Fatal("stored private key is not valid PEM")
	}
	if keyBlock.Type != "RSA PRIVATE KEY" {
		t.Errorf("key PEM type = %q, want %q", keyBlock.Type, "RSA PRIVATE KEY")
	}
}

func TestSAMLGenerateKeypair_MethodNotAllowed(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestSAMLGenerateKeypair_NoCredentialStore(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestSAMLGetCertificate_NotFound(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	resolver := secrets.NewCredentialResolver(cs)
	r := NewRouter(&mockStore{}, testConfig(), NewEventHub(),
		WithCredentialStore(cs), WithCredentialResolver(resolver))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/saml/sp-certificate", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSAMLGetCertificate_AfterGenerate(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	resolver := secrets.NewCredentialResolver(cs)
	r := NewRouter(&mockStore{}, testConfig(), NewEventHub(),
		WithCredentialStore(cs), WithCredentialResolver(resolver))

	// Generate first.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("generate: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Now fetch.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/saml/sp-certificate", nil)
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get cert: status = %d, want %d; body: %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	var resp struct {
		CertificatePEM    string `json:"certificate_pem"`
		FingerprintSHA256 string `json:"fingerprint_sha256"`
		NotAfter          string `json:"not_after"`
		Subject           string `json:"subject"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.CertificatePEM == "" {
		t.Error("certificate_pem should not be empty")
	}
	if resp.Subject != "chef-migration-metrics-sp" {
		t.Errorf("subject = %q, want %q", resp.Subject, "chef-migration-metrics-sp")
	}
}

func TestSAMLGetCertificate_NoResolver(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/saml/sp-certificate", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestSAMLGenerateKeypair_Regenerate(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	// First generation.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first generate: status = %d", w.Code)
	}

	var first struct {
		FingerprintSHA256 string `json:"fingerprint_sha256"`
	}
	json.NewDecoder(w.Body).Decode(&first)

	// Second generation (regenerate).
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/saml/generate-keypair", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second generate: status = %d; body: %s", w2.Code, w2.Body.String())
	}

	var second struct {
		FingerprintSHA256 string `json:"fingerprint_sha256"`
	}
	json.NewDecoder(w2.Body).Decode(&second)

	// Fingerprints should differ (different keys).
	if first.FingerprintSHA256 == second.FingerprintSHA256 {
		t.Error("regeneration should produce a different fingerprint")
	}
}
