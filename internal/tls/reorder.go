// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// ChainIncompleteWarning is the non-fatal warning recorded when a submitted
// bundle cannot be linked into a single complete leaf → root chain (a missing
// intermediate, or unrelated certificates). It is operator-safe — it names no
// key material — and never blocks a save (tls-static.md § 2.2: warn and store,
// never reject).
const ChainIncompleteWarning = "certificate chain is incomplete or contains certificates that do not chain together; stored in best-effort order"

// ReorderChainPEM sorts the certificates in an operator-supplied static PEM
// bundle into leaf → intermediate(s) → root order before storing, by matching
// each certificate's issuer to the next certificate's subject (tls-static.md
// § 2.2). It does not trust the submitted order.
//
// The true (non-self-signed) leaf is placed first so it survives as cert[0],
// which the key-pair preflight (tls.X509KeyPair) matches the private key
// against — reordering runs before that preflight, so a valid-but-misordered
// bundle must not be left with a CA cert in front or it would be wrongly
// rejected.
//
// The chain is built greedily from the leaf, following issuer → subject links.
// A missing root is not an error — clients typically hold it — so a leaf (+
// intermediates) that simply stops short of a self-signed root reorders cleanly
// with no warning. If certificates remain that do not link into the chain (a
// missing intermediate, or unrelated certs), they are appended and a non-fatal
// warning is returned: the bundle is still stored, never rejected.
//
// Non-certificate PEM blocks (e.g. a stray key) are dropped — only certificate
// material is ever returned. An error is returned only when the bundle contains
// no parseable certificate at all; the caller's save-time preflight (§ 2.6)
// owns rejection of unparseable input.
func ReorderChainPEM(certPEM []byte) (reordered []byte, warning string, err error) {
	if len(certPEM) == 0 {
		return nil, "", errors.New("no certificate PEM provided")
	}

	type entry struct {
		block *pem.Block
		cert  *x509.Certificate
	}

	var entries []entry
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
		cert, perr := x509.ParseCertificate(block.Bytes)
		if perr != nil {
			return nil, "", fmt.Errorf("parsing certificate: %w", perr)
		}
		entries = append(entries, entry{block: block, cert: cert})
	}
	if len(entries) == 0 {
		return nil, "", errors.New("no PEM certificate block found")
	}

	selfSigned := func(c *x509.Certificate) bool {
		return bytes.Equal(c.RawSubject, c.RawIssuer)
	}
	// isIssuerOfAnother reports whether e's subject signs some other cert in the
	// bundle (i.e. e is a CA above another present cert), ignoring self-issuance.
	isIssuerOfAnother := func(e entry) bool {
		for _, other := range entries {
			if other.cert == e.cert {
				continue
			}
			if bytes.Equal(other.cert.RawIssuer, e.cert.RawSubject) {
				return true
			}
		}
		return false
	}

	// The leaf is the end-entity cert: not self-signed, and the issuer of no
	// other cert in the bundle. Prefer that strictly so a self-signed root that
	// also issues nothing present does not get mistaken for the leaf. Fall back
	// to the first entry only for a pathological bundle with no such cert.
	leafIdx := -1
	for i, e := range entries {
		if !selfSigned(e.cert) && !isIssuerOfAnother(e) {
			leafIdx = i
			break
		}
	}
	if leafIdx == -1 {
		leafIdx = 0
	}

	used := make([]bool, len(entries))
	ordered := make([]entry, 0, len(entries))
	for cur := leafIdx; cur != -1; {
		ordered = append(ordered, entries[cur])
		used[cur] = true
		c := entries[cur].cert
		if selfSigned(c) {
			break // a self-signed root terminates the chain
		}
		next := -1
		for i, e := range entries {
			if used[i] {
				continue
			}
			if bytes.Equal(e.cert.RawSubject, c.RawIssuer) {
				next = i
				break
			}
		}
		cur = next
	}

	// Any cert not linked into the chain means the bundle is not a single
	// complete chain (missing intermediate, unrelated certs). Keep it — append
	// in original order — but record the non-fatal warning.
	for i, e := range entries {
		if !used[i] {
			ordered = append(ordered, e)
			warning = ChainIncompleteWarning
		}
	}

	var buf bytes.Buffer
	for _, e := range ordered {
		if encErr := pem.Encode(&buf, e.block); encErr != nil {
			return nil, "", fmt.Errorf("encoding certificate: %w", encErr)
		}
	}
	return buf.Bytes(), warning, nil
}
