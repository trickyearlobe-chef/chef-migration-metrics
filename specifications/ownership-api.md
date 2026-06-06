# Ownership — Web API Endpoints (Owner & Assignment Management)

## 4. Web API Endpoints

### 4.1 Owner Management

#### `GET /api/v1/owners`

List all owners.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `owner_type` | string | Filter by owner type |
| `search` | string | Search by name or display name (case-insensitive substring match) |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 25, max: 100) |

**Response (200):**

```json
{
  "data": [
    {
      "name": "web-platform",
      "display_name": "Web Platform Team",
      "contact_email": "web-platform@example.com",
      "contact_channel": "#web-platform-ops",
      "owner_type": "team",
      "metadata": { "department": "engineering" },
      "assignment_counts": {
        "node": 45,
        "cookbook": 12,
        "git_repo": 3,
        "role": 5,
        "policy": 2
      },
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 25,
    "total_items": 8,
    "total_pages": 1
  }
}
```

**Authorisation:** `viewer`, `operator`, `admin`

---

#### `POST /api/v1/owners`

Create a new owner.

**Request body:**

```json
{
  "name": "payments-team",
  "display_name": "Payments Team",
  "contact_email": "payments@example.com",
  "contact_channel": "#payments-oncall",
  "owner_type": "team",
  "metadata": { "cost_centre": "CC-1234" }
}
```

**Response (201):** Returns the created owner object.

**Validation:**
- `name` is required, must be unique, must match `^[a-z0-9][a-z0-9._-]*$` (lowercase, alphanumeric, dots, underscores, hyphens; must start with alphanumeric).
- `owner_type` is required, must be one of the valid types.
- `display_name`, `contact_email`, `contact_channel`, `metadata` are optional.

**Authorisation:** `operator`, `admin`

---

#### `PUT /api/v1/owners/:name`

Update an existing owner.

**Request body:** Same fields as POST (except `name` which is the URL parameter and cannot be changed).

**Response (200):** Returns the updated owner object.

**Authorisation:** `operator`, `admin`

---

#### `DELETE /api/v1/owners/:name`

Delete an owner and all associated ownership assignments (cascading).

**Response (204):** No content.

**Authorisation:** `admin`

---

#### `GET /api/v1/owners/:name`

Get a single owner with detailed assignment summary and migration progress.

**Response (200):**

```json
{
  "name": "web-platform",
  "display_name": "Web Platform Team",
  "contact_email": "web-platform@example.com",
  "contact_channel": "#web-platform-ops",
  "owner_type": "team",
  "metadata": { "department": "engineering" },
  "assignment_counts": {
    "node": 45,
    "cookbook": 12,
    "git_repo": 3,
    "role": 5,
    "policy": 2
  },
  "readiness_summary": {
    "target_chef_version": "18.5.0",
    "total_nodes": 45,
    "ready": 30,
    "blocked": 12,
    "stale": 3,
    "blocking_cookbooks": [
      {
        "cookbook_name": "acme-web",
        "complexity_label": "medium",
        "affected_node_count": 8
      }
    ]
  },
  "cookbook_summary": {
    "total": 12,
    "compatible": 9,
    "incompatible": 2,
    "untested": 1
  },
  "git_repo_summary": {
    "total": 3,
    "compatible": 2,
    "incompatible": 1
  },
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-15T10:00:00Z"
}
```

**Authorisation:** `viewer`, `operator`, `admin`

---

### 4.2 Ownership Assignment Management

#### `GET /api/v1/owners/:name/assignments`

List all assignments for a specific owner.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `entity_type` | string | Filter by entity type |
| `organisation` | string | Filter by organisation name |
| `assignment_source` | string | Filter by source (`manual`, `auto_rule`, `import`) |
| `page` | integer | Page number |
| `per_page` | integer | Items per page |

**Response (200):**

```json
{
  "data": [
    {
      "id": "a1b2c3d4-...",
      "entity_type": "node",
      "entity_key": "web-prod-01.example.com",
      "organisation": "myorg-production",
      "assignment_source": "auto_rule",
      "auto_rule_name": "web-prod-nodes",
      "confidence": "inferred",
      "notes": null,
      "created_at": "2024-01-15T10:00:00Z"
    },
    {
      "id": "e5f6g7h8-...",
      "entity_type": "cookbook",
      "entity_key": "acme-web",
      "organisation": null,
      "assignment_source": "manual",
      "auto_rule_name": null,
      "confidence": "definitive",
      "notes": "Maintained by web platform team per JIRA PLAT-1234",
      "created_at": "2024-01-14T09:00:00Z"
    }
  ],
  "pagination": { ... }
}
```

**Authorisation:** `viewer`, `operator`, `admin`

---

#### `POST /api/v1/owners/:name/assignments`

Create one or more ownership assignments.

**Request body:**

```json
{
  "assignments": [
    {
      "entity_type": "cookbook",
      "entity_key": "acme-web",
      "organisation": null,
      "notes": "Maintained by web platform team"
    },
    {
      "entity_type": "node",
      "entity_key": "web-prod-01.example.com",
      "organisation": "myorg-production",
      "notes": null
    }
  ]
}
```

**Response (201):**

```json
{
  "created": 2,
  "assignments": [ ... ]
}
```

**Validation:**
- `entity_type` must be one of the valid types.
- `entity_key` is required and non-empty.
- `organisation` is optional; if provided, must match an existing organisation name.
- Duplicate assignments (same owner, entity type, entity key, organisation) return `409 Conflict`.

**Authorisation:** `operator`, `admin`

---

#### `DELETE /api/v1/owners/:name/assignments/:id`

Delete a specific ownership assignment.

**Response (204):** No content.

**Authorisation:** `operator`, `admin`

---

#### `POST /api/v1/ownership/reassign`

Bulk reassign ownership assignments from one owner to another. This is the primary mechanism for handling team reorganisations, staff departures, or ownership handovers. All matching assignments are moved from the source owner to the target owner in a single operation.

**Request body:**

```json
{
  "from_owner": "old-platform-team",
  "to_owner": "new-platform-team",
  "entity_type": null,
  "organisation": null,
  "delete_source_owner": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_owner` | string | Yes | Name of the owner to reassign from |
| `to_owner` | string | Yes | Name of the owner to reassign to. Must already exist. |
| `entity_type` | string | No | Limit reassignment to a specific entity type (`node`, `cookbook`, `git_repo`, `role`, `policy`). If null, all assignment types are reassigned. |
| `organisation` | string | No | Limit reassignment to assignments scoped to a specific organisation. If null, all organisations (including cross-org assignments) are included. |
| `delete_source_owner` | boolean | No | If `true`, delete the source owner after all assignments have been moved. Default: `false`. Useful when an individual leaves or a team is dissolved. |

**Response (200):**

```json
{
  "reassigned": 47,
  "skipped": 2,
  "from_owner": "old-platform-team",
  "to_owner": "new-platform-team",
  "source_owner_deleted": false
}
```

**Behaviour:**
- Both `from_owner` and `to_owner` must exist. Returns `404` if either does not.
- `from_owner` and `to_owner` must be different. Returns `400` if they are the same.
- If the target owner already has an assignment for the same `(entity_type, entity_key, organisation_id)`, the duplicate is skipped (not treated as an error). The `skipped` count in the response reflects these duplicates.
- Reassigned assignments retain their original `entity_type`, `entity_key`, `organisation_id`, and `notes`. The `assignment_source` is changed to `manual`, the `confidence` is set to `definitive`, and the `auto_rule_name` is cleared — because a human explicitly decided to move these assignments.
- If `delete_source_owner` is `true` and all assignments were successfully moved (or skipped as duplicates), the source owner is deleted. If the source owner still has remaining assignments not covered by the filter (e.g. `entity_type` was specified and the source owner has other entity types), the source owner is **not** deleted and `source_owner_deleted` is `false`.
- Each reassigned assignment generates an audit log entry (see § 4.4).
- The `delete_source_owner` option is only available to `admin` users. If an `operator` sends `delete_source_owner: true`, the request returns `403 Forbidden`.

**Authorisation:** `operator`, `admin`

---

#### `GET /api/v1/cookbooks/:name/committers`

List git committers for a cookbook's source repository. Only available for git-sourced cookbooks. Returns the committer history collected from the repository, sorted by most recent commit first.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `since` | string | ISO 8601 date. Only include committers with commits after this date. Default: no limit. |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 25, max: 100) |
| `sort` | string | Sort field: `last_commit_at` (default), `commit_count`, `author_name` |
| `order` | string | `asc` or `desc` (default: `desc`) |

**Response (200):**

```json
{
  "cookbook_name": "nginx",
  "git_repo_url": "https://gitlab.example.com/cookbooks/nginx.git",
  "data": [
    {
      "author_name": "Jane Smith",
      "author_email": "jsmith@example.com",
      "commit_count": 47,
      "first_commit_at": "2022-03-10T09:15:00Z",
      "last_commit_at": "2024-06-12T16:42:00Z"
    },
    {
      "author_name": "Bob Chen",
      "author_email": "bchen@example.com",
      "commit_count": 23,
      "first_commit_at": "2023-01-05T11:30:00Z",
      "last_commit_at": "2024-05-28T14:10:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 25,
    "total_items": 2,
    "total_pages": 1
  }
}
```

Returns `404` if the cookbook is not git-sourced or does not exist.

**Authorisation:** `viewer`, `operator`, `admin`

---

#### `POST /api/v1/cookbooks/:name/committers/assign`

Assign one or more committers from the cookbook's git repo as owners. Creates owner records for committers that don't already exist as owners, then creates `git_repo` ownership assignments linking them to the repository.

**Request body:**

```json
{
  "committers": [
    {
      "author_email": "jsmith@example.com",
      "owner_name": "jsmith",
      "display_name": "Jane Smith"
    },
    {
      "author_email": "bchen@example.com",
      "owner_name": "bchen",
      "display_name": "Bob Chen"
    }
  ]
}
```

**Behaviour:**

- For each committer, if an owner with the given `owner_name` does not exist, it is created with `owner_type = 'individual'` and `contact_email` set to the committer's `author_email`.
- If an owner with the given `owner_name` already exists, it is reused (not modified).
- A `git_repo` ownership assignment is created linking each owner to the cookbook's `git_repo_url`, with `assignment_source = 'manual'` and `confidence = 'definitive'`.
- Duplicate assignments (owner already assigned to this repo) are skipped.

**Response (200):**

```json
{
  "owners_created": 1,
  "assignments_created": 2,
  "skipped": 0
}
```

Returns `404` if the cookbook is not git-sourced or does not exist.

**Authorisation:** `operator`, `admin`

---

#### `GET /api/v1/ownership/lookup`

Look up ownership for a specific entity. Returns all resolved owners for the entity using the resolution precedence from § 1.3.

**Query parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `entity_type` | string | Yes | Entity type to look up |
| `entity_key` | string | Yes | Entity key to look up |
| `organisation` | string | No | Organisation name for scoped lookup |

**Response (200):**

```json
{
  "entity_type": "node",
  "entity_key": "web-prod-01.example.com",
  "organisation": "myorg-production",
  "owners": [
    {
      "name": "web-platform",
      "display_name": "Web Platform Team",
      "assignment_source": "manual",
      "confidence": "definitive",
      "resolution": "direct"
    },
    {
      "name": "acme-platform",
      "display_name": "ACME Platform Team",
      "assignment_source": "auto_rule",
      "confidence": "inferred",
      "resolution": "git_repo_inherited",
      "inherited_from": {
        "entity_type": "git_repo",
        "entity_key": "https://gitlab.example.com/cookbooks/nginx.git"
      }
    }
  ]
}
```

**Authorisation:** `viewer`, `operator`, `admin`

---
