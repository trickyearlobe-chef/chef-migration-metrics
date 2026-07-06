# Configuration — Validation

On startup, the application must validate the configuration and fail fast with a descriptive error message if:

- Any required field is missing
- A referenced key file does not exist or is not readable
- A `*_credential` reference names a credential that does not exist in the `credentials` table (log `ERROR` with the credential name and the config field that references it)
- A `*_credential` reference exists but `CMM_CREDENTIAL_ENCRYPTION_KEY` is not set or is invalid (fatal — cannot decrypt database credentials)
- `CMM_CREDENTIAL_ENCRYPTION_KEY` is set but is not valid Base64 or decodes to fewer than 32 bytes (fatal)
- A credential in the `credentials` table cannot be decrypted with the current or previous master key (log `ERROR` per credential; fatal if the credential is required by an active organisation or provider)
- An organisation has neither `client_key_path` nor `client_key_credential` configured (no credential source available)
- The datastore is not reachable
- An unknown configuration key is present (to catch typos)
- A cron expression is invalid
- A target Chef Client version string is not a valid semver
- The `server.port` is not a valid port number (1–65535)
- `server.tls.mode` is not one of `off`, `static`, or `acme`
- `server.tls.min_version` is not one of `"1.2"` or `"1.3"` (when mode is `static` or `acme`)
- `server.tls.http_redirect_port` is set but is not a valid port number (1–65535)
- `server.tls.http_redirect_port` equals the HTTPS listen port (`server.port`) when TLS is active (`static`/`acme`) — both listeners would bind the same port and one would fail
- **Static mode validation:**
  - `server.tls.mode` is `static` but `cert_path` or `key_path` is missing or empty
  - The certificate file at `cert_path` does not exist or is not readable
  - The key file at `key_path` does not exist or is not readable
  - The certificate and key do not form a valid pair
  - The certificate is expired at startup time (log `WARN` — do not prevent startup, as the operator may be in the process of renewing)
  - `ca_path` is set but the file does not exist or is not a valid PEM bundle
- **ACME mode validation:**
  - `server.tls.mode` is `acme` but `acme.domains` is empty
  - `server.tls.mode` is `acme` but `acme.email` is empty
  - `server.tls.mode` is `acme` but `acme.agree_to_tos` is not `true`
  - `acme.challenge` is not one of `http-01`, `tls-alpn-01`, or `dns-01`
  - `acme.challenge` is `dns-01` but `acme.dns_provider` is empty
  - `acme.challenge` is `dns-01` but required `dns_provider_config` keys for the selected provider are missing
  - `acme.challenge` is `http-01` but `http_redirect_port` is `0` (fatal — the HTTP-01 challenge cannot be served)
  - `acme.renew_before_days` is less than 1 or greater than 89
  - `acme.ca_url` is not a valid URL
  - `acme.trusted_roots` is set but the file does not exist or is not a valid PEM bundle
- **Backward compatibility:** Both `server.tls.enabled` and `server.tls.mode` are present (log `WARN` — `mode` takes precedence)
- The exports output directory does not exist or is not writable
- `stale_node_threshold_days` or `stale_cookbook_threshold_days` is less than 1
- `analysis_tools.cookstyle_timeout_minutes` is less than 1
- `analysis_tools.test_kitchen_timeout_minutes` is less than 1
- `elasticsearch.output_directory` does not exist or is not writable when `elasticsearch.enabled` is `true`
- `elasticsearch.retention_hours` is less than 1
- `ownership.auto_rules[].name` must be unique across all rules
- `ownership.auto_rules[].owner` must reference an existing owner when auto-derivation runs (validated at rule evaluation time, not startup — owners may be created after config is written)
- `ownership.auto_rules[].type` must be one of: `node_attribute`, `node_name_pattern`, `policy_match`, `cookbook_name_pattern`, `git_repo_url_pattern`, `role_match`
- `ownership.auto_rules[].pattern` must be a valid Go regex when required by the rule type
- `ownership.auto_rules[].attribute_path` is required when type is `node_attribute`
- `ownership.auto_rules[].match_value` is required when type is `node_attribute`
- `ownership.auto_rules[].policy_name` is required when type is `policy_match`
- `ownership.audit_log.retention_days` must be a non-negative integer

---

### Save-time preflight (admin config API)

Configuration changed through the admin API (`PUT /api/v1/admin/config/server`)
must be validated **before it is persisted**, not only at the next startup. This
prevents an operator from saving a server/TLS configuration that would brick the
listener on restart (the config store is the source of truth at boot, so a bad
value cannot be corrected through the UI once the server fails to start).

The handler applies the same checks as startup and returns `422 Unprocessable
Entity` (error code `validation_error`) without writing to the config store when:

- `server.tls.mode` is `static` and the cert/key pair fails to load exactly as
  the listener loads it at startup — files unreadable, PEM unparseable, or the
  private key does not match the certificate (`ca_path`, when set, must also be a
  valid PEM bundle). Implemented by `tls.ValidateStaticPair`, shared with startup.
- `server.tls.http_redirect_port` equals the effective HTTPS listen port when TLS
  is active (the submitted `server.port` if present, otherwise the running port).

The error message names the failing element and never includes key material.

`PUT /api/v1/admin/config/git-urls` applies the same 422 `validation_error`
preflight: each git base URL must be a well-formed git remote — scp-style
`[user@]host:path`, or a scheme URL (`ssh://[user@]host[:PORT]/path` with a
numeric PORT, `https://`, `http://`, `git://`). The `ssh://host:<non-numeric>`
scp/URL hybrid (a scp-style path after the `ssh://` scheme, where `:` is read as
a port) is rejected with a message suggesting `git@host:path` or
`ssh://git@host/path`. This is a **save-time-only** check (not startup
validation): an already-stored bad value must never block boot — the config
store is the boot source of truth. Implemented by `config.ValidateGitBaseURLs`.
Saving the list also triggers a background collection so new/edited URLs
re-fetch immediately (see [data-collection.md](data-collection.md)).

---

> **Note:** See [Web API specification § WebSocket Real-Time Events](web-api.md#websocket-real-time-events) for the event types, envelope format, and client reconnection behaviour.
