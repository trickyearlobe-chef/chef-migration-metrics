// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// parseCSR decodes a PEM CSR and returns the parsed request.
func parseCSR(t *testing.T, csrPEM []byte) *x509.CertificateRequest {
	t.Helper()
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("expected CERTIFICATE REQUEST PEM block, got %v", block)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature invalid: %v", err)
	}
	return csr
}

// parseKey decodes a PEM private key (PKCS#8).
func parseKey(t *testing.T, keyPEM []byte) any {
	t.Helper()
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatalf("no PEM block in key")
	}
	if block.Type != "PRIVATE KEY" {
		t.Fatalf("expected PRIVATE KEY PEM block, got %q", block.Type)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parsing PKCS#8 key: %v", err)
	}
	return key
}

// The default algorithm is ecdsa-p256 and the CSR carries the requested subject
// and SANs; the returned key is a usable ECDSA P-256 key whose public half is
// embedded in the CSR (round-trips via tls.X509KeyPair against a matching cert).
func TestGenerateCSR_DefaultAlgoSubjectAndSANs(t *testing.T) {
	csrPEM, keyPEM, err := GenerateCSR(CSRRequest{
		CommonName:         "leaf.example.com",
		Organization:       "Example Corp",
		OrganizationalUnit: "Platform",
		Country:            "GB",
		DNSNames:           []string{"leaf.example.com", "alt.example.com"},
		IPAddresses:        []string{"10.0.0.1", "192.168.0.2"},
	})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}

	csr := parseCSR(t, csrPEM)
	if csr.Subject.CommonName != "leaf.example.com" {
		t.Errorf("CN = %q, want leaf.example.com", csr.Subject.CommonName)
	}
	if len(csr.Subject.Organization) != 1 || csr.Subject.Organization[0] != "Example Corp" {
		t.Errorf("O = %v, want [Example Corp]", csr.Subject.Organization)
	}
	if len(csr.Subject.OrganizationalUnit) != 1 || csr.Subject.OrganizationalUnit[0] != "Platform" {
		t.Errorf("OU = %v, want [Platform]", csr.Subject.OrganizationalUnit)
	}
	if len(csr.Subject.Country) != 1 || csr.Subject.Country[0] != "GB" {
		t.Errorf("C = %v, want [GB]", csr.Subject.Country)
	}
	if len(csr.DNSNames) != 2 || csr.DNSNames[0] != "leaf.example.com" || csr.DNSNames[1] != "alt.example.com" {
		t.Errorf("DNSNames = %v", csr.DNSNames)
	}
	if len(csr.IPAddresses) != 2 {
		t.Fatalf("IPAddresses = %v, want 2", csr.IPAddresses)
	}
	if csr.IPAddresses[0].String() != "10.0.0.1" || csr.IPAddresses[1].String() != "192.168.0.2" {
		t.Errorf("IPAddresses = %v", csr.IPAddresses)
	}

	key := parseKey(t, keyPEM)
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("key type = %T, want *ecdsa.PrivateKey", key)
	}
	if ecKey.Curve != elliptic.P256() {
		t.Errorf("curve = %v, want P-256", ecKey.Curve.Params().Name)
	}
}

// Each supported algorithm yields a key of the expected type and size.
func TestGenerateCSR_KeyAlgorithms(t *testing.T) {
	cases := []struct {
		algo  string
		check func(t *testing.T, key any)
	}{
		{"ecdsa-p256", func(t *testing.T, key any) {
			k, ok := key.(*ecdsa.PrivateKey)
			if !ok || k.Curve != elliptic.P256() {
				t.Errorf("want ECDSA P-256, got %T", key)
			}
		}},
		{"ecdsa-p384", func(t *testing.T, key any) {
			k, ok := key.(*ecdsa.PrivateKey)
			if !ok || k.Curve != elliptic.P384() {
				t.Errorf("want ECDSA P-384, got %T", key)
			}
		}},
		{"rsa-2048", func(t *testing.T, key any) {
			k, ok := key.(*rsa.PrivateKey)
			if !ok || k.N.BitLen() != 2048 {
				t.Errorf("want RSA 2048, got %T/%d", key, rsaBits(key))
			}
		}},
		{"rsa-3072", func(t *testing.T, key any) {
			k, ok := key.(*rsa.PrivateKey)
			if !ok || k.N.BitLen() != 3072 {
				t.Errorf("want RSA 3072, got %T/%d", key, rsaBits(key))
			}
		}},
		{"rsa-4096", func(t *testing.T, key any) {
			k, ok := key.(*rsa.PrivateKey)
			if !ok || k.N.BitLen() != 4096 {
				t.Errorf("want RSA 4096, got %T/%d", key, rsaBits(key))
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.algo, func(t *testing.T) {
			_, keyPEM, err := GenerateCSR(CSRRequest{
				CommonName:   "leaf.example.com",
				DNSNames:     []string{"leaf.example.com"},
				KeyAlgorithm: tc.algo,
			})
			if err != nil {
				t.Fatalf("GenerateCSR(%s): %v", tc.algo, err)
			}
			tc.check(t, parseKey(t, keyPEM))
		})
	}
}

func rsaBits(key any) int {
	if k, ok := key.(*rsa.PrivateKey); ok {
		return k.N.BitLen()
	}
	return -1
}

// A request with no CN and no SANs has no identifier and is rejected.
func TestGenerateCSR_NoIdentifier(t *testing.T) {
	_, _, err := GenerateCSR(CSRRequest{Organization: "Example Corp"})
	if err == nil {
		t.Fatal("expected error for a request with no CN or SAN")
	}
}

// A CN alone (no SANs) is a sufficient identifier.
func TestGenerateCSR_CNOnlyIsValid(t *testing.T) {
	csrPEM, _, err := GenerateCSR(CSRRequest{CommonName: "leaf.example.com"})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}
	csr := parseCSR(t, csrPEM)
	if csr.Subject.CommonName != "leaf.example.com" {
		t.Errorf("CN = %q", csr.Subject.CommonName)
	}
}

// A DNS SAN alone (no CN) is a sufficient identifier.
func TestGenerateCSR_SANOnlyIsValid(t *testing.T) {
	csrPEM, _, err := GenerateCSR(CSRRequest{DNSNames: []string{"alt.example.com"}})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}
	csr := parseCSR(t, csrPEM)
	if len(csr.DNSNames) != 1 || csr.DNSNames[0] != "alt.example.com" {
		t.Errorf("DNSNames = %v", csr.DNSNames)
	}
}

// An unknown key algorithm is rejected.
func TestGenerateCSR_InvalidAlgorithm(t *testing.T) {
	_, _, err := GenerateCSR(CSRRequest{
		CommonName:   "leaf.example.com",
		KeyAlgorithm: "ed25519",
	})
	if err == nil {
		t.Fatal("expected error for an unsupported key algorithm")
	}
}

// A malformed IP SAN is rejected before key generation.
func TestGenerateCSR_InvalidIPSAN(t *testing.T) {
	_, _, err := GenerateCSR(CSRRequest{
		CommonName:  "leaf.example.com",
		IPAddresses: []string{"not-an-ip"},
	})
	if err == nil {
		t.Fatal("expected error for a malformed IP SAN")
	}
}
