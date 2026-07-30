# Web API — Admin Endpoints

## Admin Endpoints

All endpoints in this section require the `admin` role.

### Credential Management

These endpoints manage encrypted credentials stored in the database. Credentials are used for Chef API private keys and other generic secrets. All credential values are encrypted at the application layer using AES-256-GCM before storage — see the [Datastore Specification](datastore.md) for the encryption model.

> **Security principles:**
> - The API **never** returns the plaintext or encrypted value of a credential in any response.
> - Credential values can be created and replaced but never read back.
> - A "test" endpoint validates that a credential works without revealing its value.
> - All credential operations are logged at `INFO` severity with the credential name, type, and acting user — but never the value.

#### `GET /api/v1/admin/credentials`

Lists all stored credentials (metadata only, never values).

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `type` | Filter by `credential_type` (e.g. `chef_client_key`, `generic`) |

**Response (200):**

```json
{
  "data": [
    {
      "name": "myorg-production-key",
      "credential_type": "chef_client_key",
      "metadata": { "key_format": "pkcs1", "bits": 2048 },
      "referenced_by": ["organisation:myorg-production"],
      "last_rotated_at": "2024-06-01T10:00:00Z",
      "created_by": "alice",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-06-01T10:00:00Z"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `referenced_by` | List of entities that reference this credential. Helps identify impact before rotation or deletion. Format: `organisation:<name>` for Chef keys, `config:<path>` for YAML config references. |
| `last_rotated_at` | When the credential value was last changed (null if never rotated since creation). |

#### `POST /api/v1/admin/credentials`

Creates a new encrypted credential.

**Request:**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "value": "<PEM-encoded RSA private key>",
  "metadata": { "key_format": "pkcs1", "bits": 2048 }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique name for this credential |
| `credential_type` | Yes | One of: `chef_client_key`, `generic` |
| `value` | Yes | The plaintext credential value. Validated per type (e.g. RSA key must be parseable, URL must be valid). Encrypted before storage. **Never logged.** |
| `metadata` | No | Non-sensitive metadata object. Must not contain the credential value. |

**Response (201):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "metadata": { "key_format": "pkcs1", "bits": 2048 },
  "created_by": "alice",
  "created_at": "2024-06-15T16:00:00Z"
}
```

The `value` field is **never** included in the response.

**Validation per credential type:**

| `credential_type` | Validation |
|--------------------|------------|
| `chef_client_key` | Must be a PEM-encoded RSA private key. Parsed to extract key size for metadata. |
| `generic` | Non-empty string. No format validation. |

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Value fails type-specific validation (e.g. invalid PEM) |
| `409` | A credential with this name already exists |
| `422` | Required fields missing or `credential_type` is not a recognised value |
| `503` | Credential encryption key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is not configured |

#### `PUT /api/v1/admin/credentials/:name`

Rotates (replaces) the value of an existing credential. The new value is encrypted and stored; the old ciphertext is overwritten.

**Request:**

```json
{
  "value": "<new PEM-encoded RSA private key>",
  "metadata": { "key_format": "pkcs1", "bits": 4096 }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `value` | Yes | The new plaintext credential value. Validated and encrypted before storage. |
| `metadata` | No | Updated metadata. If omitted, existing metadata is preserved. |

**Response (200):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "metadata": { "key_format": "pkcs1", "bits": 4096 },
  "last_rotated_at": "2024-06-15T17:00:00Z",
  "updated_by": "alice",
  "updated_at": "2024-06-15T17:00:00Z"
}
```

The `value` field is **never** included in the response.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Value fails type-specific validation |
| `404` | Credential not found |
| `503` | Credential encryption key not configured |

> **Note:** The `credential_type` cannot be changed after creation. To change the type, delete and re-create the credential.

#### `DELETE /api/v1/admin/credentials/:name`

Deletes a credential. The encrypted value is permanently removed from the database.

**Request query parameters:**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `confirm` | Yes | Must be `true`. Prevents accidental deletion. |

**Response (204):** No content.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `confirm=true` not provided |
| `404` | Credential not found |
| `409` | Credential is still referenced by one or more organisations or config entries. The response body lists the references. Must unlink before deleting. |

**Response (409):**

```json
{
  "error": "conflict",
  "message": "Credential is still referenced and cannot be deleted",
  "referenced_by": ["organisation:myorg-staging"]
}
```

#### `POST /api/v1/admin/credentials/:name/test`

Tests a stored credential by performing a lightweight validation appropriate to its type. The credential is decrypted in memory for the duration of the test and then discarded.

| `credential_type` | Test action |
|--------------------|------------|
| `chef_client_key` | Parse the RSA key and verify it can produce a valid signature. If linked to an organisation, optionally make a test API call to the Chef server. |
| `generic` | Verify the credential can be decrypted (confirms master key is correct). |

**Response (200):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "status": "ok",
  "message": "RSA key is valid (2048-bit). Chef server authentication succeeded."
}
```

**Response (200, failure):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "status": "error",
  "message": "RSA key is valid but Chef server authentication failed: 401 Unauthorized"
}
```

> Like the organisation test endpoint, this always returns HTTP 200 — the `status` field indicates whether the credential is valid.

---

### User Management

#### `GET /api/v1/admin/users`

Returns a list of local user accounts.

**Response (200):**

```json
{
  "data": [
    {
      "username": "alice",
      "display_name": "Alice Smith",
      "email": "alice@example.com",
      "role": "admin",
      "provider": "local",
      "locked": false,
      "created_at": "2024-01-15T10:00:00Z",
      "last_login_at": "2024-06-15T08:30:00Z"
    }
  ],
  "pagination": { ... }
}
```

#### `POST /api/v1/admin/users`

Creates a new local user account.

**Request:**

```json
{
  "username": "bob",
  "display_name": "Bob Jones",
  "email": "bob@example.com",
  "password": "s3cur3Pa$$w0rd",
  "role": "viewer"
}
```

**Response (201):**

```json
{
  "username": "bob",
  "display_name": "Bob Jones",
  "email": "bob@example.com",
  "role": "viewer",
  "provider": "local",
  "locked": false,
  "created_at": "2024-06-15T16:00:00Z"
}
```

**Errors:**

| Status | Condition |
|--------|-----------|
| `409` | Username already exists |
| `422` | Validation error (password too short, invalid role, etc.) |

#### `PUT /api/v1/admin/users/:username`

Updates an existing user account (display name, email, role, locked status). Password changes use a separate endpoint.

**Request:**

```json
{
  "display_name": "Robert Jones",
  "role": "admin",
  "locked": false
}
```

**Response (200):** Updated user object.

#### `PUT /api/v1/admin/users/:username/password`

Resets a user's password.

**Request:**

```json
{
  "password": "newS3cur3Pa$$w0rd"
}
```

**Response (204):** No content.

#### `DELETE /api/v1/admin/users/:username`

Deletes a local user account. SAML users cannot be deleted via this endpoint.

**Response (204):** No content.

### Rescan

#### `POST /api/v1/admin/rescan-all-cookstyle`

Destructive, fleet-wide, and irreversible without re-scanning. Documented for what it
destroys rather than for its response shape:

- Deletes **all** CookStyle results, complexity records and auto-correct previews, for
  both server cookbooks and git repos.
- Clears the materialised cookstyle and compatibility verdicts on git repos, so the repo
  list cannot keep showing a verdict whose backing rows are gone. Test Kitchen verdicts
  are preserved — a CookStyle rescan does not invalidate kitchen results.
- Marks every server cookbook for reprocessing, then triggers an immediate collection run.

It does **not** re-download cookbooks. A populated, version-keyed cache directory is
authoritative regardless of download status, so the cost is CookStyle CPU rather than
network — bounded by `concurrency.cookstyle_scan`. At fleet scale the run takes hours,
during which results are absent and pages read as untested.

### System Status

#### `GET /api/v1/admin/status`

Returns the system health status including datastore connectivity, credential encryption status, last collection run times, and pending job information.

**Response (200):**

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "datastore": {
    "status": "connected",
    "pending_migrations": 0
  },
  "credential_storage": {
    "encryption_key_configured": true,
    "total_credentials": 4,
    "credential_types": {
      "chef_client_key": 3
    },
    "orphaned_credentials": 0
  },
  "collection": {
    "next_run_at": "2024-06-15T13:00:00Z",
    "last_run_at": "2024-06-15T12:00:00Z",
    "last_run_status": "completed"
  },
  "organisations": [
    {
      "name": "myorg-production",
      "credential_source": "file",
      "last_collected_at": "2024-06-15T12:00:00Z",
      "status": "completed",
      "node_count": 2000
    },
    {
      "name": "myorg-staging",
      "credential_source": "database",
      "last_collected_at": "2024-06-15T12:00:00Z",
      "status": "completed",
      "node_count": 500
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `credential_storage.encryption_key_configured` | Whether `CMM_CREDENTIAL_ENCRYPTION_KEY` is set and valid. If `false`, database credential operations are unavailable. |
| `credential_storage.total_credentials` | Total number of credentials in the database. |
| `credential_storage.credential_types` | Breakdown by `credential_type`. |
| `credential_storage.orphaned_credentials` | Credentials not referenced by any organisation or config. May be candidates for cleanup. |
| `status` | Overall health: `healthy`, or `degraded` when the datastore is unreachable or migrations are pending. A missing encryption key alone is not degraded (file-credential deployments are valid). |
| `datastore.status` | `connected` or `error`. |
| `collection.last_run_status` / `organisations[].status` | The latest collection run's status (`completed`, `failed`, `running`, `interrupted`), `never_collected` if the organisation has no runs yet, or `unknown` if the status could not be read. |
| `organisations[].credential_source` | `database` when the org uses a stored credential, else `file`. |

---
