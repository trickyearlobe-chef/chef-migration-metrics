# Web API — Organisation Endpoints

## Organisation Endpoints

### List Organisations

#### `GET /api/v1/organisations`

Returns all configured Chef Infra Server organisations.

**Response (200):**

```json
{
  "data": [
    {
      "name": "myorg-production",
      "chef_server_url": "https://chef.example.com",
      "org_name": "myorg-production",
      "client_name": "chef-migration-metrics",
      "credential_source": "file",
      "source": "config",
      "node_count": 2000,
      "last_collected_at": "2024-06-15T12:00:00Z",
      "last_collection_status": "success"
    },
    {
      "name": "myorg-staging",
      "chef_server_url": "https://chef.example.com",
      "org_name": "myorg-staging",
      "client_name": "chef-migration-metrics",
      "credential_source": "database",
      "credential_name": "myorg-staging-key",
      "source": "api",
      "node_count": 500,
      "last_collected_at": "2024-06-15T12:00:00Z",
      "last_collection_status": "partial_failure"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `credential_source` | One of `file`, `database`, `environment`. Indicates how the Chef API key is supplied. |
| `credential_name` | Name of the database credential (only present when `credential_source` is `database`). |
| `source` | One of `config`, `api`. Whether the org was defined in the YAML config or created via the Web API. |

> **Security note:** The API never returns the private key value, file path, or any portion of the key material. Only the credential source and name are disclosed.

> **Config/table synchronisation:** `config`-sourced organisations live in the
> `config_store` `organisations` section (the write model) and are reconciled into
> the operational `organisations` table (the read model the collector enumerates).
> This reconciliation runs on **every** change to the org config section — not only
> at startup — and then triggers a collection, so a newly added org is collected
> without a restart. Reconciliation upserts each configured org and removes
> `source='config'` rows no longer present; `source='api'` rows are never touched.

### Create Organisation

#### `POST /api/v1/organisations`

Creates a new Chef Infra Server organisation with its API credentials stored in the database. Requires the `admin` role.

The private key is encrypted using AES-256-GCM before storage. After this call, the plaintext key is not retrievable via any API endpoint.

**Request:**

```json
{
  "name": "myorg-staging",
  "chef_server_url": "https://chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics",
  "client_key_pem": "<PEM-encoded RSA private key>"
|-------|----------|-------------|
| `name` | Yes | Unique friendly name for this organisation |
| `chef_server_url` | Yes | Base URL of the Chef Infra Server |
| `org_name` | Yes | Organisation name on the Chef server |
| `client_name` | Yes | Chef API client name |
| `client_key_pem` | Yes | PEM-encoded RSA private key. Validated as a parseable RSA key before storage. |

**Response (201):**

```json
{
  "name": "myorg-staging",
  "chef_server_url": "https://chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics",
  "credential_source": "database",
  "credential_name": "myorg-staging-key",
  "source": "api"
}
```

The `client_key_pem` value is **never** included in the response.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `client_key_pem` is not a valid PEM-encoded RSA private key |
| `409` | An organisation with this `name` or `(chef_server_url, org_name)` already exists |
| `422` | Required fields missing or invalid |
| `503` | Credential encryption key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is not configured — database credential storage is unavailable |

### Update Organisation

#### `PUT /api/v1/organisations/:name`

Updates an existing organisation. Requires the `admin` role. Only API-sourced organisations can be fully updated; config-sourced organisations allow only credential rotation (the `client_key_pem` field).

**Request:**

```json
{
  "chef_server_url": "https://new-chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics-v2",
  "client_key_pem": "<PEM-encoded RSA private key>" Only provided fields are updated. If `client_key_pem` is provided, the stored credential is re-encrypted with the new value.

**Response (200):** Updated organisation object (same shape as the create response).

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `client_key_pem` is not a valid PEM-encoded RSA private key |
| `403` | Attempted to modify `chef_server_url`, `org_name`, or `client_name` on a config-sourced organisation |
| `404` | Organisation not found |
| `503` | Credential encryption key not configured |

### Test Organisation Credentials

#### `POST /api/v1/organisations/:name/test`

Validates that the stored credentials can successfully authenticate to the Chef Infra Server by making a lightweight API call (`GET /organizations/<org>/nodes?rows=0`). Requires the `admin` role.

**Response (200):**

```json
{
  "name": "myorg-staging",
  "status": "ok",
  "message": "Successfully authenticated to Chef Infra Server",
  "server_version": "15.9.38"
}
```

**Response (200, failure):**

```json
{
  "name": "myorg-staging",
  "status": "error",
  "message": "Authentication failed: 401 Unauthorized"
}
```

> This endpoint always returns 200 (the *test* succeeded in running) — the `status` field indicates whether the *credentials* are valid. This avoids ambiguity between "test couldn't run" (5xx) and "credentials are bad" (status: error).

### Delete Organisation

#### `DELETE /api/v1/organisations/:name`

Deletes an API-sourced organisation and its associated database credential. Config-sourced organisations cannot be deleted via the API. Requires the `admin` role.

**Response (204):** No content.

**Errors:**

| Status | Condition |
|--------|-----------|
| `403` | Organisation is config-sourced (must be removed from the YAML config file) |
| `404` | Organisation not found |

> **Cascade behaviour:** Deleting an organisation removes all associated `collection_runs`, `node_snapshots`, `cookbooks`, analysis results, and other dependent data via foreign key cascades. The associated credential row in the `credentials` table is also deleted. This is an irreversible operation — the API should require confirmation (e.g. a `confirm=true` query parameter).

---
