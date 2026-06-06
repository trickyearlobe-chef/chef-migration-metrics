# Secrets Storage — Validation, Web API, Configuration & Implementation

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
    - type: saml
      idp_metadata_url: https://idp.example.com/saml/metadata
      sp_entity_id: chef-migration-metrics
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

The `internal/secrets/` package is the only package that performs encryption/decryption operations. Other packages (`internal/chefapi/`, `internal/auth/`) call through the `CredentialStore` interface to obtain plaintext for their operations. `internal/notify/` will also use this interface once the notification subsystem is implemented (currently planned, not yet built).

### Dependencies

- `crypto/aes`, `crypto/cipher` — AES-256-GCM encryption
- `golang.org/x/crypto/hkdf` — HKDF-SHA256 key derivation
- `crypto/rand` — Nonce generation
- `crypto/x509`, `encoding/pem` — RSA key parsing and validation
- `encoding/base64` — Master key decoding
- `encoding/hex` — Nonce and ciphertext serialisation

No external cryptography libraries are required.
