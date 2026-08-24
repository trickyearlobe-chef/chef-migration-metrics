// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// genChainPEM builds a leaf → intermediate → root chain and returns each cert
// as its own PEM string plus the leaf's private-key PEM, so a test can submit a
// misordered or incomplete bundle through the cert_source: db save path.
func genChainPEM(t *testing.T) (leafPEM, interPEM, rootPEM, leafKeyPEM string) {
	t.Helper()
	now := time.Now()
	mkKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("key: %v", err)
		}
		return k
	}
	serial := func() *big.Int { return big.NewInt(time.Now().UnixNano()) }
	toPEM := func(der []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}

	rootKey := mkKey()
	rootTmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: "root.example.com"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(72 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	rootCert, _ := x509.ParseCertificate(rootDER)

	interKey := mkKey()
	interTmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: "intermediate.example.com"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
	}
	interDER, err := x509.CreateCertificate(rand.Reader, interTmpl, rootCert, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("intermediate: %v", err)
	}
	interCert, _ := x509.ParseCertificate(interDER)

	leafKey := mkKey()
	leafTmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: "leaf.example.com"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"leaf.example.com"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, interCert, &leafKey.PublicKey, interKey)
	if err != nil {
		t.Fatalf("leaf: %v", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}))
	return toPEM(leafDER), toPEM(interDER), toPEM(rootDER), leafKeyPEM
}

// chainSubjects fetches the stored db certificate and returns its per-cert
// subjects in stored order, via the public chain-metadata surface.
func storedChainSubjects(t *testing.T, store *configstore.Store) []string {
	t.Helper()
	raw, err := store.Get(context.Background(), configstore.KeyServerTLSCertificate)
	if err != nil {
		t.Fatalf("store.Get certificate: %v", err)
	}
	var pemStr string
	if err := json.Unmarshal(raw, &pemStr); err != nil {
		t.Fatalf("unmarshal stored cert: %v", err)
	}
	var subs []string
	rest := []byte(pemStr)
	for {
		block, r := pem.Decode(rest)
		rest = r
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("parse stored cert: %v", err)
		}
		subs = append(subs, c.Subject.CommonName)
	}
	return subs
}

// A misordered but complete bundle (root → intermediate → leaf) is reordered to
// leaf → intermediate → root before storage, with no warning.
// The leaf must land first so the key still matches cert[0] at preflight.
func TestAdminConfigServer_PUT_DBCert_ReordersChain(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	leafPEM, interPEM, rootPEM, leafKeyPEM := genChainPEM(t)
	bundle := rootPEM + interPEM + leafPEM // deliberately reversed

	w := putServer(r, dbCertBody(t, bundle, leafKeyPEM))
	assertStatus(t, w, http.StatusOK)

	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if len(resp.Warnings) != 0 {
		t.Errorf("warnings = %v, want none for a complete chain", resp.Warnings)
	}

	got := storedChainSubjects(t, store)
	want := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	if len(got) != len(want) {
		t.Fatalf("stored subjects = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stored[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// An incomplete bundle (leaf + root, missing intermediate) is stored anyway with
// the leaf first and a non-fatal warning in the response — never rejected.
func TestAdminConfigServer_PUT_DBCert_IncompleteChainWarns(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	leafPEM, _, rootPEM, leafKeyPEM := genChainPEM(t)
	bundle := rootPEM + leafPEM // intermediate omitted

	w := putServer(r, dbCertBody(t, bundle, leafKeyPEM))
	assertStatus(t, w, http.StatusOK)

	var resp putConfigResponse
	decodeBody(t, w, &resp)
	if len(resp.Warnings) == 0 {
		t.Fatal("expected a non-fatal warning for an incomplete chain")
	}

	got := storedChainSubjects(t, store)
	if len(got) != 2 || got[0] != "leaf.example.com" {
		t.Errorf("stored subjects = %v, want leaf first, both preserved", got)
	}
}

// A CSR-promoted bundle (cert submitted with no key) is NOT reordered — the
// pending-key path is left as-is per spec. Here we only assert the reorder path
// does not interfere: a complete in-order chain promotes cleanly.
func TestAdminConfigServer_PUT_DBCert_ReorderSkipsCSRPromote(t *testing.T) {
	store := newTestConfigStore(t)
	cfg := testConfig()
	cfg.Server.TLS = config.TLSConfig{Mode: "static", CertSource: "db"}
	r := newTestRouterForAdminConfig(cfg, store, nil)

	// Seed a pending CSR key by storing it directly, then submit a matching cert.
	leafPEM, interPEM, _, leafKeyPEM := genChainPEM(t)
	keyJSON, _ := json.Marshal(leafKeyPEM)
	if err := store.Set(context.Background(), configstore.KeyServerTLSPrivateKeyPending, keyJSON, true, "test"); err != nil {
		t.Fatalf("seed pending key: %v", err)
	}

	// Cert only (in-order leaf+intermediate), no key → match-and-promote path.
	bundle := leafPEM + interPEM
	w := putServer(r, dbCertBody(t, bundle, ""))
	assertStatus(t, w, http.StatusOK)

	if got := storedChainSubjects(t, store); len(got) == 0 || got[0] != "leaf.example.com" {
		t.Errorf("promoted bundle subjects = %v, want leaf first", got)
	}
}
