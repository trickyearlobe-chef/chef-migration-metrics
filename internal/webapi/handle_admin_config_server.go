// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
)

// dbCertKeySubmission extracts the write-only cert_source: db PEM material from
// the admin save body. These live under `tls` in the request but are routed to
// dedicated encrypted config-store keys (configstore.KeyServerTLSCertificate /
// KeyServerTLSPrivateKey) — never into the server.tls config section — so PEM
// material is kept out of the assembled config struct and GET responses.
type dbCertKeySubmission struct {
	TLS struct {
		Certificate string `yaml:"certificate"`
		PrivateKey  string `yaml:"private_key"`
	} `yaml:"tls"`
}

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
		// For cert_source: db, surface the installed certificate's metadata
		// (subject/SANs/expiry) so the UI can show what's active. The private
		// key is never read or returned here.
		data = r.attachDBCertInfo(req.Context(), cfg, data)
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

	body, err := io.ReadAll(req.Body)
	if err != nil {
		WriteBadRequest(w, "Failed to read request body.")
		return
	}

	var input config.ServerConfig
	if err := yaml.Unmarshal(body, &input); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// cert_source: db submits the cert/key PEM under `tls` in the same body;
	// they are routed to dedicated encrypted store keys, never the server.tls
	// section (which has no PEM fields).
	var dbCerts dbCertKeySubmission
	_ = yaml.Unmarshal(body, &dbCerts)
	dbCertPEM := []byte(dbCerts.TLS.Certificate)
	dbKeyPEM := []byte(dbCerts.TLS.PrivateKey)

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

	// dbCertProvided is true when a complete cert/key pair is ready to persist
	// for the DB source (either submitted directly, or a cert matched against a
	// pending CSR key); set during static validation and used at persist time.
	// promotePending is true when the pair came from a CSR pending-key match and
	// the pending key must be deleted after a successful promote.
	dbCertProvided := false
	promotePending := false

	if input.TLS.Mode == "static" {
		switch input.TLS.CertSource {
		case "", "file":
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
			// Preflight the certificate exactly as the listener does at startup
			// (files readable, PEM parses, key matches cert). This prevents saving
			// a TLS configuration that would brick the listener on the next restart.
			if err := apptls.ValidateStaticPair(input.TLS.CertPath, input.TLS.KeyPath, input.TLS.CAPath); err != nil {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("server.tls: %v — fix the certificate before saving; the server cannot start TLS with an unusable certificate.", err))
				return
			}
		case "db":
			// The cert/key live in the encrypted config store, not on disk, so
			// cert_path/key_path are not required. The pair is submitted in the
			// request (or already stored from a prior save / CSR promotion).
			haveCert := len(dbCertPEM) > 0
			haveKey := len(dbKeyPEM) > 0
			switch {
			case haveCert && haveKey:
				// Preflight the submitted pair before persisting, so a bad pair
				// can never brick the listener on the next reload/restart.
				if err := apptls.ValidateStaticPairBytes(dbCertPEM, dbKeyPEM, input.TLS.CAPath); err != nil {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("server.tls: %v — fix the certificate before saving; the server cannot start TLS with an unusable certificate.", err))
					return
				}
				dbCertProvided = true
			case haveCert && !haveKey:
				// Match-and-promote (tls-csr.md § 4.5): an operator pasting a
				// signed certificate with no key promotes the pending CSR key if
				// the certificate's public key matches it. The active cert/key are
				// only replaced on a successful match, preserving fail-open.
				pendingKeyPEM, ok := r.pendingKeyPEM(req.Context())
				if !ok {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						"server.tls: a certificate was supplied without a private key and no pending CSR key is stored — submit the matching private key, or generate a CSR first.")
					return
				}
				if err := apptls.ValidateStaticPairBytes(dbCertPEM, pendingKeyPEM, input.TLS.CAPath); err != nil {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						fmt.Sprintf("server.tls: %v — the certificate does not match the pending CSR key; upload the certificate issued for the most recent CSR.", err))
					return
				}
				dbKeyPEM = pendingKeyPEM
				dbCertProvided = true
				promotePending = true
			case !haveCert && haveKey:
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					"server.tls: certificate and private_key must be provided together for cert_source 'db'.")
				return
			default:
				// Neither submitted — only valid if a pair is already stored.
				if !r.dbCertPairStored(req.Context()) {
					WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
						"server.tls: cert_source 'db' requires a certificate and private_key (none submitted and none stored).")
					return
				}
			}
		default:
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("server.tls.cert_source: must be 'file' or 'db', got %q.", input.TLS.CertSource))
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

	if input.TLS.HTTPRedirectPort != 0 {
		if input.TLS.HTTPRedirectPort < 1 || input.TLS.HTTPRedirectPort > 65535 {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("server.tls.http_redirect_port: %d is not a valid port number (1-65535).", input.TLS.HTTPRedirectPort))
			return
		}
		// The redirect listener only runs when TLS is active, and it must not
		// collide with the HTTPS listen port (both would bind the same port and
		// one would fail at startup). Compare against the port that will be in
		// effect: the submitted port if present, else the running listen port.
		if input.TLS.Mode == "static" || input.TLS.Mode == "acme" {
			effPort := input.Port
			if effPort == 0 {
				if lc := r.liveConfig(); lc != nil {
					effPort = lc.Server.Port
				}
			}
			if effPort != 0 && input.TLS.HTTPRedirectPort == effPort {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("server.tls.http_redirect_port (%d) must differ from the HTTPS listen port; both would bind the same port.", effPort))
				return
			}
		}
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

	// --- Listen address / port validation ---

	if input.Port != 0 && (input.Port < 1 || input.Port > 65535) {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("server.port: %d is not a valid port number (1-65535).", input.Port))
		return
	}

	// Test-bind the listen target as a preflight when a concrete port is given
	// that differs from the running listener. A zero port means "unchanged /
	// use default" and is skipped. The running process already holds the
	// current address/port, so test-binding the unchanged value would always
	// fail — hence the change check. This catches an unbindable address/port
	// before it is persisted and forces the degraded fallback on the next
	// restart (encrypted-config-store.md).
	if input.Port != 0 {
		live := r.liveConfig()
		changed := live == nil ||
			input.Port != live.Server.Port || input.ListenAddress != live.Server.ListenAddress
		if changed {
			if err := apptls.TestBind(input.ListenAddress, input.Port); err != nil {
				addr := input.ListenAddress
				if addr == "" {
					addr = "0.0.0.0"
				}
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("server: cannot bind %s:%d (%v) — choose a bindable address and port; the server cannot start on a port it cannot bind.",
						addr, input.Port, err))
				return
			}
		}
	}

	// --- Persist all listen/TLS/WebSocket/shutdown sub-keys then reload ---

	sections, err := configstore.ConfigToSections(&config.Config{Server: input})
	if err != nil {
		r.logf("ERROR", "admin/config/server: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise server config.")
		return
	}

	ctx := req.Context()
	for _, key := range []string{configstore.KeyServerListen, configstore.KeyServerTLS, configstore.KeyServerWebSocket, configstore.KeyServerGracefulShutdown, configstore.KeyServerTrustedProxy} {
		if err := r.configStore.Set(ctx, key, sections[key], false, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store %s: %v", key, err)
			WriteInternalError(w, "Failed to store server config.")
			return
		}
	}

	// Persist a submitted cert_source: db pair to its dedicated encrypted keys:
	// the certificate non-secret (public), the private key secret (never
	// returned by any API). Already validated above.
	if dbCertProvided {
		certJSON, _ := json.Marshal(string(dbCertPEM))
		if err := r.configStore.Set(ctx, configstore.KeyServerTLSCertificate, certJSON, false, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store TLS certificate: %v", err)
			WriteInternalError(w, "Failed to store TLS certificate.")
			return
		}
		keyJSON, _ := json.Marshal(string(dbKeyPEM))
		if err := r.configStore.Set(ctx, configstore.KeyServerTLSPrivateKey, keyJSON, true, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store TLS private key: %v", err)
			WriteInternalError(w, "Failed to store TLS private key.")
			return
		}
		// A promoted CSR key is now active — remove the pending copy (tls-csr.md
		// § 4.5). Non-fatal: the key is still secret and a new CSR overwrites it.
		if promotePending {
			if err := r.configStore.Delete(ctx, configstore.KeyServerTLSPrivateKeyPending); err != nil {
				r.logf("WARN", "admin/config/server: delete promoted pending key: %v", err)
			}
		}
	}

	if r.configHolder != nil {
		if err := r.configHolder.Reload(ctx); err != nil {
			r.logf("ERROR", "admin/config/server: reload: %v", err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}

	// Swap the running static-TLS certificate in place so the listener serves
	// the new pair without a restart (tls-static.md § 2.3). Best-effort: when
	// no reloader is wired (file source, plain HTTP, or no DB listener yet) the
	// pair is already persisted and the next restart applies it.
	if dbCertProvided && r.tlsReload != nil {
		if err := r.tlsReload.Reload(dbCertPEM, dbKeyPEM); err != nil && !errors.Is(err, ErrNoTLSReloader) {
			r.logf("WARN", "admin/config/server: in-place TLS reload failed (%v); restart applies the new certificate", err)
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

// dbCertPairStored reports whether a cert_source: db certificate AND private
// key are both already present in the config store. Used to allow re-saving a
// db configuration without resubmitting the pair.
func (r *Router) dbCertPairStored(ctx context.Context) bool {
	if r.configStore == nil {
		return false
	}
	if _, err := r.configStore.Get(ctx, configstore.KeyServerTLSCertificate); err != nil {
		return false
	}
	if _, err := r.configStore.GetSecret(ctx, configstore.KeyServerTLSPrivateKey); err != nil {
		return false
	}
	return true
}

// pendingKeyPEM returns the stored pending CSR private key PEM and true when one
// is present. The pending key is a secret (server.tls.private_key.pending); it
// is never returned by any API, only used internally to match-and-promote an
// uploaded signed certificate (tls-csr.md § 4.5).
func (r *Router) pendingKeyPEM(ctx context.Context) ([]byte, bool) {
	if r.configStore == nil {
		return nil, false
	}
	raw, err := r.configStore.GetSecret(ctx, configstore.KeyServerTLSPrivateKeyPending)
	if err != nil {
		return nil, false
	}
	var pem string
	if err := json.Unmarshal(raw, &pem); err != nil || pem == "" {
		return nil, false
	}
	return []byte(pem), true
}

// attachDBCertInfo augments a serialised server-config JSON object with a
// `tls_certificate_info` field carrying the installed DB certificate's
// operator-safe metadata (subject/SANs/expiry). It is a no-op unless the live
// cert_source is `db` and a certificate is stored. The private key is never
// read. On any error it returns data unchanged.
func (r *Router) attachDBCertInfo(ctx context.Context, cfg *config.Config, data json.RawMessage) json.RawMessage {
	if cfg == nil || cfg.Server.TLS.CertSource != "db" || r.configStore == nil {
		return data
	}
	raw, err := r.configStore.Get(ctx, configstore.KeyServerTLSCertificate)
	if err != nil {
		return data
	}
	var pemStr string
	if err := json.Unmarshal(raw, &pemStr); err != nil {
		return data
	}
	meta, err := apptls.CertMetadataFromPEM([]byte(pemStr))
	if err != nil {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return data
	}
	obj["tls_certificate_info"] = metaJSON
	merged, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return merged
}
