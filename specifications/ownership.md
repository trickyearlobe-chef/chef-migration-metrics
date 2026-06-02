# Ownership Tracking - Component Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

> Component specification for Ownership Tracking in Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

Ownership tracking lets organisations assign **owners** (teams, individuals, or cost centres) to nodes, roles, policyfiles, cookbooks, and git repositories so that migration progress, remediation work, and upgrade readiness can be viewed, filtered, and exported per owner. Ownership can be assigned manually via the Web API/UI, imported from CSV/JSON, or auto-derived from Chef node attributes, policy names, git repo URL patterns, or configurable attribute paths. For git-sourced cookbooks, committer history is collected from the repository and surfaced in the cookbook detail view so that operators can identify and select contributors as owners. Bulk reassignment moves all (or a filtered subset of) assignments from one owner to another in a single operation, supporting team reorganisations and staff departures. All ownership mutations are recorded in an append-only audit log with the acting user, timestamp, and change details. Both `definitive` and `inferred` owners are displayed for every entity — the UI visually distinguishes them but does not suppress lower-priority owners. Owners are stored in a dedicated `owners` table with many-to-many `ownership_assignments` linking owners to the entities they are responsible for. The feature is optional — when no owners are configured, all existing behaviour is unchanged. Related specs: `datastore/`, `web-api/`, `visualisation/`, `configuration/`, `data-collection/`.

---

## Overview

Large Chef estates are managed by multiple teams. When planning a Chef Client upgrade project, a common question is: **"Which team owns this cookbook / node / policy, and who needs to do the remediation work?"** Without ownership data, the migration dashboard shows a flat view of all entities, making it difficult to delegate work, track progress per team, or report to management by organisational unit.

The Ownership Tracking feature adds:

1. **Owner entities** — named owners representing teams, individuals, business units, or cost centres.
2. **Ownership assignments** — many-to-many links between owners and nodes, cookbooks, git repositories, roles, and policyfiles.
3. **Auto-derivation rules** — configurable rules that automatically assign ownership based on Chef attributes, naming conventions, policy metadata, or git repository URL patterns.
4. **Bulk import** — CSV and JSON import of ownership mappings for bootstrapping from external CMDBs or spreadsheets.
5. **Bulk reassignment** — move all (or a filtered subset of) assignments from one owner to another, supporting team reorganisations and staff departures.
6. **Audit log** — a record of all ownership changes (assignments created, removed, reassigned, owners created/updated/deleted) with the acting user and timestamp.
7. **Owner-scoped views** — filtering, grouping, and exporting all dashboard data by owner.

---

## 1. Owner Model

### 1.1 Owner Entity

An owner represents a responsible party. Owners are lightweight — they carry a name, optional contact information, and optional metadata.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | UUID | Yes | Primary key |
| `name` | TEXT | Yes | Unique human-readable name (e.g. `platform-team`, `app-payments`, `sre-emea`, `jsmith`) |
| `display_name` | TEXT | No | Friendly display name (e.g. `Platform Engineering Team`) |
| `contact_email` | TEXT | No | Contact email for notifications and reports |
| `contact_channel` | TEXT | No | Slack channel, Teams channel, or other contact reference |
| `owner_type` | TEXT | Yes | One of: `team`, `individual`, `business_unit`, `cost_centre`, `custom` |
| `metadata` | JSONB | No | Arbitrary key-value metadata (e.g. `{"department": "engineering", "region": "emea"}`) |
| `created_at` | TIMESTAMPTZ | Yes | Row creation time |
| `updated_at` | TIMESTAMPTZ | Yes | Last update time |

**Unique constraints:**
- `name`

### 1.2 Ownership Assignments

Ownership assignments are many-to-many links between owners and the entities they are responsible for. A single entity can have multiple owners (shared ownership), and a single owner can own many entities.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | UUID | Yes | Primary key |
| `owner_id` | UUID | Yes | FK → `owners.id` |
| `entity_type` | TEXT | Yes | One of: `node`, `cookbook`, `git_repo`, `role`, `policy` |
| `entity_key` | TEXT | Yes | Identifier for the entity. Format depends on `entity_type` (see below) |
| `organisation_id` | UUID | No | FK → `organisations.id`. Scopes the assignment to a specific Chef org. Null means the assignment applies across all organisations. |
| `assignment_source` | TEXT | Yes | One of: `manual`, `auto_rule`, `import`. Indicates how this assignment was created. |
| `auto_rule_name` | TEXT | No | Name of the auto-derivation rule that created this assignment (null for manual/import assignments) |
| `confidence` | TEXT | Yes | One of: `definitive`, `inferred`. Manual and import assignments are `definitive`; auto-derived assignments are `inferred`. |
| `notes` | TEXT | No | Optional free-text notes about this assignment |
| `created_at` | TIMESTAMPTZ | Yes | Row creation time |
| `updated_at` | TIMESTAMPTZ | Yes | Last update time |

**Entity key formats by type:**

| `entity_type` | `entity_key` format | Examples |
|----------------|---------------------|----------|
| `node` | Node name | `web-prod-01.example.com` |
| `cookbook` | Cookbook name (version-agnostic — ownership applies to all versions of a cookbook) | `nginx`, `base_hardening` |
| `git_repo` | Git repository URL (as stored in `cookbooks.git_repo_url`) | `https://gitlab.example.com/cookbooks/nginx.git`, `git@github.com:acme/base_hardening.git` |
| `role` | Role name | `web-server`, `database` |
| `policy` | Policy name | `web-app`, `payment-app` |

> **Git repo vs. cookbook ownership:** The system assumes a 1:1 mapping between a git repository and a cookbook — each repo contains exactly one cookbook. Assigning ownership to a `git_repo` entity means the team is responsible for the repository itself — including CI/CD, branching strategy, and code review. Assigning ownership to a `cookbook` entity means the team is responsible for that cookbook's compatibility and remediation. In practice these often resolve to the same team, but both entity types are supported so that ownership can be assigned at whichever level is natural for the organisation. When resolving cookbook ownership, a direct `cookbook` assignment takes precedence over an inherited `git_repo` assignment (see § 1.3).

**Unique constraints:**
- `(owner_id, entity_type, entity_key, organisation_id)` — prevents duplicate assignments. The unique constraint must handle the nullable `organisation_id` so that two assignments differing only by one having a null organisation are treated as distinct.

### 1.3 Ownership Resolution

When determining the owner(s) of an entity, the system resolves ownership using the following precedence:

1. **Direct assignment** — An explicit assignment matching the entity type and key.
2. **Git-repo-inherited (cookbooks only)** — If the entity is a cookbook with no direct owner and it is git-sourced, inherit ownership from the cookbook's `git_repo_url` if a `git_repo` assignment exists.
3. **Policy-inherited (nodes only)** — If the entity is a node with no direct owner, inherit ownership from the node's `policy_name` if a `policy` assignment exists.
4. **Unowned** — If no ownership can be resolved, the entity is marked as unowned.

Resolution precedence for determining which assignment takes priority when multiple match:

| Priority | Source | Confidence |
|----------|--------|------------|
| 1 (highest) | `manual` | `definitive` |
| 2 | `import` | `definitive` |
| 3 | `auto_rule` | `inferred` |

Within the same source, a more specific match takes priority over a broader one.

Ownership resolution is computed at query time, not materialised. This avoids complex synchronisation when nodes change policies.

### 1.4 Owner Display

All resolved owners for an entity are returned by the API and displayed in the UI, regardless of confidence level. Both `definitive` and `inferred` owners are listed. The UI visually distinguishes them — `definitive` owners are displayed with a solid badge and `inferred` owners with a dashed-outline badge — so that operators can see at a glance which assignments are confirmed and which were auto-derived. The precedence order from § 1.3 determines the **sort order** (definitive first, inferred second) but does not suppress lower-priority owners from the response.

> **Future consideration:** A later iteration may introduce a mode where only the highest-precedence owner is displayed and lower-precedence owners are collapsed behind a disclosure control. For now, all owners are always visible.

---

## 2. Auto-Derivation Rules

Auto-derivation rules allow ownership to be assigned automatically based on patterns in Chef data. Rules are evaluated after each collection run and produce `auto_rule` / `inferred` assignments.

### 2.1 Rule Types

| Rule Type | Description | Example |
|-----------|-------------|---------|
| `node_attribute` | Match nodes by a value at a configurable attribute path in the node's collected data | Assign all nodes with `automatic.cloud.provider = "aws"` to owner `cloud-team` |
| `node_name_pattern` | Match nodes by a regex pattern on the node name | Assign all nodes matching `^web-prod-.*` to owner `web-platform` |
| `policy_match` | Match nodes by policy name (exact or pattern) | Assign all nodes with `policy_name = "payment-app"` to owner `payments-team` |
| `cookbook_name_pattern` | Match cookbooks by a regex pattern on the cookbook name | Assign all cookbooks matching `^acme-.*` to owner `acme-platform` |
| `git_repo_url_pattern` | Match git repositories by a regex pattern on the repository URL | Assign all repos matching `gitlab.example.com/team-web/.*` to owner `web-platform` |
| `role_match` | Match roles by name (exact or pattern) | Assign role `web-server` to owner `web-platform` |

### 2.2 Rule Configuration

Auto-derivation rules are defined in the YAML configuration file:

```yaml
ownership:
  enabled: true
  auto_rules:
    - name: aws-nodes-to-cloud-team
      owner: cloud-team
      type: node_attribute
      attribute_path: automatic.cloud.provider
      match_value: "aws"

    - name: web-prod-nodes
      owner: web-platform
      type: node_name_pattern
      pattern: "^web-prod-.*"

    - name: payment-policy
      owner: payments-team
      type: policy_match
      policy_name: "payment-app"

    - name: acme-cookbooks
      owner: acme-platform
      type: cookbook_name_pattern
      pattern: "^acme-.*"

    - name: web-team-repos
      owner: web-platform
      type: git_repo_url_pattern
      pattern: "gitlab\\.example\\.com/team-web/.*"

    - name: web-server-role
      owner: web-platform
      type: role_match
      pattern: "web-server"
```

**Rule fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the rule (used in `ownership_assignments.auto_rule_name`) |
| `owner` | string | Yes | Name of the owner to assign. Must match an existing owner. |
| `type` | string | Yes | One of the rule types from § 2.1 |
| `attribute_path` | string | Conditional | Dot-separated path into the node's collected attributes (required for `node_attribute` type) |
| `match_value` | string | Conditional | Value to match at the attribute path (required for `node_attribute` type). Supports exact match only. |
| `pattern` | string | Conditional | Regex pattern (required for `node_name_pattern`, `cookbook_name_pattern`, `git_repo_url_pattern`, `role_match` types; optional for `policy_match`) |
| `policy_name` | string | Conditional | Exact policy name to match (required for `policy_match` type) |
| `organisation` | string | No | Limit this rule to a specific organisation name. If omitted, the rule applies across all organisations. |

### 2.3 Rule Evaluation

- Auto-derivation rules are evaluated **after each collection run completes** for the affected organisation(s).
- For each rule, the system queries the latest node snapshots (or cookbooks, git repo URLs, or roles depending on rule type) and generates `ownership_assignments` for matching entities.
- For `git_repo_url_pattern` rules, the system queries the distinct `git_repo_url` values from the `cookbooks` table where `source = 'git'` and matches the regex against each URL.
- Existing `auto_rule` assignments from the same rule that no longer match are **deleted** (the entity no longer matches the pattern). This ensures auto-derived ownership stays in sync with the current state.
- Existing `manual` or `import` assignments are **never modified or deleted** by auto-derivation. Manual assignments always take precedence.
- Rule evaluation must be logged at `DEBUG` severity with the `ownership` scope, including the rule name, match count, and any errors.
- If a rule references an owner name that does not exist, the rule is skipped and a `WARN` log is emitted.

### 2.4 Node Attribute Access for Auto-Derivation

For `node_attribute` rules, the system needs access to node attributes beyond the standard collected set (§ 1.4 of the Data Collection specification). The `attribute_path` field specifies a dot-separated path into the node's data.

**Supported attribute paths:**

- Paths starting with `automatic.` are resolved against the node's automatic attributes (already collected via partial search).
- The standard collected attributes (`automatic.platform`, `automatic.chef_packages.chef.version`, etc.) are always available.
- For attributes not in the standard set, the collector must extend the partial search query to include the requested attribute paths. The ownership configuration is read at collector startup, and any `attribute_path` values from `node_attribute` rules are merged into the partial search keys.

**Implementation:**

- At startup, scan all `ownership.auto_rules` entries of type `node_attribute`.
- Extract the `attribute_path` values and add them to the partial search key map sent to the Chef API.
- Store the additional attributes in a new `custom_attributes` JSONB column on the `node_snapshots` table (see § 3).
- At rule evaluation time, resolve the `attribute_path` against the combined standard and custom attributes.

---

## 3. Datastore Changes

### 3.1 New Tables

#### `owners`

Stores named owners. Owner names must be unique.

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `name` | TEXT | No | Unique human-readable owner name |
| `display_name` | TEXT | Yes | Friendly display name |
| `contact_email` | TEXT | Yes | Contact email |
| `contact_channel` | TEXT | Yes | Contact channel reference |
| `owner_type` | TEXT | No | One of: `team`, `individual`, `business_unit`, `cost_centre`, `custom` |
| `metadata` | JSONB | Yes | Arbitrary metadata |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

---

#### `ownership_assignments`

Links owners to the entities they are responsible for. An owner can have many assignments; an entity can have multiple owners.

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `owner_id` | UUID | No | FK → `owners.id` |
| `entity_type` | TEXT | No | One of: `node`, `cookbook`, `git_repo`, `role`, `policy` |
| `entity_key` | TEXT | No | Identifier for the owned entity |
| `organisation_id` | UUID | Yes | FK → `organisations.id`. Null = cross-org assignment. |
| `assignment_source` | TEXT | No | One of: `manual`, `auto_rule`, `import` |
| `auto_rule_name` | TEXT | Yes | Name of the auto-derivation rule (null for manual/import) |
| `confidence` | TEXT | No | One of: `definitive`, `inferred` |
| `notes` | TEXT | Yes | Optional notes |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

The combination of `(owner_id, entity_type, entity_key, organisation_id)` must be unique to prevent duplicate assignments.

Deleting an owner cascades to all its assignments. Deleting an organisation cascades to all assignments scoped to that organisation.

#### `git_repo_committers`

Stores committer information extracted from git repository history. Updated during each collection run for git-sourced cookbooks. This data supports the committer sub-page on the cookbook detail view, where operators can identify active contributors and assign them as owners.

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `git_repo_url` | TEXT | No | Git repository URL (matches `cookbooks.git_repo_url`) |
| `author_name` | TEXT | No | Committer's name as recorded in git |
| `author_email` | TEXT | No | Committer's email as recorded in git |
| `commit_count` | INTEGER | No | Total number of commits by this author |
| `first_commit_at` | TIMESTAMPTZ | No | Timestamp of this author's earliest commit |
| `last_commit_at` | TIMESTAMPTZ | No | Timestamp of this author's most recent commit |
| `collected_at` | TIMESTAMPTZ | No | When this data was last refreshed |

The combination of `(git_repo_url, author_email)` must be unique. Rows are replaced in full on each collection run for a given repository.

#### `ownership_audit_log`

Append-only log of all ownership changes. Every mutation to the `owners` and `ownership_assignments` tables generates one or more audit log entries. Rows are never updated or deleted by application code — only time-based retention purges old entries.

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `timestamp` | TIMESTAMPTZ | No | When the action occurred |
| `action` | TEXT | No | One of: `owner_created`, `owner_updated`, `owner_deleted`, `assignment_created`, `assignment_deleted`, `assignment_reassigned` |
| `actor` | TEXT | No | Username of the authenticated user who performed the action, or `system` for auto-derivation and startup cleanup operations |
| `owner_name` | TEXT | No | Name of the owner involved |
| `entity_type` | TEXT | Yes | Entity type of the assignment (null for owner-level actions like `owner_created`, `owner_updated`) |
| `entity_key` | TEXT | Yes | Entity key of the assignment (null for owner-level actions) |
| `organisation` | TEXT | Yes | Organisation name for org-scoped assignments (null for cross-org or owner-level actions) |
| `details` | JSONB | Yes | Additional context. Contents vary by action type (see below). |

**`details` field contents by action type:**

| Action | `details` contents |
|--------|-------------------|
| `owner_created` | `{"owner_type": "team"}` |
| `owner_updated` | `{"changed_fields": ["display_name", "contact_email"]}` — list of fields that were modified |
| `owner_deleted` | `{"assignments_cascaded": 12}` — number of assignments deleted by cascade |
| `assignment_created` | `{"assignment_source": "manual", "confidence": "definitive"}` |
| `assignment_deleted` | `{"assignment_source": "manual", "confidence": "definitive"}` |
| `assignment_reassigned` | `{"from_owner": "old-team", "to_owner": "new-team", "previous_source": "auto_rule", "new_source": "manual"}` |

The `ownership_audit_log` table is not subject to the same retention rules as other ownership data. It has its own configurable retention period (see § 6).

### 3.2 Modified Tables

#### `node_snapshots` — New Field

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `custom_attributes` | JSONB | Yes | Additional node attributes collected for auto-derivation rules. Keyed by the dot-separated attribute path. |

### 3.3 Entity Relationships

Ownership assignments use soft references (by name/key) rather than foreign keys to the entity tables. This allows ownership to be pre-assigned for entities that haven't been collected yet (e.g. assigning ownership of a cookbook or git repository before the first collection run).

The `entity_type` determines which existing entity the assignment relates to:

| `entity_type` | Related entity |
|----------------|----------------|
| `node` | Node snapshots (by node name) |
| `cookbook` | Cookbooks (by cookbook name) |
| `git_repo` | Git-sourced cookbooks (by git repo URL) |
| `role` | Roles (by role name) |
| `policy` | Policyfile nodes (by policy name) |

Additionally, `git_repo_committers` relates to cookbooks via `git_repo_url` and provides contributor data used to inform ownership assignment decisions.

---

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

## 5. Dashboard / Visualisation Changes

### 5.1 Owner Filter

An **Owner** filter is added to the interactive filter bar alongside the existing filters (organisation, environment, role, policy, platform, etc.). The owner filter:

- Supports multi-select (choose one or more owners).
- Includes an **Unowned** option to show only entities without ownership.
- Applies to all dashboard views consistently (version distribution, readiness, cookbook compatibility, dependency graph, remediation).

### 5.2 Ownership Summary View

A new **Ownership** dashboard view is added as a top-level navigation item. This view provides:

#### Migration Progress by Owner

A table showing each owner's migration progress:

| Column | Description |
|--------|-------------|
| Owner | Name and display name |
| Total Nodes | Number of nodes owned |
| Ready | Nodes ready for upgrade |
| Blocked | Nodes blocked by incompatible cookbooks or insufficient disk space |
| Stale | Nodes with stale data |
| % Ready | Percentage of nodes ready |
| Owned Cookbooks | Total cookbooks owned |
| Compatible Cookbooks | Cookbooks compatible with target version |
| Blocking Cookbooks | Cookbooks blocking one or more nodes |
| Complexity | Aggregate remediation complexity (sum of scores for owned incompatible cookbooks) |

The table supports sorting by any column and filtering by organisation and target Chef Client version.

#### Ownership Coverage

A summary panel showing:
- Total entities with ownership assigned vs. total entities
- Percentage of nodes with an owner
- Percentage of cookbooks with an owner
- Percentage of git repositories with an owner
- Number of unowned nodes / cookbooks / git repos / roles (with a link to the unowned view)

#### Owner Detail Drill-Down

Clicking an owner in the summary table drills down to a filtered view of the standard dashboard showing only that owner's entities — version distribution, readiness, cookbook compatibility, and remediation priority scoped to the owner.

### 5.3 Ownership Indicators on Existing Views

- **Node list** — An `Owner` column showing the resolved owner(s) for each node. Definitive owners are displayed with a solid badge; inferred owners with a dashed-outline badge (see § 1.4).
- **Node detail** — An ownership section showing all resolved owners, their confidence level (`definitive` / `inferred`), assignment source, and the resolution path (direct, inherited). Definitive owners are listed first.
- **Cookbook list** — An `Owner` column showing the resolved owner(s) for each cookbook (including git-repo-inherited ownership), with badge styling per § 1.4.
- **Cookbook detail** — An ownership section. For git-sourced cookbooks, this section includes a link to the associated git repository and a link to the **Committers** sub-page.
- **Cookbook detail → Committers sub-page** — For git-sourced cookbooks, a sub-page listing all committers extracted from the git history. The table shows each committer's name, email, commit count, first commit date, and most recent commit date. The **commit count** and **most recent commit** columns are sortable (click to toggle ascending/descending) so that operators can quickly identify who is currently most active in the repository. A date filter allows narrowing to recent contributors (e.g. last 6 months, last year). Each committer row has a checkbox; the operator can select one or more committers and click an **Assign as Owners** action to create ownership assignments for the repository. If a selected committer does not yet exist as an owner, the system creates an `individual` owner record using the committer's name and email. Committers who are already assigned as owners of the repository are visually indicated and excluded from the selection.
- **Remediation priority list** — An `Owner` column to help identify which team needs to act on each cookbook, with badge styling per § 1.4.

### 5.4 Ownership Management UI

An **Ownership** management page under the admin section provides:

- **Owner list** — CRUD for owners (name, display name, contact info, type, metadata).
- **Assignment list** — View and manage assignments per owner, with filters for entity type and source.
- **Bulk import** — A file upload form for CSV/JSON import with a preview of the parsed data before confirming.
- **Bulk reassignment** — A form to move assignments between owners. The operator selects a source owner and a target owner, optionally filters by entity type and/or organisation, and previews the list of assignments that will be moved before confirming. A checkbox option to delete the source owner after reassignment is available (disabled unless the user has `admin` role). After confirmation, a summary shows the number of assignments moved, skipped, and whether the source owner was deleted.
- **Auto-rule status** — A read-only view showing configured auto-derivation rules, their last evaluation time, and the number of assignments they produced.
- **Audit log** — A filterable, paginated table showing the ownership audit log. Columns: timestamp, action, actor, owner, entity type, entity key, organisation. Each row can be expanded to show the full `details` JSON. Filters for action type, actor, owner name, entity type, and date range are available above the table. The audit log is read-only for all roles.

**Authorisation:**
- Viewers can see ownership data and the audit log but cannot modify anything.
- Operators can create/update owners and assignments, and perform bulk reassignment.
- Admins can delete owners, perform bulk reassignment with the delete-source-owner option, and manage all aspects of ownership.

---

## 6. Configuration

### 6.1 Configuration Schema

```yaml
ownership:
  enabled: true  # Default: false. When false, all ownership features are hidden.

  audit_log:
    retention_days: 365  # Default: 365. Set to 0 to retain indefinitely.

  auto_rules:
    - name: aws-nodes-to-cloud-team
      owner: cloud-team
      type: node_attribute
      attribute_path: automatic.cloud.provider
      match_value: "aws"

    - name: web-prod-nodes
      owner: web-platform
      type: node_name_pattern
      pattern: "^web-prod-.*"

    - name: payment-policy
      owner: payments-team
      type: policy_match
      policy_name: "payment-app"

    - name: acme-cookbooks
      owner: acme-platform
      type: cookbook_name_pattern
      pattern: "^acme-.*"

    - name: web-team-repos
      owner: web-platform
      type: git_repo_url_pattern
      pattern: "gitlab\\.example\\.com/team-web/.*"
```

### 6.2 Configuration Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ownership.enabled` | boolean | `false` | Enable ownership tracking features. When disabled, ownership tables still exist but are not populated, and UI elements are hidden. |
| `ownership.auto_rules` | list | `[]` | List of auto-derivation rule definitions (see § 2.2) |
| `ownership.audit_log.retention_days` | integer | `365` | Number of days to retain audit log entries. Entries older than this are purged daily. Set to `0` to disable purging (retain indefinitely). |

### 6.3 Environment Variable Overrides

| Variable | Overrides |
|----------|-----------|
| `CMM_OWNERSHIP_ENABLED` | `ownership.enabled` |
| `CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS` | `ownership.audit_log.retention_days` |

Auto-derivation rules are not overridable via environment variables due to their complex structure. They must be defined in the YAML configuration file.

---

## 7. Data Collection Integration

### 7.1 Additional Partial Search Keys

When `ownership.enabled` is `true` and `node_attribute` auto-derivation rules are configured, the data collection component must include the configured `attribute_path` values in the partial search key map sent to the Chef API. The returned values are stored in the `custom_attributes` field on node snapshots, keyed by the dot-separated attribute path.

### 7.2 Git Committer Collection

When `ownership.enabled` is `true`, the data collection component must extract committer information from each git-sourced cookbook repository during the fetch cycle. After fetching/pulling a repository, the collector gathers the distinct committers from the git log of the default branch — recording each committer's name, email, total commit count, earliest commit date, and most recent commit date.

This data is stored in the `git_repo_committers` table and fully refreshed on each collection run (the previous set of committer rows for the repository is replaced). The committer data is read-only from the application's perspective — it reflects the state of the git history, not user input.

### 7.3 Auto-Derivation Trigger

After each collection run completes for an organisation:

- If `ownership.enabled` is `false`, skip.
- Evaluate all auto-derivation rules against the newly collected data.
- Create assignments for new matches and remove stale `auto_rule` assignments from rules that no longer match.
- Log a summary at `INFO` severity with the rule count, assignments created, and stale assignments removed.

---

## 8. Export Integration

### 8.1 Export Columns

When ownership is enabled, all export types (CSV, JSON) include ownership columns:

| Export Type | Additional Columns |
|-------------|-------------------|
| Ready node export | `owners` (comma-separated list of owner names) |
| Blocked node export | `owners`, `blocking_cookbook_owners` (owners of the blocking cookbooks) |
| Cookbook remediation export | `owners`, `git_repo_url` (for git-sourced cookbooks) |

### 8.2 Export Filters

The `owner` and `unowned` filters are available in export requests, allowing operators to export data scoped to a specific team.

---

## 9. Notification Integration

### 9.1 Owner-Scoped Notifications

Notification channels (see [Configuration Specification](../configuration/Specification.md)) gain an optional `owners` filter:

```yaml
notifications:
  channels:
    - name: web-platform-alerts
      type: webhook
      url_env: WEB_PLATFORM_WEBHOOK_URL
      events:
        - cookbook_status_change
        - readiness_milestone
      filters:
        owners:
          - web-platform
```

When an `owners` filter is set on a channel, the channel only fires for events related to entities owned by the specified owners.

### 9.2 Ownership Change Events

Three notification event types for ownership:

| Event | Description |
|-------|-------------|
| `ownership_assigned` | Fired when ownership is assigned (manual, import, or auto-rule). Payload includes the owner name, entity type/key, and source. |
| `ownership_removed` | Fired when ownership is removed. Payload includes the owner name and entity type/key. |
| `ownership_reassigned` | Fired when assignments are bulk-reassigned between owners. Payload includes the source owner, target owner, number of assignments moved, and whether the source owner was deleted. Fired once per reassignment operation (not per individual assignment). |

These events are low-volume (ownership changes are infrequent) and are primarily useful for audit trails and team communication.

---

## 10. Retention and Cleanup

- **Owner deletion** cascades to all `ownership_assignments` for that owner. The cascade is recorded in the audit log as an `owner_deleted` entry with the count of cascaded assignments in `details.assignments_cascaded`.
- **Organisation deletion** cascades to all `ownership_assignments` scoped to that organisation.
- **Auto-rule assignments** are cleaned up when the rule is removed from configuration. On startup, if a rule name no longer appears in the config, all `auto_rule` assignments with that `auto_rule_name` are deleted. These deletions are logged in the audit log with `actor = 'system'`.
- Ownership assignments are not subject to time-based retention. They persist until explicitly deleted or removed by auto-rule evaluation.
- **Audit log retention** — Audit log entries are purged based on the `ownership.audit_log.retention_days` configuration setting (default: 365 days). A background cleanup job runs daily and deletes entries older than the configured threshold. The audit log itself is append-only — entries are never modified, only purged by the retention job.

---

## 11. Scalability Considerations

- **Ownership resolution at query time** avoids materialisation costs but means ownership lookups add overhead to dashboard queries. Implementations should ensure this remains responsive at scale.
- **Auto-derivation** runs after collection and may involve pattern matching against thousands of entity names. For very large fleets, auto-derivation should be parallelised.
- **Bulk import** is limited to 10,000 assignments per request to keep individual operations bounded.
- **Bulk reassignment** may involve thousands of assignments for a large owner. The operation runs in a single transaction to ensure consistency. For very large reassignments, the transaction size should remain manageable since it involves only UPDATE and INSERT operations on the `ownership_assignments` table plus INSERT operations on the `ownership_audit_log` table.
- **Audit log** volume is proportional to the rate of ownership changes, not to fleet size. Auto-derivation runs may produce bursts of entries after collection, but these are bounded by the number of rule matches that changed. The retention purge job should use batched deletes to avoid long-running transactions.

---

## 12. Migration Path

Ownership tracking is a new, additive feature. The migration path is:

1. **Database migration** creates the `owners`, `ownership_assignments`, `git_repo_committers`, and `ownership_audit_log` tables, and adds the `custom_attributes` column to `node_snapshots`.
2. **Feature disabled by default** — existing deployments see no change until `ownership.enabled` is set to `true`.
3. **No breaking changes** — all existing API endpoints, filters, and behaviour remain unchanged. The `owner` filter parameter is simply ignored when ownership is disabled.
4. **Incremental adoption** — teams can start by importing ownership for their most critical cookbooks and nodes, then gradually expand coverage and add auto-derivation rules.

---

## Related Specifications

- [Top-level Specification](../Specification.md) — project overview and scope
- [Data Collection Specification](../data-collection/Specification.md) — node collection, cookbook fetching, partial search
- [Datastore Specification](../datastore/Specification.md) — database schema and tables
- [Web API Specification](../web-api/Specification.md) — HTTP API endpoints
- [Visualisation Specification](../visualisation/Specification.md) — dashboard views, filters, drill-downs
- [Configuration Specification](../configuration/Specification.md) — YAML configuration schema
- [Analysis Specification](../analysis/Specification.md) — complexity scoring, blast radius (related to ownership-scoped remediation)
- [Logging Specification](../logging/Specification.md) — structured logging with ownership scope