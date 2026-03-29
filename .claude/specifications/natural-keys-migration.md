# Natural Keys Migration — Specification (B0)

Migrate every table from synthetic UUID primary keys to composite natural keys.
UUIDs add a fragile indirection layer — when they drift (re-collection, upsert
race, DISTINCT ON picking a different row) all joins break silently.

## Scope

Every table in `0001_initial_schema.up.sql` that uses `UUID PRIMARY KEY` today.
The migration replaces UUID PKs and FKs with composite natural keys that match
each entity's real-world identity.

## Natural Key Mapping

### Tier 1 — Root Entities (no FK dependencies)

| Table | Current PK | Natural PK | Notes |
|---|---|---|---|
| `credentials` | `id UUID` | `(name)` | Already has `UNIQUE(name)` |
| `organisations` | `id UUID` | `(name)` | Already has `UNIQUE(name)` |
| `users` | `id UUID` | `(username)` | Already has `UNIQUE(username)` |
| `owners` | `id UUID` | `(name)` | Already has `UNIQUE(name)` |
| `git_repos` | `id UUID` | `(name, git_repo_url)` | Already has `UNIQUE(name, git_repo_url)` |

### Tier 2 — Depend on Tier 1

| Table | Current PK | Natural PK | FK changes |
|---|---|---|---|
| `collection_runs` | `id UUID` | `(organisation_name)` | Replace `organisation_id` with `organisation_name TEXT REFERENCES organisations(name)`. One row per org (entity semantics). |
| `server_cookbooks` | `id UUID` | `(organisation_name, name, version)` | Replace `organisation_id` with `organisation_name`. Already has `UNIQUE(organisation_id, name, version)`. |
| `node_snapshots` | `id UUID` | `(organisation_name, node_name)` | Replace `organisation_id` and `collection_run_id` with `organisation_name`. `collection_run_id` becomes `collection_run_org TEXT REFERENCES collection_runs(organisation_name)`. Already has `UNIQUE(organisation_id, node_name)`. |
| `sessions` | `id UUID` | `(id UUID)` | **Keep UUID PK.** Sessions are ephemeral tokens with no natural key. Replace `user_id` with `username TEXT REFERENCES users(username)`. |
| `ownership_assignments` | `id UUID` | `(owner_name, entity_type, entity_key, organisation_name)` | Replace `owner_id` with `owner_name`. Replace `organisation_id` with `organisation_name` (nullable). Matches existing unique index. |
| `role_dependencies` | `id UUID` | `(organisation_name, role_name, dependency_type, dependency_name)` | Replace `organisation_id` with `organisation_name`. Already has matching `UNIQUE`. |
| `metric_snapshots` | `id UUID` | `(id BIGSERIAL)` | **Switch to BIGSERIAL.** Timeseries table — append-only, no natural key. Replace `organisation_id` and `collection_run_id` with `organisation_name`. |
| `cookbook_usage_analysis` | `id UUID` | `(organisation_name)` | Replace `organisation_id` and `collection_run_id` with `organisation_name`. One row per org. |
| `log_entries` | `id UUID` | `(id BIGSERIAL)` | **Switch to BIGSERIAL.** Append-only log, no natural key. Replace `collection_run_id` with `collection_run_org TEXT`. |
| `export_jobs` | `id UUID` | `(id UUID)` | **Keep UUID PK.** Export jobs are referenced by opaque token in download URLs. No natural key. |
| `git_repo_committers` | `id UUID` | `(git_repo_url, author_email)` | Already has matching `UNIQUE`. No UUID FK deps. |
| `ownership_audit_log` | `id UUID` | `(id BIGSERIAL)` | **Switch to BIGSERIAL.** Append-only, no natural key. |

### Tier 3 — Depend on Tier 2

| Table | Current PK | Natural PK | FK changes |
|---|---|---|---|
| `node_readiness` | `id UUID` | `(organisation_name, node_name, target_chef_version)` | Replace `node_snapshot_id` and `organisation_id` with `(organisation_name, node_name)` referencing `node_snapshots`. Already has `UNIQUE(node_snapshot_id, target_chef_version)` — equivalent. |
| `server_cookbook_cookstyle_results` | `id UUID` | `(organisation_name, cookbook_name, cookbook_version, target_chef_version)` | Replace `server_cookbook_id` with `(organisation_name, cookbook_name, cookbook_version)` referencing `server_cookbooks`. |
| `server_cookbook_complexity` | `id UUID` | `(organisation_name, cookbook_name, cookbook_version, target_chef_version)` | Same FK pattern as cookstyle results. |
| `git_repo_cookstyle_results` | `id UUID` | `(git_repo_name, git_repo_url, target_chef_version)` | Replace `git_repo_id` with `(git_repo_name, git_repo_url)` referencing `git_repos`. |
| `git_repo_complexity` | `id UUID` | `(git_repo_name, git_repo_url, target_chef_version)` | Same FK pattern. |
| `git_repo_test_kitchen_results` | `id UUID` | `(git_repo_name, git_repo_url, target_chef_version)` | Same FK pattern. |
| `cookbook_usage_detail` | `id UUID` | `(organisation_name, cookbook_name, cookbook_version)` | Replace `analysis_id` and `organisation_id` with `organisation_name`. |
| `cookbook_platform_coverage` | `id UUID` | `(cookbook_name)` | Replace `git_repo_id` with `(git_repo_name, git_repo_url)` nullable. |

### Tier 4 — Depend on Tier 3

| Table | Current PK | Natural PK | FK changes |
|---|---|---|---|
| `server_cookbook_autocorrect_previews` | `id UUID` | `(organisation_name, cookbook_name, cookbook_version, target_chef_version)` | Replace `server_cookbook_id` and `cookstyle_result_id` with the natural composite. One preview per cookstyle result (1:1). |
| `git_repo_autocorrect_previews` | `id UUID` | `(git_repo_name, git_repo_url, target_chef_version)` | Replace `git_repo_id` and `cookstyle_result_id` with the natural composite. |

## Tables Keeping Non-Natural PKs

| Table | PK Type | Reason |
|---|---|---|
| `sessions` | UUID | Ephemeral auth token, no natural key; UUID is the lookup token |
| `export_jobs` | UUID | Opaque download token referenced in URLs |
| `log_entries` | BIGSERIAL | Append-only, no natural key; BIGSERIAL is cheaper than UUID |
| `metric_snapshots` | BIGSERIAL | Append-only timeseries; BIGSERIAL for ordering |
| `ownership_audit_log` | BIGSERIAL | Append-only audit trail; BIGSERIAL for ordering |

## Migration Strategy

### Phase 1 — New Schema Migration File

Create `migrations/0009_natural_keys.up.sql` and `.down.sql`.

The up migration must:
1. Add new natural-key columns where they don't exist yet (e.g.
   `cookbook_name`, `cookbook_version` on cookstyle results tables).
2. Populate new columns from JOINs against parent tables.
3. Drop old UUID FK columns and UUID PK columns.
4. Add new composite PRIMARY KEY constraints.
5. Add new composite FOREIGN KEY constraints.
6. Recreate affected indexes using natural key columns.
7. Drop the `pgcrypto` extension if no UUID PKs remain that need it.

Order: Tier 4 → Tier 3 → Tier 2 → Tier 1 (drop child FKs before parent PKs).

The down migration reverses the process (re-adds UUID columns, repopulates
with `gen_random_uuid()`, restores old constraints).

### Phase 2 — Go Struct Changes

For each entity struct in `internal/datastore/`:
- Remove the `ID string` field (or change to `int64` for BIGSERIAL tables).
- Remove UUID FK fields (e.g. `OrganisationID`, `ServerCookbookID`).
- Add natural key fields if not already present (e.g. `OrganisationName`,
  `CookbookName`, `CookbookVersion` on cookstyle result structs).
- Update all SQL queries to use natural key columns in WHERE, JOIN, INSERT,
  and RETURNING clauses.
- Update Params structs to accept natural keys instead of UUIDs.

### Phase 3 — Web API Changes

Handlers already predominantly use natural keys for routing. Changes needed:

- **Remove UUID `id` fields from JSON responses.** Nodes, cookbooks, git
  repos, readiness results, cookstyle results, complexity results, log
  entries (switch to BIGSERIAL), and collection runs all currently leak a
  UUID `id` in their JSON. Remove it or replace with the natural key.
- **`GET /api/v1/logs/:id`** — change to `BIGSERIAL` ID in path.
- **`GET /api/v1/exports/:id[/download]`** — keep UUID (opaque token).
- **`DELETE /api/v1/owners/:name/assignments/:id`** — change to composite
  key params (owner_name + entity_type + entity_key + organisation_name).
- **Dashboard trend endpoints** — remove `collection_run_id` from response
  payloads; it carries no user-facing meaning.

### Phase 4 — Frontend Changes

- Remove `id` and UUID FK fields from TypeScript interfaces in `types.ts`.
- Replace `key={item.id}` in React component lists with composite keys
  (e.g. `key={\`${node.organisation_name}/${node.node_name}\`}`).
- Update `fetchLogDetail(id)` to use BIGSERIAL number.
- Update `deleteAssignment()` to use composite key params instead of UUID.
- Remove `fetchExportStatus` UUID only if export_jobs changes (it doesn't).
- Update `CookbookCommittersPage` selection tracking from UUID Set to
  composite key Set.

### Phase 5 — Collector Changes

The collector in `internal/collector/` passes UUIDs between pipeline steps
(e.g. organisation ID → collection run ID → node snapshot IDs → readiness
IDs). All of these become natural key lookups:
- `organisation.Name` replaces `organisation.ID` everywhere.
- `GetServerCookbookIDMap()` is eliminated — cookstyle/complexity/TK
  results are upserted by `(organisation_name, cookbook_name, version,
  target_chef_version)` directly.
- `node_snapshot.ID` is no longer passed to readiness evaluation —
  `(organisation_name, node_name)` is used instead.

## Interface Changes

The `DataStore` interface methods change signatures. Examples:

| Before | After |
|---|---|
| `GetOrganisation(ctx, id string)` | `GetOrganisation(ctx, name string)` — same signature, different semantics |
| `GetNodeSnapshot(ctx, id string)` | Remove — use `GetNodeSnapshotByName(ctx, orgName, nodeName)` |
| `GetServerCookbook(ctx, id string)` | Remove — use `GetServerCookbookByKey(ctx, orgName, name, version)` |
| `GetServerCookbookIDMap(ctx, orgID)` | Remove entirely |
| `UpsertNodeReadiness(ctx, p)` | `p.NodeSnapshotID` → `p.OrganisationName` + `p.NodeName` |
| `DeleteAssignment(ctx, id string)` | `DeleteAssignment(ctx, ownerName, entityType, entityKey, orgName)` |
| `GetLogEntry(ctx, id string)` | `GetLogEntry(ctx, id int64)` |

Methods already using natural keys (e.g. `GetOrganisationByName`,
`GetOwnerByName`, `GetNodeSnapshotByName`) become the canonical lookups.
The `ByID` variants are removed.

## Execution Order

1. Write and review this specification.
2. Create migration SQL (0009) — test with `go test ./...` (migration runs
   on startup).
3. Update Go structs and datastore methods — batch by tier.
4. Update collector pipeline.
5. Update web API handlers and response shapes.
6. Update frontend types and components.
7. Run full test suite. Fix breakages.
8. Remove B0 from tech debt list.

## Risks

- **Data migration correctness** — the up migration populates natural key
  columns via JOINs. If any orphaned rows exist (child rows whose parent
  UUID was deleted without CASCADE), the JOIN will produce NULLs. Mitigate
  by adding `WHERE new_col IS NOT NULL` assertions or `DELETE` orphans
  before altering constraints.
- **Performance** — composite key indexes are wider than single UUID
  indexes. At 100k nodes this is negligible. The B-tree on
  `(organisation_name, node_name)` is actually faster for the dominant
  query pattern (filter by org, then by node) than a UUID PK with a
  separate composite unique index.
- **API breaking change** — removing `id` fields from JSON responses is a
  breaking change for any external consumers. This is acceptable because
  the API is internal (dashboard frontend only) and has no versioning
  contract with third parties.
- **Down migration fidelity** — regenerated UUIDs in the down migration
  will differ from originals. This is acceptable because UUIDs were
  synthetic and not referenced externally.

## Out of Scope

- Renaming tables or columns beyond what is needed for the key migration.
- Changing the `credentials` encryption model.
- Adding new API endpoints.
- Changing the migration framework itself.