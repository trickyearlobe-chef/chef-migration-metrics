// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

const (
	samlSPCertCredentialName = "saml-sp-cert"
	samlSPKeyCredentialName  = "saml-sp-key"
	samlSPCertType           = "generic"
	samlKeyBits              = 2048
	samlCertValidityYears    = 10
)

// handleSAMLGenerateKeypair generates a new SP signing keypair, stores it in
// the credential store, and returns the certificate PEM for copy/paste into
// the IdP configuration.
func (r *Router) handleSAMLGenerateKeypair(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "POST only")
		return
	}
	if r.credentialStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Credential storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	// Generate RSA keypair.
	privKey, err := rsa.GenerateKey(rand.Reader, samlKeyBits)
	if err != nil {
		r.logf("ERROR", "saml: generating RSA key: %v", err)
		WriteInternalError(w, "Failed to generate keypair.")
		return
	}

	// Determine CN from config SP entity ID (fall back to generic).
	cn := "chef-migration-metrics-sp"
	for _, p := range r.cfg.Auth.Providers {
		if p.Type == "saml" && p.SPEntityID != "" {
			cn = p.SPEntityID
			break
		}
	}

	// Create self-signed certificate.
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		r.logf("ERROR", "saml: generating serial: %v", err)
		WriteInternalError(w, "Failed to generate keypair.")
		return
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(samlCertValidityYears, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		r.logf("ERROR", "saml: creating certificate: %v", err)
		WriteInternalError(w, "Failed to generate certificate.")
		return
	}

	// Encode to PEM.
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER := x509.MarshalPKCS1PrivateKey(privKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})

	// Store in credential store (create or update).
	ctx := req.Context()
	username := adminUsername(req)

	if err := r.upsertCredential(ctx, samlSPCertCredentialName, certPEM, username); err != nil {
		r.logf("ERROR", "saml: storing SP certificate: %v", err)
		WriteInternalError(w, "Failed to store SP certificate.")
		return
	}
	if err := r.upsertCredential(ctx, samlSPKeyCredentialName, keyPEM, username); err != nil {
		r.logf("ERROR", "saml: storing SP private key: %v", err)
		WriteInternalError(w, "Failed to store SP private key.")
		return
	}

	// Compute fingerprint.
	fingerprint := sha256.Sum256(certDER)

	r.logf("INFO", "saml: generated new SP keypair by %s (fingerprint: %x)",
		username, fingerprint[:8])

	type response struct {
		CertificatePEM    string `json:"certificate_pem"`
		FingerprintSHA256 string `json:"fingerprint_sha256"`
		NotAfter          string `json:"not_after"`
	}

	WriteJSON(w, http.StatusOK, response{
		CertificatePEM:    string(certPEM),
		FingerprintSHA256: fmt.Sprintf("%x", fingerprint),
		NotAfter:          tmpl.NotAfter.Format(time.RFC3339),
	})
}

// handleSAMLGetCertificate retrieves the current SP certificate from the
// credential store and returns the PEM + metadata.
func (r *Router) handleSAMLGetCertificate(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "GET only")
		return
	}
	if r.credResolver == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Credential storage is not configured.")
		return
	}

	rc, err := r.credResolver.Resolve(req.Context(), secrets.CredentialSource{
		CredentialName: samlSPCertCredentialName,
	})
	if err != nil {
		// Treat any resolve failure as "not found" for this endpoint.
		WriteNotFound(w, "No SP certificate has been generated yet.")
		return
	}
	defer secrets.ZeroBytes(rc.Plaintext)

	// Parse the PEM to extract metadata.
	block, _ := pem.Decode(rc.Plaintext)
	if block == nil {
		WriteInternalError(w, "Stored SP certificate is malformed.")
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		WriteInternalError(w, "Failed to parse stored SP certificate.")
		return
	}

	fingerprint := sha256.Sum256(cert.Raw)

	type response struct {
		CertificatePEM    string `json:"certificate_pem"`
		FingerprintSHA256 string `json:"fingerprint_sha256"`
		NotAfter          string `json:"not_after"`
		Subject           string `json:"subject"`
	}

	WriteJSON(w, http.StatusOK, response{
		CertificatePEM:    string(rc.Plaintext),
		FingerprintSHA256: fmt.Sprintf("%x", fingerprint),
		NotAfter:          cert.NotAfter.Format(time.RFC3339),
		Subject:           cert.Subject.CommonName,
	})
}

// upsertCredential creates or updates a credential in the store.
func (r *Router) upsertCredential(ctx context.Context, name string, value []byte, username string) error {
	// Try create first, fall back to update.
	input := secrets.CreateCredentialInput{
		Name:           name,
		CredentialType: samlSPCertType,
		Plaintext:      value,
		CreatedBy:      username,
	}
	_, err := r.credentialStore.Create(ctx, input)
	if err == nil {
		return nil
	}
	if !errors.Is(err, secrets.ErrCredentialAlreadyExists) {
		return err
	}

	// Already exists — update.
	update := secrets.UpdateCredentialInput{
		Name:      name,
		Plaintext: value,
		UpdatedBy: username,
	}
	_, updateErr := r.credentialStore.Update(ctx, update)
	return updateErr
}
