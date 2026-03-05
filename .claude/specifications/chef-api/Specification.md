# Chef Infra Server API - Technical Specification

This document captures the Chef Infra Server API endpoints and usage patterns relevant to the Chef Migration Metrics project. All API calls must conform to this specification and the official documentation at https://docs.chef.io/server/api_chef_server.

---

## TL;DR

Native Go implementation of Chef Server API authentication (RSA-signed headers, no external libs). Endpoints used: **partial search** (`POST /organizations/ORG/search/node`) for node collection, **cookbook download** (`GET /organizations/ORG/cookbooks/NAME/VERSION`), and **role listing/detail** (`GET /organizations/ORG/roles[/NAME]`) for dependency graphs. Partial search collects: name, environment, chef version, platform, filesystem, cookbooks, run_list, roles, policy_name, policy_group, ohai_time. Pagination via `start`/`rows` query params (default rows: 1000). Known quirks documented (e.g. `total` count inaccuracy with partial search). See `data-collection/Specification.md` for how these endpoints are orchestrated.

---

## Authentication

All requests to the Chef Infra Server API must be signed using the client's RSA private key. The following HTTP headers are required on every request:

| Header | Description |
|--------|-------------|
| `Accept` | Must be set to `application/json` |
| `Content-Type` | Required for `PUT` and `POST` requests. Must be set to `application/json` |
| `User-Agent` | Identifies the calling application. See [Client Identification](#client-identification) below |
| `X-Chef-Version` | The version of Chef Infra Client making the request. e.g. `17.0.0` |
| `X-Ops-Sign` | Set to `algorithm=sha1;version=1.0` (v1.0) or `version=1.3` (v1.3) |
| `X-Ops-Timestamp` | UTC timestamp in ISO-8601 format e.g. `2024-01-01T00:00:00Z` |
| `X-Ops-UserId` | The name of the API client whose private key is used to sign the request |
| `X-Ops-Content-Hash` | Base64-encoded SHA-256 hash of the request body |
| `X-Ops-Authorization-N` | One or more 60-character Base64-encoded segments of the RSA-signed canonical header |
| `X-Ops-Server-API-Version` | The Chef Infra Server API version to use. Use `1` |

### Signing Protocol

The recommended signing protocol is **version 1.3** (SHA-256, supported on Chef Infra Server 12.4.0+). The canonical header string to sign is formed by concatenating the following fields in order:

```
Method:HTTP_METHOD
Path:PATH
X-Ops-Content-Hash:HASHED_BODY
X-Ops-Sign:version=1.3
X-Ops-Timestamp:TIME
X-Ops-UserId:USERID
X-Ops-Server-API-Version:API_VERSION
```

The concatenated string is signed using the client RSA private key with SHA-256 hashing and PKCS1v15 padding, then Base64-encoded and split into 60-character `X-Ops-Authorization-N` headers.

Signing MUST be implemented natively within this project without relying on external libraries such as `mixlib-authentication`. This keeps the Chef API client self-contained and avoids introducing external runtime dependencies for a core concern.

### Client Identification

All API requests MUST include a `User-Agent` header that identifies the application, its version, and the specific organisation collection job that is running. This allows Chef server administrators to identify the source of API calls in server logs without needing to cross-reference client key names.

The `User-Agent` header should follow this format:

```
chef-migration-metrics/<APP_VERSION> (org:<ORG_NAME>)
```

For example:

```
chef-migration-metrics/1.0.0 (org:mycompany-prod)
```

The `X-Ops-UserId` header serves a complementary role — it must be set to the name of the dedicated API client configured for this tool (see [Credentials Security](#credentials-security)). Administrators can therefore identify traffic by either the `User-Agent` or the client name in the Chef server access logs.

It is recommended that the dedicated API client be named consistently, e.g. `chef-migration-metrics`, so that it is immediately recognisable in Chef server audit logs.

---

## Base URL

All organisation-scoped endpoints are prefixed with:

```
https://<CHEF_SERVER_HOST>/organizations/<ORG_NAME>
```

---

## Endpoints Used by This Project

### Node Collection

#### Note: JSON Structure Differences Between Node Endpoints

The JSON structure returned for a node differs depending on which endpoint is used:

- **`GET /organizations/<ORG_NAME>/nodes/<NODE_NAME>`** returns the node object with attributes nested under their attribute level keys: `automatic`, `normal`, `default`, and `override`.
- **Full and partial search** (`POST .../search/node`) returns a flattened structure where automatic attributes are **hoisted to the root** of each node's `data` object. For example, `automatic.platform` is accessed as `platform` in the search response, not `automatic.platform`.

This means partial search attribute paths must be written as if addressing the hoisted structure. For example, to retrieve `automatic.chef_packages.chef.version`, the path in the partial search request body is `["automatic", "chef_packages", "chef", "version"]`, but the key is returned at the top level of `data` under whatever alias you assign it (e.g. `"chef_version"`).

When writing code that processes node data, take care not to assume a consistent structure between data retrieved via `GET /nodes/:name` and data retrieved via search.

---

#### Partial Search — Node Index

Used to efficiently collect node attributes at scale. Only the specified attributes are returned, minimising payload size and Chef server load.

```
POST /organizations/<ORG_NAME>/search/node?q=*:*&rows=1000&start=0
```

**Request body** — specifies the attributes to return as dot-notation paths:

```json
{
  "name":             ["name"],
  "chef_environment": ["chef_environment"],
  "chef_version":     ["automatic", "chef_packages", "chef", "version"],
  "platform":         ["automatic", "platform"],
  "platform_version": ["automatic", "platform_version"],
  "platform_family":  ["automatic", "platform_family"],
  "filesystem":       ["automatic", "filesystem"],
  "cookbooks":        ["automatic", "cookbooks"],
  "run_list":         ["run_list"],
  "roles":            ["automatic", "roles"],
  "policy_name":      ["policy_name"],
  "policy_group":     ["policy_group"],
  "ohai_time":        ["automatic", "ohai_time"]
}
```

**Attribute notes:**

- **`cookbooks`** (`automatic.cookbooks`) contains the fully-resolved set of cookbooks applied to the node after the Chef client run, keyed by cookbook name with version and other metadata. This is the preferred source for determining which cookbooks a node uses, as it is deduplicated and version-resolved. Use this for cookbook usage analysis.
- **`run_list`** contains the unexpanded run list entries (roles and recipes) as specified on the node or role. A single cookbook may appear multiple times across run list entries (once per recipe). This is not suitable for cookbook enumeration but is useful for generating Test Kitchen configurations for cookbooks downloaded from the Chef server, where knowing which specific recipes to run matters.
- **`recipes`** is omitted as `cookbooks` supersedes it for cookbook discovery purposes.
- **`policy_name`** and **`policy_group`** are top-level node attributes (not under `automatic`) present on nodes managed via Policyfiles. For classic nodes (roles and run-lists), these fields are null. The partial search paths are `["policy_name"]` and `["policy_group"]` respectively. These attributes enable dashboard filtering by policy name and policy group.
- **`ohai_time`** (`automatic.ohai_time`) is a Unix timestamp (seconds since epoch, as a float) recording when Ohai last ran on the node. This is used to detect stale nodes whose data may be outdated. Nodes whose `ohai_time` is older than the configured `collection.stale_node_threshold_days` are flagged as stale.

**Response:**

```json
{
  "total": 1234,
  "start": 0,
  "rows": [
    {
      "url": "https://<CHEF_SERVER>/organizations/<ORG>/nodes/<NODE>",
      "data": {
        "name": "web-node-01",
        "chef_environment": "production",
        "chef_version": "17.10.0",
        "platform": "ubuntu",
        "platform_version": "22.04",
        "platform_family": "debian",
        "filesystem": { "/": { "kb_available": 102400, ... } },
        "cookbooks": {
          "nginx":   { "version": "5.1.0" },
          "base":    { "version": "1.3.2" },
          "apt":     { "version": "7.4.0" }
        },
        "run_list": ["role[base]", "recipe[nginx::default]", "recipe[nginx::config]"],
        "roles": ["base", "webserver"],
        "policy_name": null,
        "policy_group": null,
        "ohai_time": 1718444400.123456
      }
    }
  ]
}
```

**Pagination:**

The `total` field indicates the total number of matching nodes. Use the `start` query parameter to paginate through results in batches (recommended batch size: 1000).

```
POST /organizations/<ORG_NAME>/search/node?q=*:*&rows=1000&start=0
POST /organizations/<ORG_NAME>/search/node?q=*:*&rows=1000&start=1000
POST /organizations/<ORG_NAME>/search/node?q=*:*&rows=1000&start=2000
```

Continue paginating until `start >= total`.

---

### Cookbook Fetching

#### List All Cookbooks

Returns all cookbooks and their available versions for an organisation.

```
GET /organizations/<ORG_NAME>/cookbooks?num_versions=all
```

**Response:**

```json
{
  "nginx": {
    "url": "https://<CHEF_SERVER>/organizations/<ORG>/cookbooks/nginx",
    "versions": [
      { "url": "https://.../cookbooks/nginx/5.1.0", "version": "5.1.0" },
      { "url": "https://.../cookbooks/nginx/4.0.0", "version": "4.0.0" }
    ]
  }
}
```

#### Download a Specific Cookbook Version

Returns the metadata and file manifest for a specific cookbook version. Individual cookbook files must be downloaded separately using the URLs returned in the manifest.

```
GET /organizations/<ORG_NAME>/cookbooks/<COOKBOOK_NAME>/<VERSION>
```

`VERSION` may be `_latest` to retrieve the most recent version.

**Response** includes `metadata`, `recipes`, `attributes`, `templates`, `files`, `libraries`, `resources`, `providers`, `definitions`, and `root_files`, each as an array of objects with `name`, `path`, `checksum`, and `url` fields.

---

### Organisation Discovery

#### List Organisations

Returns all organisations on the Chef Infra Server. Accessible only by the `pivotal` superuser.

```
GET /organizations
```

**Response:**

```json
{
  "org1": "https://<CHEF_SERVER>/organizations/org1",
  "org2": "https://<CHEF_SERVER>/organizations/org2"
}
```

#### Get Organisation Details

```
GET /organizations/<ORG_NAME>
```

**Response:**

```json
{
  "name": "org1",
  "full_name": "Organisation One",
  "guid": "f980d1asdfda0331235s00ff36862"
}
```

---

### Environment and Role Enumeration

These endpoints may be used to enrich node data and support dashboard filtering.

#### List Environments

```
GET /organizations/<ORG_NAME>/environments
```

#### List Roles

```
GET /organizations/<ORG_NAME>/roles
```

#### Get Role Details

```
GET /organizations/<ORG_NAME>/roles/<ROLE_NAME>
```

Returns the role's `run_list`, `env_run_lists`, `default_attributes`, and `override_attributes`.

---

## Implementation Notes

### Efficiency

- **Always use partial search** for node collection. Full node objects can be very large; fetching unnecessary data wastes bandwidth and increases Chef server load.
- **Paginate all search requests** using `rows` and `start`. Do not assume all results are returned in a single response.
- **Cache organisation and role listings** within a collection run to avoid redundant API calls.

### Error Handling

| HTTP Status | Meaning | Action |
|-------------|---------|--------|
| `200` / `201` | Success | Continue |
| `401` | Authentication failure | Check client name, key, and clock skew |
| `403` | Authorisation failure | Check client permissions in the organisation |
| `404` | Resource not found | Log and skip; do not abort the collection run |
| `413` | Request too large | Reduce batch size |
| `429` | Rate limited | Back off and retry with exponential delay |
| `5xx` | Server error | Retry with backoff; log and continue to next org if persistent |

### Clock Skew

The Chef Infra Server rejects requests where the `X-Ops-Timestamp` differs from the server's clock by more than **15 minutes**. Ensure the host running the collection job has NTP synchronisation enabled.

### Credentials Security

- Private key material must **never** be stored in source control.
- Each organisation must have its own dedicated API client and key.
- Three credential storage methods are supported, in order of resolution precedence:

  1. **Database** — The RSA private key is stored encrypted (AES-256-GCM) in the `credentials` table and referenced by the organisation via `client_key_credential_id`. The application decrypts the key in memory at the point of request signing and discards it immediately after. This is the recommended approach for multi-organisation and containerised deployments. See the [Datastore Specification](../datastore/Specification.md) for the encryption model and the [Web API Specification](../web-api/Specification.md) for the credential management endpoints.
  2. **Environment variable** — For container orchestrators that inject secrets (Kubernetes Secrets, ECS task definitions). The environment variable name is referenced in the configuration.
  3. **File path** — The traditional approach: `client_key_path` in the YAML configuration file points to a PEM file on disk. The file must be readable only by the application's service account (`0600` or `0400` permissions).

- Regardless of storage method, the plaintext key must only be held in memory for the duration of the signing operation. It must not be cached in a long-lived in-memory store, written to temporary files, or included in log output, error messages, or API responses.
- When using database storage, the credential encryption master key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) must be managed separately from the database — typically as a Kubernetes Secret, a HashiCorp Vault value, or a host-level environment variable. The master key and the encrypted credentials must never reside in the same storage system.
- Key rotation (both the Chef API client key and the credential encryption master key) must be possible without application downtime. See the [Configuration Specification](../configuration/Specification.md) for the rotation procedure.

---

## Known Bugs and Quirks

This section documents discovered bugs, unexpected behaviours, and undocumented quirks in the Chef Infra Server API that affect this project. When a new issue is discovered during development or operation, it must be recorded here with enough detail to explain the workaround.

Each entry should include:
- The affected endpoint
- A description of the unexpected behaviour
- The Chef Infra Server version(s) where the issue has been observed (if known)
- The workaround applied in this project

---

*No bugs or quirks have been documented yet. Add entries here as they are discovered.*