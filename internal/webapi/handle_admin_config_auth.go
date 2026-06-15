// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/auth
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigAuth(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Auth: cfg.Auth}, configstore.KeyAuth)
	case http.MethodPut:
		r.putAdminConfigAuth(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigAuth(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.AuthConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	for i, p := range input.Providers {
		prefix := fmt.Sprintf("auth.providers[%d]", i)
		switch p.Type {
		case "local":
			// no additional fields required
		case "saml":
			sources := 0
			if p.IDPMetadataURL != "" {
				sources++
			}
			if p.IDPMetadataPath != "" {
				sources++
			}
			if p.IDPMetadataXML != "" {
				sources++
			}
			if sources == 0 {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: idp_metadata_url, idp_metadata_path, or idp_metadata_xml is required for saml provider", prefix))
				return
			}
			if sources > 1 {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: idp_metadata_url, idp_metadata_path, and idp_metadata_xml are mutually exclusive (set exactly one)", prefix))
				return
			}
			if p.SPEntityID == "" {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: sp_entity_id is required for saml provider", prefix))
				return
			}
			if p.SPBaseURL != "" && !config.IsValidSPBaseURL(p.SPBaseURL) {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: sp_base_url must be an absolute http(s) URL with no path (e.g. https://cmm.example.com)", prefix))
				return
			}
			if p.SPPrivateKeyCredential == "" {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: sp_private_key_credential is required for saml provider", prefix))
				return
			}
			if p.SPCertificateCredential == "" {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: sp_certificate_credential is required for saml provider", prefix))
				return
			}
			// Validate role_mapping values.
			validRoles := map[string]bool{"viewer": true, "operator": true, "admin": true}
			for group, role := range p.RoleMapping {
				if !validRoles[role] {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("%s: role_mapping[%q] has invalid role %q (expected viewer, operator, or admin)", prefix, group, role))
					return
				}
			}
		default:
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: unknown provider type %q (expected local or saml).", prefix, p.Type))
			return
		}
	}

	// session_expiry / lockout_attempts / min_password_length are read live at
	// point of use (applied). The SAML provider is rebuilt in place when a
	// reconciler is wired (subsystem); without one (no SAML handler) the section
	// is still fully live via the applied reads. Worst granularity decides the
	// flag — restart_required stays false either way.
	appliers := []Applier{appliedApplier}
	if r.samlReconciler != nil {
		appliers = append(appliers, r.samlApplier())
	}
	r.storeAdminConfigSection(w, req, &config.Config{Auth: input}, configstore.KeyAuth, appliers...)
}
