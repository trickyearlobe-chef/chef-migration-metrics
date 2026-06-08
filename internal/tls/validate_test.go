// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateStaticPair(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	cert := generateTestCert(t, dir, "valid", now.Add(-time.Hour), now.Add(time.Hour), "localhost")

	t.Run("valid pair", func(t *testing.T) {
		if err := ValidateStaticPair(cert.CertPath, cert.KeyPath, ""); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("empty cert path", func(t *testing.T) {
		if err := ValidateStaticPair("", cert.KeyPath, ""); err == nil {
			t.Fatal("expected error for empty cert_path")
		}
	})

	t.Run("empty key path", func(t *testing.T) {
		if err := ValidateStaticPair(cert.CertPath, "", ""); err == nil {
			t.Fatal("expected error for empty key_path")
		}
	})

	t.Run("missing cert file", func(t *testing.T) {
		if err := ValidateStaticPair(filepath.Join(dir, "nope.crt"), cert.KeyPath, ""); err == nil {
			t.Fatal("expected error for missing cert file")
		}
	})

	t.Run("mismatched pair", func(t *testing.T) {
		other := generateTestCert(t, dir, "other", now.Add(-time.Hour), now.Add(time.Hour), "localhost")
		if err := ValidateStaticPair(cert.CertPath, other.KeyPath, ""); err == nil {
			t.Fatal("expected error for mismatched cert/key")
		}
	})

	t.Run("bad ca bundle", func(t *testing.T) {
		badCA := filepath.Join(dir, "bad-ca.pem")
		if err := os.WriteFile(badCA, []byte("not a pem"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateStaticPair(cert.CertPath, cert.KeyPath, badCA); err == nil {
			t.Fatal("expected error for invalid ca bundle")
		}
	})
}
