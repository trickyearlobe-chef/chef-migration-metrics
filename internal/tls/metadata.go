// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// CertMetadata is the operator-safe view of an installed certificate. It
// carries only public certificate fields — subject, issuer, SANs, and validity
// window — so the admin UI can show what is installed (especially for
// cert_source: db) without the private key ever leaving the server.
type CertMetadata struct {
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	DNSNames    []string  `json:"dns_names,omitempty"`
	IPAddresses []string  `json:"ip_addresses,omitempty"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
}

// CertMetadataFromPEM parses the leaf (first) certificate from a PEM chain and
// returns its operator-safe metadata. It never returns or inspects private key
// material. An error is returned if the PEM contains no parseable certificate.
func CertMetadataFromPEM(certPEM []byte) (*CertMetadata, error) {
	if len(certPEM) == 0 {
		return nil, errors.New("no certificate PEM provided")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("no PEM certificate block found")
	}

	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	ips := make([]string, 0, len(leaf.IPAddresses))
	for _, ip := range leaf.IPAddresses {
		ips = append(ips, ip.String())
	}

	meta := &CertMetadata{
		Subject:   leaf.Subject.CommonName,
		Issuer:    leaf.Issuer.CommonName,
		DNSNames:  leaf.DNSNames,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
	}
	if len(ips) > 0 {
		meta.IPAddresses = ips
	}
	return meta, nil
}
