// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers — generate self-signed certificates on the fly
// ---------------------------------------------------------------------------

type testCert struct {
	CertPath string
	KeyPath  string
	Leaf     *x509.Certificate
}

// generateTestCert creates a self-signed ECDSA certificate and key in the
// given directory. The certificate is valid from notBefore to notAfter with
// the given common name and optional DNS SANs.
func generateTestCert(t *testing.T, dir, cn string, notBefore, notAfter time.Time, dnsNames ...string) testCert {
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

	certPath := filepath.Join(dir, cn+".crt")
	keyPath := filepath.Join(dir, cn+".key")

	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("creating cert file: %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("encoding cert PEM: %v", err)
	}
	certFile.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling key: %v", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("creating key file: %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("encoding key PEM: %v", err)
	}
	keyFile.Close()

	leaf, _ := x509.ParseCertificate(certDER)

	return testCert{
		CertPath: certPath,
		KeyPath:  keyPath,
		Leaf:     leaf,
	}
}

// generateTestCA creates a self-signed CA certificate and returns the cert
// PEM bytes, the CA certificate, and the CA private key. It also writes the
// CA cert PEM to a file in the given directory.
func generateTestCA(t *testing.T, dir, cn string) (caPath string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generating CA serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating CA certificate: %v", err)
	}

	caPath = filepath.Join(dir, cn+"-ca.crt")
	f, err := os.Create(caPath)
	if err != nil {
		t.Fatalf("creating CA file: %v", err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("encoding CA PEM: %v", err)
	}
	f.Close()

	caCert, _ = x509.ParseCertificate(certDER)
	return caPath, caCert, key
}

// collectLogs returns a LogFunc that appends all messages to a slice.
// logCapture is a thread-safe log collector for tests. The LogFunc
// returned by collectLogs appends entries under a mutex; Snapshot and
// Contains read them under the same mutex so there is no data race
// when a watcher goroutine is still logging.
type logCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (lc *logCapture) append(entry string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.msgs = append(lc.msgs, entry)
}

// Snapshot returns a copy of the collected log entries.
func (lc *logCapture) Snapshot() []string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	cp := make([]string, len(lc.msgs))
	copy(cp, lc.msgs)
	return cp
}

// Contains reports whether any collected entry contains substr.
func (lc *logCapture) Contains(substr string) bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	for _, l := range lc.msgs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func collectLogs() (LogFunc, *logCapture) {
	lc := &logCapture{}
	fn := func(level, msg string) {
		lc.append(level + ": " + msg)
	}
	return fn, lc
}

// ---------------------------------------------------------------------------
// Tests: NewCertManager
// ---------------------------------------------------------------------------

func TestNewCertManager_EmptyCertPath(t *testing.T) {
	_, err := NewCertManager("", "/some/key.pem")
	if err == nil {
		t.Fatal("expected error for empty cert_path, got nil")
	}
	if !strings.Contains(err.Error(), "cert_path is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManager_EmptyKeyPath(t *testing.T) {
	_, err := NewCertManager("/some/cert.pem", "")
	if err == nil {
		t.Fatal("expected error for empty key_path, got nil")
	}
	if !strings.Contains(err.Error(), "key_path is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManager_CertFileNotFound(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "server.key")
	os.WriteFile(keyPath, []byte("dummy"), 0600)

	_, err := NewCertManager(filepath.Join(dir, "nonexistent.crt"), keyPath)
	if err == nil {
		t.Fatal("expected error for missing cert file, got nil")
	}
	if !strings.Contains(err.Error(), "initial certificate load failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManager_KeyFileNotFound(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	os.WriteFile(certPath, []byte("dummy"), 0644)

	_, err := NewCertManager(certPath, filepath.Join(dir, "nonexistent.key"))
	if err == nil {
		t.Fatal("expected error for missing key file, got nil")
	}
}

func TestNewCertManager_MismatchedKeyPair(t *testing.T) {
	dir := t.TempDir()
	tc1 := generateTestCert(t, dir, "cert1", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))
	tc2 := generateTestCert(t, dir, "cert2", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))

	_, err := NewCertManager(tc1.CertPath, tc2.KeyPath)
	if err == nil {
		t.Fatal("expected error for mismatched cert/key, got nil")
	}
}

func TestNewCertManager_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "bad.crt")
	keyPath := filepath.Join(dir, "bad.key")
	os.WriteFile(certPath, []byte("not a PEM"), 0644)
	os.WriteFile(keyPath, []byte("not a PEM"), 0600)

	_, err := NewCertManager(certPath, keyPath)
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

func TestNewCertManager_ValidCert(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "server",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(30*24*time.Hour),
		"localhost", "example.com",
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	if cm.CertPath() != tc.CertPath {
		t.Errorf("CertPath() = %q, want %q", cm.CertPath(), tc.CertPath)
	}
	if cm.KeyPath() != tc.KeyPath {
		t.Errorf("KeyPath() = %q, want %q", cm.KeyPath(), tc.KeyPath)
	}

	leaf := cm.LeafCert()
	if leaf == nil {
		t.Fatal("LeafCert() returned nil")
	}
	if leaf.Subject.CommonName != "server" {
		t.Errorf("leaf CN = %q, want %q", leaf.Subject.CommonName, "server")
	}

	if !logs.Contains("TLS certificate loaded") {
		t.Errorf("expected 'TLS certificate loaded' log, got: %v", logs.Snapshot())
	}
	if !logs.Contains("TLS certificate valid") {
		t.Errorf("expected 'TLS certificate valid' log, got: %v", logs.Snapshot())
	}
}

func TestNewCertManager_ExpiredCert_WarnsButSucceeds(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "expired",
		time.Now().Add(-48*time.Hour),
		time.Now().Add(-1*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("expected success for expired cert, got error: %v", err)
	}
	defer cm.Close()

	if !logs.Contains("EXPIRED") {
		t.Errorf("expected EXPIRED warning in logs, got: %v", logs.Snapshot())
	}
}

func TestNewCertManager_NearExpiryCert_Warns(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "nearexpiry",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(3*24*time.Hour), // expires in 3 days
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	if !logs.Contains("expires soon") {
		t.Errorf("expected 'expires soon' warning in logs, got: %v", logs.Snapshot())
	}
}

func TestNewCertManager_KeyPermissionsWarning(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "permtest",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	// Make key world-readable.
	if err := os.Chmod(tc.KeyPath, 0644); err != nil {
		t.Skipf("cannot change permissions: %v", err)
	}

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	if !logs.Contains("permissions") {
		t.Errorf("expected key permissions warning in logs, got: %v", logs.Snapshot())
	}
}

// ---------------------------------------------------------------------------
// Tests: mTLS (WithCAPath)
// ---------------------------------------------------------------------------

func TestNewCertManager_WithCAPath_Valid(t *testing.T) {
	dir := t.TempDir()
	logFn, _ := collectLogs()

	tc := generateTestCert(t, dir, "mtls-server",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)
	caPath, _, _ := generateTestCA(t, dir, "test-ca")

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath,
		WithCAPath(caPath),
		WithLogger(logFn),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	// Verify TLSConfig enables mTLS.
	tlsCfg := cm.TLSConfig("1.2")
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
	}
	if tlsCfg.ClientCAs == nil {
		t.Error("ClientCAs is nil, expected non-nil pool")
	}
}

func TestNewCertManager_WithCAPath_FileNotFound(t *testing.T) {
	dir := t.TempDir()

	tc := generateTestCert(t, dir, "mtls-server2",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	_, err := NewCertManager(tc.CertPath, tc.KeyPath,
		WithCAPath(filepath.Join(dir, "nonexistent-ca.crt")),
	)
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
	if !strings.Contains(err.Error(), "loading client CA bundle") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManager_WithCAPath_InvalidPEM(t *testing.T) {
	dir := t.TempDir()

	tc := generateTestCert(t, dir, "mtls-server3",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	badCAPath := filepath.Join(dir, "bad-ca.crt")
	os.WriteFile(badCAPath, []byte("not a PEM certificate"), 0644)

	_, err := NewCertManager(tc.CertPath, tc.KeyPath,
		WithCAPath(badCAPath),
	)
	if err == nil {
		t.Fatal("expected error for invalid CA PEM")
	}
	if !strings.Contains(err.Error(), "no valid PEM") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewCertManager_WithoutCAPath_NoMTLS(t *testing.T) {
	dir := t.TempDir()

	tc := generateTestCert(t, dir, "no-mtls",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	tlsCfg := cm.TLSConfig("1.2")
	if tlsCfg.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %v, want NoClientCert", tlsCfg.ClientAuth)
	}
	if tlsCfg.ClientCAs != nil {
		t.Error("ClientCAs should be nil when no CA path is set")
	}
}

// ---------------------------------------------------------------------------
// Tests: Reload
// ---------------------------------------------------------------------------

func TestCertManager_Reload_Success(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "reload-v1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	// Overwrite the cert/key with a new one (same filenames).
	tc2 := generateTestCert(t, dir, "reload-v2",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(48*time.Hour),
		"new.example.com",
	)

	// Copy v2 over v1 paths.
	copyFile(t, tc2.CertPath, tc.CertPath)
	copyFile(t, tc2.KeyPath, tc.KeyPath)

	if err := cm.Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	leaf := cm.LeafCert()
	if leaf == nil {
		t.Fatal("LeafCert() returned nil after reload")
	}
	if leaf.Subject.CommonName != "reload-v2" {
		t.Errorf("leaf CN after reload = %q, want %q", leaf.Subject.CommonName, "reload-v2")
	}

	if !logs.Contains("certificate reloaded") {
		t.Errorf("expected 'certificate reloaded' log, got: %v", logs.Snapshot())
	}
}

func TestCertManager_Reload_FailureKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "reload-keep",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	originalLeaf := cm.LeafCert()

	// Corrupt the cert file.
	os.WriteFile(tc.CertPath, []byte("corrupted"), 0644)

	err = cm.Reload()
	if err == nil {
		t.Fatal("expected reload error for corrupted cert, got nil")
	}

	// Previous certificate should still be served.
	currentLeaf := cm.LeafCert()
	if currentLeaf == nil {
		t.Fatal("LeafCert() returned nil after failed reload")
	}
	if currentLeaf.Subject.CommonName != originalLeaf.Subject.CommonName {
		t.Errorf("CN changed after failed reload: got %q, want %q",
			currentLeaf.Subject.CommonName, originalLeaf.Subject.CommonName)
	}

	if !logs.Contains("certificate reload failed") {
		t.Errorf("expected 'certificate reload failed' log, got: %v", logs.Snapshot())
	}
}

// ---------------------------------------------------------------------------
// Tests: TLSConfig
// ---------------------------------------------------------------------------

func TestTLSConfig_MinVersion12(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "tlscfg12",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.2")
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = 0x%04x, want 0x%04x (TLS 1.2)", cfg.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSConfig_MinVersion13(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "tlscfg13",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.3")
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = 0x%04x, want 0x%04x (TLS 1.3)", cfg.MinVersion, tls.VersionTLS13)
	}
}

func TestTLSConfig_InvalidMinVersionDefaultsTo12(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "tlscfgbad",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("banana")
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = 0x%04x, want 0x%04x (TLS 1.2 default)", cfg.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSConfig_GetCertificateCallback(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "callback",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.2")
	if cfg.GetCertificate == nil {
		t.Fatal("GetCertificate is nil")
	}

	cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate error: %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificate returned nil cert")
	}
	if cert.Leaf == nil {
		t.Fatal("GetCertificate returned cert with nil Leaf")
	}
	if cert.Leaf.Subject.CommonName != "callback" {
		t.Errorf("cert CN = %q, want %q", cert.Leaf.Subject.CommonName, "callback")
	}
}

func TestTLSConfig_CipherSuites(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "ciphers",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.2")

	if len(cfg.CipherSuites) == 0 {
		t.Error("CipherSuites is empty, expected curated list")
	}

	// All suites should be ECDHE + AEAD (GCM or ChaCha20).
	for _, id := range cfg.CipherSuites {
		name := tls.CipherSuiteName(id)
		if !strings.Contains(name, "ECDHE") {
			t.Errorf("cipher suite %s is not ECDHE-based", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: GetCertificate after Reload (hot swap)
// ---------------------------------------------------------------------------

func TestGetCertificate_ReflectsReload(t *testing.T) {
	dir := t.TempDir()

	tc1 := generateTestCert(t, dir, "hotswap-v1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc1.CertPath, tc1.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.2")

	// Verify initial cert.
	cert1, _ := cfg.GetCertificate(&tls.ClientHelloInfo{})
	if cert1.Leaf.Subject.CommonName != "hotswap-v1" {
		t.Fatalf("initial cert CN = %q, want %q", cert1.Leaf.Subject.CommonName, "hotswap-v1")
	}

	// Generate and swap in a new cert.
	tc2 := generateTestCert(t, dir, "hotswap-v2",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(48*time.Hour),
	)
	copyFile(t, tc2.CertPath, tc1.CertPath)
	copyFile(t, tc2.KeyPath, tc1.KeyPath)

	if err := cm.Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	// Same TLSConfig should now return the new cert.
	cert2, _ := cfg.GetCertificate(&tls.ClientHelloInfo{})
	if cert2.Leaf.Subject.CommonName != "hotswap-v2" {
		t.Errorf("cert CN after reload = %q, want %q", cert2.Leaf.Subject.CommonName, "hotswap-v2")
	}
}

// ---------------------------------------------------------------------------
// Tests: CurrentCert and LeafCert
// ---------------------------------------------------------------------------

func TestCurrentCert_ReturnsNonNil(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "current",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cert := cm.CurrentCert()
	if cert == nil {
		t.Fatal("CurrentCert() returned nil")
	}
}

func TestLeafCert_SubjectAndDNSNames(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "leafcheck",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"a.example.com", "b.example.com",
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	leaf := cm.LeafCert()
	if leaf == nil {
		t.Fatal("LeafCert() returned nil")
	}
	if leaf.Subject.CommonName != "leafcheck" {
		t.Errorf("CN = %q, want %q", leaf.Subject.CommonName, "leafcheck")
	}
	if len(leaf.DNSNames) != 2 {
		t.Errorf("DNSNames count = %d, want 2", len(leaf.DNSNames))
	}
}

// ---------------------------------------------------------------------------
// Tests: Close
// ---------------------------------------------------------------------------

func TestClose_Idempotent(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "closeme",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic on multiple Close calls.
	if err := cm.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := cm.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: WatchForChanges
// ---------------------------------------------------------------------------

func TestWatchForChanges_DetectsFileChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file watcher test in short mode")
	}

	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "watch-v1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	// Use a very short poll interval for the test.
	cm.WatchForChanges(500 * time.Millisecond)

	// Wait a moment, then swap the cert.
	time.Sleep(200 * time.Millisecond)

	tc2 := generateTestCert(t, dir, "watch-v2",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(48*time.Hour),
	)
	copyFile(t, tc2.CertPath, tc.CertPath)
	copyFile(t, tc2.KeyPath, tc.KeyPath)

	// Wait for the watcher to detect the change and reload (poll + 1s delay + margin).
	time.Sleep(3 * time.Second)

	leaf := cm.LeafCert()
	if leaf == nil {
		t.Fatal("LeafCert() returned nil after watch-triggered reload")
	}
	if leaf.Subject.CommonName != "watch-v2" {
		t.Errorf("leaf CN = %q, want %q", leaf.Subject.CommonName, "watch-v2")
	}

	if !logs.Contains("file change detected") {
		t.Errorf("expected 'file change detected' log, got: %v", logs.Snapshot())
	}
}

func TestWatchForChanges_StopsOnClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file watcher test in short mode")
	}

	dir := t.TempDir()
	logFn, logs := collectLogs()

	tc := generateTestCert(t, dir, "watch-stop",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath, WithLogger(logFn))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cm.WatchForChanges(200 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	cm.Close()
	time.Sleep(500 * time.Millisecond)

	if !logs.Contains("watcher stopped") {
		t.Errorf("expected 'watcher stopped' log, got: %v", logs.Snapshot())
	}
}

func TestWatchForChanges_DefaultPollInterval(t *testing.T) {
	// Just verify it doesn't panic with a zero/negative interval.
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "watch-default",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cm.WatchForChanges(0) // should default to 30s
	cm.Close()            // stop immediately
}

// ---------------------------------------------------------------------------
// Tests: Concurrent access
// ---------------------------------------------------------------------------

func TestCertManager_ConcurrentReloadAndGetCertificate(t *testing.T) {
	dir := t.TempDir()

	tc := generateTestCert(t, dir, "concurrent",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	cfg := cm.TLSConfig("1.2")

	var wg sync.WaitGroup
	errs := make(chan error, 200)

	// Spawn readers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
			if err != nil {
				errs <- err
				return
			}
			if cert == nil {
				errs <- errors.New("got nil cert")
				return
			}
			// Also exercise LeafCert.
			_ = cm.LeafCert()
		}()
	}

	// Spawn reloaders (they'll reload the same valid cert).
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.Reload()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent access error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: parseTLSVersion helper
// ---------------------------------------------------------------------------

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		input string
		want  uint16
	}{
		{"1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
		{"", tls.VersionTLS12},
		{"1.0", tls.VersionTLS12},
		{"1.1", tls.VersionTLS12},
		{"invalid", tls.VersionTLS12},
	}
	for _, tt := range tests {
		got := parseTLSVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseTLSVersion(%q) = 0x%04x, want 0x%04x", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: defaultCipherSuites helper
// ---------------------------------------------------------------------------

func TestDefaultCipherSuites_NonEmpty(t *testing.T) {
	suites := defaultCipherSuites()
	if len(suites) == 0 {
		t.Error("defaultCipherSuites() returned empty slice")
	}
}

func TestDefaultCipherSuites_AllValid(t *testing.T) {
	suites := defaultCipherSuites()
	for _, id := range suites {
		name := tls.CipherSuiteName(id)
		if name == "" {
			t.Errorf("cipher suite 0x%04x has no name — may be invalid", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: loadCACertPool helper
// ---------------------------------------------------------------------------

func TestLoadCACertPool_Valid(t *testing.T) {
	dir := t.TempDir()
	caPath, _, _ := generateTestCA(t, dir, "pool-test-ca")

	pool, err := loadCACertPool(caPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}
}

func TestLoadCACertPool_NotFound(t *testing.T) {
	_, err := loadCACertPool("/nonexistent/ca.crt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadCACertPool_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-ca.crt")
	os.WriteFile(path, []byte("not PEM data"), 0644)

	_, err := loadCACertPool(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if !strings.Contains(err.Error(), "no valid PEM") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: fileState helper
// ---------------------------------------------------------------------------

func TestFileState_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	modTime, target := fileState(path)
	if modTime.IsZero() {
		t.Error("modTime is zero for existing file")
	}
	if target == "" {
		t.Error("target is empty for existing file")
	}
}

func TestFileState_Symlink(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.txt")
	os.WriteFile(realPath, []byte("hello"), 0644)

	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, target := fileState(linkPath)
	if target == "" {
		t.Error("target is empty for symlink")
	}
	// The resolved target should point to realPath. Use EvalSymlinks on
	// both sides to normalise macOS /var → /private/var differences.
	absReal, _ := filepath.EvalSymlinks(realPath)
	absTarget, _ := filepath.EvalSymlinks(target)
	if absTarget != absReal {
		t.Errorf("target = %q, want %q", absTarget, absReal)
	}
}

func TestFileState_NonexistentFile(t *testing.T) {
	modTime, target := fileState("/nonexistent/file.txt")
	if !modTime.IsZero() {
		t.Error("modTime should be zero for nonexistent file")
	}
	if target != "" {
		t.Error("target should be empty for nonexistent file")
	}
}

// ---------------------------------------------------------------------------
// Tests: No logger (nil log func)
// ---------------------------------------------------------------------------

func TestNewCertManager_NilLogger(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "nolog",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	// Should not panic with no logger set.
	cm, err := NewCertManager(tc.CertPath, tc.KeyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cm.Close()

	// Reload should also work without a logger.
	if err := cm.Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// copyFile copies src to dst, overwriting dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
}

// errors import for concurrent test
var _ = errors.New // ensure errors is used
