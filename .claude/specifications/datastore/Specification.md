# Datastore - Component Specification

> Component specification for the Chef Migration Metrics datastore schema.
> See the [top-level specification](../Specification.md) for project context.

---

## TL;DR

PostgreSQL schema for all persisted data. Key tables: `credentials` (AES-256-GCM encrypted secrets — Chef API keys, LDAP passwords, SMTP passwords, webhook URLs), `organisations` (with optional FK to stored credentials), `collection_runs`, `node_snapshots` (with `is_stale`, `policy_name`, `policy_group`), `cookbook_versions`, `cookstyle_results`, `test_kitchen_results`, `readiness_results`, `autocorrect_previews`, `cookbook_complexity`, `role_dependencies`, `notification_history`, `export_jobs`, `log_entries`, `metric_snapshots`. Schema managed via sequential numbered migrations. Retention policies apply to snapshots, logs, exports, and metric data.

---

## Overview

Chef Migration Metrics uses PostgreSQL as its persistence backend. The schema stores all collected node data, cookbook metadata, test and scan results, readiness evaluations, log entries, and timestamped metric snapshots for historical trending.

All schema changes are managed through versioned migration files as described in the [Configuration specification](../configuration/Specification.md). This document defines the logical schema — the actual DDL is maintained in the `migrations/` directory.

---

## Conventions

- All tables use `id` (UUID) as the primary key unless otherwise noted.
- All tables include `created_at` and `updated_at` timestamps (UTC, ISO-8601).
- Soft deletes are not used — rows are removed when no longer needed, subject to retention policies.
- Foreign keys enforce referential integrity. Cascading deletes are used where a parent record's removal should remove dependent records (e.g. collection run → node snapshots).
- Index names follow the pattern `idx_<table>_<columns>`.
- Enum-like values are stored as `TEXT` with application-level validation, not PostgreSQL enums, to simplify migrations.
- Sensitive values (private keys, passwords, tokens) stored in the database are encrypted at the application layer before writing. See the `credentials` table for the encryption model.

---

## Schema Migrations Tracking

```sql
CREATE TABLE schema_migrations (
    version     BIGINT  PRIMARY KEY,
    dirty       BOOLEAN NOT NULL DEFAULT FALSE
);
```

Managed by `golang-migrate/migrate`. Do not modify this table manually.

---

## Tables

### 1. `credentials`

Stores encrypted credentials (private keys, passwords, tokens) that the application needs at runtime. All sensitive material is encrypted at the application layer using AES-256-GCM before being written to the database. The database never sees plaintext secrets.

This table provides an alternative to managing secrets exclusively through config files and environment variables. Administrators can choose per-credential whether to use file-based (`client_key_path`), environment variable, or database storage. Database-stored credentials can be managed through the Web API, making multi-organisation deployments easier to operate — especially in containerised environments where managing per-org PEM files is cumbersome.

#### Encryption Model

| Property | Value |
|----------|-------|
| Algorithm | AES-256-GCM (authenticated encryption with associated data) |
| Key derivation | HKDF-SHA256 from the master credential encryption key |
| IV/Nonce | 12-byte random nonce, generated per encryption operation, stored alongside the ciphertext |
| Associated data (AAD) | `<credential_type>:<name>` — binds the ciphertext to its identity so it cannot be swapped between rows |
| Master key source | `credential_encryption_key` configuration setting or `CMM_CREDENTIAL_ENCRYPTION_KEY` environment variable (see [Configuration Specification](../configuration/Specification.md)) |
| Key rotation | When the master key changes, a startup migration re-encrypts all rows. The old key must be provided via `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` during the rotation window. |
| At-rest format | `<nonce_hex>:<ciphertext_hex>` stored in the `encrypted_value` column |

**Security properties:**

- **Confidentiality** — AES-256-GCM encryption ensures the plaintext is unrecoverable without the master key, even if the database is compromised.
- **Integrity** — GCM's authentication tag detects any tampering with the ciphertext.
- **Binding** — The AAD ties each ciphertext to its `credential_type` and `name`, preventing an attacker with database write access from swapping encrypted values between rows.
- **Uniqueness** — A fresh random nonce per encryption means identical plaintext values produce different ciphertext, preventing comparison attacks.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `name` | TEXT | No | Unique human-readable name for this credential (e.g. `myorg-production-key`, `ldap-bind-password`, `smtp-password`) |
| `credential_type` | TEXT | No | One of: `chef_client_key`, `ldap_bind_password`, `smtp_password`, `webhook_url`, `generic` |
| `encrypted_value` | TEXT | No | The encrypted credential in `<nonce_hex>:<ciphertext_hex>` format |
| `metadata` | JSONB | Yes | Non-sensitive metadata about the credential (e.g. `{"key_format": "pkcs1", "bits": 2048}` for RSA keys; `{"host": "ldap.example.com"}` for LDAP passwords). Never contains the plaintext value. |
| `last_rotated_at` | TIMESTAMPTZ | Yes | When the credential value was last updated (tracks rotation) |
| `created_by` | TEXT | No | Username of the admin who created this credential |
| `updated_by` | TEXT | Yes | Username of the admin who last updated this credential |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Unique constraints:**
- `name`
- `(credential_type, name)`

**Indexes:**
- `idx_credentials_name` on `name`
- `idx_credentials_credential_type` on `credential_type`

**Important constraints:**

- The `encrypted_value` column must **never** be included in log output, error messages, API responses, or Elasticsearch exports.
- The plaintext value is only ever held in memory for the duration of the operation that needs it (e.g. signing a Chef API request, binding to LDAP). It is not cached.
- When a credential is deleted, the row is hard-deleted immediately. There is no soft-delete or recycle bin for credentials.
- The Web API credential endpoints return metadata only — never the encrypted or plaintext value. A one-way "test" endpoint validates that a stored credential works without revealing it.

---

### 2. `organisations`

Stores the configured Chef Infra Server organisations. Organisations may be populated from the YAML configuration file on startup, or created dynamically via the Web API.

Chef API credentials for each organisation can come from one of three sources:
1. **File path** — `client_key_path` in the YAML config (existing behaviour)
2. **Environment variable** — for containerised deployments
3. **Database** — a reference to a `credentials` table row via `client_key_credential_id` (new)

When `client_key_credential_id` is set, the application decrypts the stored key at runtime for API request signing. The `client_key_path` field is ignored if a database credential is linked.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `name` | TEXT | No | Unique friendly name for this organisation |
| `chef_server_url` | TEXT | No | Base URL of the Chef Infra Server |
| `org_name` | TEXT | No | Organisation name on the Chef server |
| `client_name` | TEXT | No | Chef API client name used for authentication |
| `client_key_credential_id` | UUID | Yes | FK → `credentials.id`. When set, the Chef API private key is read from the `credentials` table instead of from a file on disk. |
| `source` | TEXT | No | One of: `config`, `api`. Indicates whether this organisation was defined in the YAML config file or created via the Web API. Config-sourced orgs are reconciled on startup; API-sourced orgs persist independently. |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:**
- `client_key_credential_id` → `credentials(id)` ON DELETE SET NULL

**Unique constraints:**
- `name`
- `(chef_server_url, org_name)`

**Indexes:**
- `idx_organisations_name` on `name`
- `idx_organisations_client_key_credential_id` on `client_key_credential_id`

> **Credential resolution order:** When the application needs the Chef API private key for an organisation:
> 1. If `client_key_credential_id` is set and the referenced credential exists, decrypt and use it.
> 2. Otherwise, if `client_key_path` is configured for this organisation in the YAML config, read the file from disk.
> 3. Otherwise, fail with a descriptive error identifying the organisation and the missing credential.
>
> This order means database credentials take precedence over file-based credentials, allowing operators to migrate incrementally without changing the config file.

---

### 2. `collection_runs`

Records each periodic data collection run. One row per organisation per run.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `status` | TEXT | No | One of: `running`, `completed`, `failed`, `interrupted` |
| `started_at` | TIMESTAMPTZ | No | When the run started |
| `completed_at` | TIMESTAMPTZ | Yes | When the run finished (null if still running or interrupted) |
| `total_nodes` | INTEGER | Yes | Total nodes discovered in the organisation |
| `nodes_collected` | INTEGER | Yes | Nodes successfully collected (for checkpoint/resume) |
| `checkpoint_start` | INTEGER | Yes | The `start` offset to resume pagination from if interrupted |
| `error_message` | TEXT | Yes | Summary error if the run failed |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:**
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Indexes:**
- `idx_collection_runs_organisation_id` on `organisation_id`
- `idx_collection_runs_status` on `status`
- `idx_collection_runs_started_at` on `started_at`

---

### 3. `node_snapshots`

Stores a point-in-time snapshot of each node's attributes as collected during a collection run. Each collection run produces a full set of node snapshots, enabling historical trending.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `collection_run_id` | UUID | No | FK → `collection_runs.id` |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `node_name` | TEXT | No | Node name |
| `chef_environment` | TEXT | Yes | Chef environment |
| `chef_version` | TEXT | Yes | Chef Client version (`automatic.chef_packages.chef.version`) |
| `platform` | TEXT | Yes | OS platform (`automatic.platform`) |
| `platform_version` | TEXT | Yes | OS platform version (`automatic.platform_version`) |
| `platform_family` | TEXT | Yes | OS platform family (`automatic.platform_family`) |
| `filesystem` | JSONB | Yes | Filesystem data (`automatic.filesystem`) |
| `cookbooks` | JSONB | Yes | Resolved cookbook map (`automatic.cookbooks`) |
| `run_list` | JSONB | Yes | Raw run list array |
| `roles` | JSONB | Yes | Expanded roles array |
| `policy_name` | TEXT | Yes | Policyfile policy name (null for classic nodes) |
| `policy_group` | TEXT | Yes | Policyfile policy group (null for classic nodes) |
| `ohai_time` | DOUBLE PRECISION | Yes | Unix timestamp of last Chef client run (from `automatic.ohai_time`) |
| `is_stale` | BOOLEAN | No | Whether the node's data is stale (ohai_time older than configured threshold) |
| `collected_at` | TIMESTAMPTZ | No | Timestamp of collection |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `collection_run_id` → `collection_runs(id)` ON DELETE CASCADE
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Indexes:**
- `idx_node_snapshots_collection_run_id` on `collection_run_id`
- `idx_node_snapshots_organisation_id` on `organisation_id`
- `idx_node_snapshots_node_name` on `node_name`
- `idx_node_snapshots_chef_version` on `chef_version`
- `idx_node_snapshots_platform` on `(platform, platform_version)`
- `idx_node_snapshots_platform_family` on `platform_family`
- `idx_node_snapshots_chef_environment` on `chef_environment`
- `idx_node_snapshots_collected_at` on `collected_at`
- `idx_node_snapshots_policy_name` on `policy_name`
- `idx_node_snapshots_policy_group` on `policy_group`
- `idx_node_snapshots_is_stale` on `is_stale`

---

### 4. `cookbooks`

Stores metadata about each known cookbook. A cookbook is uniquely identified by organisation + name + version for Chef server-sourced cookbooks. Git-sourced cookbooks are identified by name alone (versions are managed via git).

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `organisation_id` | UUID | Yes | FK → `organisations.id`. Null for git-sourced cookbooks that span organisations |
| `name` | TEXT | No | Cookbook name |
| `version` | TEXT | Yes | Cookbook version. Null for git-sourced cookbooks (HEAD is tested) |
| `source` | TEXT | No | One of: `git`, `chef_server` |
| `git_repo_url` | TEXT | Yes | Full git repository URL (git-sourced only) |
| `head_commit_sha` | TEXT | Yes | Latest HEAD commit SHA of the default branch (git-sourced only) |
| `default_branch` | TEXT | Yes | Detected default branch name, e.g. `main` or `master` (git-sourced only) |
| `has_test_suite` | BOOLEAN | No | Whether the cookbook contains a Test Kitchen configuration |
| `is_active` | BOOLEAN | No | Whether the cookbook is applied to at least one node |
| `is_stale_cookbook` | BOOLEAN | No | Whether the cookbook's most recent version is older than the configured stale threshold |
| `first_seen_at` | TIMESTAMPTZ | Yes | When this cookbook version was first observed by the application (proxy for upload date) |
| `last_fetched_at` | TIMESTAMPTZ | Yes | When the cookbook was last fetched or pulled |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Unique constraints:**
- `(organisation_id, name, version)` WHERE `source = 'chef_server'`
- `(name, git_repo_url)` WHERE `source = 'git'`

**Indexes:**
- `idx_cookbooks_organisation_id` on `organisation_id`
- `idx_cookbooks_name` on `name`
- `idx_cookbooks_source` on `source`
- `idx_cookbooks_is_active` on `is_active`
- `idx_cookbooks_is_stale_cookbook` on `is_stale_cookbook`
- `idx_cookbooks_name_version` on `(name, version)`
- `idx_cookbooks_first_seen_at` on `first_seen_at`

---

### 5. `cookbook_node_usage`

Junction table recording which nodes use which cookbooks. Derived from `node_snapshots.cookbooks` during each collection run. Represents the current state (most recent collection run).

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_id` | UUID | No | FK → `cookbooks.id` |
| `node_snapshot_id` | UUID | No | FK → `node_snapshots.id` |
| `cookbook_version` | TEXT | No | The specific version of the cookbook the node is running |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `cookbook_id` → `cookbooks(id)` ON DELETE CASCADE
- `node_snapshot_id` → `node_snapshots(id)` ON DELETE CASCADE

**Indexes:**
- `idx_cookbook_node_usage_cookbook_id` on `cookbook_id`
- `idx_cookbook_node_usage_node_snapshot_id` on `node_snapshot_id`
- `idx_cookbook_node_usage_cookbook_version` on `(cookbook_id, cookbook_version)`

---

### 6. `cookbook_role_usage`

Records which roles reference which cookbooks. Derived from role data collected via the Chef API.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_name` | TEXT | No | Cookbook name |
| `role_name` | TEXT | No | Role name |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Unique constraints:**
- `(cookbook_name, role_name, organisation_id)`

**Foreign keys:**
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Indexes:**
- `idx_cookbook_role_usage_cookbook_name` on `cookbook_name`
- `idx_cookbook_role_usage_role_name` on `role_name`
- `idx_cookbook_role_usage_organisation_id` on `organisation_id`

---

### 7. `test_kitchen_results`

Stores Test Kitchen test results for git-sourced cookbooks tested against target Chef Client versions.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_id` | UUID | No | FK → `cookbooks.id` |
| `target_chef_version` | TEXT | No | Target Chef Client version tested against |
| `commit_sha` | TEXT | No | Git commit SHA at the time of testing |
| `converge_passed` | BOOLEAN | No | Whether the cookbook converged successfully |
| `tests_passed` | BOOLEAN | No | Whether the test suite passed |
| `compatible` | BOOLEAN | No | `true` only if both `converge_passed` AND `tests_passed` are true |
| `process_stdout` | TEXT | Yes | Captured stdout from the Test Kitchen process |
| `process_stderr` | TEXT | Yes | Captured stderr from the Test Kitchen process |
| `duration_seconds` | INTEGER | Yes | Wall-clock time for the Test Kitchen run |
| `started_at` | TIMESTAMPTZ | No | When the test run started |
| `completed_at` | TIMESTAMPTZ | Yes | When the test run finished |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `cookbook_id` → `cookbooks(id)` ON DELETE CASCADE

**Unique constraints:**
- `(cookbook_id, target_chef_version, commit_sha)` — ensures one result per cookbook + version + commit

**Indexes:**
- `idx_test_kitchen_results_cookbook_id` on `cookbook_id`
- `idx_test_kitchen_results_target_chef_version` on `target_chef_version`
- `idx_test_kitchen_results_commit_sha` on `commit_sha`
- `idx_test_kitchen_results_compatible` on `compatible`
- `idx_test_kitchen_results_cookbook_target` on `(cookbook_id, target_chef_version)`

---

### 8. `cookstyle_results`

Stores CookStyle scan results for Chef server-sourced cookbooks. Offenses are enriched with remediation guidance from the built-in cop-to-documentation mapping (see [Analysis Specification](../analysis/Specification.md)).

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_id` | UUID | No | FK → `cookbooks.id` |
| `target_chef_version` | TEXT | Yes | Target Chef Client version this scan was run for (null if run with full rule set) |
| `passed` | BOOLEAN | No | Whether CookStyle reported no errors |
| `offence_count` | INTEGER | No | Total number of CookStyle offences |
| `deprecation_count` | INTEGER | No | Number of `ChefDeprecations/*` offences |
| `correctness_count` | INTEGER | No | Number of `ChefCorrectness/*` offences |
| `deprecation_warnings` | JSONB | Yes | Array of deprecation warning objects |
| `offences` | JSONB | Yes | Full CookStyle offence report (JSON format output), enriched with remediation guidance |
| `process_stdout` | TEXT | Yes | Captured stdout from the CookStyle process |
| `process_stderr` | TEXT | Yes | Captured stderr from the CookStyle process |
| `duration_seconds` | INTEGER | Yes | Wall-clock time for the scan |
| `scanned_at` | TIMESTAMPTZ | No | When the scan was performed |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `cookbook_id` → `cookbooks(id)` ON DELETE CASCADE

**Unique constraints:**
- `(cookbook_id, target_chef_version)` — one result per Chef server cookbook version per target Chef version (immutable; rescan replaces the row)

**Indexes:**
- `idx_cookstyle_results_cookbook_id` on `cookbook_id`
- `idx_cookstyle_results_target_chef_version` on `target_chef_version`
- `idx_cookstyle_results_passed` on `passed`

---

### 9. `autocorrect_previews`

Stores CookStyle auto-correct preview results — a diff showing what `cookstyle --auto-correct` would change for cookbooks with offenses. Generated by the Analysis component as part of the remediation guidance pipeline.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_id` | UUID | No | FK → `cookbooks.id` |
| `cookstyle_result_id` | UUID | No | FK → `cookstyle_results.id` — links to the scan that triggered this preview |
| `total_offenses` | INTEGER | No | Total offenses before auto-correct |
| `correctable_offenses` | INTEGER | No | Offenses that auto-correct can fix |
| `remaining_offenses` | INTEGER | No | Offenses requiring manual intervention |
| `files_modified` | INTEGER | No | Number of files that would be changed |
| `diff_output` | TEXT | Yes | Unified diff of all changes |
| `generated_at` | TIMESTAMPTZ | No | When the preview was generated |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `cookbook_id` → `cookbooks(id)` ON DELETE CASCADE
- `cookstyle_result_id` → `cookstyle_results(id)` ON DELETE CASCADE

**Unique constraints:**
- `(cookstyle_result_id)` — one preview per CookStyle result

**Indexes:**
- `idx_autocorrect_previews_cookbook_id` on `cookbook_id`
- `idx_autocorrect_previews_cookstyle_result_id` on `cookstyle_result_id`

---

### 10. `cookbook_complexity`

Stores computed complexity scores and blast radius metrics per cookbook per target Chef Client version. Used by the dashboard to help teams prioritise remediation effort.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `cookbook_id` | UUID | No | FK → `cookbooks.id` |
| `target_chef_version` | TEXT | No | Target Chef Client version |
| `complexity_score` | INTEGER | No | Numeric complexity score |
| `complexity_label` | TEXT | No | One of: `none`, `low`, `medium`, `high`, `critical` |
| `error_count` | INTEGER | No | Count of error/fatal CookStyle offenses |
| `deprecation_count` | INTEGER | No | Count of `ChefDeprecations/*` offenses |
| `correctness_count` | INTEGER | No | Count of `ChefCorrectness/*` offenses |
| `modernize_count` | INTEGER | No | Count of `ChefModernize/*` offenses |
| `auto_correctable_count` | INTEGER | No | Offenses fixable by auto-correct |
| `manual_fix_count` | INTEGER | No | Offenses requiring manual intervention |
| `affected_node_count` | INTEGER | No | Blast radius — number of nodes running this cookbook |
| `affected_role_count` | INTEGER | No | Blast radius — number of roles that include this cookbook (directly or transitively) |
| `affected_policy_count` | INTEGER | No | Blast radius — number of Policyfile policy names that include this cookbook |
| `evaluated_at` | TIMESTAMPTZ | No | When the complexity score was computed |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:**
- `cookbook_id` → `cookbooks(id)` ON DELETE CASCADE

**Unique constraints:**
- `(cookbook_id, target_chef_version)` — one score per cookbook per target version

**Indexes:**
- `idx_cookbook_complexity_cookbook_id` on `cookbook_id`
- `idx_cookbook_complexity_target_chef_version` on `target_chef_version`
- `idx_cookbook_complexity_complexity_score` on `complexity_score`
- `idx_cookbook_complexity_complexity_label` on `complexity_label`
- `idx_cookbook_complexity_affected_node_count` on `affected_node_count`

---

### 11. `node_readiness`

Stores computed upgrade readiness status per node per target Chef Client version.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `node_snapshot_id` | UUID | No | FK → `node_snapshots.id` |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `node_name` | TEXT | No | Node name (denormalised for query convenience) |
| `target_chef_version` | TEXT | No | Target Chef Client version |
| `is_ready` | BOOLEAN | No | Whether the node is ready to upgrade |
| `all_cookbooks_compatible` | BOOLEAN | No | Whether all cookbooks in the expanded run-list are compatible |
| `sufficient_disk_space` | BOOLEAN | No | Whether disk space meets the configured threshold |
| `blocking_cookbooks` | JSONB | Yes | Array of `{ name, version, reason, source, complexity_score, complexity_label }` objects |
| `available_disk_mb` | INTEGER | Yes | Available disk space in MB (extracted from filesystem attribute) |
| `required_disk_mb` | INTEGER | Yes | Required disk space from configuration |
| `stale_data` | BOOLEAN | No | Whether the node's data is stale (ohai_time exceeds threshold) |
| `evaluated_at` | TIMESTAMPTZ | No | When readiness was last evaluated |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:**
- `node_snapshot_id` → `node_snapshots(id)` ON DELETE CASCADE
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Unique constraints:**
- `(node_snapshot_id, target_chef_version)` — one readiness evaluation per node snapshot per target version

**Indexes:**
- `idx_node_readiness_node_snapshot_id` on `node_snapshot_id`
- `idx_node_readiness_organisation_id` on `organisation_id`
- `idx_node_readiness_target_chef_version` on `target_chef_version`
- `idx_node_readiness_is_ready` on `is_ready`
- `idx_node_readiness_stale_data` on `stale_data`
- `idx_node_readiness_node_name` on `node_name`

---

### 12. `role_dependencies`

Stores the directed dependency graph from roles to other roles and from roles to cookbooks. Populated by the Data Collection component from role detail API responses.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `role_name` | TEXT | No | The role that has the dependency |
| `dependency_type` | TEXT | No | One of: `role`, `cookbook` — what the role depends on |
| `dependency_name` | TEXT | No | Name of the dependent role or cookbook |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:**
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Unique constraints:**
- `(organisation_id, role_name, dependency_type, dependency_name)`

**Indexes:**
- `idx_role_dependencies_organisation_id` on `organisation_id`
- `idx_role_dependencies_role_name` on `role_name`
- `idx_role_dependencies_dependency_type` on `dependency_type`
- `idx_role_dependencies_dependency_name` on `dependency_name`

---

### 13. `metric_snapshots`

Stores pre-aggregated metric snapshots at the end of each collection and analysis cycle. Used for historical trending charts. Pre-aggregation ensures the dashboard remains responsive at scale.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `collection_run_id` | UUID | Yes | FK → `collection_runs.id` (null for manually triggered snapshots) |
| `organisation_id` | UUID | No | FK → `organisations.id` |
| `snapshot_type` | TEXT | No | One of: `chef_version_distribution`, `readiness_summary`, `cookbook_compatibility` |
| `target_chef_version` | TEXT | Yes | Target Chef Client version (applicable for `readiness_summary` and `cookbook_compatibility`) |
| `data` | JSONB | No | Aggregated metric data (structure varies by `snapshot_type`, see below) |
| `snapshot_at` | TIMESTAMPTZ | No | The point in time this snapshot represents |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `collection_run_id` → `collection_runs(id)` ON DELETE SET NULL
- `organisation_id` → `organisations(id)` ON DELETE CASCADE

**Indexes:**
- `idx_metric_snapshots_organisation_id` on `organisation_id`
- `idx_metric_snapshots_snapshot_type` on `snapshot_type`
- `idx_metric_snapshots_snapshot_at` on `snapshot_at`
- `idx_metric_snapshots_target_chef_version` on `target_chef_version`

#### Snapshot Data Structures

**`chef_version_distribution`:**

```json
{
  "total_nodes": 5000,
  "versions": {
    "17.10.0": { "count": 3200, "percentage": 64.0 },
    "18.5.0":  { "count": 1500, "percentage": 30.0 },
    "16.18.0": { "count": 300,  "percentage": 6.0 }
  }
}
```

**`readiness_summary`:**

```json
{
  "total_nodes": 5000,
  "ready": 3800,
  "blocked": 1200,
  "blocking_reasons": {
    "incompatible_cookbooks": 900,
    "insufficient_disk_space": 250,
    "both": 50
  }
}
```

**`cookbook_compatibility`:**

```json
{
  "total_cookbooks": 150,
  "compatible": 120,
  "incompatible": 15,
  "cookstyle_only": 10,
  "untested": 5
}
```

---

### 14. `log_entries`

Stores structured log entries from all components. See the [Logging specification](../logging/Specification.md) for the log entry structure and scope definitions.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `timestamp` | TIMESTAMPTZ | No | When the event occurred |
| `severity` | TEXT | No | One of: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `scope` | TEXT | No | One of: `collection_run`, `git_operation`, `test_kitchen_run`, `cookstyle_scan` |
| `message` | TEXT | No | Human-readable event description |
| `organisation` | TEXT | Yes | Chef server organisation name |
| `cookbook_name` | TEXT | Yes | Cookbook name |
| `cookbook_version` | TEXT | Yes | Cookbook version |
| `commit_sha` | TEXT | Yes | Git commit SHA |
| `chef_client_version` | TEXT | Yes | Target Chef Client version |
| `process_output` | TEXT | Yes | Captured stdout/stderr from an external process |
| `collection_run_id` | UUID | Yes | FK → `collection_runs.id` (links log to its collection run) |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Foreign keys:**
- `collection_run_id` → `collection_runs(id)` ON DELETE SET NULL

**Indexes:**
- `idx_log_entries_timestamp` on `timestamp`
- `idx_log_entries_severity` on `severity`
- `idx_log_entries_scope` on `scope`
- `idx_log_entries_organisation` on `organisation`
- `idx_log_entries_cookbook_name` on `cookbook_name`
- `idx_log_entries_collection_run_id` on `collection_run_id`
- `idx_log_entries_retention` on `timestamp` — used by the retention purge job

---

### 15. `notification_history`

Records all notifications that have been sent, providing an audit trail viewable from the dashboard.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `channel_name` | TEXT | No | Name of the notification channel that sent this notification |
| `channel_type` | TEXT | No | One of: `webhook`, `email` |
| `event_type` | TEXT | No | One of: `cookbook_status_change`, `readiness_milestone`, `new_incompatible_cookbook`, `collection_failure`, `stale_node_threshold_exceeded` |
| `summary` | TEXT | No | Human-readable summary of what triggered the notification |
| `payload` | JSONB | No | Full notification payload as sent |
| `status` | TEXT | No | One of: `sent`, `failed`, `retrying` |
| `error_message` | TEXT | Yes | Error message if delivery failed |
| `retry_count` | INTEGER | No | Number of delivery attempts |
| `sent_at` | TIMESTAMPTZ | No | When the notification was sent (or last attempted) |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Indexes:**
- `idx_notification_history_event_type` on `event_type`
- `idx_notification_history_channel_name` on `channel_name`
- `idx_notification_history_status` on `status`
- `idx_notification_history_sent_at` on `sent_at`

---

### 16. `export_jobs`

Tracks asynchronous data export operations. When an export is estimated to be large (exceeding the configured `exports.async_threshold`), the API creates a job record and processes the export in the background.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key (also serves as the job ID returned to the client) |
| `export_type` | TEXT | No | One of: `ready_nodes`, `blocked_nodes`, `cookbook_remediation` |
| `format` | TEXT | No | One of: `csv`, `json`, `chef_search_query` |
| `filters` | JSONB | No | The filters applied to the export (organisation, environment, role, platform, etc.) |
| `status` | TEXT | No | One of: `pending`, `processing`, `completed`, `failed` |
| `row_count` | INTEGER | Yes | Number of rows in the completed export |
| `file_path` | TEXT | Yes | Path to the generated export file (set on completion) |
| `file_size_bytes` | BIGINT | Yes | Size of the generated file |
| `error_message` | TEXT | Yes | Error message if the export failed |
| `requested_by` | TEXT | No | Username of the user who requested the export |
| `requested_at` | TIMESTAMPTZ | No | When the export was requested |
| `completed_at` | TIMESTAMPTZ | Yes | When the export finished |
| `expires_at` | TIMESTAMPTZ | No | When the export file will be deleted (based on `exports.retention_hours`) |
| `created_at` | TIMESTAMPTZ | No | Row creation time |

**Indexes:**
- `idx_export_jobs_status` on `status`
- `idx_export_jobs_export_type` on `export_type`
- `idx_export_jobs_requested_by` on `requested_by`
- `idx_export_jobs_expires_at` on `expires_at`

---

### 17. `users`

Stores local user accounts for authentication. LDAP and SAML users are not stored here — they are authenticated externally and mapped to sessions.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `username` | TEXT | No | Unique username |
| `display_name` | TEXT | Yes | Human-readable display name |
| `email` | TEXT | Yes | Email address |
| `password_hash` | TEXT | No | Bcrypt-hashed password |
| `role` | TEXT | No | One of: `admin`, `viewer` |
| `auth_provider` | TEXT | No | One of: `local`, `ldap`, `saml` |
| `is_locked` | BOOLEAN | No | Whether the account is locked |
| `failed_login_attempts` | INTEGER | No | Consecutive failed login attempts (reset on success) |
| `last_login_at` | TIMESTAMPTZ | Yes | Last successful login time |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Unique constraints:**
- `username`

**Indexes:**
- `idx_users_username` on `username`
- `idx_users_auth_provider` on `auth_provider`

---

### 18. `sessions`

Stores active user sessions for session management and explicit logout/invalidation.

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key (also serves as the session token) |
| `user_id` | UUID | Yes | FK → `users.id` (null for externally authenticated users not in `users` table) |
| `username` | TEXT | No | Username (denormalised for external auth providers) |
| `auth_provider` | TEXT | No | One of: `local`, `ldap`, `saml` |
| `role` | TEXT | No | Role for this session (`admin` or `viewer`) |
| `expires_at` | TIMESTAMPTZ | No | When the session expires |
| `created_at` | TIMESTAMPTZ | No | Session creation time |

**Foreign keys:**
- `user_id` → `users(id)` ON DELETE CASCADE

**Indexes:**
- `idx_sessions_user_id` on `user_id`
- `idx_sessions_expires_at` on `expires_at`

---

## Entity Relationship Summary

```
credentials ◄───────────────────────────────────────────────────────────────────┐
    │                                                                           │
    │ 0..1 (client_key_credential_id)                                           │
    ▼                                                                           │
organisations ──────────┬────────────────────────────────────────────────────────┤
    │                   │                                                       │
    │ 1:N               │ 1:N                                                   │ 1:N
    ▼                   ▼                                                       ▼
collection_runs    cookbooks                                             metric_snapshots
    │                   │
    │ 1:N               ├── 1:N ──► test_kitchen_results
    │                   ├── 1:N ──► cookstyle_results (per target version)
    │                   │              │
    │                   │              └── 1:1 ──► autocorrect_previews
    │                   ├── 1:N ──► cookbook_complexity (per target version)
    ▼                   │
node_snapshots ◄────────┤ (via cookbook_node_usage)
    │                   │
    │ 1:N               │ N:M
    │                   ▼
    │              cookbook_node_usage
    │
    │ 1:N
    ▼
node_readiness

organisations ── 1:N ── role_dependencies

credentials ── referenced by ── organisations (chef client key)
credentials ── referenced by ── application config (LDAP bind password, SMTP password, webhook URLs)

log_entries ── optionally linked to ── collection_runs

notification_history (standalone — references channels by name)

export_jobs (standalone — references users by username)

users ── 1:N ── sessions
```

> **Note on `credentials` relationships:** The `credentials` table has a direct FK relationship with `organisations` for Chef client keys. For other credential types (LDAP bind password, SMTP password, webhook URLs), the link is by name — the YAML config references a credential by its `name` value (e.g. `bind_password_credential: ldap-bind-password`) rather than by a foreign key. This keeps the schema simple and avoids adding nullable FK columns to tables that don't exist (LDAP/SMTP config lives in the YAML file, not in the database).

---

## Credential Storage Security

This section summarises the security properties and operational procedures for database-stored credentials. The `credentials` table is the only table that holds encrypted sensitive material.

### Defence in Depth

| Layer | Protection |
|-------|-----------|
| **Application** | AES-256-GCM encryption with HKDF-derived key; plaintext never logged; API never returns values |
| **Database** | Standard PostgreSQL access controls; `encrypted_value` column contains only ciphertext |
| **Transport** | PostgreSQL connections should use TLS (`sslmode=verify-full` recommended) |
| **Backups** | Database backups contain only ciphertext; restoring a backup without the master key renders credentials unusable |
| **Key management** | Master key is external to the database (env var or config); separation of key and data |

### Key Rotation Procedure

1. Set `CMM_CREDENTIAL_ENCRYPTION_KEY` to the **new** master key.
2. Set `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` to the **old** master key.
3. Restart the application. On startup, it detects the key change and re-encrypts all `credentials` rows using the new key.
4. After successful startup, remove the `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` variable.
5. Log entry at `INFO` severity: `Credential encryption key rotated: <count> credentials re-encrypted`.

If the old key is not provided during rotation, credentials encrypted with the previous key cannot be decrypted. The application logs an `ERROR` for each affected credential and continues startup — those credentials are marked as unusable until the old key is provided or the credentials are re-entered.

### Credential Deletion

When a credential is deleted (via the Web API or when an organisation is removed), the row is hard-deleted immediately. PostgreSQL's MVCC may retain the old row version in dead tuples until `VACUUM` runs. For high-security environments, operators should configure aggressive autovacuum settings on the `credentials` table or run `VACUUM FULL` after bulk credential deletion.

---

## Retention and Cleanup

### Node Snapshot Retention

Node snapshots accumulate over time to support historical trending. A configurable retention policy determines how long raw snapshots are kept. Pre-aggregated `metric_snapshots` persist independently and are not subject to node snapshot retention, ensuring trend data survives even after raw snapshots are purged.

### Log Entry Retention

Log entries are purged according to the `logging.retention_days` configuration setting. The purge runs automatically during each collection cycle. See the [Logging specification](../logging/Specification.md).

### Session Cleanup

Expired sessions should be purged periodically (e.g. during each collection run or on a separate schedule) to prevent unbounded table growth.

### Notification History Retention

Notification history entries should be retained for the same period as log entries (`logging.retention_days`). Entries older than the retention period are purged alongside log entries.

### Export Job Cleanup

Completed export files are deleted after the configured `exports.retention_hours`. The corresponding `export_jobs` rows are updated to `status = 'expired'` and retained for audit purposes until the next log retention purge cycle.

### Auto-Correct Preview Retention

Auto-correct previews are linked to their parent `cookstyle_results` row via CASCADE delete. When a CookStyle result is replaced by a rescan, the associated preview is automatically deleted.

---

### Credential Retention

- Credentials are not subject to time-based retention. They persist until explicitly deleted by an administrator.
- When an organisation is deleted, the `client_key_credential_id` FK is set to NULL (ON DELETE SET NULL), **not** cascading. This is deliberate — the same credential may be referenced by configuration outside the database (e.g. a future organisation re-using the same key). Orphaned credentials (not referenced by any organisation or config) can be identified and cleaned up via the Web API.
- The application logs a `WARN` at startup if it detects orphaned credentials (credentials not referenced by any organisation's `client_key_credential_id` and not referenced by name in the YAML config).

---

## Performance Considerations

- **JSONB columns** (`filesystem`, `cookbooks`, `run_list`, `roles`, `data`, `blocking_cookbooks`, `deprecation_warnings`, `offences`, `payload`, `filters`) support efficient querying with GIN indexes if needed, but GIN indexes should only be added if query patterns demand it — they are expensive to maintain.
- **`metric_snapshots`** pre-aggregates data specifically so that dashboard summary views avoid scanning the full `node_snapshots` table. The dashboard should query `metric_snapshots` for trend charts and only fall back to `node_snapshots` for drill-down detail.
- **`cookbook_complexity`** is queried heavily by the remediation guidance view. The composite index on `(cookbook_id, target_chef_version)` ensures fast lookups. The `affected_node_count` index supports sorting by blast radius.
- **`role_dependencies`** is queried to build the dependency graph. For organisations with hundreds of roles, the graph traversal should be performed as a recursive CTE in PostgreSQL rather than multiple round-trips from the application.
- **`autocorrect_previews`** can contain large `diff_output` values. The diff should not be included in list queries — only fetched when the user views a specific cookbook's remediation detail.
- **Partitioning** `node_snapshots` by `collected_at` (range partitioning) should be considered if the table grows beyond tens of millions of rows, as it enables efficient retention purge (drop partition) and faster time-scoped queries.
- **`log_entries`** should similarly be considered for partitioning by `timestamp` at scale.
- **`notification_history`** and **`export_jobs`** are low-volume tables and do not require special performance considerations.

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Data Collection Specification](../data-collection/Specification.md)
- [Analysis Specification](../analysis/Specification.md)
- [Visualisation Specification](../visualisation/Specification.md)
- [Logging Specification](../logging/Specification.md)
- [Authentication Specification](../auth/Specification.md)
- [Configuration Specification](../configuration/Specification.md)