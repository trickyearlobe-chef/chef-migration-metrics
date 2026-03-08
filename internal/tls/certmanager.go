// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package tls provides TLS listener setup, certificate loading and validation,
// automatic reload on SIGHUP and filesystem changes, HTTP-to-HTTPS redirect,
// and HSTS middleware for the Chef Migration Metrics server.
//
// This package implements the "static" TLS mode described in the TLS
// specification. ACME automatic certificate management is not yet implemented.
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogFunc is a logging callback. The level parameter is one of "DEBUG",
// "INFO", "WARN", "ERROR". The tls package does not import the logging
// package directly to avoid circular dependencies — the caller provides
// a logging function at construction time (same pattern as webapi.Router).
type LogFunc func(level, msg string)

// CertManager manages a TLS certificate and key pair. It supports:
//   - Loading and validating a certificate + key from PEM files at startup
//   - Hot-reloading the certificate on demand (e.g. from SIGHUP)
//   - Filesystem watching for automatic reload when cert/key files change
//   - Constructing a crypto/tls.Config that always serves the latest cert
//   - Optional mutual TLS (mTLS) via a client CA bundle
//
// CertManager is safe for concurrent use.
type CertManager struct {
	certPath string
	keyPath  string
	caPath   string

	mu   sync.RWMutex
	cert *tls.Certificate

	// clientCAs is set once at startup if caPath is provided. It is not
	// reloaded on SIGHUP because changing the client CA at runtime could
	// break existing mTLS connections in unpredictable ways.
	clientCAs *x509.CertPool

	log LogFunc

	// watcher lifecycle
	watchDone chan struct{}
	watchOnce sync.Once
}

// CertManagerOption is a functional option for NewCertManager.
type CertManagerOption func(*CertManager)

// WithCAPath enables mutual TLS by loading a client CA bundle from the
// given path. When set, the TLS listener will require and verify client
// certificates against this CA pool.
func WithCAPath(path string) CertManagerOption {
	return func(cm *CertManager) {
		cm.caPath = path
	}
}

// WithLogger sets a logging callback. If not set, log messages are
// silently discarded.
func WithLogger(fn LogFunc) CertManagerOption {
	return func(cm *CertManager) {
		cm.log = fn
	}
}

// NewCertManager creates a new CertManager and performs initial certificate
// loading and validation. It returns an error if:
//   - certPath or keyPath is empty
//   - the certificate or key file cannot be read
//   - the certificate and key do not form a valid pair
//   - caPath is set but cannot be read or parsed
//
// If the certificate is expired at startup, a WARN is logged but the
// manager is still created (operators may be in the process of renewing).
func NewCertManager(certPath, keyPath string, opts ...CertManagerOption) (*CertManager, error) {
	if certPath == "" {
		return nil, errors.New("tls: cert_path is required")
	}
	if keyPath == "" {
		return nil, errors.New("tls: key_path is required")
	}

	cm := &CertManager{
		certPath:  certPath,
		keyPath:   keyPath,
		watchDone: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(cm)
	}

	// Load and validate the initial certificate.
	if err := cm.loadCertificate(); err != nil {
		return nil, fmt.Errorf("tls: initial certificate load failed: %w", err)
	}

	// Check expiry — warn but don't fail.
	cm.checkExpiry()

	// Check key file permissions — warn if too open.
	cm.checkKeyPermissions()

	// Load client CA bundle for mTLS if configured.
	if cm.caPath != "" {
		pool, err := loadCACertPool(cm.caPath)
		if err != nil {
			return nil, fmt.Errorf("tls: loading client CA bundle: %w", err)
		}
		cm.clientCAs = pool
		cm.logf("INFO", "mTLS enabled — loaded client CA bundle from %s", cm.caPath)
	}

	cm.logf("INFO", "TLS certificate loaded from %s (key: %s)", cm.certPath, cm.keyPath)
	return cm, nil
}

// Reload re-reads the certificate and key files from disk. If the new files
// are valid, the certificate is swapped atomically for subsequent TLS
// handshakes. Existing connections are not affected.
//
// If the reload fails (unparseable files, mismatched key, etc.), the
// previous valid certificate continues to be served and an error is returned.
func (cm *CertManager) Reload() error {
	if err := cm.loadCertificate(); err != nil {
		cm.logf("ERROR", "certificate reload failed (continuing with previous certificate): %v", err)
		return fmt.Errorf("tls: certificate reload failed: %w", err)
	}
	cm.checkExpiry()
	cm.logf("INFO", "TLS certificate reloaded from %s", cm.certPath)
	return nil
}

// TLSConfig returns a *tls.Config suitable for use with an HTTPS listener.
// The returned config uses GetCertificate to always serve the latest loaded
// certificate, enabling hot-reload without restarting the listener.
//
// minVersion must be "1.2" or "1.3". Any other value defaults to TLS 1.2.
func (cm *CertManager) TLSConfig(minVersion string) *tls.Config {
	tlsCfg := &tls.Config{
		GetCertificate: cm.getCertificate,
		MinVersion:     parseTLSVersion(minVersion),

		// Prefer server cipher suites for TLS 1.2. TLS 1.3 ignores this
		// because cipher suite selection is not configurable.
		PreferServerCipherSuites: true,

		// Use only secure cipher suites for TLS 1.2. TLS 1.3 suites are
		// always secure and cannot be configured.
		CipherSuites: defaultCipherSuites(),
	}

	// Enable mTLS if a client CA pool is configured.
	if cm.clientCAs != nil {
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		tlsCfg.ClientCAs = cm.clientCAs
		cm.logf("DEBUG", "TLS config: mTLS enabled (RequireAndVerifyClientCert)")
	}

	cm.logf("DEBUG", "TLS config: min_version=%s cipher_suites=%d",
		minVersion, len(tlsCfg.CipherSuites))

	return tlsCfg
}

// CertPath returns the configured certificate file path.
func (cm *CertManager) CertPath() string {
	return cm.certPath
}

// KeyPath returns the configured key file path.
func (cm *CertManager) KeyPath() string {
	return cm.keyPath
}

// CurrentCert returns a copy of the currently loaded certificate. This is
// primarily useful for diagnostics and logging (e.g. reporting the subject,
// issuer, and expiry of the active certificate).
func (cm *CertManager) CurrentCert() *tls.Certificate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cert
}

// LeafCert returns the parsed x509 leaf certificate, or nil if no
// certificate is loaded or the leaf has not been parsed.
func (cm *CertManager) LeafCert() *x509.Certificate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.cert == nil {
		return nil
	}
	return cm.cert.Leaf
}

// Close stops the filesystem watcher (if running) and releases resources.
func (cm *CertManager) Close() error {
	cm.watchOnce.Do(func() {
		close(cm.watchDone)
	})
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// loadCertificate reads the cert and key files, parses them, validates the
// pair, and stores the result atomically.
func (cm *CertManager) loadCertificate() error {
	cert, err := tls.LoadX509KeyPair(cm.certPath, cm.keyPath)
	if err != nil {
		return fmt.Errorf("loading key pair (%s, %s): %w", cm.certPath, cm.keyPath, err)
	}

	// Parse the leaf certificate so we can inspect it (expiry, subject,
	// etc.) without re-parsing later.
	if cert.Leaf == nil && len(cert.Certificate) > 0 {
		leaf, parseErr := x509.ParseCertificate(cert.Certificate[0])
		if parseErr != nil {
			return fmt.Errorf("parsing leaf certificate: %w", parseErr)
		}
		cert.Leaf = leaf
	}

	cm.mu.Lock()
	cm.cert = &cert
	cm.mu.Unlock()

	return nil
}

// getCertificate is the callback for tls.Config.GetCertificate. It returns
// the currently loaded certificate for every ClientHello.
func (cm *CertManager) getCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.cert == nil {
		return nil, errors.New("tls: no certificate loaded")
	}
	return cm.cert, nil
}

// checkExpiry logs a warning if the current certificate is expired or will
// expire within 7 days.
func (cm *CertManager) checkExpiry() {
	leaf := cm.LeafCert()
	if leaf == nil {
		return
	}
	now := time.Now()
	if now.After(leaf.NotAfter) {
		cm.logf("WARN", "TLS certificate is EXPIRED (not_after=%s, subject=%s)",
			leaf.NotAfter.Format(time.RFC3339), leaf.Subject.CommonName)
	} else if leaf.NotAfter.Before(now.Add(7 * 24 * time.Hour)) {
		cm.logf("WARN", "TLS certificate expires soon (not_after=%s, subject=%s, days_remaining=%.0f)",
			leaf.NotAfter.Format(time.RFC3339), leaf.Subject.CommonName,
			time.Until(leaf.NotAfter).Hours()/24)
	} else {
		cm.logf("INFO", "TLS certificate valid (subject=%s, not_before=%s, not_after=%s)",
			leaf.Subject.CommonName,
			leaf.NotBefore.Format(time.RFC3339),
			leaf.NotAfter.Format(time.RFC3339))
	}
}

// checkKeyPermissions logs a warning if the key file has permissions more
// permissive than 0600.
func (cm *CertManager) checkKeyPermissions() {
	info, err := os.Stat(cm.keyPath)
	if err != nil {
		// Stat failure is not fatal here — the key was already loaded
		// successfully, so the file definitely existed a moment ago.
		return
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		cm.logf("WARN", "TLS key file %s has permissions %04o — recommended 0600", cm.keyPath, perm)
	}
}

// logf logs a formatted message if a logger is configured.
func (cm *CertManager) logf(level, format string, args ...any) {
	if cm.log != nil {
		cm.log(level, fmt.Sprintf(format, args...))
	}
}

// ---------------------------------------------------------------------------
// CA pool loading
// ---------------------------------------------------------------------------

// loadCACertPool reads a PEM-encoded CA bundle file and returns a
// *x509.CertPool. Returns an error if the file cannot be read or contains
// no valid PEM certificates.
func loadCACertPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no valid PEM certificates found in %s", path)
	}
	return pool, nil
}

// ---------------------------------------------------------------------------
// TLS version and cipher suite helpers
// ---------------------------------------------------------------------------

// parseTLSVersion converts a string version ("1.2" or "1.3") to the
// corresponding tls.VersionTLS constant. Defaults to TLS 1.2.
func parseTLSVersion(v string) uint16 {
	switch v {
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

// defaultCipherSuites returns a curated list of secure TLS 1.2 cipher suites.
// TLS 1.3 cipher suites are not configurable in Go's crypto/tls — they are
// always the full set of secure suites.
func defaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	}
}

// ---------------------------------------------------------------------------
// Filesystem watcher
// ---------------------------------------------------------------------------

// WatchForChanges starts a background goroutine that watches the certificate
// and key files for modifications. When a change is detected, the certificate
// is reloaded automatically. This is particularly useful in Kubernetes where
// cert-manager updates mounted Secret files in place.
//
// The watcher uses a simple polling strategy (checking file modification times)
// rather than inotify/fsnotify to avoid an external dependency. The poll
// interval is configurable.
//
// Call Close() to stop the watcher.
func (cm *CertManager) WatchForChanges(pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}

	cm.logf("INFO", "starting certificate file watcher (poll_interval=%s, cert=%s, key=%s)",
		pollInterval, cm.certPath, cm.keyPath)

	go cm.watchLoop(pollInterval)
}

// watchLoop is the main polling loop for filesystem-based certificate
// reload. It tracks the modification time and resolved symlink target of
// each file to detect both in-place edits and Kubernetes Secret rotations
// (which swap the symlink target).
func (cm *CertManager) watchLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Snapshot the initial state.
	certMod, certTarget := fileState(cm.certPath)
	keyMod, keyTarget := fileState(cm.keyPath)

	for {
		select {
		case <-cm.watchDone:
			cm.logf("INFO", "certificate file watcher stopped")
			return
		case <-ticker.C:
			newCertMod, newCertTarget := fileState(cm.certPath)
			newKeyMod, newKeyTarget := fileState(cm.keyPath)

			changed := false
			if !newCertMod.Equal(certMod) || newCertTarget != certTarget {
				changed = true
			}
			if !newKeyMod.Equal(keyMod) || newKeyTarget != keyTarget {
				changed = true
			}

			if changed {
				cm.logf("INFO", "certificate file change detected — reloading")
				// Small delay to allow atomic file operations to complete
				// (e.g. Kubernetes Secret updates write multiple files).
				time.Sleep(1 * time.Second)

				if err := cm.Reload(); err != nil {
					cm.logf("ERROR", "automatic certificate reload failed: %v", err)
				} else {
					certMod = newCertMod
					certTarget = newCertTarget
					keyMod = newKeyMod
					keyTarget = newKeyTarget
				}
			}
		}
	}
}

// fileState returns the modification time and resolved symlink target for a
// file path. On any error, it returns the zero time and an empty string,
// which will cause the next comparison to detect a change (triggering a
// reload attempt that will produce a proper error message).
func fileState(path string) (modTime time.Time, resolvedTarget string) {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}, ""
	}

	// Resolve symlinks — Kubernetes Secrets are mounted as a chain of
	// symlinks that get re-pointed during rotation.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}

	// Use the resolved file's mod time if the path is a symlink.
	if resolved != path {
		rInfo, rErr := os.Stat(resolved)
		if rErr == nil {
			return rInfo.ModTime(), resolved
		}
	}

	return info.ModTime(), resolved
}
