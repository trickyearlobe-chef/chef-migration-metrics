// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
)

// generateCSRRequest is the JSON body for the generate-csr endpoint (tls-csr.md
// § 4.3). All fields except an identifier (common_name or a SAN) are optional.
type generateCSRRequest struct {
	CommonName         string   `json:"common_name"`
	Organization       string   `json:"organization"`
	OrganizationalUnit string   `json:"organizational_unit"`
	Country            string   `json:"country"`
	DNSSANs            []string `json:"dns_sans"`
	IPSANs             []string `json:"ip_sans"`
	KeyAlgorithm       string   `json:"key_algorithm"`
}

type generateCSRResponse struct {
	CSRPEM       string `json:"csr_pem"`
	KeyAlgorithm string `json:"key_algorithm"`
}

// handleAdminConfigServerGenerateCSR generates a keypair and a CSR for static
// (cert_source: db) certificate issuance. The private key is stored as a pending
// secret (server.tls.private_key.pending) and the CSR PEM is returned for the
// operator to submit to their CA. The key is never returned (tls-csr.md § 4).
func (r *Router) handleAdminConfigServerGenerateCSR(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports POST.")
		return
	}
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		WriteBadRequest(w, "Failed to read request body.")
		return
	}
	var in generateCSRRequest
	if err := json.Unmarshal(body, &in); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// GenerateCSR validates the inputs (identifier present, key algorithm
	// supported, IP SANs parseable) and returns operator-safe errors that never
	// contain key material — surface them as a 422 so the operator can correct
	// the request.
	csrPEM, keyPEM, err := apptls.GenerateCSR(apptls.CSRRequest{
		CommonName:         in.CommonName,
		Organization:       in.Organization,
		OrganizationalUnit: in.OrganizationalUnit,
		Country:            in.Country,
		DNSNames:           in.DNSSANs,
		IPAddresses:        in.IPSANs,
		KeyAlgorithm:       in.KeyAlgorithm,
	})
	if err != nil {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"generate-csr: "+err.Error())
		return
	}

	// Persist the new private key as pending (secret, encrypted at rest),
	// overwriting any prior pending key (tls-csr.md § 4.5). The active cert/key
	// are untouched, so the listener keeps serving the current certificate.
	keyJSON, _ := json.Marshal(string(keyPEM))
	if err := r.configStore.Set(req.Context(), configstore.KeyServerTLSPrivateKeyPending, keyJSON, true, "admin"); err != nil {
		r.logf("ERROR", "admin/config/server/generate-csr: store pending key: %v", err)
		WriteInternalError(w, "Failed to store the pending private key.")
		return
	}

	algo := in.KeyAlgorithm
	if algo == "" {
		algo = apptls.DefaultKeyAlgorithm
	}
	r.logf("INFO", "admin/config/server/generate-csr: generated CSR (cn=%q, algo=%s) by %s",
		in.CommonName, algo, adminUsername(req))

	WriteJSON(w, http.StatusOK, generateCSRResponse{
		CSRPEM:       string(csrPEM),
		KeyAlgorithm: algo,
	})
}
