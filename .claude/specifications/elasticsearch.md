# Elasticsearch Export - Component Specification

> **TL;DR:** The app writes NDJSON files (one per document type per export cycle) to a configurable directory. Logstash reads these files and indexes them into a single `chef-migration-metrics` Elasticsearch index. The app has no direct Elasticsearch dependency — it only writes files. Exports are incremental (high-water-mark tracked in the datastore) and idempotent (deterministic `_id` per document). Document types cover nodes, cookbooks, compatibility results, individual CookStyle offenses, readiness, complexity, roles, and metric snapshots. All documents are structured for easy analysis in Kibana — every document carries denormalised cross-cutting fields, a top-level `@timestamp`, and flattened (non-nested) data so that standard Kibana visualisations, aggregations, and the time-picker work without scripted fields or runtime fields.

## Overview

Chef Migration Metrics supports exporting all collected and analysed data to Elasticsearch for advanced analysis with Kibana. The application writes JSON documents to a configurable directory, one file per document type per export cycle. A Logstash pipeline reads these files and indexes them into a single Elasticsearch index. This decoupled design means the application has no direct dependency on Elasticsearch — it only writes files.

## Design Principles

- **Decoupled architecture** — the Go application writes NDJSON (newline-delimited JSON) files to disk. Logstash handles ingestion into Elasticsearch. This keeps the application simple and avoids embedding an Elasticsearch client.
- **Single index** — all document types are stored in a single Elasticsearch index (`chef-migration-metrics`) distinguished by a `doc_type` field. This simplifies Kibana dashboard creation and cross-type correlation.
- **Idempotent exports** — each document includes a deterministic `_id` derived from its natural key so that re-exports overwrite rather than duplicate.
- **Incremental by default** — each export cycle writes only data that has changed since the last successful export, tracked by a high-water-mark timestamp persisted in the datastore.
- **Kibana-first structure** — every document is structured so that standard Kibana visualisations (pie charts, bar charts, data tables, time series, heatmaps, Lens) work out of the box without scripted fields, runtime fields, or complex Painless scripts. This means:
  - **Flat field layout** — fields live at the top level of the document, not nested under a `data` object. Kibana's field discovery, autocomplete, and aggregation UX works best with flat documents.
  - **Denormalised cross-cutting fields** — every document carries the fields that users commonly filter or aggregate on (`organisation_name`, `platform`, `chef_environment`, `target_chef_version`, etc.) even if that means repeating values across document types. This avoids the need for cross-index joins or parent-child relationships which Kibana does not support natively.
  - **`@timestamp` on every document** — Kibana's time-picker binds to `@timestamp`. Every document sets `@timestamp` to the most semantically meaningful time for that document type (e.g. `collected_at` for node snapshots, `scanned_at` for CookStyle results). This lets users filter any document type by time range using the standard Kibana time picker.
  - **No opaque JSON objects** — fields that contain structured data (e.g. metric snapshot payloads) are flattened into explicit top-level fields so Kibana can aggregate on them directly.
  - **Minimal use of nested type** — the Elasticsearch `nested` type is avoided where possible because it requires specialised nested aggregations in Kibana which are unintuitive for most users. Where an array of objects is unavoidable (e.g. `blocking_cookbooks` on readiness), it is typed as `nested` but the most-queried scalar fields are also promoted to top-level keyword arrays for easy non-nested filtering.

---

## Document Types

All documents share a common set of envelope fields at the top level:

| Field | Type | Description |
|-------|------|-------------|
| `doc_type` | keyword | Discriminator: one of the types listed below |
| `doc_id` | keyword | Deterministic document ID (used as Elasticsearch `_id`) |
| `@timestamp` | date (ISO 8601) | Primary time field for Kibana time-picker. Set to the most meaningful time per type (see each type below). |
| `organisation_name` | keyword | Chef organisation name (where applicable; null for git-sourced cookbooks that span orgs) |
| `exported_at` | date (ISO 8601) | Timestamp of the export cycle that produced this document |

> **Note on field naming:** Fields use `snake_case` to match the application's naming conventions. Kibana displays field names verbatim, so consistent casing improves readability.

---

### 1. `node_snapshot`

One document per node per collection run. `@timestamp` is set to `collected_at`.

| Field | Type | Source |
|-------|------|--------|
| `node_name` | keyword | `node_snapshots.node_name` |
| `chef_environment` | keyword | `node_snapshots.chef_environment` |
| `chef_version` | keyword | `node_snapshots.chef_version` |
| `platform` | keyword | `node_snapshots.platform` |
| `platform_version` | keyword | `node_snapshots.platform_version` |
| `platform_family` | keyword | `node_snapshots.platform_family` |
| `policy_name` | keyword | `node_snapshots.policy_name` |
| `policy_group` | keyword | `node_snapshots.policy_group` |
| `ohai_time` | double | `node_snapshots.ohai_time` |
| `is_stale` | boolean | `node_snapshots.is_stale` |
| `cookbook_names` | keyword[] | Extracted from `node_snapshots.cookbooks` JSONB keys |
| `cookbook_count` | integer | Length of `cookbook_names` array |
| `roles` | keyword[] | Extracted from `node_snapshots.roles` JSONB |
| `role_count` | integer | Length of `roles` array |
| `run_list` | keyword[] | Extracted from `node_snapshots.run_list` JSONB |
| `is_policyfile_node` | boolean | `true` if both `policy_name` and `policy_group` are non-null |
| `collected_at` | date | `node_snapshots.collected_at` |
| `stale_days` | integer | Number of days since last check-in, computed from `ohai_time`. Null if `ohai_time` is null. Useful for range-based Kibana filters (e.g. "stale > 14 days"). |

**`doc_id`**: `node_snapshot:<node_snapshots.id>`

**`@timestamp`**: `collected_at`

---

### 2. `cookbook`

One document per cookbook record. `@timestamp` is set to `last_fetched_at` (or `first_seen_at` if never fetched).

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | `cookbooks.name` |
| `cookbook_version` | keyword | `cookbooks.version` |
| `source` | keyword | `cookbooks.source` (`git` or `chef_server`) |
| `git_repo_url` | keyword | `cookbooks.git_repo_url` |
| `head_commit_sha` | keyword | `cookbooks.head_commit_sha` |
| `has_test_suite` | boolean | `cookbooks.has_test_suite` |
| `is_active` | boolean | `cookbooks.is_active` |
| `is_stale_cookbook` | boolean | `cookbooks.is_stale_cookbook` |
| `first_seen_at` | date | `cookbooks.first_seen_at` |
| `last_fetched_at` | date | `cookbooks.last_fetched_at` |

**`doc_id`**: `cookbook:<cookbooks.id>`

**`@timestamp`**: `last_fetched_at` (falls back to `first_seen_at`)

---

### 3. `cookstyle_result`

One document per CookStyle scan result (summary level). `@timestamp` is set to `scanned_at`.

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | Joined from `cookbooks.name` |
| `cookbook_version` | keyword | Joined from `cookbooks.version` |
| `source` | keyword | Joined from `cookbooks.source` |
| `target_chef_version` | keyword | `cookstyle_results.target_chef_version` |
| `passed` | boolean | `cookstyle_results.passed` |
| `offense_count` | integer | `cookstyle_results.offence_count` |
| `deprecation_count` | integer | `cookstyle_results.deprecation_count` |
| `correctness_count` | integer | `cookstyle_results.correctness_count` |
| `style_count` | integer | Count of `ChefStyle/*` offenses (computed during export) |
| `modernize_count` | integer | Count of `ChefModernize/*` offenses (computed during export) |
| `duration_seconds` | integer | `cookstyle_results.duration_seconds` |
| `scanned_at` | date | `cookstyle_results.scanned_at` |

**`doc_id`**: `cookstyle_result:<cookstyle_results.id>`

**`@timestamp`**: `scanned_at`

---

### 4. `cookstyle_offense`

**One document per individual CookStyle offense.** This is the key document type for Kibana analysis of code quality and deprecation patterns. Exporting offenses individually (rather than as a nested array on the summary) lets Kibana natively aggregate by cop name, severity, namespace, file path, etc. without nested aggregations.

`@timestamp` is set to the parent scan's `scanned_at`.

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | Joined from `cookbooks.name` |
| `cookbook_version` | keyword | Joined from `cookbooks.version` |
| `source` | keyword | Joined from `cookbooks.source` |
| `target_chef_version` | keyword | From the parent `cookstyle_results.target_chef_version` |
| `cookstyle_result_id` | keyword | FK back to the parent scan for correlation |
| `file_path` | keyword | `files[].path` from CookStyle JSON output |
| `cop_name` | keyword | `offenses[].cop_name` (e.g. `ChefDeprecations/ResourceWithoutUnifiedTrue`) |
| `cop_namespace` | keyword | Extracted prefix of `cop_name` (e.g. `ChefDeprecations`, `ChefCorrectness`, `ChefStyle`, `ChefModernize`) |
| `severity` | keyword | One of: `convention`, `warning`, `error`, `fatal` |
| `message` | text | Offense message (mapped as `text` for full-text search, with a `.keyword` sub-field) |
| `corrected` | boolean | Whether CookStyle auto-corrected this offense |
| `correctable` | boolean | Whether auto-correct *can* fix this offense (from auto-correct preview, if available) |
| `start_line` | integer | `location.start_line` |
| `start_column` | integer | `location.start_column` |
| `last_line` | integer | `location.last_line` |
| `last_column` | integer | `location.last_column` |
| `remediation_description` | text | From the cop-to-documentation mapping (null if no mapping exists) |
| `remediation_migration_url` | keyword | URL to Chef migration documentation |
| `remediation_introduced_in` | keyword | Chef version where the deprecation was introduced |
| `remediation_removed_in` | keyword | Chef version where the deprecated feature was removed (null if not yet removed) |
| `scanned_at` | date | From the parent `cookstyle_results.scanned_at` |

**`doc_id`**: `cookstyle_offense:<cookstyle_results.id>:<file_path>:<start_line>:<cop_name>` (deterministic composite key)

**`@timestamp`**: `scanned_at`

> **Kibana usage examples:**
> - *Top 20 most common deprecation cops across all cookbooks* — Terms aggregation on `cop_name` filtered by `cop_namespace: ChefDeprecations`.
> - *Offense severity breakdown by cookbook* — Stacked bar chart: X-axis = `cookbook_name`, split by `severity`.
> - *Which Chef version deprecations affect us most?* — Terms aggregation on `remediation_introduced_in`.
> - *Search for specific deprecation patterns* — Full-text search on `message`.

---

### 5. `test_kitchen_result`

One document per Test Kitchen test result. `@timestamp` is set to `completed_at` (or `started_at` if not yet completed).

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | Joined from `cookbooks.name` |
| `target_chef_version` | keyword | `test_kitchen_results.target_chef_version` |
| `commit_sha` | keyword | `test_kitchen_results.commit_sha` |
| `converge_passed` | boolean | `test_kitchen_results.converge_passed` |
| `tests_passed` | boolean | `test_kitchen_results.tests_passed` |
| `compatible` | boolean | `test_kitchen_results.compatible` |
| `duration_seconds` | integer | `test_kitchen_results.duration_seconds` |
| `started_at` | date | `test_kitchen_results.started_at` |
| `completed_at` | date | `test_kitchen_results.completed_at` |

**`doc_id`**: `test_kitchen_result:<test_kitchen_results.id>`

**`@timestamp`**: `completed_at` (falls back to `started_at`)

---

### 6. `cookbook_complexity`

One document per cookbook complexity evaluation. `@timestamp` is set to `evaluated_at`.

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | Joined from `cookbooks.name` |
| `cookbook_version` | keyword | Joined from `cookbooks.version` |
| `source` | keyword | Joined from `cookbooks.source` |
| `target_chef_version` | keyword | `cookbook_complexity.target_chef_version` |
| `complexity_score` | integer | `cookbook_complexity.complexity_score` |
| `complexity_label` | keyword | `cookbook_complexity.complexity_label` |
| `error_count` | integer | `cookbook_complexity.error_count` |
| `deprecation_count` | integer | `cookbook_complexity.deprecation_count` |
| `correctness_count` | integer | `cookbook_complexity.correctness_count` |
| `modernize_count` | integer | `cookbook_complexity.modernize_count` |
| `auto_correctable_count` | integer | `cookbook_complexity.auto_correctable_count` |
| `manual_fix_count` | integer | `cookbook_complexity.manual_fix_count` |
| `affected_node_count` | integer | `cookbook_complexity.affected_node_count` |
| `affected_role_count` | integer | `cookbook_complexity.affected_role_count` |
| `affected_policy_count` | integer | `cookbook_complexity.affected_policy_count` |
| `total_blast_radius` | integer | `affected_node_count + affected_role_count + affected_policy_count` (pre-computed for easy sorting) |
| `priority_score` | float | `complexity_score * log2(affected_node_count + 1)` — balances complexity against blast radius for remediation prioritisation. Pre-computed during export. |
| `evaluated_at` | date | `cookbook_complexity.evaluated_at` |

**`doc_id`**: `cookbook_complexity:<cookbook_complexity.id>`

**`@timestamp`**: `evaluated_at`

---

### 7. `node_readiness`

One document per node readiness evaluation. `@timestamp` is set to `evaluated_at`.

To avoid the Kibana limitations of `nested` aggregations, the most-queried fields from `blocking_cookbooks` are promoted to top-level keyword arrays:

| Field | Type | Source |
|-------|------|--------|
| `node_name` | keyword | `node_readiness.node_name` |
| `target_chef_version` | keyword | `node_readiness.target_chef_version` |
| `chef_environment` | keyword | Denormalised from the node snapshot |
| `platform` | keyword | Denormalised from the node snapshot |
| `platform_family` | keyword | Denormalised from the node snapshot |
| `chef_version` | keyword | Denormalised from the node snapshot (current Chef version on the node) |
| `policy_name` | keyword | Denormalised from the node snapshot |
| `policy_group` | keyword | Denormalised from the node snapshot |
| `is_ready` | boolean | `node_readiness.is_ready` |
| `all_cookbooks_compatible` | boolean | `node_readiness.all_cookbooks_compatible` |
| `sufficient_disk_space` | boolean | `node_readiness.sufficient_disk_space` |
| `blocking_cookbook_names` | keyword[] | Extracted from `blocking_cookbooks[].name` — flat array for easy terms aggregation |
| `blocking_cookbook_count` | integer | Length of `blocking_cookbook_names` |
| `blocking_reason` | keyword | Derived summary: one of `none`, `incompatible_cookbooks`, `insufficient_disk_space`, `both` |
| `blocking_cookbooks` | nested[] | Full `blocking_cookbooks` JSONB (retained for detail drill-down) |
| `available_disk_mb` | integer | `node_readiness.available_disk_mb` |
| `required_disk_mb` | integer | `node_readiness.required_disk_mb` |
| `disk_headroom_mb` | integer | `available_disk_mb - required_disk_mb` (pre-computed; negative means insufficient) |
| `stale_data` | boolean | `node_readiness.stale_data` |
| `evaluated_at` | date | `node_readiness.evaluated_at` |

**`doc_id`**: `node_readiness:<node_readiness.id>`

**`@timestamp`**: `evaluated_at`

> **Kibana usage examples:**
> - *Readiness by environment* — Pie chart on `is_ready` filtered by `chef_environment`.
> - *Which cookbooks block the most nodes?* — Terms aggregation on `blocking_cookbook_names` (no nested query needed).
> - *Disk space distribution of blocked nodes* — Histogram on `disk_headroom_mb` filtered by `sufficient_disk_space: false`.

---

### 8. `role_dependency`

One document per role dependency edge. `@timestamp` is set to `exported_at` (no inherent time dimension).

| Field | Type | Source |
|-------|------|--------|
| `role_name` | keyword | `role_dependencies.role_name` |
| `dependency_type` | keyword | `role_dependencies.dependency_type` (`role` or `cookbook`) |
| `dependency_name` | keyword | `role_dependencies.dependency_name` |

**`doc_id`**: `role_dependency:<role_dependencies.id>`

**`@timestamp`**: `exported_at`

---

### 9. `metric_snapshot`

Metric snapshots are **flattened** during export so that Kibana can aggregate on individual metric values. Rather than storing an opaque `metrics` JSON object, each metric snapshot is exported as one document with all metrics as explicit top-level fields. The `snapshot_type` field distinguishes the three snapshot types, and each type populates only its relevant fields (the others are null/absent).

`@timestamp` is set to `snapshot_at`.

**Common fields (all snapshot types):**

| Field | Type | Source |
|-------|------|--------|
| `snapshot_type` | keyword | `metric_snapshots.snapshot_type` |
| `target_chef_version` | keyword | `metric_snapshots.target_chef_version` |
| `snapshot_at` | date | `metric_snapshots.snapshot_at` |

**Fields for `snapshot_type: chef_version_distribution`:**

| Field | Type | Source |
|-------|------|--------|
| `total_nodes` | integer | `data.total_nodes` |
| `version_name` | keyword | Flattened: one document per version entry. The version string (e.g. `18.5.0`). |
| `version_count` | integer | `data.versions[version].count` |
| `version_percentage` | float | `data.versions[version].percentage` |

> **Flattening strategy:** The `chef_version_distribution` snapshot contains a `versions` map with variable keys. This is flattened into **one document per version per snapshot** so that Kibana can aggregate on `version_name` directly. Each flattened document carries the same `total_nodes` value and `snapshot_at` timestamp. The `doc_id` includes the version string to ensure idempotency.

**`doc_id`**: `metric_snapshot:cvd:<metric_snapshots.id>:<version_name>`

**Fields for `snapshot_type: readiness_summary`:**

| Field | Type | Source |
|-------|------|--------|
| `total_nodes` | integer | `data.total_nodes` |
| `ready_count` | integer | `data.ready` |
| `blocked_count` | integer | `data.blocked` |
| `ready_percentage` | float | Computed: `ready / total_nodes * 100` |
| `blocked_by_cookbooks` | integer | `data.blocking_reasons.incompatible_cookbooks` |
| `blocked_by_disk` | integer | `data.blocking_reasons.insufficient_disk_space` |
| `blocked_by_both` | integer | `data.blocking_reasons.both` |

**`doc_id`**: `metric_snapshot:rs:<metric_snapshots.id>`

**Fields for `snapshot_type: cookbook_compatibility`:**

| Field | Type | Source |
|-------|------|--------|
| `total_cookbooks` | integer | `data.total_cookbooks` |
| `compatible_count` | integer | `data.compatible` |
| `incompatible_count` | integer | `data.incompatible` |
| `cookstyle_only_count` | integer | `data.cookstyle_only` |
| `untested_count` | integer | `data.untested` |
| `compatible_percentage` | float | Computed: `compatible / total_cookbooks * 100` |

**`doc_id`**: `metric_snapshot:cc:<metric_snapshots.id>`

**`@timestamp`**: `snapshot_at` (all snapshot types)

> **Kibana usage examples:**
> - *Readiness trend over time* — Line chart: X-axis = `@timestamp`, Y-axis = `ready_percentage`, filtered by `doc_type: metric_snapshot` and `snapshot_type: readiness_summary`.
> - *Chef version adoption over time* — Area chart: X-axis = `@timestamp`, split by `version_name`, Y-axis = `version_percentage`, filtered by `snapshot_type: chef_version_distribution`.
> - *Compatibility progress* — Stacked area: X-axis = `@timestamp`, Y-axis = `compatible_count` + `incompatible_count` + `untested_count`.

---

### 10. `autocorrect_preview`

One document per auto-correct preview. `@timestamp` is set to `generated_at`.

| Field | Type | Source |
|-------|------|--------|
| `cookbook_name` | keyword | Joined from `cookbooks.name` |
| `cookbook_version` | keyword | Joined from `cookbooks.version` |
| `source` | keyword | Joined from `cookbooks.source` |
| `target_chef_version` | keyword | Joined from `cookstyle_results.target_chef_version` via `cookstyle_result_id` |
| `total_offenses` | integer | `autocorrect_previews.total_offenses` |
| `correctable_offenses` | integer | `autocorrect_previews.correctable_offenses` |
| `remaining_offenses` | integer | `autocorrect_previews.remaining_offenses` |
| `correctable_percentage` | float | Computed: `correctable_offenses / total_offenses * 100` (0 if total is 0) |
| `files_modified` | integer | `autocorrect_previews.files_modified` |
| `generated_at` | date | `autocorrect_previews.generated_at` |

The `diff_output` text is intentionally **not** exported to Elasticsearch — it is large, not useful for aggregation, and is available via the Web API for detail views. Keeping it out reduces index size significantly.

**`doc_id`**: `autocorrect_preview:<autocorrect_previews.id>`

**`@timestamp`**: `generated_at`

---

## Export File Format

Each export cycle produces one or more NDJSON files in the configured `elasticsearch.output_directory`. Each line is a self-contained JSON document.

### File Naming

```
<doc_type>_<timestamp>.ndjson
```

Example: `node_snapshot_20250110T143022Z.ndjson`, `cookstyle_offense_20250110T143022Z.ndjson`

### Line Format

Each line is a JSON object with the envelope fields and the type-specific fields at the top level (no `data` wrapper):

```json
{"doc_type":"node_snapshot","doc_id":"node_snapshot:a1b2c3d4","@timestamp":"2025-01-10T14:00:00Z","organisation_name":"myorg-production","exported_at":"2025-01-10T14:30:22Z","node_name":"web01.example.com","chef_version":"18.5.0","platform":"ubuntu","platform_version":"22.04","platform_family":"debian","chef_environment":"production","is_stale":false,"is_policyfile_node":false,"cookbook_names":["apache2","apt","users"],"cookbook_count":3,"roles":["webserver"],"role_count":1,"collected_at":"2025-01-10T14:00:00Z","stale_days":0}
```

```json
{"doc_type":"cookstyle_offense","doc_id":"cookstyle_offense:x1y2z3:recipes/default.rb:10:ChefDeprecations/ResourceWithoutUnifiedTrue","@timestamp":"2025-01-10T14:15:00Z","organisation_name":"myorg-production","exported_at":"2025-01-10T14:30:22Z","cookbook_name":"mycookbook","cookbook_version":"1.2.0","source":"chef_server","target_chef_version":"18.5.0","cop_name":"ChefDeprecations/ResourceWithoutUnifiedTrue","cop_namespace":"ChefDeprecations","severity":"warning","message":"Set unified_mode true in Chef Infra Client 15.3+","file_path":"recipes/default.rb","start_line":10,"start_column":1,"last_line":10,"last_column":30,"corrected":false,"correctable":true,"remediation_description":"Custom resources should enable unified mode for compatibility with Chef 18+.","remediation_migration_url":"https://docs.chef.io/unified_mode/","remediation_introduced_in":"15.3","remediation_removed_in":null,"scanned_at":"2025-01-10T14:15:00Z"}
```

### File Lifecycle

1. Files are written to the output directory with a `.tmp` suffix during writing.
2. Once fully written, the file is atomically renamed to remove the `.tmp` suffix.
3. Logstash is configured to only read files without the `.tmp` suffix, avoiding partial reads.
4. After Logstash processes a file, it tracks the sincedb position. The application retains files for the configured `elasticsearch.retention_hours` before deleting them.

---

## Export Scheduling

The Elasticsearch export runs as a post-processing step after each collection and analysis cycle completes. It can also be triggered manually via the Web API.

### High-Water-Mark Tracking

The application tracks the last successful Elasticsearch export timestamp in the `export_jobs` table (with `export_type = 'elasticsearch'`). Each export cycle queries for records with `created_at` or `updated_at` after the high-water mark, writes the NDJSON files, and then updates the high-water mark.

On first run (no high-water mark), all data is exported.

### Volume Considerations for `cookstyle_offense`

The `cookstyle_offense` document type produces significantly more documents than other types — potentially hundreds of offenses per cookbook scan. To manage volume:

- Offenses are only exported for the **most recent** CookStyle scan per cookbook per target version (matching the incremental high-water-mark model).
- When a cookbook is rescanned, the new offense documents overwrite the previous ones (deterministic `doc_id`).
- The export logs the offense count per cycle at `INFO` severity so operators can monitor volume.

---

## Configuration

The Elasticsearch export is configured under the `elasticsearch` key in the application configuration file. See the [Configuration Specification](../configuration/Specification.md) for full details.

```yaml
elasticsearch:
  enabled: false
  output_directory: /var/lib/chef-migration-metrics/elasticsearch
  retention_hours: 48
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `false` | Whether the Elasticsearch export is active. When disabled, no NDJSON files are written. |
| `output_directory` | `/var/lib/chef-migration-metrics/elasticsearch` | Directory where NDJSON files are written for Logstash to pick up. Must be writable by the application and readable by Logstash. |
| `retention_hours` | `48` | How long NDJSON files are retained before deletion. Should be long enough for Logstash to process them. |

---

## Logstash Pipeline

A Logstash pipeline definition is maintained at `deploy/elk/logstash/pipeline/chef-migration-metrics.conf`. It reads NDJSON files from the output directory, extracts the `doc_id` for use as the Elasticsearch `_id`, and indexes all documents into a single index.

### Pipeline Design

```
input (file) → filter (json, mutate) → output (elasticsearch)
```

- **Input**: Reads `*.ndjson` files from the output directory using the `file` input plugin with `sincedb` tracking to avoid re-processing.
- **Filter**: Parses each line as JSON. Extracts `doc_id` for use as the Elasticsearch document `_id`. Removes Logstash-internal fields (`@version`, `host`, `path`, `log`). Parses all ISO 8601 date strings into proper date types.
- **Output**: Indexes into the `chef-migration-metrics` index with the extracted `_id` for upsert behaviour.

### `@timestamp` Handling

The application sets `@timestamp` on every document before writing. The Logstash pipeline must **not** overwrite `@timestamp` with the Logstash ingestion time. The filter section includes:

```
filter {
  # Preserve the application-set @timestamp — do not overwrite with Logstash ingestion time
  date {
    match => ["@timestamp", "ISO8601"]
    target => "@timestamp"
  }
}
```

### Index Mapping

The pipeline relies on an explicit index template (`deploy/elk/logstash/pipeline/chef-migration-metrics-template.json`) that defines field mappings for all document types. Key mapping decisions:

- All string fields default to `keyword` (via a dynamic template) for exact-match filtering and terms aggregation.
- The `message` field on `cookstyle_offense` is explicitly mapped as `text` with a `.keyword` sub-field to support both full-text search and exact aggregation.
- Date fields are explicitly mapped as `date` type.
- Numeric fields are explicitly mapped as `integer`, `float`, or `double` as appropriate.
- `blocking_cookbooks` on `node_readiness` is mapped as `nested` for users who need exact co-occurrence queries, but the top-level `blocking_cookbook_names` keyword array serves most Kibana use cases.
- The `metrics` object type used in the old spec is **removed** — metric snapshots are fully flattened.

The index template is applied automatically by the Logstash pipeline on first run.

---

## ELK Testing Stack

A Docker Compose file at `deploy/elk/docker-compose.yml` provides a local Elasticsearch, Logstash, and Kibana stack for testing the export pipeline. It mounts the Logstash pipeline configuration and a shared volume where the application writes NDJSON files.

### Services

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| `elasticsearch` | `docker.elastic.co/elasticsearch/elasticsearch:8.17.0` | `9200:9200` | Stores indexed documents |
| `logstash` | `docker.elastic.co/logstash/logstash:8.17.0` | `9600:9600` (monitoring) | Reads NDJSON files, indexes into Elasticsearch |
| `kibana` | `docker.elastic.co/kibana/kibana:8.17.0` | `5601:5601` | Visual analysis and dashboarding |

### Shared Volume

A named volume `es_export_data` is shared between the application (writer) and Logstash (reader). When running the ELK stack alongside the main application Docker Compose stack, the application's `elasticsearch.output_directory` should be configured to write to this shared volume path.

### Security

The testing stack runs with Elasticsearch security disabled (`xpack.security.enabled=false`) and Logstash monitoring disabled for simplicity. This is suitable for local development and testing only — production deployments should use a properly secured Elasticsearch cluster.

---

## Kibana Data View Setup

After the ELK stack is running and data has been indexed, users must create a Kibana **data view** (formerly "index pattern") to begin exploring the data.

### Creating the Data View

1. Open Kibana at `http://localhost:5601`.
2. Navigate to **Stack Management → Data Views** (or **Management → Index Patterns** in older Kibana versions).
3. Click **Create data view**.
4. Set the **Name** to `chef-migration-metrics`.
5. Set the **Index pattern** to `chef-migration-metrics*`.
6. Set the **Timestamp field** to `@timestamp`.
7. Click **Save data view to Kibana**.

The `@timestamp` binding means the Kibana time-picker (top-right corner) now controls time filtering for all document types — users can scope any visualisation to "Last 24 hours", "Last 7 days", or a custom range.

### Recommended Field Formatters

In the data view settings, apply these Kibana field formatters for improved readability:

| Field | Formatter | Format |
|-------|-----------|--------|
| `ohai_time` | Date | Unix timestamp (seconds) → human-readable date |
| `duration_seconds` | Duration | Seconds → `HH:mm:ss` |
| `version_percentage`, `ready_percentage`, `compatible_percentage`, `correctable_percentage` | Percent | Number with 1 decimal place |
| `remediation_migration_url` | URL | Clickable link |
| `complexity_score` | Number | No decimal places |
| `disk_headroom_mb` | Bytes | MB display |

---

## Kibana Dashboards

The ELK testing stack does not ship pre-built Kibana saved objects. Users create dashboards interactively in Kibana using the `chef-migration-metrics` data view. The following sections describe recommended visualisations and the exact queries/aggregations to use.

### Migration Progress Overview Dashboard

A single-page executive summary. All panels share the same time range via the Kibana time-picker.

**1. Fleet Readiness Gauge**

- Type: Metric / Gauge
- Query: `doc_type: metric_snapshot AND snapshot_type: readiness_summary`
- Metric: Latest `ready_percentage`
- Colour bands: 0–50% red, 50–80% amber, 80–100% green

**2. Node Readiness Pie**

- Type: Pie chart
- Query: `doc_type: node_readiness`
- Slice by: `is_ready` (true/false)
- Split by: `target_chef_version` (one pie per target)

**3. Blocking Reasons Breakdown**

- Type: Horizontal bar chart
- Query: `doc_type: node_readiness AND is_ready: false`
- X-axis: Count
- Y-axis: `blocking_reason` terms

**4. Chef Client Version Distribution**

- Type: Donut chart
- Query: `doc_type: node_snapshot`
- Slice by: `chef_version` terms (top 15)

**5. Chef Version Trend**

- Type: Area chart (stacked)
- Query: `doc_type: metric_snapshot AND snapshot_type: chef_version_distribution`
- X-axis: `@timestamp` (date histogram)
- Y-axis: `version_count`
- Split by: `version_name`

**6. Readiness Trend**

- Type: Line chart
- Query: `doc_type: metric_snapshot AND snapshot_type: readiness_summary`
- X-axis: `@timestamp` (date histogram)
- Y-axis (lines): `ready_count`, `blocked_count`

**7. Platform Distribution**

- Type: Horizontal bar chart
- Query: `doc_type: node_snapshot`
- Y-axis: `platform_family` terms
- X-axis: Count
- Split bar by: `is_stale` for visual distinction

### Cookbook Remediation Dashboard

Focused on practitioners doing remediation work.

**8. Remediation Priority Table**

- Type: Data table
- Query: `doc_type: cookbook_complexity AND complexity_label: (low OR medium OR high OR critical)`
- Columns: `cookbook_name`, `target_chef_version`, `complexity_label`, `complexity_score`, `priority_score`, `affected_node_count`, `auto_correctable_count`, `manual_fix_count`
- Sort by: `priority_score` descending

**9. Complexity Distribution**

- Type: Pie chart
- Query: `doc_type: cookbook_complexity`
- Slice by: `complexity_label` terms
- Colour mapping: `none` → green, `low` → blue, `medium` → amber, `high` → orange, `critical` → red

**10. Top 20 Deprecation Cops**

- Type: Horizontal bar chart
- Query: `doc_type: cookstyle_offense AND cop_namespace: ChefDeprecations`
- Y-axis: `cop_name` terms (top 20)
- X-axis: Count

**11. Offense Severity Breakdown by Cookbook**

- Type: Stacked bar chart
- Query: `doc_type: cookstyle_offense`
- X-axis: `cookbook_name` terms (top 20)
- Stack by: `severity` terms
- Colour mapping: `convention` → grey, `warning` → amber, `error` → red, `fatal` → dark red

**12. Auto-Correct Coverage**

- Type: Metric + bar chart
- Query: `doc_type: autocorrect_preview`
- Metrics: Average `correctable_percentage`, sum of `correctable_offenses`, sum of `remaining_offenses`
- Bar chart: `cookbook_name` on X-axis, stacked bars of `correctable_offenses` (green) and `remaining_offenses` (red)

**13. Deprecation Impact by Chef Version**

- Type: Heatmap
- Query: `doc_type: cookstyle_offense AND cop_namespace: ChefDeprecations`
- Y-axis: `remediation_introduced_in` terms
- X-axis: `cookbook_name` terms
- Cell value: Count

**14. CookStyle Pass/Fail Over Time**

- Type: Stacked area chart
- Query: `doc_type: cookstyle_result`
- X-axis: `@timestamp` (date histogram)
- Y-axis: Count
- Split by: `passed` (true/false)

### Node Detail Dashboard

For investigating individual nodes or node groups.

**15. Stale Node Count by Environment**

- Type: Data table
- Query: `doc_type: node_snapshot AND is_stale: true`
- Row split: `chef_environment` terms
- Metric: Count, average `stale_days`

**16. Blocking Cookbook Frequency**

- Type: Tag cloud or horizontal bar
- Query: `doc_type: node_readiness AND is_ready: false`
- Terms: `blocking_cookbook_names` (top 30)

**17. Disk Space Distribution**

- Type: Histogram
- Query: `doc_type: node_readiness`
- X-axis: `disk_headroom_mb` histogram (bucket size: 500)
- Y-axis: Count
- Split by: `sufficient_disk_space` (true/false)

**18. Policyfile vs Classic Node Breakdown**

- Type: Pie chart
- Query: `doc_type: node_snapshot`
- Slice by: `is_policyfile_node`

**19. Node Readiness by Environment and Target Version**

- Type: Heatmap
- Query: `doc_type: node_readiness`
- Y-axis: `chef_environment` terms
- X-axis: `target_chef_version` terms
- Cell value: Average of `is_ready` (displays as percentage ready, 0.0–1.0)

---

## Kibana Discover Tips

Include the following guidance in the ELK stack README for users exploring data in Kibana Discover:

- **Filter by document type first** — Add a filter `doc_type: <type>` to scope the field list. Since all types share one index, the field list is long; filtering by type makes it manageable.
- **Use the time-picker** — `@timestamp` is set on every document, so the time-picker always works. For historical trend analysis, set it to a wide range.
- **Saved searches per type** — Create saved searches like "All node snapshots", "CookStyle offenses — deprecations only", "Blocked nodes" to quickly switch contexts.
- **Pin common filters** — Pin `organisation_name` and `target_chef_version` filters to maintain context across tab switches.

---

## Error Handling

- If the output directory is not writable, the export logs an `ERROR` and skips the cycle. The high-water mark is not updated so the next cycle retries.
- If a database query fails during export, the error is logged and the entire cycle is aborted without updating the high-water mark.
- File writes use a `.tmp` suffix and atomic rename to prevent Logstash from reading partial files.
- The Logstash pipeline is configured with `sincedb_write_interval` to track read positions, ensuring files are not reprocessed after a Logstash restart.
- If a single document fails to serialise (e.g. invalid UTF-8 in a CookStyle message), the document is skipped and the error is logged at `WARN`. The rest of the export continues.

---

## Logging

The Elasticsearch export uses the `export_job` log scope. The following events are logged:

| Event | Severity | Message |
|-------|----------|---------|
| Export cycle started | `INFO` | `Elasticsearch export started` |
| Document type exported | `INFO` | `Exported <count> <doc_type> documents` |
| Offense documents exported | `INFO` | `Exported <count> cookstyle_offense documents from <scan_count> scans` |
| Export cycle completed | `INFO` | `Elasticsearch export completed: <total> documents in <duration>` |
| Output directory not writable | `ERROR` | `Elasticsearch export skipped: output directory not writable: <path>` |
| Database query failed | `ERROR` | `Elasticsearch export aborted: <error>` |
| File write failed | `ERROR` | `Elasticsearch export failed writing <filename>: <error>` |
| Document serialisation failed | `WARN` | `Elasticsearch export skipped document <doc_id>: <error>` |
| Stale file cleaned up | `DEBUG` | `Deleted expired Elasticsearch export file: <filename>` |
| Export disabled | `DEBUG` | `Elasticsearch export is disabled, skipping` |

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Configuration Specification](../configuration/Specification.md) — `elasticsearch` configuration section
- [Datastore Specification](../datastore/Specification.md) — source tables for export data
- [Logging Specification](../logging/Specification.md) — `export_job` log scope
- [Packaging Specification](../packaging/Specification.md) — ELK Docker Compose stack
- [Analysis Specification](../analysis/Specification.md) — CookStyle offenses, complexity scoring, remediation guidance
- [Visualisation Specification](../visualisation/Specification.md) — dashboard views that parallel the Kibana dashboards