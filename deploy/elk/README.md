# ELK Testing Stack

A Docker Compose environment providing Elasticsearch, Logstash, and Kibana for testing the Chef Migration Metrics Elasticsearch export pipeline.

## Overview

Chef Migration Metrics can export all collected and analysed data as NDJSON (newline-delimited JSON) files. A Logstash pipeline reads these files and indexes every document type into a single Elasticsearch index (`chef-migration-metrics`), where Kibana can be used for interactive analysis and dashboarding.

This stack is intended for **local development and testing only**. It runs with Elasticsearch security disabled and default credentials. Do not use it in production.

### Architecture

```
┌──────────────────────────┐
│  Chef Migration Metrics  │
│  (application)           │
│                          │
│  Writes NDJSON files to  │
│  elasticsearch.output_   │
│  directory               │
└────────────┬─────────────┘
             │  *.ndjson files
             ▼
┌──────────────────────────┐
│  Shared Volume           │
│  (es_export_data)        │
└────────────┬─────────────┘
             │  reads files
             ▼
┌──────────────────────────┐       ┌──────────────────────────┐
│  Logstash                │       │  Kibana                  │
│  :9600 (monitoring)      │──────▶│  :5601 (web UI)          │
│                          │       │                          │
│  Parses JSON, extracts   │       │  Query, visualise, and   │
│  doc_id, indexes into ES │       │  dashboard the data      │
└────────────┬─────────────┘       └────────────┬─────────────┘
             │  indexes                          │  queries
             ▼                                   ▼
┌──────────────────────────────────────────────────────────────┐
│  Elasticsearch                                               │
│  :9200 (HTTP API)                                            │
│                                                              │
│  Single index: chef-migration-metrics                        │
│  All document types distinguished by doc_type field          │
└──────────────────────────────────────────────────────────────┘
```

### Document Structure

All documents use a **flat field layout** — fields sit at the top level of the JSON document, not nested under a `data` wrapper. This design is optimised for Kibana:

- Kibana's field discovery, autocomplete, and aggregation UX works best with flat documents.
- Every document carries an `@timestamp` field bound to the Kibana time-picker.
- Denormalised cross-cutting fields (`organisation_name`, `platform`, `chef_environment`, etc.) appear directly on documents that need them, avoiding the need for cross-index joins.
- No opaque JSON objects — metric snapshots are fully flattened into explicit fields.

## Prerequisites

- **Docker** and **Docker Compose** (v2)
- At least **2 GB of free RAM** for the ELK stack (Elasticsearch 512 MB + Logstash 256 MB + Kibana)

## Quick Start

```bash
cd deploy/elk

# Copy and optionally edit the environment file
cp .env.example .env

# Start the stack
docker compose up -d

# Watch logs until all services are healthy
docker compose logs -f
```

Wait for all three services to report healthy:

```bash
docker compose ps
```

| Service | URL | Purpose |
|---------|-----|---------|
| Elasticsearch | http://localhost:9200 | Index and search API |
| Logstash | http://localhost:9600 | Monitoring API |
| Kibana | http://localhost:5601 | Web UI for analysis |

## Connecting to the Application

### Option A: Shared Host Directory (Recommended for Local Development)

Configure the application to write NDJSON files to a directory on the host, and bind-mount that directory into the Logstash container.

1. Set the application configuration:

   ```yaml
   elasticsearch:
     enabled: true
     output_directory: /path/to/export/directory
   ```

2. Create a `docker-compose.override.yml` in `deploy/elk/`:

   ```yaml
   services:
     logstash:
       volumes:
         - /path/to/export/directory:/export-data:ro
   ```

3. Restart Logstash to pick up the new mount:

   ```bash
   docker compose up -d logstash
   ```

### Option B: Docker Volume (When App Runs in Docker)

When the application runs in the main Docker Compose stack (`deploy/docker-compose/`), both stacks can share the `es_export_data` named volume.

1. In the main `deploy/docker-compose/docker-compose.yml`, add the external volume to the app service:

   ```yaml
   services:
     app:
       volumes:
         - es_export_data:/var/lib/chef-migration-metrics/elasticsearch

   volumes:
     es_export_data:
       external: true
       name: elk_es_export_data
   ```

2. Set the application configuration:

   ```yaml
   elasticsearch:
     enabled: true
     output_directory: /var/lib/chef-migration-metrics/elasticsearch
   ```

3. Start the ELK stack first (to create the volume), then the application stack:

   ```bash
   cd deploy/elk && docker compose up -d
   cd ../docker-compose && docker compose up -d
   ```

### Option C: Manual Testing with Sample Data

You can write NDJSON files directly into the Logstash input volume to test the pipeline without running the application.

Create a sample file:

```bash
cat > /tmp/test_data.ndjson << 'EOF'
{"doc_type":"node_snapshot","doc_id":"node_snapshot:test-001","@timestamp":"2025-01-10T14:00:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","node_name":"web01.example.com","chef_environment":"production","chef_version":"18.5.0","platform":"ubuntu","platform_version":"22.04","platform_family":"debian","is_stale":false,"is_policyfile_node":false,"cookbook_names":["apache2","apt","users"],"cookbook_count":3,"roles":["webserver"],"role_count":1,"run_list":["role[webserver]"],"collected_at":"2025-01-10T14:00:00Z","stale_days":0}
{"doc_type":"node_snapshot","doc_id":"node_snapshot:test-002","@timestamp":"2025-01-10T14:00:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","node_name":"db01.example.com","chef_environment":"production","chef_version":"17.10.0","platform":"centos","platform_version":"8","platform_family":"rhel","is_stale":true,"is_policyfile_node":false,"cookbook_names":["postgresql","apt"],"cookbook_count":2,"roles":["database"],"role_count":1,"run_list":["role[database]"],"collected_at":"2025-01-10T14:00:00Z","stale_days":12}
{"doc_type":"cookstyle_result","doc_id":"cookstyle_result:test-010","@timestamp":"2025-01-10T14:15:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","cookbook_name":"my_webapp","cookbook_version":"2.1.0","source":"chef_server","target_chef_version":"19.0.0","passed":false,"offense_count":10,"deprecation_count":8,"correctness_count":2,"style_count":0,"modernize_count":0,"duration_seconds":12,"scanned_at":"2025-01-10T14:15:00Z"}
{"doc_type":"cookstyle_offense","doc_id":"cookstyle_offense:test-010:recipes/default.rb:10:ChefDeprecations/ResourceWithoutUnifiedTrue","@timestamp":"2025-01-10T14:15:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","cookbook_name":"my_webapp","cookbook_version":"2.1.0","source":"chef_server","target_chef_version":"19.0.0","cookstyle_result_id":"test-010","file_path":"recipes/default.rb","cop_name":"ChefDeprecations/ResourceWithoutUnifiedTrue","cop_namespace":"ChefDeprecations","severity":"warning","message":"Set unified_mode true in Chef Infra Client 15.3+","corrected":false,"correctable":true,"start_line":10,"start_column":1,"last_line":10,"last_column":30,"remediation_description":"Custom resources should enable unified mode for compatibility with Chef 18+.","remediation_migration_url":"https://docs.chef.io/unified_mode/","remediation_introduced_in":"15.3","remediation_removed_in":null,"scanned_at":"2025-01-10T14:15:00Z"}
{"doc_type":"cookbook_complexity","doc_id":"cookbook_complexity:test-003","@timestamp":"2025-01-10T14:20:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","cookbook_name":"my_webapp","cookbook_version":"2.1.0","source":"chef_server","target_chef_version":"19.0.0","complexity_score":35,"complexity_label":"high","error_count":2,"deprecation_count":8,"correctness_count":2,"modernize_count":0,"auto_correctable_count":6,"manual_fix_count":4,"affected_node_count":120,"affected_role_count":3,"affected_policy_count":2,"total_blast_radius":125,"priority_score":243.5,"evaluated_at":"2025-01-10T14:20:00Z"}
{"doc_type":"node_readiness","doc_id":"node_readiness:test-004","@timestamp":"2025-01-10T14:25:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","node_name":"web01.example.com","target_chef_version":"19.0.0","chef_environment":"production","platform":"ubuntu","platform_family":"debian","chef_version":"18.5.0","is_ready":false,"all_cookbooks_compatible":false,"sufficient_disk_space":true,"blocking_cookbook_names":["my_webapp"],"blocking_cookbook_count":1,"blocking_reason":"incompatible_cookbooks","blocking_cookbooks":[{"name":"my_webapp","version":"2.1.0","reason":"incompatible","source":"chef_server","complexity_score":35,"complexity_label":"high"}],"available_disk_mb":4096,"required_disk_mb":2048,"disk_headroom_mb":2048,"stale_data":false,"evaluated_at":"2025-01-10T14:25:00Z"}
{"doc_type":"autocorrect_preview","doc_id":"autocorrect_preview:test-020","@timestamp":"2025-01-10T14:16:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","cookbook_name":"my_webapp","cookbook_version":"2.1.0","source":"chef_server","target_chef_version":"19.0.0","total_offenses":10,"correctable_offenses":6,"remaining_offenses":4,"correctable_percentage":60.0,"files_modified":3,"generated_at":"2025-01-10T14:16:00Z"}
{"doc_type":"metric_snapshot","doc_id":"metric_snapshot:rs:test-030","@timestamp":"2025-01-10T14:20:00Z","organisation_name":"test-org","exported_at":"2025-01-10T14:30:22Z","snapshot_type":"readiness_summary","target_chef_version":"19.0.0","snapshot_at":"2025-01-10T14:20:00Z","total_nodes":500,"ready_count":380,"blocked_count":120,"ready_percentage":76.0,"blocked_by_cookbooks":90,"blocked_by_disk":25,"blocked_by_both":5}
EOF
```

Copy it into the shared volume:

```bash
docker compose cp /tmp/test_data.ndjson cmm-logstash:/export-data/test_data.ndjson
```

Verify the data was indexed:

```bash
curl -s 'http://localhost:9200/chef-migration-metrics/_count' | jq .
curl -s 'http://localhost:9200/chef-migration-metrics/_search?pretty&size=5'
```

## Setting Up Kibana

Once data is indexed, you need to create a **data view** in Kibana before you can explore or visualise the data. The `@timestamp` binding is critical — it enables Kibana's time-picker to control time filtering for all document types.

### Step 1: Create the Data View

1. Open Kibana at http://localhost:5601
2. Navigate to **Stack Management** → **Data Views** (under the Kibana section)
3. Click **Create data view**
4. Set the **Name** to `chef-migration-metrics`
5. Set the **Index pattern** to `chef-migration-metrics*`
6. Set the **Timestamp field** to `@timestamp`
7. Click **Save data view to Kibana**

> **Why `@timestamp`?** The application sets `@timestamp` on every document to the most semantically meaningful time for that document type — `collected_at` for node snapshots, `scanned_at` for CookStyle results, `evaluated_at` for readiness evaluations, `snapshot_at` for metric snapshots, etc. This means the Kibana time-picker (top-right corner) always controls time filtering correctly, regardless of document type.

### Step 2: Recommended Field Formatters

In the data view settings (**Stack Management → Data Views → chef-migration-metrics → Fields**), apply these formatters for improved readability:

| Field | Formatter | Notes |
|-------|-----------|-------|
| `ohai_time` | Date (Unix seconds) | Converts Unix timestamp to human-readable date |
| `duration_seconds` | Duration (seconds) | Displays as `HH:mm:ss` |
| `version_percentage` | Percent | 1 decimal place |
| `ready_percentage` | Percent | 1 decimal place |
| `compatible_percentage` | Percent | 1 decimal place |
| `correctable_percentage` | Percent | 1 decimal place |
| `remediation_migration_url` | URL | Renders as a clickable link |
| `disk_headroom_mb` | Number | Suffix: ` MB` |

### Step 3: Create Saved Searches

Create saved searches in **Discover** to quickly switch between document types. Since all types share a single index, scoping by `doc_type` keeps the field list manageable.

| Saved Search Name | KQL Query |
|-------------------|-----------|
| All Node Snapshots | `doc_type: "node_snapshot"` |
| Stale Nodes | `doc_type: "node_snapshot" and is_stale: true` |
| CookStyle Results | `doc_type: "cookstyle_result"` |
| CookStyle Offenses — Deprecations | `doc_type: "cookstyle_offense" and cop_namespace: "ChefDeprecations"` |
| CookStyle Offenses — Errors | `doc_type: "cookstyle_offense" and severity: ("error" or "fatal")` |
| Blocked Nodes | `doc_type: "node_readiness" and is_ready: false` |
| Cookbook Complexity — Critical/High | `doc_type: "cookbook_complexity" and complexity_label: ("critical" or "high")` |
| Readiness Trend | `doc_type: "metric_snapshot" and snapshot_type: "readiness_summary"` |

## Kibana Dashboards

The ELK testing stack does not ship pre-built Kibana saved objects. Users create dashboards interactively using the `chef-migration-metrics` data view. The following sections describe recommended dashboards with specific panel configurations.

### Migration Progress Overview

A single-page executive summary. All panels share the same time range via the Kibana time-picker.

#### 1. Fleet Readiness Gauge

- **Type:** Metric / Gauge (Lens)
- **Query:** `doc_type: "metric_snapshot" and snapshot_type: "readiness_summary"`
- **Metric:** Latest value of `ready_percentage`
- **Colour bands:** 0–50% red, 50–80% amber, 80–100% green

#### 2. Node Readiness Pie

- **Type:** Pie chart (Lens)
- **Query:** `doc_type: "node_readiness"`
- **Slice by:** `is_ready` (true/false)
- **Tip:** Create one pie per target version by adding `target_chef_version` as a breakdown dimension

#### 3. Blocking Reasons Breakdown

- **Type:** Horizontal bar chart (Lens)
- **Query:** `doc_type: "node_readiness" and is_ready: false`
- **X-axis:** Count of records
- **Y-axis:** `blocking_reason` (terms aggregation)

#### 4. Chef Client Version Distribution

- **Type:** Donut chart (Lens)
- **Query:** `doc_type: "node_snapshot"`
- **Slice by:** `chef_version` (top 15 terms)

#### 5. Chef Version Adoption Trend

- **Type:** Area chart, stacked (Lens)
- **Query:** `doc_type: "metric_snapshot" and snapshot_type: "chef_version_distribution"`
- **X-axis:** `@timestamp` (date histogram, auto interval)
- **Y-axis:** `version_count`
- **Breakdown:** `version_name`

#### 6. Readiness Trend Over Time

- **Type:** Line chart (Lens)
- **Query:** `doc_type: "metric_snapshot" and snapshot_type: "readiness_summary"`
- **X-axis:** `@timestamp` (date histogram)
- **Lines:** `ready_count` (green), `blocked_count` (red)

#### 7. Platform Distribution

- **Type:** Horizontal bar chart (Lens)
- **Query:** `doc_type: "node_snapshot"`
- **Y-axis:** `platform_family` (terms)
- **X-axis:** Count
- **Breakdown:** `is_stale` to distinguish stale vs. active nodes

#### 8. Policyfile vs Classic Nodes

- **Type:** Pie chart (Lens)
- **Query:** `doc_type: "node_snapshot"`
- **Slice by:** `is_policyfile_node`

### Cookbook Remediation Dashboard

Focused on practitioners actively doing remediation work.

#### 9. Remediation Priority Table

- **Type:** Data table (Lens)
- **Query:** `doc_type: "cookbook_complexity" and complexity_label: ("low" or "medium" or "high" or "critical")`
- **Columns:** `cookbook_name`, `target_chef_version`, `complexity_label`, `complexity_score`, `priority_score`, `affected_node_count`, `auto_correctable_count`, `manual_fix_count`
- **Sort:** `priority_score` descending
- **Tip:** `priority_score` balances complexity against blast radius — high-impact, low-effort cookbooks float to the top

#### 10. Complexity Distribution

- **Type:** Pie chart (Lens)
- **Query:** `doc_type: "cookbook_complexity"`
- **Slice by:** `complexity_label` (terms)
- **Colour mapping:** `none` → green, `low` → blue, `medium` → amber, `high` → orange, `critical` → red

#### 11. Top 20 Deprecation Cops

- **Type:** Horizontal bar chart (Lens)
- **Query:** `doc_type: "cookstyle_offense" and cop_namespace: "ChefDeprecations"`
- **Y-axis:** `cop_name` (top 20 terms)
- **X-axis:** Count
- **Why this works:** Because individual offenses are exported as separate `cookstyle_offense` documents, Kibana can natively aggregate by `cop_name` without nested queries

#### 12. Offense Severity Breakdown by Cookbook

- **Type:** Stacked bar chart (Lens)
- **Query:** `doc_type: "cookstyle_offense"`
- **X-axis:** `cookbook_name` (top 20 terms)
- **Stack by:** `severity` (terms)
- **Colour mapping:** `convention` → grey, `warning` → amber, `error` → red, `fatal` → dark red

#### 13. Auto-Correct Coverage

- **Type:** Bar chart + metric (Lens)
- **Query:** `doc_type: "autocorrect_preview"`
- **Metric panels:** Average `correctable_percentage`, sum `correctable_offenses`, sum `remaining_offenses`
- **Bar chart:** X-axis = `cookbook_name`, stacked bars of `correctable_offenses` (green) and `remaining_offenses` (red)

#### 14. Deprecation Impact by Chef Version

- **Type:** Heatmap (Lens)
- **Query:** `doc_type: "cookstyle_offense" and cop_namespace: "ChefDeprecations"`
- **Y-axis:** `remediation_introduced_in` (terms)
- **X-axis:** `cookbook_name` (terms)
- **Cell value:** Count
- **Tip:** This reveals which Chef version transitions introduce the most breaking changes for your codebase

#### 15. CookStyle Pass/Fail Over Time

- **Type:** Stacked area chart (Lens)
- **Query:** `doc_type: "cookstyle_result"`
- **X-axis:** `@timestamp` (date histogram)
- **Y-axis:** Count
- **Breakdown:** `passed` (true/false)

### Node Detail Dashboard

For investigating individual nodes or node groups.

#### 16. Stale Nodes by Environment

- **Type:** Data table (Lens)
- **Query:** `doc_type: "node_snapshot" and is_stale: true`
- **Row split:** `chef_environment` (terms)
- **Metrics:** Count, average `stale_days`

#### 17. Most Common Blocking Cookbooks

- **Type:** Tag cloud or horizontal bar (Lens)
- **Query:** `doc_type: "node_readiness" and is_ready: false`
- **Terms:** `blocking_cookbook_names` (top 30)
- **Why this works:** `blocking_cookbook_names` is a flat keyword array promoted to the top level, so Kibana can aggregate on it directly without nested queries

#### 18. Disk Space Distribution

- **Type:** Histogram (Lens)
- **Query:** `doc_type: "node_readiness"`
- **X-axis:** `disk_headroom_mb` (histogram, bucket size: 500)
- **Y-axis:** Count
- **Breakdown:** `sufficient_disk_space` (true/false)
- **Tip:** Negative `disk_headroom_mb` values indicate nodes with insufficient space

#### 19. Node Readiness by Environment and Target Version

- **Type:** Heatmap (Lens)
- **Query:** `doc_type: "node_readiness"`
- **Y-axis:** `chef_environment` (terms)
- **X-axis:** `target_chef_version` (terms)
- **Cell value:** Average of `is_ready` (displays as 0.0–1.0, representing percentage ready)

## Document Types

The following document types are exported by the application and indexed by the Logstash pipeline. All are stored in the single `chef-migration-metrics` index, distinguished by the `doc_type` field. Fields are top-level (flat) — there is no `data` wrapper.

| `doc_type` | Description | `@timestamp` Source | Key Fields |
|------------|-------------|---------------------|------------|
| `node_snapshot` | Point-in-time node attributes from a collection run | `collected_at` | `node_name`, `chef_version`, `platform`, `platform_family`, `chef_environment`, `is_stale`, `stale_days`, `cookbook_names`, `cookbook_count`, `is_policyfile_node` |
| `cookbook` | Cookbook metadata | `last_fetched_at` | `cookbook_name`, `cookbook_version`, `source`, `is_active`, `is_stale_cookbook`, `has_test_suite` |
| `cookstyle_result` | CookStyle scan summary per cookbook | `scanned_at` | `cookbook_name`, `target_chef_version`, `passed`, `offense_count`, `deprecation_count`, `correctness_count` |
| `cookstyle_offense` | Individual CookStyle offense (one doc per offense) | `scanned_at` | `cookbook_name`, `cop_name`, `cop_namespace`, `severity`, `message`, `file_path`, `correctable`, `remediation_migration_url` |
| `test_kitchen_result` | Test Kitchen result per cookbook per target version | `completed_at` | `cookbook_name`, `target_chef_version`, `compatible`, `converge_passed`, `tests_passed`, `duration_seconds` |
| `cookbook_complexity` | Complexity score and blast radius per cookbook | `evaluated_at` | `cookbook_name`, `complexity_score`, `complexity_label`, `priority_score`, `affected_node_count`, `total_blast_radius` |
| `node_readiness` | Upgrade readiness per node per target version | `evaluated_at` | `node_name`, `target_chef_version`, `is_ready`, `blocking_reason`, `blocking_cookbook_names`, `disk_headroom_mb`, `chef_environment`, `platform` |
| `role_dependency` | Role-to-role or role-to-cookbook dependency edge | `exported_at` | `role_name`, `dependency_type`, `dependency_name` |
| `metric_snapshot` | Flattened metric snapshot for trending | `snapshot_at` | `snapshot_type`, `target_chef_version` — plus type-specific fields (see below) |
| `autocorrect_preview` | Auto-correct preview summary per cookbook | `generated_at` | `cookbook_name`, `total_offenses`, `correctable_offenses`, `remaining_offenses`, `correctable_percentage` |

### Metric Snapshot Sub-Types

Metric snapshots are flattened — each snapshot type populates specific top-level fields (others are absent). The `snapshot_type` field discriminates between them.

| `snapshot_type` | Specific Fields |
|-----------------|----------------|
| `chef_version_distribution` | `total_nodes`, `version_name`, `version_count`, `version_percentage` (one doc per version per snapshot) |
| `readiness_summary` | `total_nodes`, `ready_count`, `blocked_count`, `ready_percentage`, `blocked_by_cookbooks`, `blocked_by_disk`, `blocked_by_both` |
| `cookbook_compatibility` | `total_cookbooks`, `compatible_count`, `incompatible_count`, `cookstyle_only_count`, `untested_count`, `compatible_percentage` |

### The `cookstyle_offense` Document Type

This is the most important document type for detailed code quality analysis. Instead of storing offenses as a nested array inside the `cookstyle_result` document, each individual offense is exported as its own document. This means:

- **Native Kibana aggregations** — You can build terms aggregations on `cop_name`, `cop_namespace`, `severity`, `file_path`, and `remediation_introduced_in` without specialised nested queries.
- **Full-text search** — The `message` field is mapped as `text`, so you can search for specific deprecation patterns in the Discover view.
- **Cross-cookbook analysis** — Aggregate offenses across all cookbooks to find the most common deprecation patterns in your fleet.

See the [Elasticsearch Export Specification](../../.claude/specifications/elasticsearch/Specification.md) for the complete field reference.

## Kibana Discover Tips

- **Filter by document type first** — Add a filter `doc_type: <type>` to scope the field list. Since all types share one index, the unfiltered field list is long; filtering by type makes it manageable.
- **Use the time-picker** — `@timestamp` is set on every document, so the time-picker (top-right corner) always works. For historical trend analysis, set it to a wide range. For recent data, use "Last 24 hours" or "Last 7 days".
- **Pin common filters** — Pin `organisation_name` and `target_chef_version` filters to maintain context when switching between saved searches or tabs.
- **Use KQL for quick filtering** — Examples:

  ```
  doc_type: "node_snapshot"
  doc_type: "cookbook_complexity" and complexity_label: "critical"
  doc_type: "node_readiness" and is_ready: false
  doc_type: "cookstyle_offense" and cop_namespace: "ChefDeprecations"
  doc_type: "cookstyle_offense" and message: "unified_mode"
  doc_type: "node_snapshot" and chef_version: "17*"
  ```

## Logstash Pipeline

The pipeline definition is at `logstash/pipeline/chef-migration-metrics.conf`. It:

1. **Reads** `*.ndjson` files from the input directory (skips `.tmp` files to avoid reading partial writes)
2. **Parses** each line as JSON using the `json_lines` codec
3. **Preserves** the application-set `@timestamp` — does NOT overwrite it with Logstash ingestion time
4. **Extracts** the `doc_id` field for use as the Elasticsearch `_id`, enabling upsert (re-exports overwrite rather than duplicate)
5. **Parses** all ISO 8601 date strings into proper Elasticsearch date types
6. **Removes** Logstash-internal fields (`@version`, `host`, `path`, `log`)
7. **Indexes** every document into the `chef-migration-metrics` index

The pipeline uses an index template (`logstash/pipeline/chef-migration-metrics-template.json`) that defines explicit mappings for all known fields. Unknown fields are dynamically mapped as `keyword` by default (via a dynamic template).

### Key Mapping Decisions

| Field | Mapping | Reason |
|-------|---------|--------|
| All string fields (default) | `keyword` | Exact-match filtering and terms aggregation |
| `message` | `text` + `.keyword` sub-field | Full-text search on offense messages, plus exact aggregation via `.keyword` |
| `remediation_description` | `text` + `.keyword` sub-field | Full-text search on remediation text |
| `blocking_cookbooks` | `nested` | Retained for exact co-occurrence queries on blocking cookbook properties |
| `blocking_cookbook_names` | `keyword[]` | Flat array — serves most Kibana aggregation use cases without nested queries |
| `priority_score` | `float` | Supports range queries and sorting for remediation prioritisation |

### Keeping the Pipeline Up to Date

When document types are added or field schemas change in the application, the following files must be updated:

1. `logstash/pipeline/chef-migration-metrics.conf` — add date parsing for any new date fields
2. `logstash/pipeline/chef-migration-metrics-template.json` — add explicit mappings for new fields

## Configuration

All configuration is via environment variables in the `.env` file. See `.env.example` for the full list with descriptions.

| Variable | Default | Description |
|----------|---------|-------------|
| `ELK_VERSION` | `8.17.0` | Version tag for all ELK container images |
| `ELASTICSEARCH_PORT` | `9200` | Host port for Elasticsearch HTTP API |
| `ELASTICSEARCH_INDEX` | `chef-migration-metrics` | Name of the Elasticsearch index |
| `ES_JAVA_OPTS` | `-Xms512m -Xmx512m` | JVM heap for Elasticsearch |
| `LOGSTASH_MONITORING_PORT` | `9600` | Host port for Logstash monitoring API |
| `LS_JAVA_OPTS` | `-Xms256m -Xmx256m` | JVM heap for Logstash |
| `KIBANA_PORT` | `5601` | Host port for Kibana web UI |
| `KIBANA_ENCRYPTION_KEY` | *(set in .env.example)* | Encryption key for Kibana saved objects (min 32 chars) |

## Operations

### View Logs

```bash
# All services
docker compose logs -f

# Single service
docker compose logs -f logstash
```

### Check Index Health

```bash
# Document count
curl -s 'http://localhost:9200/chef-migration-metrics/_count' | jq .

# Document count by type
curl -s 'http://localhost:9200/chef-migration-metrics/_search?size=0' \
  -H 'Content-Type: application/json' \
  -d '{"aggs":{"by_type":{"terms":{"field":"doc_type","size":20}}}}' | jq '.aggregations.by_type.buckets'

# Index stats
curl -s 'http://localhost:9200/chef-migration-metrics/_stats' | jq '.indices["chef-migration-metrics"].primaries'

# View mappings
curl -s 'http://localhost:9200/chef-migration-metrics/_mapping' | jq .

# Verify @timestamp is being set correctly
curl -s 'http://localhost:9200/chef-migration-metrics/_search?size=3' \
  -H 'Content-Type: application/json' \
  -d '{"_source":["doc_type","@timestamp","doc_id"],"sort":[{"@timestamp":"desc"}]}' | jq '.hits.hits[]._source'
```

### Reindex from Scratch

If you need to reindex all data (for example after a mapping change):

```bash
# Delete the index
curl -X DELETE 'http://localhost:9200/chef-migration-metrics'

# Reset Logstash sincedb to reprocess all files
docker compose exec logstash rm -f /usr/share/logstash/data/plugins/inputs/file/sincedb_chef_migration_metrics

# Restart Logstash
docker compose restart logstash
```

Then trigger a full re-export from the application (clear the high-water mark or use the manual export API endpoint).

### Stop and Clean Up

```bash
# Stop all services (data volumes preserved)
docker compose down

# Stop and remove all data volumes
docker compose down -v
```

## Troubleshooting

### Logstash Not Picking Up Files

1. Check that files have the `.ndjson` extension (not `.tmp`).
2. Verify the files are in the correct volume path:
   ```bash
   docker compose exec logstash ls -la /export-data/
   ```
3. Check Logstash logs for errors:
   ```bash
   docker compose logs logstash | tail -50
   ```
4. Verify the sincedb has not already tracked past the file's content:
   ```bash
   docker compose exec logstash cat /usr/share/logstash/data/plugins/inputs/file/sincedb_chef_migration_metrics
   ```

### Kibana Time-Picker Shows No Data

1. Confirm documents exist in Elasticsearch:
   ```bash
   curl -s 'http://localhost:9200/chef-migration-metrics/_count' | jq .
   ```
2. Verify the Kibana data view uses `@timestamp` as the timestamp field (not `exported_at` or another field).
3. Widen the time-picker range — the `@timestamp` values correspond to when the data was collected/scanned/evaluated, which may not be "now".
4. Check that `@timestamp` is being preserved (not overwritten by Logstash ingestion time):
   ```bash
   curl -s 'http://localhost:9200/chef-migration-metrics/_search?size=1' \
     -H 'Content-Type: application/json' \
     -d '{"_source":["@timestamp","exported_at","doc_type"]}' | jq '.hits.hits[]._source'
   ```

### Elasticsearch Out of Disk Space

The single-node test cluster may enter a read-only state if disk usage exceeds 95%. Free up space or increase the disk watermark:

```bash
curl -X PUT 'http://localhost:9200/_cluster/settings' \
  -H 'Content-Type: application/json' \
  -d '{"transient":{"cluster.routing.allocation.disk.watermark.flood_stage":"99%"}}'

curl -X PUT 'http://localhost:9200/chef-migration-metrics/_settings' \
  -H 'Content-Type: application/json' \
  -d '{"index.blocks.read_only_allow_delete": null}'
```

### High Document Count from `cookstyle_offense`

The `cookstyle_offense` type produces many more documents than other types — potentially hundreds per cookbook scan. If index size becomes a concern in the test environment:

1. Check the offense document count:
   ```bash
   curl -s 'http://localhost:9200/chef-migration-metrics/_count' \
     -H 'Content-Type: application/json' \
     -d '{"query":{"term":{"doc_type":"cookstyle_offense"}}}' | jq .
   ```
2. If needed, delete only offense documents to free space while keeping summaries:
   ```bash
   curl -X POST 'http://localhost:9200/chef-migration-metrics/_delete_by_query' \
     -H 'Content-Type: application/json' \
     -d '{"query":{"term":{"doc_type":"cookstyle_offense"}}}'
   ```

## Security Notice

This stack is configured for **local testing only**:

- Elasticsearch security (`xpack.security`) is disabled
- No authentication is required for any service
- All ports are exposed on `localhost`

Do not expose this stack to untrusted networks. For production Elasticsearch deployments, use a properly secured cluster with TLS, authentication, and role-based access control.