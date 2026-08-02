# Ownership — Web API Endpoints (Bulk Import, Audit Log & Owner Filter)

### 4.3 Bulk Import

#### `POST /api/v1/ownership/import`

Bulk import ownership assignments from CSV or JSON.

**Request:** `multipart/form-data` with a `file` field containing the import data and a `format` field (`csv` or `json`).

**CSV format:**

```csv
owner,entity_type,entity_key,organisation,notes
web-platform,cookbook,acme-web,,Maintained by web platform team
web-platform,git_repo,https://gitlab.example.com/team-web/acme-web.git,,Web platform team repo
web-platform,node,web-prod-01.example.com,myorg-production,
payments-team,policy,payment-app,,
```

**JSON format:**

```json
{
  "assignments": [
    {
      "owner": "web-platform",
      "entity_type": "cookbook",
      "entity_key": "acme-web",
      "organisation": null,
      "notes": "Maintained by web platform team"
    }
  ]
}
```

**Response (200):**

```json
{
  "imported": 42,
  "skipped": 3,
  "errors": [
    {
      "line": 7,
      "error": "Owner 'unknown-team' does not exist"
    }
  ]
}
```

**Behaviour:**
- Owners referenced in the import must already exist. Lines referencing non-existent owners are skipped and reported as errors.
- Duplicate assignments are skipped (not treated as errors).
- All successfully parsed assignments are created with `assignment_source = 'import'` and `confidence = 'definitive'`.
- The import is **not** transactional — successfully parsed lines are imported even if others fail. The response reports the full outcome.

**Authorisation:** `operator`, `admin`

**Size limit:** Imports are limited to 10,000 rows per request. Larger imports should be split into multiple requests.

---

### 4.4 Audit Log

#### `GET /api/v1/ownership/audit-log`

Query the ownership audit log. Returns entries in reverse chronological order (most recent first).

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | Filter by action type (comma-separated for multiple) |
| `actor` | string | Filter by actor username |
| `owner_name` | string | Filter by owner name |
| `entity_type` | string | Filter by entity type |
| `entity_key` | string | Filter by entity key (exact match) |
| `since` | string | ISO 8601 datetime. Only return entries after this time. |
| `until` | string | ISO 8601 datetime. Only return entries before this time. |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 50, max: 200) |

**Response (200):**

```json
{
  "data": [
    {
      "id": "f1a2b3c4-...",
      "timestamp": "2024-06-15T14:30:00Z",
      "action": "assignment_reassigned",
      "actor": "admin@example.com",
      "owner_name": "new-platform-team",
      "entity_type": "cookbook",
      "entity_key": "acme-web",
      "organisation": null,
      "details": {
        "from_owner": "old-platform-team",
        "to_owner": "new-platform-team",
        "previous_source": "auto_rule",
        "new_source": "manual"
      }
    },
    {
      "id": "a5b6c7d8-...",
      "timestamp": "2024-06-15T14:29:55Z",
      "action": "owner_created",
      "actor": "admin@example.com",
      "owner_name": "new-platform-team",
      "entity_type": null,
      "entity_key": null,
      "organisation": null,
      "details": {
        "owner_type": "team"
      }
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 234,
    "total_pages": 5
  }
}
```

**Authorisation:** `viewer`, `operator`, `admin`

---

### 4.5 Owner Filter on Existing Endpoints

The `owner` query parameter is added to all existing list and dashboard endpoints as an additional filter dimension:

| Parameter | Type | Description |
|-----------|------|-------------|
| `owner` | string | Comma-separated list of owner names. Filters results to entities owned by the specified owners (using ownership resolution from § 1.3). |
| `unowned` | boolean | When `true`, filters to entities with no resolved owner. Default: `false`. Cannot be combined with `owner`. |

**Affected endpoints:**
- `GET /api/v1/dashboard/version-distribution`
- `GET /api/v1/dashboard/readiness`
- `GET /api/v1/dashboard/cookbook-compatibility`
- `GET /api/v1/nodes`
- `GET /api/v1/nodes/by-version/:chef_version`
- `GET /api/v1/nodes/by-cookbook/:cookbook_name`
- `GET /api/v1/cookbooks`
- `GET /api/v1/remediation/priority`
- `GET /api/v1/remediation/summary`
- `GET /api/v1/dependency-graph`
- `GET /api/v1/dependency-graph/table`
- `GET /api/v1/exports` (POST — as a filter in the request body)

---

### 4.6 Identity Management

Behaviour and intent: [ownership-identity.md](ownership-identity.md). This section is the endpoint
contract only.

#### `POST /api/v1/ownership/merge`

Fold one owner into another. Unlike `/ownership/reassign`, which moves assignments and leaves the
source owner and its aliases in place, a merge moves the identities as well and removes the source
— which is what makes a correction survive the next ingest.

**Request body:**

```json
{
  "from_owner": "fat-tommy",
  "into_owner": "thomas-smith"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_owner` | string | Yes | The owner being folded away. Must exist. |
| `into_owner` | string | Yes | The surviving owner. Must exist, and must differ from `from_owner`. |

**Response (200):** the counts of what moved — assignments reassigned, assignments the target
already held (removed rather than duplicated), aliases moved, aliases dropped, and whether the
source owner's own name was added to the target as a `custom` alias. The authoritative shape is
`datastore.MergeOwnersResult`.

**Behaviour:**
- All of it happens in one transaction, or none of it does.
- An alias the target already answers to is dropped rather than moved — the uniqueness constraint
  on `(alias_type, alias_value)` is global.
- The source owner's name is seeded as a `custom` alias of the target, unless that value is already
  taken. Not seeding it is not an error.
- Writes an `owner_merged` audit entry against the surviving owner.
- `404` if either owner is unknown; `400` if either is missing or they are the same.

**Authorisation:** `admin` — a merge deletes an owner, which is what `DELETE /api/v1/owners/:name`
requires.

---

#### `GET /api/v1/ownership/duplicates`

The stored list of owner pairs that may be the same person, strongest first.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `min_similarity` | number | Floor on the similarity score. Clamped up to the scan's own floor — the list cannot report pairs the scan never recorded. |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page |

**Response (200):** `data` (the pairs, shape `datastore.OwnerDuplicateCandidate`), `pagination`,
plus:

| Field | Description |
|-------|-------------|
| `scan` | When the catalogue was last scanned and how many pairs that scan found. **Absent when it has never been scanned** — which is not the same as a scan that found nothing. |
| `scan_running` | Whether a scan is running now. The list is the previous one until it finishes. |
| `coverage` | `owners_total` and `owners_without_alias`, so the reader can see how much of the catalogue is compared by name alone. Absent if the count could not be taken — a failed count must not blank the list. |

**Authorisation:** `viewer`, `operator`, `admin`

---

#### `POST /api/v1/ownership/duplicates/rescan`

Rebuild the stored duplicate list.

**Response (202):** `{"started": true}`, or `{"started": false, "reason": "..."}` when a scan is
already running.

**Behaviour:**
- Returns as soon as the scan has been **started**. It walks every owner and every alias, which is
  tens of seconds on a large catalogue — long enough that holding the request open invites a proxy
  timeout and a retry that would start a second scan.
- Only one scan runs at a time.
- Readers keep the previous list until the new one commits.

**Authorisation:** `operator`, `admin`
