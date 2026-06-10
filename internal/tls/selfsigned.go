// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// selfSignedValidity is how long an ephemeral self-signed fallback cert is valid.
// The cert is regenerated on every boot and exists only to keep the degraded
// recovery UI on HTTPS, so the exact window is not security-relevant; a year is
// comfortably longer than any degraded session.
const selfSignedValidity = 365 * 24 * time.Hour

// GenerateSelfSigned builds an ephemeral self-signed certificate/key pair used as
// the degraded fail-open listener (tls-static.md § 2.4, tls-acme.md § 3.11). It
// keeps the admin/recovery UI on an encrypted channel (the operator's browser
// shows an untrusted-cert warning and proceeds) rather than dropping to cleartext
// HTTP.
//
// hosts are the names the cert should cover: DNS names and IP literals are sorted
// into SANs automatically. When hosts is empty it defaults to localhost / the
// loopback addresses so the recovery UI is at least reachable locally. The key is
// a fresh ECDSA P-256 key (PKCS#8 PEM); nothing is persisted — the pair lives only
// for the lifetime of the process.
func GenerateSelfSigned(hosts []string) (certPEM, keyPEM []byte, err error) {
	key, err := generateKey(DefaultKeyAlgorithm)
	if err != nil {
		return nil, nil, err
	}

	dnsNames, ips := splitHosts(hosts)
	if len(dnsNames) == 0 && len(ips) == 0 {
		dnsNames = []string{"localhost"}
		ips = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generating serial number: %w", err)
	}

	cn := "chef-migration-metrics (self-signed)"
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-time.Hour), // small backdate for clock skew
		NotAfter:              now.Add(selfSignedValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, nil, fmt.Errorf("creating self-signed certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling private key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// splitHosts sorts host entries into DNS names and IP addresses. Entries that
// parse as IPs become IP SANs; everything else is treated as a DNS name. Empty
// entries are skipped.
func splitHosts(hosts []string) (dnsNames []string, ips []net.IP) {
	for _, h := range hosts {
		if h == "" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
			continue
		}
		dnsNames = append(dnsNames, h)
	}
	return dnsNames, ips
}
