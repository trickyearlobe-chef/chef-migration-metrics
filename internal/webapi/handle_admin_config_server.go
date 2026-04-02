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
// GET/PUT /api/v1/admin/config/server
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigServer(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		data, err := configstore.SerializeValue(cfg.Server)
		if err != nil {
			r.logf("ERROR", "admin/config/server: serialise: %v", err)
			WriteInternalError(w, "Failed to serialise server config.")
			return
		}
		WriteJSON(w, http.StatusOK, data)
	case http.MethodPut:
		r.putAdminConfigServer(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigServer(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.ServerConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	// --- TLS validation ---

	switch input.TLS.Mode {
	case "", "off", "static", "acme":
		// valid
	default:
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("server.tls.mode: must be 'off', 'static', or 'acme', got %q.", input.TLS.Mode))
		return
	}

	if input.TLS.Mode == "static" || input.TLS.Mode == "acme" {
		switch input.TLS.MinVersion {
		case "", "1.2", "1.3":
			// valid
		default:
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("server.tls.min_version: must be '1.2' or '1.3', got %q.", input.TLS.MinVersion))
			return
		}
	}

	if input.TLS.Mode == "static" {
		if input.TLS.CertPath == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"server.tls.cert_path is required when tls.mode is 'static'.")
			return
		}
		if input.TLS.KeyPath == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"server.tls.key_path is required when tls.mode is 'static'.")
			return
		}
	}

	if input.TLS.Mode == "acme" {
		acme := input.TLS.ACME
		if len(acme.Domains) == 0 {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"server.tls.acme.domains is required when tls.mode is 'acme'.")
			return
		}
		if acme.Email == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"server.tls.acme.email is required when tls.mode is 'acme'.")
			return
		}
		if !acme.AgreeToTOS {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"server.tls.acme.agree_to_tos must be true when tls.mode is 'acme'.")
			return
		}
		if acme.Challenge != "" {
			switch acme.Challenge {
			case "http-01", "tls-alpn-01", "dns-01":
				// valid
			default:
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("server.tls.acme.challenge: must be 'http-01', 'tls-alpn-01', or 'dns-01', got %q.", acme.Challenge))
				return
			}
		}
		if acme.RenewBeforeDays != 0 && (acme.RenewBeforeDays < 1 || acme.RenewBeforeDays > 89) {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("server.tls.acme.renew_before_days: must be between 1 and 89, got %d.", acme.RenewBeforeDays))
			return
		}
	}

	if input.TLS.HTTPRedirectPort != 0 && (input.TLS.HTTPRedirectPort < 1 || input.TLS.HTTPRedirectPort > 65535) {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("server.tls.http_redirect_port: %d is not a valid port number (1-65535).", input.TLS.HTTPRedirectPort))
		return
	}

	// --- WebSocket validation (zero means "use default") ---

	ws := input.WebSocket
	if ws.MaxConnections != 0 && ws.MaxConnections < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"server.websocket.max_connections: must be >= 1.")
		return
	}
	if ws.SendBufferSize != 0 && ws.SendBufferSize < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"server.websocket.send_buffer_size: must be >= 1.")
		return
	}
	if ws.WriteTimeoutSeconds != 0 && ws.WriteTimeoutSeconds < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"server.websocket.write_timeout_seconds: must be >= 1.")
		return
	}
	if ws.PingIntervalSeconds != 0 && ws.PingIntervalSeconds < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"server.websocket.ping_interval_seconds: must be >= 1.")
		return
	}
	if ws.PongTimeoutSeconds != 0 && ws.PongTimeoutSeconds < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"server.websocket.pong_timeout_seconds: must be >= 1.")
		return
	}
	if ws.PongTimeoutSeconds != 0 && ws.PingIntervalSeconds != 0 && ws.PongTimeoutSeconds <= ws.PingIntervalSeconds {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("server.websocket.pong_timeout_seconds: must be greater than ping_interval_seconds (%d), got %d.",
				ws.PingIntervalSeconds, ws.PongTimeoutSeconds))
		return
	}

	// --- Persist all three sub-keys then reload ---

	sections, err := configstore.ConfigToSections(&config.Config{Server: input})
	if err != nil {
		r.logf("ERROR", "admin/config/server: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise server config.")
		return
	}

	ctx := req.Context()
	for _, key := range []string{configstore.KeyServerTLS, configstore.KeyServerWebSocket, configstore.KeyServerGracefulShutdown} {
		if err := r.configStore.Set(ctx, key, sections[key], false, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store %s: %v", key, err)
			WriteInternalError(w, "Failed to store server config.")
			return
		}
	}

	if r.configHolder != nil {
		if err := r.configHolder.Reload(ctx); err != nil {
			r.logf("ERROR", "admin/config/server: reload: %v", err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}

	data, err := configstore.SerializeValue(input)
	if err != nil {
		r.logf("ERROR", "admin/config/server: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	WriteJSON(w, http.StatusOK, putConfigResponse{Value: data, RestartRequired: true})
}
