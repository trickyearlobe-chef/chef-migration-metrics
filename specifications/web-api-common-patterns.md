# Web API — Common Patterns

## Common Patterns

### Pagination

All list endpoints support cursor-based or offset-based pagination. The default and maximum page size are configurable; defaults are shown below.

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | `1` | Page number (1-indexed) |
| `per_page` | integer | `50` | Items per page (max: `500`) |

**Response envelope:**

All paginated responses use a consistent envelope:

```json
{
  "data": [ ... ],
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 4832,
    "total_pages": 97
  }
}
```

### Filtering

Filter parameters are passed as query string parameters. Multiple values for the same filter are comma-separated. All filters are applied server-side; the full dataset is never returned to the client.

**Common filter parameters (available on most list endpoints):**

| Parameter | Type | Description |
|-----------|------|-------------|
| `organisation` | string | Comma-separated list of organisation names |
| `environment` | string | Comma-separated list of Chef environment names |
| `role` | string | Comma-separated list of role names |
| `policy_name` | string | Comma-separated list of Policyfile policy names |
| `policy_group` | string | Comma-separated list of Policyfile policy groups |
| `platform` | string | Comma-separated list of platform names |
| `platform_version` | string | Comma-separated list of platform versions |
| `target_chef_version` | string | Target Chef Client version for readiness evaluation |
| `cookbook_status` | string | `active`, `unused`, or `all` (default: `active`) |
| `stale_status` | string | `all`, `stale`, or `fresh` (default: `all`). Filters nodes by stale check-in status. |
| `complexity_label` | string | Comma-separated list of complexity labels: `low`, `medium`, `high`, `critical`. Filters cookbooks by remediation complexity. |
| `owner` | string | Comma-separated list of owner names. Filters to entities owned by specified owners (see [Ownership Specification](ownership.md) § 4.5). Only active when ownership is enabled. |
| `unowned` | boolean | When `true`, filters to entities with no resolved owner. Cannot be combined with `owner`. Only active when ownership is enabled. |

### Sorting

Sortable endpoints accept:

| Parameter | Type | Description |
|-----------|------|-------------|
| `sort` | string | Field name to sort by |
| `order` | string | `asc` or `desc` (default: `asc`) |

### Error Responses

All error responses use a consistent structure:

```json
{
  "error": "not_found",
  "message": "Node 'web-01' was not found in organisation 'myorg-production'."
}
```

Standard error codes:

| Status | `error` value | Meaning |
|--------|---------------|---------|
| `400` | `bad_request` | Malformed request, invalid parameters |
| `401` | `unauthorized` | Missing or invalid session token |
| `403` | `forbidden` | Authenticated but insufficient role |
| `404` | `not_found` | Requested resource does not exist |
| `422` | `validation_error` | Request body fails validation |
| `429` | `rate_limited` | Too many requests |
| `500` | `internal_error` | Unexpected server error |

---
