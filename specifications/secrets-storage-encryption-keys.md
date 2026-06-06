# Secrets Storage — Database Storage, Master Key, Rotation & Plaintext Handling

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

The `credentials` table (fully specified in the [Datastore Specification](datastore.md)):

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `name` | TEXT | No | Unique human-readable identifier (e.g. `myorg-production-key`) |
| `credential_type` | TEXT | No | One of: `chef_client_key`, `smtp_password`, `webhook_url`, `generic` |
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
│  SMTP auth           │   1. Read ciphertext  │                       │
│  Webhook dispatch    │   2. Decrypt in mem   │  (encrypted_value)    │
│                      │   3. Use              │                       │
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

1. **Memory lifetime** — Plaintext must only be held in a Go variable for the duration of the operation that needs it (e.g. signing a Chef API request, sending an SMTP `AUTH`). It must not be assigned to a package-level variable, cached in a map or struct field that outlives the operation, or stored in a sync.Pool.

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
