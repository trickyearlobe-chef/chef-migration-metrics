# Web API — Filter Option Endpoints

## Filter Option Endpoints

These endpoints return the distinct values available for each filter dimension, enabling the frontend to populate filter dropdowns dynamically.

### `GET /api/v1/filters/environments`

**Query parameters:** `organisation` (optional, scopes to one or more orgs).

**Response (200):**

```json
{
  "data": ["production", "staging", "development", "qa"]
}
```

### `GET /api/v1/filters/roles`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["base", "webserver", "database", "monitoring"]
}
```

### `GET /api/v1/filters/tags`

Distinct Chef node tags seen across collected snapshots. Prefix-filterable for typeahead and capped server-side — the response is always a bounded, count-ranked page, never the full set. Reflects ingested snapshots only, not a Chef-server-wide tag list. See [node-tags.md](node-tags.md).

**Query parameters:** `organisation` (optional), `q` (optional prefix filter for typeahead).

**Response (200):**

```json
{
  "data": ["prepare", "upgrade", "rollback", "eu-west", "pci"]
}
```

### `GET /api/v1/filters/policy-names`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["webserver", "database", "base"]
}
```

### `GET /api/v1/filters/policy-groups`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["production", "staging", "development"]
}
```

### `GET /api/v1/filters/platforms`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": [
    { "platform": "ubuntu", "versions": ["20.04", "22.04"] },
    { "platform": "centos", "versions": ["7", "8"] },
    { "platform": "windows", "versions": ["2019", "2022"] }
  ]
}
```

### `GET /api/v1/filters/target-chef-versions`

Returns the configured target Chef Client versions.

**Response (200):**

```json
{
  "data": ["18.5.0", "19.0.0"]
}
```

---

### `GET /api/v1/filters/complexity-labels`

Returns the available complexity labels for filtering.

**Response (200):**

```json
{
  "data": ["low", "medium", "high", "critical"]
}
```

---
