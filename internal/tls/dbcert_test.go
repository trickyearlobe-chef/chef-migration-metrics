// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ValidateStaticPairBytes — preflight for cert_source: db (in-memory PEM)
// ---------------------------------------------------------------------------

func TestValidateStaticPairBytes_ValidPair(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes(certPEM, keyPEM, ""); err != nil {
		t.Fatalf("ValidateStaticPairBytes(valid) = %v, want nil", err)
	}
}

func TestValidateStaticPairBytes_MissingCert(t *testing.T) {
	_, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes(nil, keyPEM, ""); err == nil {
		t.Fatal("ValidateStaticPairBytes(no cert) = nil, want error")
	}
}

func TestValidateStaticPairBytes_MissingKey(t *testing.T) {
	certPEM, _, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes(certPEM, nil, ""); err == nil {
		t.Fatal("ValidateStaticPairBytes(no key) = nil, want error")
	}
}

func TestValidateStaticPairBytes_MismatchedPair(t *testing.T) {
	certPEM, _, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	_, otherKey, _ := generateTestCertPEM(t, "other.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes(certPEM, otherKey, ""); err == nil {
		t.Fatal("ValidateStaticPairBytes(mismatched) = nil, want error")
	}
}

func TestValidateStaticPairBytes_UnparseableCert(t *testing.T) {
	_, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes([]byte("not a pem"), keyPEM, ""); err == nil {
		t.Fatal("ValidateStaticPairBytes(garbage cert) = nil, want error")
	}
}

func TestValidateStaticPairBytes_WithValidCAFile(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	// A CA bundle file may still be referenced for mTLS with cert_source: db.
	caPEM, _, _ := generateTestCertPEM(t, "ca.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("writing ca: %v", err)
	}

	if err := ValidateStaticPairBytes(certPEM, keyPEM, caPath); err != nil {
		t.Fatalf("ValidateStaticPairBytes(valid + ca) = %v, want nil", err)
	}
}

func TestValidateStaticPairBytes_BadCAFile(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := ValidateStaticPairBytes(certPEM, keyPEM, "/no/such/ca.pem"); err == nil {
		t.Fatal("ValidateStaticPairBytes(bad ca) = nil, want error")
	}
}

// ---------------------------------------------------------------------------
// CertMetadataFromPEM — subject / SANs / expiry for the admin API
// ---------------------------------------------------------------------------

func TestCertMetadataFromPEM_ExtractsFields(t *testing.T) {
	notBefore := time.Now().Add(-time.Hour).Truncate(time.Second)
	notAfter := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	certPEM, _, _ := generateTestCertPEM(t, "leaf.example.com", notBefore, notAfter,
		"leaf.example.com", "alt.example.com")

	meta, err := CertMetadataFromPEM(certPEM)
	if err != nil {
		t.Fatalf("CertMetadataFromPEM = %v, want nil", err)
	}
	if meta.Subject != "leaf.example.com" {
		t.Errorf("Subject = %q, want leaf.example.com", meta.Subject)
	}
	if len(meta.DNSNames) != 2 || meta.DNSNames[0] != "leaf.example.com" {
		t.Errorf("DNSNames = %v, want [leaf.example.com alt.example.com]", meta.DNSNames)
	}
	if !meta.NotAfter.Equal(notAfter) {
		t.Errorf("NotAfter = %s, want %s", meta.NotAfter, notAfter)
	}
	if !meta.NotBefore.Equal(notBefore) {
		t.Errorf("NotBefore = %s, want %s", meta.NotBefore, notBefore)
	}
}

func TestCertMetadataFromPEM_Unparseable(t *testing.T) {
	if _, err := CertMetadataFromPEM([]byte("garbage")); err == nil {
		t.Fatal("CertMetadataFromPEM(garbage) = nil, want error")
	}
}

func TestCertMetadataFromPEM_Empty(t *testing.T) {
	if _, err := CertMetadataFromPEM(nil); err == nil {
		t.Fatal("CertMetadataFromPEM(nil) = nil, want error")
	}
}
