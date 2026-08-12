// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
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

// acmeRoute53CredSubmission extracts the write-only Route 53 DNS-01 credentials
// from the admin save body (tls.acme.route53). Like the cert/key PEM material,
// these are routed to dedicated encrypted config-store secret keys
// (configstore.KeyServerTLSACMERoute53AccessKeyID / SecretAccessKey) — never
// into the server.tls config section — so credentials stay out of the assembled
// config struct and GET responses (tls-acme.md § 3.4 / § 3.5). region and
// hosted_zone_id are non-secret and travel in dns_provider_config as normal.
type acmeRoute53CredSubmission struct {
	TLS struct {
		ACME struct {
			Route53 struct {
				AccessKeyID     string `yaml:"access_key_id"`
				SecretAccessKey string `yaml:"secret_access_key"`
			} `yaml:"route53"`
		} `yaml:"acme"`
	} `yaml:"tls"`
}

// serverKeyGranularity maps each persisted server sub-key to the reload
// granularity a change to it currently needs. graceful_shutdown_seconds is read
// live at shutdown time, so it applies without a restart; websocket is a
// subsystem rebuild (the hub is reconfigured in place — see putAdminConfigServer);
// trusted_proxy stays pessimistically process until its in-place rebind lands.
// The listen and tls keys are NOT here: their granularity is resolved
// per-request from the in-place rebind outcome (an off↔static mode toggle or a
// listen change rebinds to ReloadListener on success, ReloadProcess when no
// rebinder is wired) — see putAdminConfigServer.
var serverKeyGranularity = map[string]ReloadGranularity{
	configstore.KeyServerGracefulShutdown: ReloadApplied,
	configstore.KeyServerWebSocket:        ReloadSubsystem,
	configstore.KeyServerTrustedProxy:     ReloadProcess,
}

// serverReloadGranularity reports the worst reload granularity across the server
// sub-keys whose stored value actually changed between the pre-save live config
// and the submitted config. Unchanged keys are ignored; with nothing changed the
// save is applied (a no-op needs no restart). A nil live snapshot (no holder and
// no boot config) is treated pessimistically as changed for every key. listenGran
// and tlsGran are the listen and tls keys' already-resolved granularities (folded
// in directly because they are applied via the in-place rebinder, not the static
// map).
func serverReloadGranularity(newSections, liveSections map[string]json.RawMessage, listenGran, tlsGran ReloadGranularity) ReloadGranularity {
	worst := listenGran
	if tlsGran > worst {
		worst = tlsGran
	}
	for key, g := range serverKeyGranularity {
		changed := liveSections == nil || !bytes.Equal(newSections[key], liveSections[key])
		if changed && g > worst {
			worst = g
		}
	}
	return worst
}

// listenSectionChanged reports whether the persisted server.listen section
// differs between the submitted and pre-save live config. A nil live snapshot is
// treated pessimistically as changed.
func listenSectionChanged(newSections, liveSections map[string]json.RawMessage) bool {
	return liveSections == nil ||
		!bytes.Equal(newSections[configstore.KeyServerListen], liveSections[configstore.KeyServerListen])
}

// websocketSectionChanged reports whether the persisted server.websocket section
// differs between the submitted and pre-save live config. A nil live snapshot is
// treated pessimistically as changed.
func websocketSectionChanged(newSections, liveSections map[string]json.RawMessage) bool {
	return liveSections == nil ||
		!bytes.Equal(newSections[configstore.KeyServerWebSocket], liveSections[configstore.KeyServerWebSocket])
}

// tlsSectionChanged reports whether the persisted server.tls section differs
// between the submitted and pre-save live config. A nil live snapshot is treated
// pessimistically as changed.
func tlsSectionChanged(newSections, liveSections map[string]json.RawMessage) bool {
	return liveSections == nil ||
		!bytes.Equal(newSections[configstore.KeyServerTLS], liveSections[configstore.KeyServerTLS])
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/server
// ---------------------------------------------------------------------------

// serverConfigResponse is the server settings as a caller receives them.
//
// The settings themselves are embedded, so they stay reflected off the real
// type. What is added here is what the handler attaches on the way out: the
// metadata of the certificate actually installed, and the state of the last
// ACME renewal. Neither is part of the settings, and neither carries a private
// key.
type serverConfigResponse struct {
	config.ServerConfig
	TLSCertificateInfo []apptls.CertMetadata `json:"tls_certificate_info,omitempty"`
	// Whatever the renewer last recorded. The service does not decide this
	// shape, so describing one would refuse a caller something real.
	ACMEStatus json.RawMessage `json:"acme_status,omitempty"`
}

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
		// In ACME mode, surface the issued certificate's metadata and the
		// operator status panel data (tls-acme.md § 3.14).
		data = r.attachACMEInfo(req.Context(), cfg, data)
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

	// Route 53 DNS-01 credentials are submitted under tls.acme.route53 and routed
	// to dedicated encrypted secret keys, never the server.tls section.
	var r53Creds acmeRoute53CredSubmission
	_ = yaml.Unmarshal(body, &r53Creds)

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
	// dbCertWarning carries a non-fatal chain-reorder warning (incomplete or
	// non-linking bundle) surfaced to the operator in the save response.
	dbCertWarning := ""

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
				// Reorder the operator-supplied bundle into leaf → intermediate(s)
				// → root before preflight and storage (tls-static.md § 2.2). The
				// preflight below matches the key against cert[0], so the leaf must
				// lead; an incomplete chain is stored with a non-fatal warning, never
				// rejected. Only CSR-promoted bundles (below) are left as-is.
				if reordered, warn, rerr := apptls.ReorderChainPEM(dbCertPEM); rerr == nil {
					dbCertPEM = reordered
					dbCertWarning = warn
				}
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
			// Automatic HTTPS binds 443 when TLS is active (tls.md § 1.5), so the
			// redirect listener must not collide with 443 either.
			if input.TLS.HTTPRedirectPort == 443 {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					"server.tls.http_redirect_port must differ from 443; automatic HTTPS binds 443 when TLS is active.")
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

	// Snapshot the pre-save live server sections so the response can report the
	// worst reload granularity of the sub-keys that actually changed (the PUT
	// bundles listen/tls/websocket/graceful, but a graceful-only change applies
	// live and must not claim a restart). Captured before the holder reloads.
	var liveSections map[string]json.RawMessage
	if live := r.liveConfig(); live != nil {
		liveSections, _ = configstore.ConfigToSections(&config.Config{Server: live.Server})
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

	// Persist submitted Route 53 DNS-01 credentials as encrypted secrets
	// (tls-acme.md § 3.4/§ 3.5). Write-only: an empty field leaves the stored
	// secret untouched so a save that omits credentials does not wipe them.
	if v := r53Creds.TLS.ACME.Route53.AccessKeyID; v != "" {
		j, _ := json.Marshal(v)
		if err := r.configStore.Set(ctx, configstore.KeyServerTLSACMERoute53AccessKeyID, j, true, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store route53 access_key_id: %v", err)
			WriteInternalError(w, "Failed to store Route 53 credentials.")
			return
		}
	}
	if v := r53Creds.TLS.ACME.Route53.SecretAccessKey; v != "" {
		j, _ := json.Marshal(v)
		if err := r.configStore.Set(ctx, configstore.KeyServerTLSACMERoute53SecretAccessKey, j, true, "admin"); err != nil {
			r.logf("ERROR", "admin/config/server: store route53 secret_access_key: %v", err)
			WriteInternalError(w, "Failed to store Route 53 credentials.")
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

	// Reconfigure the running WebSocket hub in place when the websocket section
	// changed (configuration-live-reload.md: subsystem). Existing connections are
	// preserved — the new max_connections/send_buffer_size take effect on
	// subsequent registrations, and timeouts on subsequent connections (pulled
	// live by the handler). Values come from the reloaded live config so unset
	// fields carry their applied defaults. Reported as subsystem (no restart) via
	// serverKeyGranularity.
	if r.hub != nil && websocketSectionChanged(sections, liveSections) {
		ws := input.WebSocket
		if lc := r.liveConfig(); lc != nil {
			ws = lc.Server.WebSocket
		}
		r.hub.Reconfigure(ws.MaxConnections, ws.SendBufferSize)
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

	// An ACME save re-asserts hostname registration and re-checks issuance
	// immediately rather than waiting out the renewal interval (tls-acme.md
	// § 3.14). Non-blocking; no-op when no renewer is wired.
	if input.TLS.Mode == "acme" && r.acmeReRegister != nil {
		r.acmeReRegister()
	}

	// Rebind the running listener in place when the listen target changed or the
	// TLS section changed (configuration-live-reload.md listener-rebind). A tls
	// change covers an off↔static mode toggle (H4a), a same-mode static field
	// change — min_version / mTLS CA / cert source-or-paths (H4b-1) — and an
	// http_redirect_port change (H4b-2), each rebuilding the HTTPS (+ redirect)
	// listener topology; the applier dispatches and either rebinds in place or
	// refuses an unsupported topology. The new listener is bound first (or, on a
	// same-address:port change, validated then rebound in place); only once it is
	// serving is the old one drained. The resolved granularity is folded into the
	// response via listenGran (listen key) and tlsGran (tls key):
	//   - unchanged / no-op       → applied
	//   - rebound in place        → listener (no restart)
	//   - no rebinder / refused   → process (restart_required; persisted, applies on
	//     the next restart — ACME, the auto-443 lifeboat re-plan, and the degraded
	//     self-signed fallback fall here)
	// A bind failure (e.g. the port is now held by another process) keeps the old
	// listener serving and is surfaced as a 500.
	listenChanged := listenSectionChanged(sections, liveSections)
	tlsChanged := tlsSectionChanged(sections, liveSections)

	listenGran := ReloadApplied
	tlsGran := ReloadApplied

	if listenChanged || tlsChanged {
		applyGran := ReloadProcess
		if r.listenerRebind != nil {
			// The submitted config is the desired target. A zero submitted port means
			// "unchanged", so fill it from the live port rather than rebinding to an
			// ephemeral one.
			effCfg := input
			if effCfg.Port == 0 {
				if lc := r.liveConfig(); lc != nil {
					effCfg.Port = lc.Server.Port
				}
			}
			g, applyErr := r.listenerRebind.Apply(effCfg)
			switch {
			case applyErr == nil:
				applyGran = g
			case errors.Is(applyErr, ErrNoListenerRebinder):
				// No in-place rebinder / refused topology — restart-required
				// (applyGran stays process). The new config is already persisted.
			default:
				addr := effCfg.ListenAddress
				if addr == "" {
					addr = "0.0.0.0"
				}
				r.logf("ERROR", "admin/config/server: listener rebind to %s:%d failed: %v", addr, effCfg.Port, applyErr)
				WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
					fmt.Sprintf("server: could not rebind the listener to %s:%d (%v) — the previous listener is still serving; free that port or revert the change.", addr, effCfg.Port, applyErr))
				return
			}
		}
		if listenChanged {
			listenGran = applyGran
		}
		if tlsChanged {
			tlsGran = applyGran
		}
	}

	data, err := configstore.SerializeValue(input)
	if err != nil {
		r.logf("ERROR", "admin/config/server: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	reload := serverReloadGranularity(sections, liveSections, listenGran, tlsGran)
	resp := putConfigResponse{
		Value:           data,
		RestartRequired: reload == ReloadProcess,
		Reload:          reload.String(),
	}
	if dbCertWarning != "" {
		r.logf("WARN", "admin/config/server: TLS certificate chain stored with warning: %s", dbCertWarning)
		resp.Warnings = append(resp.Warnings, dbCertWarning)
	}
	WriteJSON(w, http.StatusOK, resp)
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
// `tls_certificate_info` field carrying the installed DB certificate chain's
// operator-safe metadata (per-cert subject/SANs/issuer/expiry/role for
// leaf → intermediate(s) → root — tls-static.md § 2.2). It is a no-op unless the
// live cert_source is `db` and a certificate is stored. The private key is never
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
	chain, err := apptls.ChainMetadataFromPEM([]byte(pemStr))
	if err != nil {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}
	metaJSON, err := json.Marshal(chain)
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

// attachACMEInfo augments a serialised server-config JSON object, when the live
// mode is `acme`, with `tls_certificate_info` (the issued certificate's
// operator-safe metadata, read from server.tls.acme.cert) and `acme_status`
// (last renewal / last error / hostname error, read from server.tls.acme.status)
// to drive the Server & TLS ACME status panel (tls-acme.md § 3.14). The private
// key is never read. On any per-field error that field is omitted; data is
// otherwise returned unchanged.
func (r *Router) attachACMEInfo(ctx context.Context, cfg *config.Config, data json.RawMessage) json.RawMessage {
	if cfg == nil || cfg.Server.TLS.Mode != "acme" || r.configStore == nil {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}

	// Issued certificate chain metadata (mirrors attachDBCertInfo but from the
	// ACME cert key). Absent until the first issuance — then simply omitted.
	if raw, err := r.configStore.Get(ctx, configstore.KeyServerTLSACMECert); err == nil {
		var pemStr string
		if json.Unmarshal(raw, &pemStr) == nil {
			if chain, merr := apptls.ChainMetadataFromPEM([]byte(pemStr)); merr == nil {
				if metaJSON, jerr := json.Marshal(chain); jerr == nil {
					obj["tls_certificate_info"] = metaJSON
				}
			}
		}
	}

	// Operator status object. A never-written entry yields an empty object so the
	// panel always has a value to render.
	status := json.RawMessage(`{}`)
	if raw, err := r.configStore.Get(ctx, configstore.KeyServerTLSACMEStatus); err == nil && len(raw) > 0 {
		status = raw
	}
	obj["acme_status"] = status

	merged, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return merged
}
