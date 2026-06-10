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

export interface ConcurrencyConfig {
  organisation_collection: number;
  node_page_fetching: number;
  git_pull: number;
  cookbook_download: number;
  cookstyle_scan: number;
  readiness_evaluation: number;
}

export interface AnalysisToolsConfig {
  embedded_bin_dir: string;
  cookstyle_enabled: boolean | null;
  cookstyle_timeout_minutes: number;
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
}

// Operator-safe metadata for the installed cert_source: db certificate.
// Returned read-only by GET /admin/config/server as `tls_certificate_info`;
// the private key is never included.
export interface CertMetadata {
  subject: string;
  issuer: string;
  dns_names?: string[];
  ip_addresses?: string[];
  not_before: string;
  not_after: string;
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
  // Read-only metadata for the installed cert_source: db certificate, attached
  // by GET. Never sent on save.
  tls_certificate_info?: CertMetadata;
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
  sp_entity_id?: string;
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
}

export interface AuthConfig {
  providers: AuthProvider[];
  session_expiry: string;
  min_password_length: number;
  lockout_attempts: number;
}

