# Ownership — Owner Model

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
