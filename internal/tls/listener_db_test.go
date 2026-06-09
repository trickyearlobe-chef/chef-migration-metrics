// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"net/http"
	"testing"
	"time"
)

// NewListener with CertSource: "db" builds its CertManager from in-memory PEM
// bytes (the encrypted config-store path) rather than files on disk, and that
// CertManager supports in-place ReloadFromPEM.
func TestNewListener_DBCertSource(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCertPEM(t, "db.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	l, err := NewListener(handler, ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          0,
		CertSource:    "db",
		CertPEM:       certPEM,
		KeyPEM:        keyPEM,
		MinVersion:    "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("NewListener(db) = %v, want nil", err)
	}

	if l.CertManager().CertPath() != "" {
		t.Errorf("CertPath() = %q, want empty for db source", l.CertManager().CertPath())
	}

	// The DB source supports config-change-driven reload.
	newCert, newKey, _ := generateTestCertPEM(t, "rotated.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	if err := l.CertManager().ReloadFromPEM(newCert, newKey); err != nil {
		t.Fatalf("ReloadFromPEM = %v, want nil", err)
	}
	if got := l.CertManager().LeafCert().Subject.CommonName; got != "rotated.example.com" {
		t.Errorf("after reload CN = %q, want rotated.example.com", got)
	}
}

// A db CertSource with an invalid pair fails listener construction (caller
// fails open to plain HTTP).
func TestNewListener_DBCertSource_BadPair(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	_, err := NewListener(handler, ListenerConfig{
		ListenAddress: "127.0.0.1",
		CertSource:    "db",
		CertPEM:       []byte("not a cert"),
		KeyPEM:        []byte("not a key"),
	}, nil)
	if err == nil {
		t.Fatal("NewListener(db, bad pair) = nil, want error")
	}
}
