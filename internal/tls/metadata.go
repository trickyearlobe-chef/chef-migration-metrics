// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// CertMetadata is the operator-safe view of one certificate in an installed
// bundle. It carries only public certificate fields — subject, issuer, SANs,
// validity window, and the structural chain role — so the admin UI can show
// what is installed (especially for cert_source: db) without the private key
// ever leaving the server.
type CertMetadata struct {
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	DNSNames    []string  `json:"dns_names,omitempty"`
	IPAddresses []string  `json:"ip_addresses,omitempty"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	// Role is the cert's position in the chain, derived structurally (not from
	// bundle order): "leaf", "intermediate", or "root".2.
	Role string `json:"role"`
}

// ChainMetadataFromPEM parses every certificate in a PEM bundle and returns
// operator-safe metadata for each, in the order supplied, with a structurally
// derived chain role. Non-certificate blocks (e.g. a
// private key) are skipped — key material is never parsed or returned. The
// returned order reflects the bundle as stored; reordering is a separate
// save-time concern (W1-B). An error is returned only when the bundle contains
// no parseable certificate at all.
func ChainMetadataFromPEM(certPEM []byte) ([]CertMetadata, error) {
	if len(certPEM) == 0 {
		return nil, errors.New("no certificate PEM provided")
	}

	var certs []*x509.Certificate
	rest := certPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("no PEM certificate block found")
	}

	chain := make([]CertMetadata, 0, len(certs))
	for _, cert := range certs {
		meta := metaFromCert(cert)
		meta.Role = deriveRole(cert, certs)
		chain = append(chain, meta)
	}
	return chain, nil
}

// metaFromCert extracts the operator-safe public fields of a single certificate.
// It never reads or returns private key material. Role is left unset; the caller
// derives it from the full bundle.
func metaFromCert(cert *x509.Certificate) CertMetadata {
	meta := CertMetadata{
		Subject:   cert.Subject.CommonName,
		Issuer:    cert.Issuer.CommonName,
		DNSNames:  cert.DNSNames,
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
	}
	if len(cert.IPAddresses) > 0 {
		ips := make([]string, 0, len(cert.IPAddresses))
		for _, ip := range cert.IPAddresses {
			ips = append(ips, ip.String())
		}
		meta.IPAddresses = ips
	}
	return meta
}

// deriveRole classifies a certificate's position in the bundle structurally,
// not by its order:
//   - root: self-signed (subject == issuer);
//   - leaf: its subject does not issue any other cert in the bundle;
//   - intermediate: it signs another cert but is not self-signed.
//
// Matching uses the raw distinguished names so it does not depend on the CN
// alone. Unrelated certs in a malformed bundle simply each classify as leaf.
func deriveRole(cert *x509.Certificate, bundle []*x509.Certificate) string {
	if bytes.Equal(cert.RawSubject, cert.RawIssuer) {
		return "root"
	}
	for _, other := range bundle {
		if other == cert {
			continue
		}
		if bytes.Equal(other.RawIssuer, cert.RawSubject) {
			return "intermediate"
		}
	}
	return "leaf"
}
