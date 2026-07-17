# Event Ingest — Component Specification

> **Implementation language:** Go. See `../CLAUDE.md` for language and concurrency rules.
> **Scope:** MVP. Decision rationale + empirical transport validation live in
> `../plans/proposal-event-ingest.md`.

---

## TL;DR

A passive HTTP receiver (`POST /api/v1/ingest`) accepts Chef run telemetry in three
producer shapes — raw `run_converge` from a node (direct), the Chef Infra Server proxy
relay, and Chef Automate's outbound Data Feed (per-node state) — **normalises** each into
an append-only, **time-partitioned** `converge_runs` table (short retention, purged by
dropping partitions), and surfaces recent runs per node on a new **Runs** tab of the Node
Detail page. Keyed by **(organisation, node_name)**; deduped on run id. **No auth in the
MVP** (tracked as tech debt). Extract-and-discard: keep the run summary + failure detail,
drop the bulk node-attribute tree. Related: [data-collection](data-collection.md),
[datastore](datastore.md), [web-api](web-api.md), [visualisation](visualisation.md).

---

## Intent / Why

- Surface real per-node converge results — especially **failures with error + backtrace** —
  which Automate's own filtering cannot isolate at fleet scale, and which are **not**
  recoverable from offline gather-bundles (a live-telemetry-only signal).
- Make **observed real cookbook usage** visible: special-job / override runlists are
  invisible to the node-object pull (see `active-cookbook-test-blindspot`). MVP surfaces
  this per-node; feeding it into test scope is out of scope (below).

CMM is today **pull-only**; this is the first **inbound** ingress. It is passive: the
producer is configured customer-side, CMM only exposes a sink.

---

## Producers & shapes (accept all three)

The same endpoint receives three shapes; a **normaliser** detects which and maps to one
`converge_runs` row.

1. **Node direct** — chef-client `data_collector.server_url` → us. Raw
   `run_start` / `run_converge` messages.
2. **Chef Infra Server proxy** — `data_collector['root_url']` → us. Same `run_converge`
   relayed verbatim (plus Server `action` events — ignored).
3. **Chef Automate Data Feed** — a webhook destination → us. Per-node
   `node` / `client_run` / `attributes` record (per-node *state*; `client_run` is the
   run). gzip + NDJSON, batched up to `node_batch_size` per POST.

The field shapes are **external** (not owned by us). Reference data: the Nuclia
field-knowledge doc *"Chef Infra Data Collector Event Formats"* (+ its 2026-07-16
empirical correction). The **normaliser in code is the source of truth** for the mapping,
pinned by a **contract test** against committed **golden fixtures** — one per shape,
captured from the lab.

---

## Endpoint behaviour (invariants)

- `POST /api/v1/ingest`. Accept `Content-Type: application/json`; **gunzip** when
  `Content-Encoding: gzip`; parse the body as **one-or-more** JSON values (NDJSON) — split
  on newlines / streaming decode; handle LF, CRLF, and single-object bodies uniformly.
- **No auth (MVP).** Accept and ignore any `Authorization` header. Customer-node auth
  setup needs change control we are deferring. Recorded as tech debt
  (`todo-tech-debt.md`) — an unauthenticated ingress is a real exposure to close later.
- **Robustness:** bound body size and **cap records-per-body** (gzip-bomb guard); commit
  **one transaction per body** (a malformed/partial body must persist nothing); respond in
  Automate's **accepted set (200–204)** on receipt — Automate drops a destination that
  answers outside it — and **200-and-drop under backpressure**. Ingest must never block the
  producer.
- Message types other than `run_converge` and Data Feed node records (`run_start`,
  `action`, `inspec_report`) are accepted and **ignored** in the MVP.

---

## Normalisation → `converge_runs` (extract-and-discard)

Detect shape, map to one row. **Keep:**

- **run_id** (dedup key) — `run_converge.id`/`run_id`; Data Feed `client_run.id`.
- **organisation**, **node_name** (the join key), source_fqdn / chef_server_fqdn.
- **status**, start_time, end_time, chef_version, run_list, expanded_run_list,
  total/updated_resource_count.
- **cookbooks** (name→version) — observed usage.
- on **failure**: `error{class, message, description}` + **backtrace** (bounded length),
  and the failed resource (cookbook_name, recipe_name, name, type).

**Discard** the `attributes` tree / full node object (~100 KB bulk).

Invariants:

- **Append-only; dedup on run_id.** A run may arrive more than once (e.g. Server proxy
  *and* Automate) — upsert on run_id, do not duplicate.
- **`converge_runs` NEVER writes node primary associations** (used/unused, blast radius).
  Node objects remain the sole source of those. Observed cookbooks here are per-run facts
  only.
- **Time-partitioned; retention by dropping whole partitions** (configurable window, MVP
  default 2 days). No row-level deletes on the hot path.
- Rows with **no matching `node_snapshots`** are retained — a node CMM does not pull is
  still valid telemetry.

---

## Node identity

Join `converge_runs.(organisation, node_name)` to `node_snapshots.(organisation, name)`.
The Data Feed record carries **no `entity_uuid`**, so name + org is the only available key
— there is no alternative to choose.

**Ingest-only nodes are first-class, not an edge case.** The customer topology is mixed:
a clustered Automate (2 orgs, ~50k nodes each) CMM *can* pull, **plus standalone Infra
Servers on DMZs (~2–3k nodes each) whose APIs CMM CANNOT reach** — CMM never pulls them,
so they have **no `node_snapshots`**. For those nodes ingest is the *only* source of data.
Their runs MUST be identifiable and surfaceable by `(organisation, node_name)` alone.
"Retained but invisible" is a defect for this population — see UI.

---

## UI

- A new **Runs** tab on the Node Detail page (`NodeDetailPage`): recent converge runs for
  the node, most-recent first — time, status, chef_version, run_list, and on failure the
  error class/message + failing cookbook·recipe + backtrace (collapsible). Reuses the
  existing Node Detail panel pattern.
- **Ingest-only telemetry surfaced via a node-centric top-level "Run events" view
  (decided; supersedes the earlier run-centric decision).** A per-node Runs tab reaches only
  *pulled* nodes, hiding the DMZ ingest-only population (no `node_snapshots`; see Node
  identity) — a primary reason the feature exists. Rather than fabricate parent node/org
  records (which would pollute pull-derived fleet truth — readiness, counts,
  version/platform distributions — and fight the org schema), CMM surfaces the telemetry
  through a **dedicated top-level "Run events" view over `converge_runs`**, a sibling of
  Nodes / Cookbooks / Git Repos. One nav entry, **two tabs** sharing one filter set:
  - **Nodes tab (default) = distinct *nodes* derived from `converge_runs`** (`DISTINCT
    (organisation, node_name)`), filterable and paginated, **defaulting to failures**. Filter
    semantics are **EXISTS / "any matching run"**: a node appears iff it has at least one run
    matching *all* active filters together, and the row shows that node's **latest matching
    run**. Row → **detail = every run for that one node** (its full `converge_runs` history,
    most-recent first), reusing the Node Detail run panel — so failures can be dug into.
  - **Runs tab = the flat run list** (one row per run) — the event firehose, same filters.
    A row **expands inline** to show that run's failure detail (error class/message, failing
    cookbook·recipe, backtrace) rather than navigating — only the Nodes tab drills into a
    node.

  Its filters are **view-level and sourced from `converge_runs` itself** — org / status /
  node / chef_version / cookbook / **failure_message** / time. The **failure_message** filter
  is a substring match on the failure reason line (`error.message`, backtrace excluded — the
  normaliser already separates them), which isolates authored prereq aborts ("not enough
  space") from real errors without a schema change.

  **As-of anchor (live firehose):** `converge_runs` is append-live, so naive
  `LIMIT/OFFSET` paging would skew as new rows arrive. The UI pins an **upper `end_time`
  bound (`until`) at view load** and carries it on every filter/page request, so the visible
  dataset is a stable slice; a manual **Refresh** re-anchors to pull newer events. No backend
  change — this is the `EndTimeTo` filter. **It must NOT use the global org filter:** that
  dropdown is populated from the `organisations` table, which never contains the ingest-only
  DMZ orgs, so binding to it would make the entire DMZ population unselectable. Org and
  chef_version options come from `SELECT DISTINCT … FROM converge_runs`. Both levels read
  `converge_runs` directly (the latest-run-per-node rollup is served by the
  `(organisation, node_name, end_time DESC)` index; retention-bounded), get their **own
  export**, and leave `node_snapshots` / `organisations` and their aggregates untouched. The
  detail read path keys on the **delivered org name**, NOT `organisations.name` resolution,
  so DMZ ingest-only nodes (absent from that table) still resolve. This is **not** the
  rejected "virtual node-list union" (which folded ingest nodes into the heavy
  `node_snapshots` list) — it is a self-contained rollup over `converge_runs` only. The
  **Node Detail Runs tab stays** for pulled nodes (per-node context) — the two are
  complementary.
- Read path: a web API endpoint returns `converge_runs` for a node (org + name), bounded /
  paginated. See [web-api](web-api.md).

### Upgrade run taxonomy (why the filters must be flexible)

The customer's upgrade emits **several run classes per node** into `converge_runs`, all keyed
the same but meaning different things on failure. **Everything except the speculative run
executes on the active client** (13/16): the normal production converge *and* every override
run list (patching, compliance, the CC19 prereq/installer, …). **Only the speculative
converge runs on the dormant CC19 client**, fired daily to see whether the node would succeed
on the target version.

- **`chef_version` is the clean axis:** because nothing but the speculative run uses the
  dormant client, `chef_version = target` unambiguously marks a **prospective-target run**.
  So the readiness signal is `chef_version = target ∧ status = failure`.
- **The active-client family is heterogeneous** (production + all overrides); only
  `run_list` / `cookbooks` content distinguishes them, and the override set is open-ended and
  customer-defined. Hence the **cookbook filter** and free-form node/version filtering, rather
  than a hardcoded lens per override type.
- A **failure** therefore means different things by class: a *speculative* fail = prospective
  target failure; a *prereq/install* fail = intentional abort (unmet prerequisite) or real
  install error; a *production* fail = pre-existing breakage. The MVP gives the operator the
  filters to separate these; it does not pre-classify runs.

Out of MVP: capturing the installer cookbook's active/dormant version node attributes into
this view (already captured elsewhere for the CC19 deployment dashboard) — a later nice-to-have.

---

## Configuration (dynamic; live accessor)

Per the project config rule, read via a live accessor; changes take effect without
restart.

- `ingest.enabled` (default `false`) — the sink accepts telemetry (POST /api/v1/ingest).
- `ingest.show_run_events` (default `false`) — gates the **display** surfaces: the Run
  events view **and** the Node Detail Runs tab (and their read endpoints + export). Kept off
  so the feature is dormant in reserve; **orthogonal to `enabled`** — telemetry can accrue
  while the UI stays hidden. A viewer-readable `GET /api/v1/features` exposes this flag so the
  frontend hides the nav entry / Runs tab (the surfaces 404 when off regardless).
- `ingest.failures_only` (default `false`) — the sink **discards success events**, keeping
  only failures. Firehose-relief valve for high-volume fleets.
- `ingest.retention_days` (default `2`)
- `ingest.max_body_bytes`, `ingest.max_records_per_body` (robustness bounds)

---

## Out of scope (MVP)

- **Auth** on the endpoint (tech debt).
- **Install / prereq / depsolve failure capture** — those arrive attributes-only (no
  `client_run`) or emit nothing; needs a separate cron-wrapper/exit-code path.
- **Feeding observed cookbooks into test scope / readiness** — MVP is display-only
  per-node.
- **Durable queue** beyond the in-process bounded buffer.

---

## Related Specifications

- [data-collection](data-collection.md) — the existing pull path and `node_snapshots`
- [datastore](datastore.md) — schema; DDL in `migrations/`
- [web-api](web-api.md) — the HTTP listener the ingest route and read route attach to
- [visualisation](visualisation.md) — Node Detail page
- [logging](logging.md), [configuration](configuration.md)
