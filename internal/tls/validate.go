// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/tls"
	"errors"
	"fmt"
)

// ValidateStaticPair verifies that a static-mode certificate and private key
// (plus an optional CA bundle) are present and usable, WITHOUT constructing a
// listener or mutating any state.
//
// It performs the same load the server does at startup: tls.LoadX509KeyPair
// checks that the files are readable, the PEM parses, and the private key
// matches the certificate. A configuration that passes here will therefore not
// brick the TLS listener on the next restart.
//
// It is intended for preflight validation of a configuration change before it
// is persisted, so an operator cannot save a TLS configuration that would
// prevent the server from starting. The returned error is safe to surface to
// an operator (it names the failing element, not key material).
func ValidateStaticPair(certPath, keyPath, caPath string) error {
	if certPath == "" {
		return errors.New("cert_path is required")
	}
	if keyPath == "" {
		return errors.New("key_path is required")
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return fmt.Errorf("certificate/key pair: %w", err)
	}
	if caPath != "" {
		if _, err := loadCACertPool(caPath); err != nil {
			return fmt.Errorf("ca_path: %w", err)
		}
	}
	return nil
}
