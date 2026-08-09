// SPDX-License-Identifier: Apache-2.0

export interface CollectionConfig {
  schedule: string;
  stale_node_threshold_days: number;
  stale_node_warning_hours: number;
  stale_node_critical_days: number;
  stale_cookbook_threshold_days: number;
  skip_server_cookbook_download: boolean;
  delete_server_cookbooks_after_scan: boolean | null;
}

// IngestConfig mirrors the backend ingest section (snake_case). The three
// booleans are *bool server-side, so GET may return null when unset (treat as
// false in the UI). See specifications/run-history.md.
export interface IngestConfig {
  enabled: boolean | null;
  show_run_events: boolean | null;
  failures_only: boolean | null;
  retention_days: number;
  max_body_bytes: number;
  max_records_per_body: number;
}

export interface ConcurrencyConfig {
  organisation_collection: number;
  node_page_fetching: number;
  role_fetching: number;
  git_pull: number;
  cookbook_download: number;
  cookstyle_scan: number;
  readiness_evaluation: number;
}

export interface AnalysisToolsConfig {
  embedded_bin_dir: string;
  cookstyle_enabled: boolean | null;
  cookstyle_timeout_minutes: number;
  // On-disk RuboCop addon cop files (files, directories, or globs on the app
  // host) loaded into every scan. Trust boundary is deploying the app.
  cookstyle_addon_cop_paths?: string[];
}

export interface LoggingConfig {
  level: string;
  retention_days: number;
}

export interface ReadinessConfig {
  install_path_linux: string;
  install_path_windows: string;
  install_size_mb_linux: number;
  install_size_mb_windows: number;
  min_remaining_free_percent: number;
  // When true, Review-level CookStyle offences gate readiness: a node whose
  // only issue is review-level cookbooks becomes "Needs review" (not ready).
  // Off (default) preserves blocker-only readiness.
  review_blocks_readiness: boolean;
  // When false, Test Kitchen results are still collected and shown but count
  // towards nothing. On (the default) a Test Kitchen failure marks the
  // cookbook incompatible, outranking a CookStyle pass.
  tk_blocks_readiness: boolean;
}

export interface ExportsConfig {
  max_rows: number;
  async_threshold: number;
  output_directory: string;
  retention_hours: number;
}

export interface ConfigOrganisation {
  name: string;
  chef_server_url: string;
  org_name: string;
  client_name: string;
  client_key_path: string;
  client_key_credential: string;
  ssl_verify: boolean | null;
}

export interface ACMEConfig {
  domains: string[];
  email: string;
  ca_url: string;
  challenge: string;
  dns_provider: string;
  // Provider-specific key/value pairs. For route53: region, hosted_zone_id.
  dns_provider_config: Record<string, string>;
  // Deprecated/unused: ACME state lives encrypted in the config store, not on
  // disk. Retained for backward-compatible parsing.
  storage_path: string;
  renew_before_days: number;
  agree_to_tos: boolean;
  // Hostname self-registration (tls-acme.md § 3.13): publish an A record per
  // domain pointing at the host. Opt-in, only meaningful with dns_provider:
  // route53. IP source precedence: hostname_ip > hostname_interface > auto.
  register_hostname: boolean;
  hostname_ttl: number;
  hostname_interface: string;
  hostname_ip: string;
  // Write-only Route 53 DNS-01 credentials. Sent under tls.acme.route53 on save
  // and routed server-side to encrypted secret keys; never returned by GET (so
  // they stay undefined after load). region/hosted_zone_id are non-secret and
  // travel in dns_provider_config. See tls-acme.md § 3.4/§ 3.5.
  route53?: {
    access_key_id?: string;
    secret_access_key?: string;
  };
}

// Operator-facing ACME health, returned read-only by GET /admin/config/server
// as `acme_status` when tls.mode is 'acme' (tls-acme.md § 3.14). All times are
// RFC 3339 strings; a field is empty/absent when not yet applicable.
export interface AcmeStatus {
  last_renewal?: string;
  last_error?: string;
  hostname_error?: string;
}

// Operator-safe metadata for one certificate in the installed bundle. Returned
// read-only by GET /admin/config/server inside the `tls_certificate_info` chain;
// the private key is never included. `role` is the structurally-derived chain
// position: 'leaf', 'intermediate', or 'root' (tls-static.md § 2.2).
export interface CertMetadata {
  subject: string;
  issuer: string;
  dns_names?: string[];
  ip_addresses?: string[];
  not_before: string;
  not_after: string;
  role: "leaf" | "intermediate" | "root";
}

export interface TLSConfig {
  mode: string;
  // Where the cert/key come from: 'file' (paths below) or 'db' (encrypted in
  // the config store, managed via the admin UI).
  cert_source: string;
  cert_path: string;
  key_path: string;
  ca_path: string;
  min_version: string;
  http_redirect_port: number;
  acme: ACMEConfig;
  // Write-only PEM material for cert_source: db. Sent on save; never returned
  // by GET (so they stay undefined after load and clear after a save). The
  // private key is secret and never echoed back by any API.
  certificate?: string;
  private_key?: string;
}

export interface WebSocketConfig {
  enabled: boolean | null;
  max_connections: number;
  send_buffer_size: number;
  write_timeout_seconds: number;
  ping_interval_seconds: number;
  pong_timeout_seconds: number;
}

export interface ServerConfig {
  listen_address: string;
  port: number;
  tls: TLSConfig;
  websocket: WebSocketConfig;
  graceful_shutdown_seconds: number;
  // True when a trusted reverse proxy terminates TLS in front of the app: the
  // local listener serves plain HTTP (tls.mode off) and X-Forwarded-Proto is
  // trusted for HSTS/scheme detection (tls.md § 9.1). Default false.
  trusted_proxy: boolean;
  // Read-only metadata for the installed certificate chain (cert_source: db, or
  // the issued ACME cert when mode is 'acme'), leaf → intermediate(s) → root,
  // attached by GET. Never sent on save.
  tls_certificate_info?: CertMetadata[];
  // Read-only ACME operator status, attached by GET when mode is 'acme'
  // (tls-acme.md § 3.14). Never sent on save.
  acme_status?: AcmeStatus;
}

export interface AuthProvider {
  type: string;
  host?: string;
  port?: number;
  base_dn?: string;
  bind_dn?: string;
  bind_password_env?: string;
  bind_password_credential?: string;
  idp_metadata_url?: string;
  idp_metadata_path?: string;
  idp_metadata_xml?: string;
  sp_entity_id?: string;
  sp_base_url?: string;
  sp_certificate_credential?: string;
  sp_private_key_credential?: string;
  username_attr?: string;
  email_attr?: string;
  display_name_attr?: string;
  groups_attr?: string;
  role_attr?: string;
  role_mapping?: Record<string, string>;
  allow_idp_initiated?: boolean;
  sign_requests?: boolean;
  debug_log_assertions?: boolean;
}

export interface AuthConfig {
  providers: AuthProvider[];
  session_expiry: string;
  min_password_length: number;
  lockout_attempts: number;
}

