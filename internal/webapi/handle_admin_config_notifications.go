// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// validNotificationEvents is the set of recognised event type names.
var validNotificationEvents = map[string]bool{
	"cookbook_status_change":        true,
	"readiness_milestone":           true,
	"new_incompatible_cookbook":     true,
	"collection_failure":            true,
	"stale_node_threshold_exceeded": true,
	"certificate_expiry_warning":    true,
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/notifications
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigNotifications(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Notifications: cfg.Notifications}, configstore.KeyNotifications)
	case http.MethodPut:
		r.putAdminConfigNotifications(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigNotifications(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.NotificationsConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	// Validation is only required when notifications are enabled.
	if input.Enabled {
		seen := make(map[string]bool)
		for i, ch := range input.Channels {
			prefix := fmt.Sprintf("notifications.channels[%d]", i)

			if ch.Name == "" {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: name is required", prefix))
				return
			}
			if seen[ch.Name] {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: duplicate channel name %q", prefix, ch.Name))
				return
			}
			seen[ch.Name] = true

			switch ch.Type {
			case "webhook":
				if ch.URL == "" && ch.URLEnv == "" {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("%s: webhook channel requires url or url_env", prefix))
					return
				}
			case "email":
				if len(ch.Recipients) == 0 {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("%s: email channel requires at least one recipient", prefix))
					return
				}
			default:
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: type must be 'webhook' or 'email', got %q.", prefix, ch.Type))
				return
			}

			for j, ev := range ch.Events {
				if !validNotificationEvents[ev] {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("%s.events[%d]: unknown event type %q.", prefix, j, ev))
					return
				}
			}
		}

		for i, m := range input.ReadinessMilestones {
			if m < 0 || m > 100 {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("notifications.readiness_milestones[%d]: %d must be between 0 and 100.", i, m))
				return
			}
		}
	}

	r.storeAdminConfigSection(w, req, &config.Config{Notifications: input}, configstore.KeyNotifications)
}
