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

// ---------------------------------------------------------------------------
// Shared test bodies
// ---------------------------------------------------------------------------

// Notifications disabled — no validation on channels.
const validNotificationsDisabledBody = `{"enabled":false}`

// Notifications enabled with one webhook channel.
const validNotificationsWebhookBody = `{
    "enabled": true,
    "channels": [{"name":"alerts","type":"webhook","url":"https://hooks.example.com/webhook","events":["collection_failure"]}]
}`

// Notifications enabled with one email channel.
const validNotificationsEmailBody = `{
    "enabled": true,
    "channels": [{"name":"email-alerts","type":"email","recipients":["admin@example.com"],"events":["readiness_milestone"]}]
}`

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/notifications
// ---------------------------------------------------------------------------

func TestAdminConfigNotifications_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Notifications = config.NotificationsConfig{Enabled: true, StaleNodeAlertCount: 5}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/notifications", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["enabled"] != true {
		t.Errorf("enabled = %v, want true", got["enabled"])
	}
	if got["stale_node_alert_count"] != float64(5) {
		t.Errorf("stale_node_alert_count = %v, want 5", got["stale_node_alert_count"])
	}
}

func TestAdminConfigNotifications_GET_NilStore(t *testing.T) {
	cfg := testConfig()
	cfg.Notifications = config.NotificationsConfig{Enabled: false}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/notifications", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigNotifications_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.Notifications = config.NotificationsConfig{Enabled: true}
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/notifications", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["enabled"] != true {
		t.Errorf("enabled = %v, want true", got["enabled"])
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/notifications
// ---------------------------------------------------------------------------

func TestAdminConfigNotifications_PUT_Success_Disabled(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(validNotificationsDisabledBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigNotifications_PUT_Success_Webhook(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(validNotificationsWebhookBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigNotifications_PUT_Success_Email(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(validNotificationsEmailBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigNotifications_PUT_DisabledSkipsValidation(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// Channel has no name — would be 422 if enabled, but disabled skips all validation.
	body := `{"enabled":false,"channels":[{"type":"webhook","url":"https://hooks.example.com"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigNotifications_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(validNotificationsDisabledBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigNotifications_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCode(t, w, ErrCodeBadRequest)
}

func TestAdminConfigNotifications_PUT_422_ChannelMissingName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[{"type":"webhook","url":"https://hooks.example.com/webhook"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_DuplicateChannelName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[
		{"name":"alerts","type":"webhook","url":"https://hooks.example.com/a"},
		{"name":"alerts","type":"webhook","url":"https://hooks.example.com/b"}
	]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_WebhookMissingURL(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[{"name":"alerts","type":"webhook"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_EmailMissingRecipients(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[{"name":"email-alerts","type":"email","recipients":[]}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_UnknownChannelType(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[{"name":"alerts","type":"slack","url":"https://hooks.example.com/webhook"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_UnknownEvent(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"channels":[{"name":"alerts","type":"webhook","url":"https://hooks.example.com/webhook","events":["unknown_event"]}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_MilestoneOutOfRange(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"readiness_milestones":[101]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_422_NegativeMilestone(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"enabled":true,"readiness_milestones":[-1]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/notifications", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigNotifications_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/notifications", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}
