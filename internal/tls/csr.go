// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
)

// CSRRequest describes a Certificate Signing Request to generate: the subject
// fields, Subject Alternative Names, and the keypair algorithm. It is the
// operator-supplied input to GenerateCSR.
type CSRRequest struct {
	CommonName         string
	Organization       string
	OrganizationalUnit string
	Country            string
	DNSNames           []string
	IPAddresses        []string
	// KeyAlgorithm selects the keypair algorithm. Empty
	// defaults to ecdsa-p256.
	KeyAlgorithm string
}

// DefaultKeyAlgorithm is used when CSRRequest.KeyAlgorithm is empty.
const DefaultKeyAlgorithm = "ecdsa-p256"

// GenerateCSR generates a fresh private key for the requested algorithm and
// builds a PKCS#10 Certificate Signing Request over the supplied subject and
// SANs. It returns the CSR and the private key, both PEM-encoded. The private
// key is PKCS#8 ("PRIVATE KEY") so tls.X509KeyPair accepts it for any algorithm.
//
// The request must carry at least one identifier — a CommonName or a DNS/IP SAN
// — otherwise the CSR would identify nothing. Malformed IP SANs and unsupported
// key algorithms are rejected before any key is generated. The returned error
// is safe to surface to an operator; it never contains key material.
func GenerateCSR(req CSRRequest) (csrPEM, keyPEM []byte, err error) {
	if req.CommonName == "" && len(req.DNSNames) == 0 && len(req.IPAddresses) == 0 {
		return nil, nil, fmt.Errorf("a common_name or at least one SAN is required")
	}

	ips := make([]net.IP, 0, len(req.IPAddresses))
	for _, s := range req.IPAddresses {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, nil, fmt.Errorf("invalid IP SAN %q", s)
		}
		ips = append(ips, ip)
	}

	key, err := generateKey(req.KeyAlgorithm)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: req.CommonName,
		},
		DNSNames:    req.DNSNames,
		IPAddresses: ips,
	}
	if req.Organization != "" {
		tmpl.Subject.Organization = []string{req.Organization}
	}
	if req.OrganizationalUnit != "" {
		tmpl.Subject.OrganizationalUnit = []string{req.OrganizationalUnit}
	}
	if req.Country != "" {
		tmpl.Subject.Country = []string{req.Country}
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, nil, fmt.Errorf("creating CSR: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling private key: %w", err)
	}

	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return csrPEM, keyPEM, nil
}

// generateKey produces a private key for the named algorithm.
// An empty name defaults to ecdsa-p256.
func generateKey(algo string) (crypto.Signer, error) {
	switch algo {
	case "", "ecdsa-p256":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ecdsa-p384":
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "rsa-2048":
		return rsa.GenerateKey(rand.Reader, 2048)
	case "rsa-3072":
		return rsa.GenerateKey(rand.Reader, 3072)
	case "rsa-4096":
		return rsa.GenerateKey(rand.Reader, 4096)
	default:
		return nil, fmt.Errorf("unsupported key_algorithm %q (want ecdsa-p256, ecdsa-p384, rsa-2048, rsa-3072, or rsa-4096)", algo)
	}
}
