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
  test_kitchen_run: number;
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
  storage_path: string;
  renew_before_days: number;
  agree_to_tos: boolean;
}

export interface TLSConfig {
  mode: string;
  cert_path: string;
  key_path: string;
  ca_path: string;
  min_version: string;
  http_redirect_port: number;
  acme: ACMEConfig;
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
  sp_entity_id?: string;
}

export interface AuthConfig {
  providers: AuthProvider[];
  session_expiry: string;
  min_password_length: number;
  lockout_attempts: number;
}

export interface NotificationChannelFilter {
  organisations: string[];
  cookbooks: string[];
}

export interface NotificationChannel {
  name: string;
  type: string;
  url: string;
  url_env: string;
  recipients: string[];
  events: string[];
  filters: NotificationChannelFilter;
}

export interface NotificationsConfig {
  enabled: boolean;
  channels: NotificationChannel[];
  readiness_milestones: number[];
  stale_node_alert_count: number;
}
