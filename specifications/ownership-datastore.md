# Ownership — Datastore Changes

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
