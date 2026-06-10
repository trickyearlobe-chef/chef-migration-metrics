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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// postGenerateCSR issues a POST against the generate-csr endpoint with a JSON body.
func postGenerateCSR(r *Router, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/config/server/generate-csr", strings.NewReader(body))
	r.ServeHTTP(w, req)
	return w
}

// signCSR acts as a throwaway CA: it parses a PEM CSR and issues a leaf
// certificate over the CSR's public key, returning the cert PEM. The issued
// cert's public key therefore matches the private key the CSR was built from —
// exactly what an external CA returns to the operator.
func signCSR(t *testing.T, csrPEM string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		t.Fatalf("no PEM block in CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      csr.Subject,
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, csr.PublicKey, caKey)
	if err != nil {
		t.Fatalf("signing leaf: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

type csrResponse struct {
	CSRPEM       string `json:"csr_pem"`
	KeyAlgorithm string `json:"key_algorithm"`
}

// Generating a CSR returns the CSR PEM, stores the private key as a pending
// secret, and never returns or leaks the key.
func TestGenerateCSR_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"common_name":"leaf.example.com","dns_sans":["leaf.example.com"],"key_algorithm":"ecdsa-p256"}`
	w := postGenerateCSR(r, body)
	assertStatus(t, w, http.StatusOK)

	if strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatalf("response leaked private key material: %s", w.Body.String())
	}

	var resp csrResponse
	decodeBody(t, w, &resp)
	if !strings.Contains(resp.CSRPEM, "CERTIFICATE REQUEST") {
		t.Errorf("csr_pem missing CSR block: %q", resp.CSRPEM)
	}

	// The pending key is stored as a secret.
	raw, err := store.GetSecret(context.Background(), configstore.KeyServerTLSPrivateKeyPending)
	if err != nil {
		t.Fatalf("pending key not stored as secret: %v", err)
	}
	var keyPEM string
	if err := json.Unmarshal(raw, &keyPEM); err != nil {
		t.Fatalf("unmarshal pending key: %v", err)
	}
	if !strings.Contains(keyPEM, "PRIVATE KEY") {
		t.Errorf("stored pending key is not a PEM key")
	}
}

// No config store configured yields 503.
func TestGenerateCSR_NoStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)
	w := postGenerateCSR(r, `{"common_name":"leaf.example.com"}`)
	assertStatus(t, w, http.StatusServiceUnavailable)
}

// A request with no CN and no SANs is rejected 422.
func TestGenerateCSR_NoIdentifier(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)
	w := postGenerateCSR(r, `{"organization":"Example Corp"}`)
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

// An unsupported key algorithm is rejected 422.
func TestGenerateCSR_InvalidAlgo(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)
	w := postGenerateCSR(r, `{"common_name":"leaf.example.com","key_algorithm":"ed25519"}`)
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

// GET is not allowed on the generate-csr endpoint.
func TestGenerateCSR_MethodNotAllowed(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/server/generate-csr", nil)
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusMethodNotAllowed)
}

// Full round-trip: generate a CSR, sign it externally, paste the signed cert
// back (no key) — the pending key is promoted to active, the cert is stored,
// and the pending key is deleted.
func TestGenerateCSR_MatchAndPromote(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := postGenerateCSR(r, `{"common_name":"leaf.example.com","dns_sans":["leaf.example.com"]}`)
	assertStatus(t, w, http.StatusOK)
	var resp csrResponse
	decodeBody(t, w, &resp)

	signedCert := signCSR(t, resp.CSRPEM)

	// Paste the signed certificate back through the static db save path, no key.
	w = putServer(r, dbCertBody(t, signedCert, ""))
	assertStatus(t, w, http.StatusOK)

	ctx := context.Background()

	// Active cert stored.
	storedCert, err := store.Get(ctx, configstore.KeyServerTLSCertificate)
	if err != nil {
		t.Fatalf("active cert not stored: %v", err)
	}
	var gotCert string
	_ = json.Unmarshal(storedCert, &gotCert)
	if gotCert != signedCert {
		t.Errorf("stored active cert mismatch")
	}

	// Active key stored (the promoted pending key).
	if _, err := store.GetSecret(ctx, configstore.KeyServerTLSPrivateKey); err != nil {
		t.Fatalf("active key not stored after promotion: %v", err)
	}

	// Pending key deleted.
	if _, err := store.GetSecret(ctx, configstore.KeyServerTLSPrivateKeyPending); err == nil {
		t.Errorf("pending key should be deleted after promotion")
	}
}

// A signed certificate that does not match the pending key is rejected 422 and
// nothing is written; the pending key is left intact for a corrected upload.
func TestGenerateCSR_PromoteMismatch(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := postGenerateCSR(r, `{"common_name":"leaf.example.com","dns_sans":["leaf.example.com"]}`)
	assertStatus(t, w, http.StatusOK)

	// A cert from an unrelated key (self-signed), which cannot match the pending key.
	otherCert, _ := genCertKeyPEM(t, "leaf.example.com")
	w = putServer(r, dbCertBody(t, otherCert, ""))
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)

	ctx := context.Background()
	if _, err := store.Get(ctx, configstore.KeyServerTLSCertificate); err == nil {
		t.Error("active cert should not be written on mismatch")
	}
	// Pending key still intact.
	if _, err := store.GetSecret(ctx, configstore.KeyServerTLSPrivateKeyPending); err != nil {
		t.Errorf("pending key should survive a mismatched upload: %v", err)
	}
}

// Pasting a certificate with no key and no pending key stored is rejected 422.
func TestGenerateCSR_PromoteNoPending(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	certPEM, _ := genCertKeyPEM(t, "leaf.example.com")
	w := putServer(r, dbCertBody(t, certPEM, ""))
	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}
