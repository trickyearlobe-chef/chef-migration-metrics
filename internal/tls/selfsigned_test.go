// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"
)

// parseSelfSignedLeaf is a test helper: decode the leaf certificate from the
// returned PEM chain.
func parseSelfSignedLeaf(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no PEM block in certificate output")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return leaf
}

func TestGenerateSelfSigned_LoadablePair(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	// The pair must be usable by the TLS stack exactly like any other cert/key.
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("X509KeyPair rejected the self-signed pair: %v", err)
	}
}

func TestGenerateSelfSigned_IsSelfSignedServerCert(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	leaf := parseSelfSignedLeaf(t, certPEM)

	if leaf.Subject.String() != leaf.Issuer.String() {
		t.Errorf("expected self-signed (subject==issuer); subject=%q issuer=%q",
			leaf.Subject, leaf.Issuer)
	}
	// Must be a server auth cert so browsers/clients accept it for TLS (modulo
	// the untrusted-root warning).
	hasServerAuth := false
	for _, eku := range leaf.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasServerAuth {
		t.Error("expected ExtKeyUsageServerAuth")
	}
	if leaf.IsCA {
		t.Error("ephemeral leaf should not be a CA")
	}
	// Verify it actually self-verifies (signature chains to itself).
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "metrics.example.com",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Errorf("self-signed cert failed to verify against itself: %v", err)
	}
}

func TestGenerateSelfSigned_SplitsDNSAndIPHosts(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"a.example.com", "10.0.0.1", "b.example.com", "::1"})
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	leaf := parseSelfSignedLeaf(t, certPEM)

	wantDNS := map[string]bool{"a.example.com": true, "b.example.com": true}
	if len(leaf.DNSNames) != len(wantDNS) {
		t.Fatalf("DNSNames = %v, want keys of %v", leaf.DNSNames, wantDNS)
	}
	for _, d := range leaf.DNSNames {
		if !wantDNS[d] {
			t.Errorf("unexpected DNS name %q", d)
		}
	}
	if len(leaf.IPAddresses) != 2 {
		t.Fatalf("IPAddresses = %v, want 2", leaf.IPAddresses)
	}
	if !leaf.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("IPAddresses[0] = %v, want 10.0.0.1", leaf.IPAddresses[0])
	}
}

func TestGenerateSelfSigned_DefaultsWhenNoHosts(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned(nil)
	if err != nil {
		t.Fatalf("GenerateSelfSigned(nil): %v", err)
	}
	leaf := parseSelfSignedLeaf(t, certPEM)

	// With no configured hostnames it must still serve locally, so the operator
	// can reach the recovery UI on localhost.
	hasLocalhost := false
	for _, d := range leaf.DNSNames {
		if d == "localhost" {
			hasLocalhost = true
		}
	}
	if !hasLocalhost {
		t.Errorf("expected localhost in DNSNames, got %v", leaf.DNSNames)
	}
	hasLoopback := false
	for _, ip := range leaf.IPAddresses {
		if ip.IsLoopback() {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Errorf("expected a loopback IP SAN, got %v", leaf.IPAddresses)
	}
}

func TestGenerateSelfSigned_ValidityWindow(t *testing.T) {
	before := time.Now()
	certPEM, _, err := GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	leaf := parseSelfSignedLeaf(t, certPEM)

	if !leaf.NotBefore.Before(before.Add(time.Minute)) {
		t.Errorf("NotBefore %s should be at/before now", leaf.NotBefore)
	}
	if !leaf.NotAfter.After(before) {
		t.Errorf("NotAfter %s should be in the future", leaf.NotAfter)
	}
}

func TestGenerateSelfSigned_EphemeralEachCall(t *testing.T) {
	c1, k1, err := GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	c2, k2, err := GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	// Distinct keys and serials each call — nothing persistent, nothing reused.
	if string(k1) == string(k2) {
		t.Error("expected a fresh private key per call")
	}
	l1 := parseSelfSignedLeaf(t, c1)
	l2 := parseSelfSignedLeaf(t, c2)
	if l1.SerialNumber.Cmp(l2.SerialNumber) == 0 {
		t.Error("expected a distinct serial number per call")
	}
}
