// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

const validAuthBodyLocal = `{"providers":[{"type":"local"}],"session_expiry":"24h","min_password_length":12}`
const validAuthBodySAML = `{"providers":[{"type":"saml","idp_metadata_url":"https://idp.example.com/metadata","sp_entity_id":"https://app.example.com","sp_private_key_credential":"saml-sp-key","sp_certificate_credential":"saml-sp-cert"}]}`
const validAuthBodyEmpty = `{"providers":[]}`

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/auth
// ---------------------------------------------------------------------------

func TestAdminConfigAuth_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.AuthConfig{Providers: []config.AuthProvider{{Type: "local"}}}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/auth", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	providers, ok := got["providers"].([]any)
	if !ok || providers == nil {
		t.Fatalf("providers is not a slice: %v", got["providers"])
	}
	if len(providers) != 1 {
		t.Errorf("want 1 provider, got %d", len(providers))
	}
}

func TestAdminConfigAuth_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/auth", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigAuth_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.AuthConfig{
		Providers: []config.AuthProvider{{
			Type:                   "saml",
			IDPMetadataURL:         "https://idp.example.com/metadata",
			SPEntityID:             "https://app.example.com",
			SPPrivateKeyCredential: "saml-sp-key",
			SPCertificateCredential: "saml-sp-cert",
		}},
	}
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/auth", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	providers, ok := got["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatalf("expected providers in response: %v", got["providers"])
	}
	first, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("provider[0] is not an object: %v", providers[0])
	}
	if first["type"] != "saml" {
		t.Errorf("provider type = %v, want \"saml\"", first["type"])
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/auth
// ---------------------------------------------------------------------------

func TestAdminConfigAuth_PUT_Success_Local(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(validAuthBodyLocal))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	restartRequired := decodePutValue(t, w, &got)
	providers, ok := got["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatalf("expected providers in response: %v", got["providers"])
	}
	first, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("provider[0] is not an object: %v", providers[0])
	}
	if first["type"] != "local" {
		t.Errorf("provider type = %v, want \"local\"", first["type"])
	}
	if !restartRequired {
		t.Error("auth PUT should set restart_required = true")
	}
}

func TestAdminConfigAuth_PUT_Success_SAML(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(validAuthBodySAML))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigAuth_PUT_Success_EmptyProviders(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(validAuthBodyEmpty))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigAuth_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(validAuthBodyLocal))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestAdminConfigAuth_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigAuth_PUT_422_SAMLMissingIDPURL(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"providers":[{"type":"saml","sp_entity_id":"https://app.example.com"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
}

func TestAdminConfigAuth_PUT_422_SAMLMissingSPEntityID(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"providers":[{"type":"saml","idp_metadata_url":"https://idp.example.com/metadata"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
}

func TestAdminConfigAuth_PUT_422_UnknownProviderType(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"providers":[{"type":"kerberos"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/auth", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAuth_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/auth", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}
