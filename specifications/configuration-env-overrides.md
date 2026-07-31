# Configuration — Environment Variable Overrides

There is **no generic override scheme**. `CHEF_MIGRATION_METRICS_<SECTION>_<KEY>` is not a
convention the code implements — each variable below is read by an individual, hand-written
lookup, and anything not listed cannot be set this way.

Environment variables exist only for **bootstrap**: values that must be present before the
UI is reachable. Everything else is configured in the database through the UI (see
[configuration.md](configuration.md) → Where configuration lives). Do not add an
environment override to make a setting configurable — wire it into the config store.

The supported variables are:

| Environment Variable | Description |
|----------------------|-------------|
| `CHEF_MIGRATION_METRICS_CONFIG` | Path to the configuration file |
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded AES-256 master key for encrypting/decrypting database-stored credentials. Required when any `*_credential` config references or Web API-created credentials exist. |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | Base64-encoded previous master key, required only during key rotation. Remove after successful rotation. |
| `DATABASE_URL` | Full datastore connection URL, overrides `datastore.url` |
| `CHEF_MIGRATION_METRICS_SERVER_PORT` | Override `server.port` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` | Override `server.tls.mode` (`off`, `static`, `acme`) |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH` | Override `server.tls.cert_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH` | Override `server.tls.key_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH` | Override `server.tls.ca_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION` | Override `server.tls.min_version` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT` | Override `server.tls.http_redirect_port` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL` | Override `server.tls.acme.email` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL` | Override `server.tls.acme.ca_url` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE` | Override `server.tls.acme.challenge` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER` | Override `server.tls.acme.dns_provider` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS` | Override `server.tls.acme.agree_to_tos` |
| `CHEF_MIGRATION_METRICS_ELASTICSEARCH_ENABLED` | Override `elasticsearch.enabled` — set to `true` to enable Elasticsearch NDJSON export |
| `CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY` | Override `elasticsearch.output_directory` — path where NDJSON files are written |
| `CMM_OWNERSHIP_ENABLED` | Override `ownership.enabled` |
| `CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS` | Override `ownership.audit_log.retention_days` |
