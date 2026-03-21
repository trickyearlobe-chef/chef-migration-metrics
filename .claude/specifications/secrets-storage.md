# Secrets Storage - Component Specification

> **TL;DR** — Three credential storage methods in precedence order: **database** (AES-256-GCM encrypted, managed via Web UI/API), **environment variable** (Kubernetes Secrets, ECS, CI/CD), **file path** (traditional on-prem PEM files). Database credentials are encrypted with a master key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) that must live outside the database. The `credentials` table stores Chef API keys, LDAP passwords, SMTP passwords, and webhook URLs — all encrypted at rest with per-row nonces and AAD binding. Plaintext is only held in memory for the duration of each operation. Key rotation (both credential values and the master encryption key) is supported without downtime. Admin-only Web API endpoints manage credentials (CRUD + test) and never return plaintext. Kubernetes deployments use `existingSecret` references or chart-managed Secrets; RPM/DEB installs use file paths and env files. See `todo/secrets-storage.md` for implementation status.

---

## Overview

This specification consolidates all secrets and credential management for the Chef Migration Metrics application. Secrets include Chef API private keys, LDAP bind passwords, SMTP credentials, webhook URLs, the database connection string, TLS private keys, and the credential encryption master key itself.

The design follows a defence-in-depth model: secrets are protected at the application layer (encryption, memory-only plaintext), the transport layer (TLS for database and API connections), the storage layer (database access controls, file permissions), and the operational layer (key separation, rotation procedures, audit logging).

This specification is the authoritative reference for secrets management. The following specs contain related sections that must remain consistent with this document:

| Specification | Related section |
|---------------|----------------|
| [`configuration/Specification.md`](../configuration/Specification.md) | § Secrets and Credentials, § Environment Variable Overrides |
| [`datastore/Specification.md`](../datastore/Specification.md) | § `credentials` table, § Credential Storage Security |
| [`web-api/Specification.md`](../web-api/Specification.md) | § Credential Management (Admin Endpoints) |
| [`chef-api/Specification.md`](../chef-api/Specification.md) | § Credentials Security |
| [`packaging/Specification.md`](../packaging/Specification.md) | § Helm Secret, § Container Configuration, § Environment File |
| [`tls/Specification.md`](../tls/Specification.md) | § Static certificate key files, § ACME storage |
| [`auth/Specification.md`](../auth/Specification.md) | § LDAP bind credentials, § Local account password hashing |

---

## Credential Types

The application manages the following categories of secrets:

| Credential Type | Storage Methods | Notes |
|-----------------|-----------------|-------|
| Chef API private key (RSA PEM) | Database, env var, file path | One per Chef server organisation. Database storage recommended for multi-org and containerised deployments. |
| LDAP bind password | Database, env var | Referenced via `bind_password_credential` (database) or `bind_password_env` (env var) in the auth config. |
| SMTP password | Database, env var | Referenced via `password_credential` (database) or `password_env` (env var) in the SMTP config. |
| Webhook URL | Database, env var | May contain authentication tokens in the URL. Referenced via `url_credential` or `url_env`. |
| Database connection string | Env var only | `DATABASE_URL`. Never stored in the database (circular dependency). |
| Credential encryption master key | Env var only | `CMM_CREDENTIAL_ENCRYPTION_KEY`. Must not reside in the same storage system as the encrypted credentials. |
| TLS private key (for `static` mode) | File path, Kubernetes Secret | Mounted at `/etc/chef-migration-metrics/tls/tls.key`. Not stored in the credentials table. |
| ACME account key | File path (auto-managed) | Stored in `acme.storage_path`. Managed by the ACME client library, not by the application's credential system. |
| Local user passwords | Database (bcrypt hash) | Stored in the `users` table as bcrypt hashes, not in the `credentials` table. Not reversible. |
| Generic secrets | Database | Catch-all type for operator-defined secrets that don't fit other categories. |

---

## Credential Resolution Precedence

When the application needs a credential, it resolves the value using this precedence order:

```
1. Database  →  2. Environment variable  →  3. File path
```

If multiple sources are configured for the same credential, the highest-precedence source wins. This allows operators to migrate incrementally from file-based to database-stored credentials without changing the config file.

### Resolution Flow

```
┌─────────────────────────────────────────────────────┐
│  Credential needed (e.g. Chef API key for org X)    │
└──────────────────────┬──────────────────────────────┘
                       ▼
        ┌──────────────────────────────┐
        │ client_key_credential_id set │
        │ in organisations table?      │
        └──────┬───────────────┬───────┘
           Yes │               │ No
               ▼               ▼
    ┌──────────────────┐  ┌────────────────────────────┐
    │ Decrypt from      │  │ client_key_env configured  │
    │ credentials table │  │ for this org in YAML?      │
    └──────────────────┘  └──────┬───────────────┬──────┘
                             Yes │               │ No
                                 ▼               ▼
                      ┌──────────────────┐  ┌──────────────────────────┐
                      │ Read from env var│  │ client_key_path set      │
                      └──────────────────┘  │ in YAML config?          │
                                            └──────┬───────────────┬───┘
                                               Yes │               │ No
                                                   ▼               ▼
                                        ┌──────────────────┐  ┌────────────────┐
                                        │ Read PEM from    │  │ ERROR:         │
                                        │ file on disk     │  │ no credential  │
                                        └──────────────────┘  │ configured     │
                                                              └────────────────┘
```

### Resolution by Credential Type

| Credential | Database reference | Env var | File path |
|------------|-------------------|---------|-----------|
| Chef API key | `client_key_credential: <name>` in org config → FK in `organisations.client_key_credential_id` | `client_key_env: VAR_NAME` in org config | `client_key_path: /path/to/key.pem` in org config |
| LDAP bind password | `bind_password_credential: <name>` in auth config | `bind_password_env: LDAP_BIND_PASSWORD` in auth config | — |
| SMTP password | `password_credential: <name>` in SMTP config | `password_env: SMTP_PASSWORD` in SMTP config | — |
| Webhook URL | `url_credential: <name>` in notification channel config | `url_env: NOTIFICATION_WEBHOOK_URL` in notification channel config | — |
| Database URL | — | `DATABASE_URL` | — |
| Master encryption key | — | `CMM_CREDENTIAL_ENCRYPTION_KEY` | — |

---

## Database Credential Storage

### Encryption Model

All credentials stored in the `credentials` table are encrypted at the application layer before being written to the database. The database never sees plaintext secret material.

| Property | Value |
|----------|-------|
| **Algorithm** | AES-256-GCM (authenticated encryption with associated data) |
| **Key derivation** | HKDF-SHA256 from the master credential encryption key |
| **IV / Nonce** | 12-byte random nonce, generated per encryption operation, stored alongside the ciphertext |
| **Associated data (AAD)** | `<credential_type>:<name>` — binds the ciphertext to its identity, preventing row-swap attacks |
| **Master key source** | `CMM_CREDENTIAL_ENCRYPTION_KEY` environment variable (Base64-encoded, ≥ 32 bytes / 256 bits) |
| **At-rest format** | `<nonce_hex>:<ciphertext_hex>` in the `encrypted_value` column |

#### Security Properties

- **Confidentiality** — AES-256-GCM encryption ensures the plaintext is unrecoverable without the master key, even if the database is compromised.
- **Integrity** — GCM's authentication tag detects any tampering with the ciphertext.
- **Binding** — The AAD ties each ciphertext to its `credential_type` and `name`, preventing an attacker with database write access from swapping encrypted values between rows.
- **Uniqueness** — A fresh random nonce per encryption means identical plaintext values produce different ciphertext, preventing comparison attacks.

### Database Schema

The `credentials` table (fully specified in the [Datastore Specification](../datastore/Specification.md)):

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `name` | TEXT | No | Unique human-readable identifier (e.g. `myorg-production-key`) |
| `credential_type` | TEXT | No | One of: `chef_client_key`, `ldap_bind_password`, `smtp_password`, `webhook_url`, `generic` |
| `encrypted_value` | TEXT | No | `<nonce_hex>:<ciphertext_hex>` |
| `metadata` | JSONB | Yes | Non-sensitive metadata (e.g. `{"key_format": "pkcs1", "bits": 2048}`). **Never** contains plaintext. |
| `last_rotated_at` | TIMESTAMPTZ | Yes | When the credential value was last updated |
| `created_by` | TEXT | No | Username of the admin who created this credential |
| `updated_by` | TEXT | Yes | Username of the admin who last updated this credential |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

### Credential Lifecycle

```
┌──────────┐     POST /api/v1/admin/credentials     ┌───────────────┐
│  Admin    │ ─────────────────────────────────────► │  Application  │
│  (Web UI) │   { name, type, value, metadata }      │               │
└──────────┘                                         │  1. Validate  │
                                                     │  2. Encrypt   │
                                                     │  3. Store     │
                                                     │  4. Log       │
                                                     │  5. Return    │
                                                     │     metadata  │
                                                     │     only      │
                                                     └───────────────┘

┌──────────┐     PUT /api/v1/admin/credentials/:name ┌───────────────┐
│  Admin    │ ─────────────────────────────────────► │  Application  │
│  (Web UI) │   { value, metadata }                   │               │
└──────────┘                                         │  1. Validate  │
                                                     │  2. Re-encrypt│
                                                     │  3. Overwrite │
                                                     │  4. Log       │
                                                     └───────────────┘

┌──────────────────────┐   Credential needed   ┌───────────────────────┐
│  Chef API signing    │ ◄──────────────────── │  credentials table    │
│  LDAP bind           │   1. Read ciphertext  │                       │
│  SMTP auth           │   2. Decrypt in mem   │  (encrypted_value)    │
│  Webhook dispatch    │   3. Use              │                       │
│                      │   4. Zero memory      │                       │
└──────────────────────┘                       └───────────────────────┘
```

---

## Master Encryption Key Management

The master key is the root of trust for all database-stored credentials. It requires special handling.

### Requirements

1. The master key must be at least 32 bytes (256 bits), Base64-encoded.
2. The master key must **never** be stored in:
   - The database (circular dependency)
   - The YAML configuration file
   - Source control
   - Log output or error messages
   - API responses
3. The master key must be provided via the `CMM_CREDENTIAL_ENCRYPTION_KEY` environment variable.
4. The master key and the encrypted credentials must **never** reside in the same storage system.

### Recommended Key Sources by Deployment Model

| Deployment | Recommended key source |
|------------|----------------------|
| Kubernetes (Helm) | Kubernetes Secret (referenced via `existingSecret` in Helm values), External Secrets Operator, Sealed Secrets, or HashiCorp Vault CSI |
| Kubernetes (external secrets) | HashiCorp Vault Agent Injector, AWS Secrets Manager via External Secrets Operator, GCP Secret Manager |
| Docker Compose | Docker secret or `.env` file (development only — not for production) |
| RPM / DEB (systemd) | `EnvironmentFile` at `/etc/sysconfig/chef-migration-metrics` (RPM) or `/etc/default/chef-migration-metrics` (DEB) with `0640` permissions |
| Manual / development | Shell environment variable |

### Key Generation

Operators should generate the master key using a cryptographically secure random source:

```sh
# Generate a 32-byte (256-bit) key, Base64-encoded
openssl rand -base64 32
```

The resulting string (e.g. `K7gNU3sdo+OL0wNhqoVWhr3g6s1xYv72ol/pe/Unols=`) is set as the environment variable value.

### Startup Behaviour

On startup, the application:

1. Reads `CMM_CREDENTIAL_ENCRYPTION_KEY` from the environment.
2. If the variable is not set and no database-stored credentials exist (no `*_credential` config references and no rows in `credentials` table): proceed without it.
3. If the variable is not set but database credentials are needed: log `ERROR` and refuse to start.
4. If the variable is set: validate length (≥ 32 bytes after Base64 decode). If invalid, log `ERROR` and refuse to start.
5. If `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` is also set: initiate key rotation (see below).

---

## Key Rotation

### Master Encryption Key Rotation

When the master encryption key needs to be rotated (e.g. scheduled rotation, suspected compromise):

1. Generate a new master key.
2. Set `CMM_CREDENTIAL_ENCRYPTION_KEY` to the **new** key.
3. Set `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` to the **old** key.
4. Restart the application.
5. On startup, the application detects both keys and:
   a. Attempts to decrypt each credential row with the new key first.
   b. If decryption fails, retries with the previous key.
   c. Re-encrypts the row using the new key.
   d. Updates `encrypted_value` and `updated_at`.
6. Logs `INFO`: `Credential encryption key rotated: <count> credentials re-encrypted`.
7. If any credential cannot be decrypted with either key, logs `ERROR` for each affected credential, marks it as unusable, and continues startup.
8. After successful startup and verification, remove `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` from the environment.

**Important:** The rotation procedure must be atomic per row (each row re-encrypted in its own transaction) so that a crash mid-rotation does not leave the system in an inconsistent state. Rows successfully re-encrypted use the new key; rows not yet processed still work with the old key on the next restart (as long as `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` is still set).

### Credential Value Rotation

Individual credential values (e.g. a Chef API key that has been regenerated on the Chef server) are rotated via:

- **Web API:** `PUT /api/v1/admin/credentials/:name` with the new plaintext value
- **Config file change:** Update `client_key_path` and restart
- **Environment variable change:** Update the env var and restart

When rotated via the Web API, the `last_rotated_at` timestamp is updated. The old ciphertext is overwritten — there is no version history for credential values.

---

## Plaintext Handling Rules

These rules apply to **all** credential storage methods (database, env var, file path):

1. **Memory lifetime** — Plaintext must only be held in a Go variable for the duration of the operation that needs it (e.g. signing a Chef API request, performing an LDAP bind, sending an SMTP `AUTH`). It must not be assigned to a package-level variable, cached in a map or struct field that outlives the operation, or stored in a sync.Pool.

2. **Zeroing** — After use, the byte slice or string holding the plaintext should be overwritten with zeros before the variable goes out of scope. While Go's garbage collector does not guarantee immediate reclamation, zeroing reduces the window of exposure. Use a helper function:

   ```go
   func zeroBytes(b []byte) {
       for i := range b {
           b[i] = 0
       }
   }
   ```

3. **No logging** — Plaintext credential values must never appear in log output at any severity level. This includes:
   - The credential value itself
   - Hex or Base64 encodings of the value
   - Substrings, prefixes, or suffixes of the value
   - Error messages that interpolate the value (e.g. `fmt.Errorf("key: %s", keyBytes)`)

4. **No API responses** — The Web API must never return plaintext or encrypted credential values. Credential list and detail endpoints return metadata only.

5. **No temporary files** — Plaintext credentials must never be written to temporary files, even briefly. When an external tool requires a file-based credential (e.g. Chef API signing), the value must be provided via an in-memory mechanism or the tool must be invoked in a way that avoids file exposure.

6. **No Elasticsearch export** — The `encrypted_value` column and any plaintext material must be excluded from all Elasticsearch NDJSON document types.

---

## File-Based Credential Security

When credentials are stored as files on disk (Chef API PEM keys, TLS private keys):

| Requirement | Detail |
|-------------|--------|
| **File permissions** | `0600` or `0400` (owner read/write or read-only). Log `WARN` on startup if more permissive. |
| **Ownership** | Must be owned by the application's service account (`chef-migration-metrics` user created by package install scripts). |
| **Directory permissions** | The `keys/` directory at `/etc/chef-migration-metrics/keys/` must be `0700`. |
| **Ignore files** | `*.pem`, `*.key`, and `keys/` patterns must appear in `.gitignore`, `.dockerignore`, and `.helmignore`. |
| **Config file references** | The YAML config references keys by path, never by inline content. |

### Standard File Locations

| File | Path | Description |
|------|------|-------------|
| Chef API key (RPM/DEB) | `/etc/chef-migration-metrics/keys/<org-name>.pem` | One PEM file per organisation |
| Chef API key (container) | `/etc/chef-migration-metrics/keys/<org-name>.pem` | Mounted from Kubernetes Secret or Docker secret |
| TLS private key | `/etc/chef-migration-metrics/tls/tls.key` | For `server.tls.mode: static` |
| TLS certificate | `/etc/chef-migration-metrics/tls/tls.crt` | For `server.tls.mode: static` |
| ACME storage | `/var/lib/chef-migration-metrics/acme/` | Auto-managed ACME account keys and certificates |
| Environment file (RPM) | `/etc/sysconfig/chef-migration-metrics` | `0640`, contains env var overrides including secrets |
| Environment file (DEB) | `/etc/default/chef-migration-metrics` | `0640`, contains env var overrides including secrets |

---

## Kubernetes Secrets Integration

### Helm Chart Secret Model

The Helm chart provides three mechanisms for injecting secrets:

#### 1. Chart-Managed Secret (`templates/secret.yaml`)

Created when `existingSecret` is not set. Renders values from `secrets.*` and `chefKeys.keys` into a Kubernetes Secret resource. Suitable for development and evaluation. **Not recommended for production** because secret values appear in Helm values (which may be stored in source control or Helm release history).

Contents:
- `DATABASE_URL` — from `secrets.databaseUrl` or auto-constructed from PostgreSQL subchart credentials
- `LDAP_BIND_PASSWORD` — from `secrets.ldapBindPassword`
- `CMM_CREDENTIAL_ENCRYPTION_KEY` — from `secrets.credentialEncryptionKey`
- `SMTP_PASSWORD` — from `secrets.smtpPassword`

#### 2. Existing Secret Reference (`existingSecret`)

When `existingSecret` is set, the chart does not create its own Secret. Instead, the Deployment references the named Secret via `envFrom`. The operator manages the Secret externally using Sealed Secrets, External Secrets Operator, Vault Agent, or `kubectl create secret`.

Expected keys in the external Secret:

| Key | Required | Description |
|-----|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | When DB credentials are used | Base64-encoded AES-256 master key |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | During key rotation only | Previous master key |
| `LDAP_BIND_PASSWORD` | When LDAP auth is configured | LDAP bind password |
| `SMTP_PASSWORD` | When email notifications are configured | SMTP password |
| `SMTP_USERNAME` | When email notifications are configured | SMTP username |

#### 3. Chef API Key Secret (`chefKeys`)

Chef API private keys are mounted as files (not env vars) because they can be multi-line PEM content.

- `chefKeys.existingSecret` — references an existing Secret where each key is a filename (e.g. `myorg-production.pem`) and the value is the PEM content. Mounted at `/etc/chef-migration-metrics/keys/`.
- `chefKeys.keys` — inline key data rendered into a chart-managed Secret. Not recommended for production.

#### 4. TLS Secret (`tlsSecret`)

For `server.tls.mode: static`:

- `tlsSecret.existingSecret` — references an existing Kubernetes TLS Secret (e.g. managed by cert-manager). Must contain `tls.crt` and `tls.key`. Mounted at `/etc/chef-migration-metrics/tls/`.
- `tlsSecret.cert` / `tlsSecret.key` — inline PEM content rendered into a chart-managed Secret. Not recommended for production.

### External Secrets Operator Pattern

For production Kubernetes deployments, the recommended pattern is:

1. Store secrets in an external secrets manager (Vault, AWS Secrets Manager, GCP Secret Manager, Azure Key Vault).
2. Deploy the External Secrets Operator (ESO) in the cluster.
3. Create an `ExternalSecret` resource that syncs from the external store to a Kubernetes Secret.
4. Reference that Kubernetes Secret via `existingSecret` and `chefKeys.existingSecret` in Helm values.

This keeps secret values out of Helm values files, Helm release history, and source control.

---

## Docker Compose Secrets

For local development and evaluation using Docker Compose:

- Sensitive environment variables are set in a `.env` file (listed in `.env.example` as a template).
- The `.env` file is listed in `.gitignore` and `.dockerignore`.
- Chef API keys are bind-mounted from a local directory into the container.
- The credential encryption master key is set via `CMM_CREDENTIAL_ENCRYPTION_KEY` in the `.env` file.

**This model is for development only.** `.env` files provide no encryption, access control, or audit trail.

---

## RPM / DEB Package Secrets

For traditional Linux installations:

- Chef API keys are placed in `/etc/chef-migration-metrics/keys/` with `0600` permissions.
- The environment file (`/etc/sysconfig/chef-migration-metrics` or `/etc/default/chef-migration-metrics`) contains sensitive env vars (`DATABASE_URL`, `CMM_CREDENTIAL_ENCRYPTION_KEY`, `LDAP_BIND_PASSWORD`). File permissions are `0640`, owned by `root:chef-migration-metrics`.
- The systemd unit file references the environment file via `EnvironmentFile=`.
- The `postinstall.sh` script sets correct ownership and permissions on the keys directory and environment file.
- The `preremove.sh` script does **not** delete credential files — this is left to the operator to avoid accidental data loss.

---

## Audit and Observability

### Logging

All credential operations are logged at `INFO` severity with the following fields:

| Field | Description |
|-------|-------------|
| `scope` | `secrets` |
| `action` | One of: `create`, `rotate`, `delete`, `test`, `decrypt`, `encrypt`, `key_rotation` |
| `credential_name` | The credential's human-readable name |
| `credential_type` | The credential type |
| `actor` | The username of the admin performing the action (for API operations) |
| `result` | `success` or `error` |

The credential **value** is never included in any log field.

Failed decryption attempts (e.g. wrong master key, tampered ciphertext) are logged at `ERROR` severity.

### System Status Endpoint

The `GET /api/v1/admin/status` endpoint includes a `credential_storage` section:

```json
{
  "credential_storage": {
    "encryption_key_configured": true,
    "total_credentials": 5,
    "credential_types": {
      "chef_client_key": 3,
      "ldap_bind_password": 1,
      "smtp_password": 1
    },
    "orphaned_credentials": 0
  }
}
```

- `encryption_key_configured` — whether `CMM_CREDENTIAL_ENCRYPTION_KEY` is set and valid.
- `orphaned_credentials` — credentials not referenced by any organisation or config entry. Non-zero values indicate cleanup may be needed.

---

## Defence in Depth Summary

| Layer | Protection |
|-------|-----------|
| **Application** | AES-256-GCM encryption with HKDF-derived key; per-row nonces; AAD binding; plaintext zeroed after use; never logged; API never returns values |
| **Database** | Standard PostgreSQL access controls; `encrypted_value` column contains only ciphertext; connection via TLS (`sslmode=verify-full` recommended) |
| **Transport** | All external connections (PostgreSQL, LDAP, SMTP, Chef API, webhooks) should use TLS |
| **Filesystem** | PEM files `0600`, key directories `0700`, env files `0640`; owned by service account |
| **Kubernetes** | Secrets are opaque resources with RBAC; `existingSecret` avoids values in Helm history; ESO pattern recommended for production |
| **Backups** | Database backups contain only ciphertext; restoring without the master key renders credentials unusable |
| **Key management** | Master key is external to the database; key and encrypted data never in the same storage system |
| **Source control** | `.gitignore`, `.dockerignore`, `.helmignore` all exclude `*.pem`, `*.key`, `.env`, `keys/` |
| **Deletion** | Credential rows are hard-deleted immediately; aggressive `VACUUM` recommended for high-security environments |

---

## Credential Deletion

When a credential is deleted (via the Web API `DELETE /api/v1/admin/credentials/:name` or when an organisation is removed):

1. The row is hard-deleted immediately. There is no soft-delete or recycle bin.
2. PostgreSQL's MVCC may retain the old row version in dead tuples until `VACUUM` runs.
3. For high-security environments, operators should configure aggressive autovacuum settings on the `credentials` table or run `VACUUM FULL` after bulk credential deletion.
4. The delete operation is blocked with HTTP `409` if the credential is still referenced by an organisation or config entry. The response lists the references so the operator can unlink them first.

---

## Validation

### Startup Validation

On startup, the application validates:

| Check | Severity | Behaviour |
|-------|----------|-----------|
| `CMM_CREDENTIAL_ENCRYPTION_KEY` set when DB credentials exist | `ERROR` | Refuse to start |
| Master key ≥ 32 bytes after Base64 decode | `ERROR` | Refuse to start |
| Each DB credential can be decrypted with current master key | `ERROR` per row | Log error, mark credential unusable, continue startup |
| Chef API key files exist and are readable | `ERROR` per org | Log error, skip organisation, continue startup |
| Chef API key file permissions ≤ `0600` | `WARN` | Log warning, continue |
| TLS key file permissions ≤ `0600` (static mode) | `WARN` | Log warning, continue |
| Keys directory permissions ≤ `0700` | `WARN` | Log warning, continue |
| Env file permissions ≤ `0640` (RPM/DEB) | `WARN` | Log warning, continue |

### API Validation

When creating or updating credentials via the Web API:

| `credential_type` | Validation |
|--------------------|------------|
| `chef_client_key` | Must be a PEM-encoded RSA private key. Key size extracted for metadata. |
| `ldap_bind_password` | Non-empty string. |
| `smtp_password` | Non-empty string. |
| `webhook_url` | Must be a valid URL with `http` or `https` scheme. |
| `generic` | Non-empty string. No format validation. |

---

## Web API Endpoints

Credential management is exposed through admin-only endpoints. Full request/response schemas are in the [Web API Specification](../web-api/Specification.md) § Credential Management.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/admin/credentials` | List all credentials (metadata only) |
| `POST` | `/api/v1/admin/credentials` | Create a new encrypted credential |
| `PUT` | `/api/v1/admin/credentials/:name` | Rotate (replace) a credential's value |
| `DELETE` | `/api/v1/admin/credentials/:name` | Delete a credential (requires `confirm=true`) |
| `POST` | `/api/v1/admin/credentials/:name/test` | Test a credential without revealing its value |

**Security invariants for all credential endpoints:**

- Require `admin` role.
- Never return `encrypted_value` or plaintext in any response.
- Return `503` if `CMM_CREDENTIAL_ENCRYPTION_KEY` is not configured.
- Log all operations at `INFO` severity with `scope: secrets`.

---

## Configuration Reference

The following YAML configuration settings relate to secrets storage. See the [Configuration Specification](../configuration/Specification.md) for the full schema.

### Master Key Configuration

```yaml
# Name of the env var containing the master encryption key.
# Only the env var NAME is stored here — never the key itself.
credential_encryption_key_env: CMM_CREDENTIAL_ENCRYPTION_KEY
```

### Per-Organisation Credential References

```yaml
organisations:
  # Database-stored credential (recommended for multi-org)
  - name: myorg-production
    chef_server_url: https://chef.example.com
    org_name: myorg-production
    client_name: chef-migration-metrics
    client_key_credential: myorg-production-key  # references credentials.name

  # File-based credential (traditional on-prem)
  - name: myorg-staging
    chef_server_url: https://chef.example.com
    org_name: myorg-staging
    client_name: chef-migration-metrics
    client_key_path: /etc/chef-migration-metrics/keys/myorg-staging.pem
```

### Auth Credential References

```yaml
auth:
  providers:
    - type: ldap
      host: ldap.example.com
      bind_dn: cn=svc-chef-metrics,ou=service-accounts,dc=example,dc=com
      # Database-stored:
      bind_password_credential: ldap-bind-password
      # Or environment variable:
      # bind_password_env: LDAP_BIND_PASSWORD
```

### SMTP Credential References

```yaml
smtp:
  host: smtp.example.com
  port: 587
  username_env: SMTP_USERNAME
  # Database-stored:
  password_credential: smtp-password
  # Or environment variable:
  # password_env: SMTP_PASSWORD
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded AES-256 master key. Required when DB credentials are used. |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | Previous master key. Required during key rotation only. |
| `DATABASE_URL` | PostgreSQL connection string. |
| `LDAP_BIND_PASSWORD` | LDAP bind password (when using env var method). |
| `SMTP_PASSWORD` | SMTP password (when using env var method). |
| `SMTP_USERNAME` | SMTP username (when using env var method). |
| `NOTIFICATION_WEBHOOK_URL` | Webhook URL (when using env var method). |

---

## Implementation Notes

### Go Package Structure

Secrets management logic lives in `internal/secrets/` (new package) with the following responsibilities:

- `encryption.go` — AES-256-GCM encrypt/decrypt with HKDF key derivation, nonce generation, AAD construction
- `store.go` — `CredentialStore` interface and database-backed implementation
- `resolver.go` — Credential resolution logic (database → env var → file path precedence)
- `rotation.go` — Master key rotation on startup
- `validation.go` — Per-type credential validation (RSA PEM parsing, URL validation, etc.)
- `zeroing.go` — Memory zeroing helpers

The `internal/secrets/` package is the only package that performs encryption/decryption operations. Other packages (`internal/chefapi/`, `internal/auth/`, `internal/notify/`) call through the `CredentialStore` interface to obtain plaintext for their operations.

### Dependencies

- `crypto/aes`, `crypto/cipher` — AES-256-GCM encryption
- `golang.org/x/crypto/hkdf` — HKDF-SHA256 key derivation
- `crypto/rand` — Nonce generation
- `crypto/x509`, `encoding/pem` — RSA key parsing and validation
- `encoding/base64` — Master key decoding
- `encoding/hex` — Nonce and ciphertext serialisation

No external cryptography libraries are required.

---

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| [`configuration/Specification.md`](../configuration/Specification.md) | YAML schema for credential references and env var overrides |
| [`datastore/Specification.md`](../datastore/Specification.md) | `credentials` table schema, encryption model, retention, deletion |
| [`web-api/Specification.md`](../web-api/Specification.md) | Admin credential CRUD + test endpoints |
| [`chef-api/Specification.md`](../chef-api/Specification.md) | Chef API signing using resolved credentials |
| [`packaging/Specification.md`](../packaging/Specification.md) | Helm Secret templates, container mounts, RPM/DEB env files |
| [`tls/Specification.md`](../tls/Specification.md) | TLS key file handling, ACME storage |
| [`auth/Specification.md`](../auth/Specification.md) | LDAP bind credential usage, local password hashing |
| [`logging/Specification.md`](../logging/Specification.md) | `secrets` log scope definition |