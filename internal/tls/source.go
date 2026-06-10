// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// pemSource abstracts where the CertManager obtains its certificate and key
// PEM bytes. Two implementations exist: fileSource (paths on disk, the default
// for cert_source: file) and bytesSource (in-memory bytes, used for
// cert_source: db where material is fetched from the encrypted config store).
type pemSource interface {
	// loadPEM returns the current certificate and key PEM bytes. It is
	// called at startup and on every reload.
	loadPEM() (certPEM, keyPEM []byte, err error)

	// description returns a short, operator-safe label for logging. It never
	// contains key material.
	description() string
}

// ---------------------------------------------------------------------------
// File source
// ---------------------------------------------------------------------------

// fileSource reads the certificate and key from files on disk. It supports
// the filesystem watcher and key-permission checks in CertManager.
type fileSource struct {
	certPath string
	keyPath  string
}

func (f *fileSource) loadPEM() (certPEM, keyPEM []byte, err error) {
	certPEM, err = os.ReadFile(f.certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading certificate file %s: %w", f.certPath, err)
	}
	keyPEM, err = os.ReadFile(f.keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading key file %s: %w", f.keyPath, err)
	}
	return certPEM, keyPEM, nil
}

func (f *fileSource) description() string {
	return fmt.Sprintf("file (cert=%s, key=%s)", f.certPath, f.keyPath)
}

// ---------------------------------------------------------------------------
// In-memory bytes source
// ---------------------------------------------------------------------------

// bytesSource holds the certificate and key PEM bytes in memory. It is used
// for cert_source: db, where the material is fetched from the encrypted
// config store and swapped in via CertManager.ReloadFromPEM on a config
// change. It is safe for concurrent use.
type bytesSource struct {
	mu      sync.RWMutex
	certPEM []byte
	keyPEM  []byte
}

func (b *bytesSource) loadPEM() (certPEM, keyPEM []byte, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.certPEM) == 0 {
		return nil, nil, errors.New("no certificate PEM provided")
	}
	if len(b.keyPEM) == 0 {
		return nil, nil, errors.New("no private key PEM provided")
	}
	// Return copies so callers cannot mutate the held slices.
	return append([]byte(nil), b.certPEM...), append([]byte(nil), b.keyPEM...), nil
}

// set replaces the held PEM bytes with copies of the provided slices.
func (b *bytesSource) set(certPEM, keyPEM []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.certPEM = append([]byte(nil), certPEM...)
	b.keyPEM = append([]byte(nil), keyPEM...)
}

func (b *bytesSource) description() string {
	return "in-memory PEM"
}
