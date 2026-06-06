# Secrets Storage — Audit, Defence in Depth & Deletion

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
    "total_credentials": 4,
    "credential_types": {
      "chef_client_key": 3,
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
| **Transport** | All external connections (PostgreSQL, SMTP, Chef API, webhooks) should use TLS |
| **Filesystem** | PEM files `0600`, key directories `0700`, env files `0640`; owned by service account |
| **Backups** | Database backups contain only ciphertext; restoring without the master key renders credentials unusable |
| **Key management** | Master key is external to the database; key and encrypted data never in the same storage system |
| **Source control** | `.gitignore` and `.dockerignore` exclude `*.pem`, `*.key`, `.env`, `keys/` |
| **Deletion** | Credential rows are hard-deleted immediately; aggressive `VACUUM` recommended for high-security environments |

---

## Credential Deletion

When a credential is deleted (via the Web API `DELETE /api/v1/admin/credentials/:name` or when an organisation is removed):

1. The row is hard-deleted immediately. There is no soft-delete or recycle bin.
2. PostgreSQL's MVCC may retain the old row version in dead tuples until `VACUUM` runs.
3. For high-security environments, operators should configure aggressive autovacuum settings on the `credentials` table or run `VACUUM FULL` after bulk credential deletion.
4. The delete operation is blocked with HTTP `409` if the credential is still referenced by an organisation or config entry. The response lists the references so the operator can unlink them first.
