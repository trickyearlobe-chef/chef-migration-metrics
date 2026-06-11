# Configuration — Full Example


```yaml
# -- Credential encryption (required when using database-stored credentials) --
# The actual key value must be set via environment variable, not inlined here.
credential_encryption_key_env: CMM_CREDENTIAL_ENCRYPTION_KEY

organisations:
  # File-based key
  - name: myorg-production
    chef_server_url: https://chef.example.com
    org_name: myorg-production
    client_name: chef-migration-metrics
    client_key_path: /etc/chef-migration-metrics/keys/myorg-production.pem

  # Database-stored key (created via Web API)
  # - name: myorg-staging
  #   chef_server_url: https://chef.example.com
  #   org_name: myorg-staging
  #   client_name: chef-migration-metrics
  #   client_key_credential: myorg-staging-key

target_chef_versions:
  - "18.5.0"
  - "19.0.0"

git_base_urls:
  - https://github.com/myorg
  - https://gitlab.example.com/chef-cookbooks

collection:
  schedule: "0 * * * *"
  stale_node_threshold_days: 7
  stale_cookbook_threshold_days: 365

# Database migrations are applied automatically on startup.
# No configuration is required — migration files are embedded in the binary.

concurrency:
  organisation_collection: 5
  node_page_fetching: 10
  git_pull: 10
  cookbook_download: 4
  cookstyle_scan: 8
  readiness_evaluation: 20

readiness:
  install_path_linux: /hab
  install_path_windows: 'C:\hab'
  install_size_mb_linux: 3072
  install_size_mb_windows: 6144
  min_remaining_free_percent: 20

datastore:
  url: postgres://localhost:5432/chef_migration_metrics

server:
  listen_address: "0.0.0.0"
  port: 8080
  tls:
    mode: "off"
  websocket:
    enabled: true
    max_connections: 100
    send_buffer_size: 64
    write_timeout_seconds: 10
    ping_interval_seconds: 30
    pong_timeout_seconds: 60
    # --- Static certificate example (uncomment and set mode: static) ---
    # cert_path: /etc/chef-migration-metrics/tls/server.crt
    # key_path: /etc/chef-migration-metrics/tls/server.key
    # ca_path: ""
    # min_version: "1.2"
    # http_redirect_port: 80
    # --- ACME example (uncomment and set mode: acme) ---
    # acme:
    #   domains:
    #     - chef-metrics.example.com
    #   email: admin@example.com
    #   ca_url: https://acme-v02.api.letsencrypt.org/directory
    #   challenge: http-01
    #   renew_before_days: 30
    #   agree_to_tos: true
  graceful_shutdown_seconds: 30

frontend:
  base_path: "/"

logging:
  level: INFO
  retention_days: 90

ownership:
  enabled: false
  audit_log:
    retention_days: 365
  auto_rules: []

auth:
  providers:
    - type: local

notifications:
  enabled: false
  channels: []
  readiness_milestones:
    - 50
    - 75
    - 90
    - 100
  stale_node_alert_count: 50

exports:
  max_rows: 100000
  async_threshold: 10000
  output_directory: /var/lib/chef-migration-metrics/exports
  retention_hours: 24

analysis_tools:
  embedded_bin_dir: /opt/chef-migration-metrics/embedded/bin
  cookstyle_timeout_minutes: 10
  test_kitchen:
    enabled: true
    timeout_minutes: 30
    max_concurrent_vms: 2            # global concurrency ceiling (no per-batch limit)
    start_rate_window_minutes: 90    # VM start-rate limiter window = DHCP lease time (0 = off)
    start_rate_max_per_window: 25    # max starts per window = usable DHCP pool size
    driver: dokken
    images:
      - name: alma9
        id: tmpl-alma9-kitchen
        install_method: download
      - name: ubuntu2204-baked
        id: tmpl-ubuntu2204-baked
        install_method: baked_in
        chef_client_path: /opt/chef/bin/chef-client

elasticsearch:
  enabled: false
  output_directory: /var/lib/chef-migration-metrics/elasticsearch
  retention_hours: 48
```
