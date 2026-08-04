# Data Export — Component Specification

## Overview

CMM supports three mechanisms for exporting data to external systems:
webhook push, Elasticsearch (via NDJSON files for Logstash), and direct
Logstash output. All three are optional and can be enabled independently.

## Export Mechanisms

### Webhook

Push event payloads to external HTTP endpoints on data changes:

- Configurable URL, headers, and authentication (bearer token or basic auth via credential reference)
- Events: `collection_complete`, `cookbook_status_change`, `readiness_milestone`, `node_stale`, `kitchen_result`
- Retry with exponential backoff (3 attempts, 1s / 5s / 30s)
- Payload format: JSON with `event_type`, `timestamp`, and event-specific data
- Timeout: 30s per request

### Elasticsearch (NDJSON File Export)

Generate NDJSON files for Logstash → Elasticsearch ingestion:

- Output to configurable directory
- One file per export cycle, named `cmm-export-YYYYMMDD-HHMMSS.ndjson`
- High-water-mark tracking to export only new/changed records
- Retention policy with configurable hours
- Document types: `node_snapshot`, `cookbook`, `cookstyle_result`, `cookstyle_offense`, `test_kitchen_result`, `node_readiness`, `cookbook_complexity`

### Logstash (Direct Output)

Send structured events directly to a Logstash TCP or HTTP input:

- Configurable host:port and protocol (`tcp` / `http`)
- JSON codec
- Same document types as NDJSON export
- Connection retry with backoff
- Optional TLS for transport

## Configuration

```yaml
exports:
  webhook:
    enabled: false
    url: "https://hooks.example.com/cmm"
    auth_credential: "webhook-token"
    events:
      - collection_complete
      - cookbook_status_change
      - kitchen_result
    timeout_seconds: 30
    retry_attempts: 3

  elasticsearch:
    enabled: false
    output_directory: /var/lib/chef-migration-metrics/elasticsearch
    retention_hours: 168

  logstash:
    enabled: false
    host: "logstash.example.com"
    port: 5044
    protocol: tcp
    tls: false
```

## Document Types

All three export mechanisms share the same document schema. Each document includes:

- `doc_type` — discriminator (e.g. `node_snapshot`, `cookbook`)
- `@timestamp` — event time
- `organisation_name` — org context
- `exported_at` — export generation time

### Type Catalogue

| Type | Key Fields |
|---|---|
| `node_snapshot` | node name, chef version, platform, environment, staleness, cookbook list |
| `cookbook` | name, version, source, org |
| `cookstyle_result` | cookbook, target version, CookStyle rollup status, offence/deprecation counts |
| `cookstyle_offense` | individual finding: cop name, severity, classification, file location |
| `test_kitchen_result` | cookbook, target version, platform, converge/test pass, duration |
| `node_readiness` | node, target version, node readiness state, blockers |
| `cookbook_complexity` | cookbook, target version, classification-weighted complexity score/label, fix counts |

CookStyle status vocabulary (see [cop-classification.md](cop-classification.md)
§ CookStyle Rollup Status — the canonical source):

- `cookstyle_result.status` carries the per-item rollup: **Ready** /
  **Needs review** / **Blocked** / **Untested**, replacing the former
  compatible/incompatible/passed/failed wording. The boolean `passed` field is
  **retained for back-compat** (`passed = status not in {Blocked}`; Untested is
  `passed = false`/null) so existing consumers don't break — new consumers read
  `status`.
- `node_readiness.status` carries the three-state node readiness:
  `ready` / `needs_review` / `blocked`.
- `cookstyle_offense.classification` carries the resolved per-offense
  classification (blocker / review / noise / unclassified) used to derive the
  rollup; CS and TK remain separate signals and are never merged in a document.
- `cookbook_complexity.complexity_score`/`label` are **classification-weighted**
  (Blocker offenses dominate; Review low; Noise ~0).

The normative document shapes are the exported Go types (with their json tags),
not this table — do not copy field schemas here. Field semantics that the types
can't express (status derivation, weighting, back-compat `passed`) are owned by
[cop-classification.md](cop-classification.md).

## Error Handling

- **Webhook** — log failed deliveries; do not block the export cycle.
- **NDJSON** — write partial file on error; retry remaining records on next cycle.
- **Logstash** — buffer in memory (bounded); drop oldest events on overflow.

## Related Specifications

- `configuration.md` — export config schema
- `logging.md` — export event logging