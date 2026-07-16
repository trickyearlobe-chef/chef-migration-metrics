# Event Ingest golden fixtures

Contract-test fixtures for the ingest normaliser (`internal/ingest`). One representative
record per producer shape, plus the shapes the MVP must **ignore**.

Provenance: authored from the empirical field-report *"Chef Infra Data Collector Event
Formats"* (Nuclia `ff19f58e…`) + the `automate-datafeed-behavior` finding, both captured
2026-07 on the lab. The raw catcher output was ephemeral (not retained), so these are
faithful reconstructions of the documented key lists — **not** raw captures. Treat the
contract test as ground-truth only after one is validated against a live capture; if the
real shape differs, the contract test + a real record will surface it.

All identifiers are generic placeholders (`node-a.example.com`, `org-a`,
`automate.example.com`, `chef.example.com`) — never lab/customer hostnames.

## Shapes the normaliser must map → ConvergeRun

| file | shape | status | notes |
|------|-------|--------|-------|
| `datafeed_success.json`        | Data Feed (node+client_run+attributes) | success | `attributes` discarded |
| `datafeed_failure.json`        | Data Feed                              | failure | converge-phase; error+backtrace+failed resource |
| `run_converge_success.json`    | raw `run_converge` (node direct)       | success | |
| `run_converge_failure.json`    | raw `run_converge` (node direct)       | failure | |
| `run_converge_proxy.json`      | `run_converge` via Chef Server proxy   | success | shape identical to direct; `chef_server_fqdn` = server |

## Shapes the normaliser must IGNORE (accepted, no row)

| file | why ignored |
|------|-------------|
| `run_start.json`              | `message_type:"run_start"` — not a converge |
| `datafeed_attributes_only.json` | depsolve/missing-cookbook abort → top-level `["attributes"]` only, no `client_run` (the known feed GAP) |

Dedup key: Data Feed = `client_run.id`; raw = `run_id`.
