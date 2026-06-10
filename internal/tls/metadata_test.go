// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helper — generate a leaf → intermediate → root chain as PEM bytes
// ---------------------------------------------------------------------------

// generateTestChainPEM builds a three-cert chain (self-signed root CA, an
// intermediate CA signed by the root, and a leaf signed by the intermediate)
// and returns each as its own PEM block. Callers concatenate them in whatever
// order the test needs.
func generateTestChainPEM(t *testing.T) (leafPEM, interPEM, rootPEM []byte) {
	t.Helper()

	now := time.Now()
	mkKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generating key: %v", err)
		}
		return k
	}
	serial := func() *big.Int {
		s, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			t.Fatalf("generating serial: %v", err)
		}
		return s
	}
	toPEM := func(der []byte) []byte {
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}

	rootKey := mkKey()
	rootTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "root.example.com"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(72 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating root: %v", err)
	}
	rootCert, _ := x509.ParseCertificate(rootDER)

	interKey := mkKey()
	interTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "intermediate.example.com"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(48 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	interDER, err := x509.CreateCertificate(rand.Reader, interTmpl, rootCert, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating intermediate: %v", err)
	}
	interCert, _ := x509.ParseCertificate(interDER)

	leafKey := mkKey()
	leafTmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "leaf.example.com"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"leaf.example.com", "alt.example.com"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, interCert, &leafKey.PublicKey, interKey)
	if err != nil {
		t.Fatalf("creating leaf: %v", err)
	}

	return toPEM(leafDER), toPEM(interDER), toPEM(rootDER)
}

// ---------------------------------------------------------------------------
// ChainMetadataFromPEM — full per-cert chain with structural role
// ---------------------------------------------------------------------------

func TestChainMetadataFromPEM_OrderedChain(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	bundle := bytes.Join([][]byte{leafPEM, interPEM, rootPEM}, nil)

	chain, err := ChainMetadataFromPEM(bundle)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM = %v, want nil", err)
	}
	if len(chain) != 3 {
		t.Fatalf("len(chain) = %d, want 3", len(chain))
	}

	// Order preserved as supplied (leaf → intermediate → root); roles derived
	// structurally regardless of position.
	wantSubject := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	wantRole := []string{"leaf", "intermediate", "root"}
	for i, want := range wantSubject {
		if chain[i].Subject != want {
			t.Errorf("chain[%d].Subject = %q, want %q", i, chain[i].Subject, want)
		}
		if chain[i].Role != wantRole[i] {
			t.Errorf("chain[%d].Role = %q, want %q", i, chain[i].Role, wantRole[i])
		}
	}
	if len(chain[0].DNSNames) != 2 || chain[0].DNSNames[0] != "leaf.example.com" {
		t.Errorf("leaf DNSNames = %v, want [leaf.example.com alt.example.com]", chain[0].DNSNames)
	}
}

func TestChainMetadataFromPEM_RolesIndependentOfOrder(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	// Supply out of order: root, leaf, intermediate. Roles must still be correct
	// for each cert (display order = input order; W1-B handles reordering).
	bundle := bytes.Join([][]byte{rootPEM, leafPEM, interPEM}, nil)

	chain, err := ChainMetadataFromPEM(bundle)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM = %v, want nil", err)
	}
	got := map[string]string{}
	for _, c := range chain {
		got[c.Subject] = c.Role
	}
	want := map[string]string{
		"root.example.com":         "root",
		"leaf.example.com":         "leaf",
		"intermediate.example.com": "intermediate",
	}
	for subj, role := range want {
		if got[subj] != role {
			t.Errorf("role for %s = %q, want %q", subj, got[subj], role)
		}
	}
}

func TestChainMetadataFromPEM_SingleSelfSigned(t *testing.T) {
	certPEM, _, _ := generateTestCertPEM(t, "solo.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))

	chain, err := ChainMetadataFromPEM(certPEM)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM = %v, want nil", err)
	}
	if len(chain) != 1 {
		t.Fatalf("len(chain) = %d, want 1", len(chain))
	}
	// A lone self-signed cert (subject == issuer) is a root.
	if chain[0].Role != "root" {
		t.Errorf("Role = %q, want root", chain[0].Role)
	}
}

func TestChainMetadataFromPEM_SingleLeafExternalCA(t *testing.T) {
	leafPEM, _, _ := generateTestChainPEM(t)

	chain, err := ChainMetadataFromPEM(leafPEM)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM = %v, want nil", err)
	}
	if len(chain) != 1 {
		t.Fatalf("len(chain) = %d, want 1", len(chain))
	}
	// Leaf alone (issuer not in bundle, not self-signed) is a leaf.
	if chain[0].Role != "leaf" {
		t.Errorf("Role = %q, want leaf", chain[0].Role)
	}
}

func TestChainMetadataFromPEM_SkipsNonCertBlocks(t *testing.T) {
	leafPEM, interPEM, _ := generateTestChainPEM(t)
	// A private-key block interleaved must be ignored, never parsed as a cert.
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not-a-real-key")})
	bundle := bytes.Join([][]byte{leafPEM, keyBlock, interPEM}, nil)

	chain, err := ChainMetadataFromPEM(bundle)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM = %v, want nil", err)
	}
	if len(chain) != 2 {
		t.Fatalf("len(chain) = %d, want 2 (key block skipped)", len(chain))
	}
}

func TestChainMetadataFromPEM_Unparseable(t *testing.T) {
	if _, err := ChainMetadataFromPEM([]byte("garbage")); err == nil {
		t.Fatal("ChainMetadataFromPEM(garbage) = nil, want error")
	}
}

func TestChainMetadataFromPEM_Empty(t *testing.T) {
	if _, err := ChainMetadataFromPEM(nil); err == nil {
		t.Fatal("ChainMetadataFromPEM(nil) = nil, want error")
	}
}
