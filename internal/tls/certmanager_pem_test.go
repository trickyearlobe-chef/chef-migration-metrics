// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helper — generate a self-signed cert/key as in-memory PEM bytes
// ---------------------------------------------------------------------------

// generateTestCertPEM creates a self-signed ECDSA certificate and returns the
// certificate and key as PEM byte slices (no files on disk), mirroring
// generateTestCert but for the in-memory PEM source.
func generateTestCertPEM(t *testing.T, cn string, notBefore, notAfter time.Time, dnsNames ...string) (certPEM, keyPEM []byte, leaf *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating ECDSA key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generating serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IsCA:         false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling key: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	leaf, _ = x509.ParseCertificate(certDER)
	return certPEM, keyPEM, leaf
}

// ---------------------------------------------------------------------------
// Tests: NewCertManagerFromPEM
// ---------------------------------------------------------------------------

func TestNewCertManagerFromPEM_ValidCert(t *testing.T) {
	certPEM, keyPEM, leaf := generateTestCertPEM(t, "pem-valid",
		time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour), "pem.example.com")

	logFn, logs := collectLogs()
	cm, err := NewCertManagerFromPEM(certPEM, keyPEM, WithLogger(logFn))
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	got := cm.LeafCert()
	if got == nil {
		t.Fatal("LeafCert returned nil")
	}
	if got.Subject.CommonName != leaf.Subject.CommonName {
		t.Errorf("CommonName = %q, want %q", got.Subject.CommonName, leaf.Subject.CommonName)
	}
	if cm.CurrentCert() == nil {
		t.Error("CurrentCert returned nil")
	}
	if !logs.Contains("TLS certificate valid") {
		t.Errorf("expected 'TLS certificate valid' log, got: %v", logs.Snapshot())
	}
}

func TestNewCertManagerFromPEM_EmptyBytes(t *testing.T) {
	cases := []struct {
		name    string
		certPEM []byte
		keyPEM  []byte
	}{
		{"empty cert", nil, []byte("key")},
		{"empty key", []byte("cert"), nil},
		{"both empty", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCertManagerFromPEM(tc.certPEM, tc.keyPEM)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "initial certificate load failed") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewCertManagerFromPEM_InvalidPEM(t *testing.T) {
	_, err := NewCertManagerFromPEM([]byte("not a pem"), []byte("also not a pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
	if !strings.Contains(err.Error(), "initial certificate load failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManagerFromPEM_MismatchedPair(t *testing.T) {
	certPEM, _, _ := generateTestCertPEM(t, "pem-a",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	_, keyPEM, _ := generateTestCertPEM(t, "pem-b",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	_, err := NewCertManagerFromPEM(certPEM, keyPEM)
	if err == nil {
		t.Fatal("expected error for mismatched cert/key, got nil")
	}
}

func TestNewCertManagerFromPEM_ExpiredWarns(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "pem-expired",
		time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))

	logFn, logs := collectLogs()
	cm, err := NewCertManagerFromPEM(certPEM, keyPEM, WithLogger(logFn))
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: expired cert should still load, got: %v", err)
	}
	defer func() { _ = cm.Close() }()

	if !logs.Contains("EXPIRED") {
		t.Errorf("expected EXPIRED warning, got: %v", logs.Snapshot())
	}
}

// ---------------------------------------------------------------------------
// Tests: ReloadFromPEM
// ---------------------------------------------------------------------------

func TestReloadFromPEM_SwapsCertificate(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "pem-before",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	logFn, logs := collectLogs()
	cm, err := NewCertManagerFromPEM(certPEM, keyPEM, WithLogger(logFn))
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	newCertPEM, newKeyPEM, newLeaf := generateTestCertPEM(t, "pem-after",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	if err := cm.ReloadFromPEM(newCertPEM, newKeyPEM); err != nil {
		t.Fatalf("ReloadFromPEM: %v", err)
	}

	got := cm.LeafCert()
	if got == nil || got.Subject.CommonName != newLeaf.Subject.CommonName {
		t.Errorf("after reload CommonName = %v, want %q", got, newLeaf.Subject.CommonName)
	}
	if !logs.Contains("certificate reloaded") {
		t.Errorf("expected 'certificate reloaded' log, got: %v", logs.Snapshot())
	}
}

func TestReloadFromPEM_BadPEMKeepsPreviousCert(t *testing.T) {
	certPEM, keyPEM, origLeaf := generateTestCertPEM(t, "pem-keep",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	logFn, logs := collectLogs()
	cm, err := NewCertManagerFromPEM(certPEM, keyPEM, WithLogger(logFn))
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	err = cm.ReloadFromPEM([]byte("garbage"), []byte("garbage"))
	if err == nil {
		t.Fatal("expected error reloading bad PEM, got nil")
	}
	if !logs.Contains("certificate reload failed") {
		t.Errorf("expected 'certificate reload failed' log, got: %v", logs.Snapshot())
	}

	// Previous certificate must still be served.
	got := cm.LeafCert()
	if got == nil || got.Subject.CommonName != origLeaf.Subject.CommonName {
		t.Errorf("previous cert not preserved: got %v, want CN %q", got, origLeaf.Subject.CommonName)
	}
}

func TestReloadFromPEM_NotSupportedForFileSource(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "file-src",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}
	defer func() { _ = cm.Close() }()

	certPEM, keyPEM, _ := generateTestCertPEM(t, "file-src-new",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	if err := cm.ReloadFromPEM(certPEM, keyPEM); err == nil {
		t.Fatal("expected ReloadFromPEM to be unsupported for a file source, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests: source-dependent accessors and watcher
// ---------------------------------------------------------------------------

func TestCertKeyPath_EmptyForPEMSource(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "pem-paths",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	cm, err := NewCertManagerFromPEM(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	if cm.CertPath() != "" {
		t.Errorf("CertPath = %q, want empty for PEM source", cm.CertPath())
	}
	if cm.KeyPath() != "" {
		t.Errorf("KeyPath = %q, want empty for PEM source", cm.KeyPath())
	}
}

func TestWatchForChanges_NoOpForPEMSource(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "pem-watch",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	cm, err := NewCertManagerFromPEM(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	// Must not panic or start a file watcher.
	cm.WatchForChanges(10 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Tests: TLSConfig serves the in-memory cert (parity with file source)
// ---------------------------------------------------------------------------

func TestTLSConfig_PEMSourceServesCert(t *testing.T) {
	certPEM, keyPEM, leaf := generateTestCertPEM(t, "pem-serve",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	cm, err := NewCertManagerFromPEM(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("NewCertManagerFromPEM: %v", err)
	}
	defer func() { _ = cm.Close() }()

	cfg := cm.TLSConfig("1.2")
	served, err := cfg.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if served == nil || served.Leaf == nil {
		t.Fatal("GetCertificate returned no leaf")
	}
	if served.Leaf.Subject.CommonName != leaf.Subject.CommonName {
		t.Errorf("served CN = %q, want %q", served.Leaf.Subject.CommonName, leaf.Subject.CommonName)
	}
}
