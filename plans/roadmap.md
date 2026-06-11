# Implementation Roadmap

Refreshed 2026-06-11 from a **verified code audit** (not todo checkboxes — those
proved unreliable). Phases below are in recommended priority order; reorder freely.
Start a clean thread per phase. Each phase names its todo/spec source and the
verified state as of the refresh.

> Audit caveat: several `todo-*.md` checkboxes were stale in both directions
> (items marked open were done; areas that sounded done had real gaps). Trust code
> over checkboxes; re-verify a phase's specifics before starting it.

---

## Shipped (archived) — verified complete 2026-06-11

- **UI Revamp** (`todo-ui-polish.md`) — nav hub, Settings regroup, Stats/Performance
  merge, all polish items. Every item verified `[x]`.
- **Disk Space Analysis** (`todo-configuration.md`) — `InstallSizeMBLinux/Windows`,
  `MinRemainingFreePercent` + defaults (`config.go:509`); Upgrade Readiness UI.
- **Parallel Deployment Tracking** (`spec parallel-deployment-tracking.md`) —
  `chef_migration` partial-search + migration `0035`, node-list badges/filters/
  Ready-to-Activate, node detail, dashboard trend + status cards, tests.
- **TLS chain display + reorder-on-save + automatic HTTPS on 443** (`tls*.md`,
  W1/W2) — see `todo-tls.md`.

Also already done but still mislabelled open in todos: Data-Layer **Phase 6**
export-filter parity (`handle_exports.go`, `ExportButton.tsx`); secrets **chefapi
CredentialResolver** integration (`collector.go:1568`); CI **MEDIUM npm gate** now
blocking (`ci.yml:169`); **log retention purge** (`collector.go:670`).

---

## Phase 1 — Finish / activate half-built features (quick, high value)

Things the codebase *claims* but doesn't actually do at runtime. Low effort, high
correctness value. **Todos:** `todo-bulk-kitchen-scanning.md`, `todo-secrets-storage.md`,
`todo-configuration.md`.

- **Wire the orphan-sweep ticker into startup.** `StartSweepTicker()`
  (`internal/hypervisor/sweep_ticker.go`) is fully implemented but **never called**
  from `cmd/.../main.go` — the scheduled sweep silently never runs. Wire it where
  other background services start; gate on TK enabled + hypervisor configured.
- **Implement `GET /api/v1/admin/status`.** Currently routed to
  `handleNotImplemented` → HTTP 501 (`router.go:755`). Build the status payload,
  including the `credential_storage` section (encryption_key_configured,
  total_credentials, credential_types, orphaned_credentials).
- ~~Add the 7th concurrency worker `test_kitchen_run`~~ — **not a code gap.** TK
  load is already bounded globally (`analysis_tools.test_kitchen.max_concurrent_vms`
  + start-rate limiter); `concurrency.test_kitchen_run` was a phantom from the old
  per-pool design. Resolved as a spec-only reconciliation
  (`specification/tk-concurrency-reconcile`).

---

## Phase 2 — Orphan Sweep completion

**Todo:** `todo-bulk-kitchen-scanning.md` (Orphan Sweep). **Spec:**
`bulk-kitchen-scanning.md`. Core sweep + manual `POST /kitchen/orphan-sweep`
(dry-run) + WebSocket event + admin UI already done; remaining:

- Folder scoping: add a folder-filter param to `ListManagedVMs` (vSphere
  `?folders=`) and restrict sweeps to `driver_settings.targetfolder`. (Not started.)
- Inter-instance sweep after a `timed_out`/`network_timeout` instance before the
  next start. (Not started.)
- Persist "last sweep" (count + timestamp) so the admin page shows it across reloads.

---

## Phase 3 — Data Layer finish (correctness + scale)

**Todo:** `todo-data-layer.md` (Phases 5 & 8). Phase 6 is **done** (verify-and-close
only). **Plan:** `data-layer-revamp.md`.

- **Phase 5 staleness/freshness (remaining):** version-distribution trend ignores
  `?stale=` (deferred pending per-tier version data in `node_metrics` —
  `handle_dashboard_version.go:336`); cookbook "fresh" filter
  (`used_by_fresh_nodes`) not started. Readiness-trend staleness is done.
- **Phase 8 performance at 120k+ nodes:** add composite filter+sort indexes on
  `node_snapshots` (only single-column / GIN indexes exist today); EXPLAIN ANALYZE
  the list endpoints at scale; revisit pool / prepared-statement caching.

---

## Phase 4 — Notifications subsystem (greenfield)

**Genuinely unbuilt** (`internal/notify/` does not exist; `notification_history`
was removed as dead code). Unblocks ~15 deferred items across
`todo-visualisation.md`, `todo-secrets-storage.md`, `todo-testing.md`. **Spec:**
config schema exists in `encrypted-config-store.md`; API in
`web-api-notifications-ownership.md`.

- `internal/notify/` package: webhook (HTTP POST JSON) + email (SMTP) channels,
  delivery retry/backoff.
- Triggers: cookbook status change, readiness milestone, new incompatible cookbook,
  collection failure, stale-node threshold.
- `notification_history` table (re-add) + history API + frontend page.
- Config: `notifications`/`smtp` sections + `url_credential` credential field;
  resolve URL/password via `CredentialResolver`.

---

## Phase 5 — Data Exports re-spec + Elasticsearch

**Spec rewrite first** — `specifications/data-export.md` incoherently mixes webhook
push / Elasticsearch NDJSON / Logstash. **Todo:** `todo-data-layer.md` (Re-specify
Data Exports), `todo-configuration.md` (Elasticsearch). Today only CSV/JSON/
chef_search_query downloads exist; `ElasticsearchConfig` is a config stub
(`config.go:613`) with no implementation or UI.

- Decide formats + delivery (CSV/NDJSON/webhook; file-based vs API push) and the
  Exports-page UX; rewrite the spec.
- Then implement: NDJSON export (high-water-mark/incremental, `.tmp` atomic write),
  ES config UI section, retention.

---

## Phase 6 — Blast Radius

**Todo:** `todo-blast-radius.md`. **Spec:** `blast-radius.md`. Underlying data
(`affected_node_count/role_count/policy_count`, dependency edges) exists; endpoint
and UI are **not started**. **Needs a design decision first** (visualisation
approach, entry points, transitive vs direct — confirm with customer).

- `GET /api/v1/cookbooks/:name/blast-radius`; impact-explorer panel from cookbook
  detail; link affected roles/nodes to filtered list pages.

---

## Phase 7 — Org-level dependency graph + compatibility filtering

**Todo:** `todo-visualisation.md`. Role/node-level `compatibility_status` colouring
is done (`handle_roles.go`, `ForceGraph.tsx`); the org/fleet-level graph and
filtering-by-compatibility are not started.

- Backend: supply compatibility data on an org-level dependency-graph endpoint.
- Frontend: org graph page; filter to paths involving incompatible/untested
  cookbooks; lazy/LOD rendering for large graphs; link role nodes to filtered lists.

---

## Phase 8 — Secrets hardening + smaller gaps

**Todo:** `todo-secrets-storage.md`, `todo-test-kitchen-config-ui.md`,
`todo-platform-mapping-ui.md`. Critical path (chefapi resolver, config fields, TLS
key-perm warning) is **done**; remaining:

- Live credential tests: `chef_client_key` (Chef API call), `smtp_password` (SMTP
  AUTH), `webhook_url` (HTTP HEAD 2xx/3xx) + unit tests.
- Startup perm warnings: keys dir > 0700, env file > 0640 (TLS key > 0600 done).
- TK credential-reference warnings on PUT (placeholder today).
- Platform-mapping dedicated UI page (handler/API exist; embedded only).

---

## Phase 9 — Release readiness (packaging + docs)

**Todos:** `todo-packaging.md`, `todo-testing.md`, `todo-documentation.md`,
`todo-ci.md`. Largely environment-dependent manual verification + writing.

- Packaging: RPM/DEB/Docker-Compose/ELK build + smoke verification; embedded Ruby
  (cookstyle/kitchen) isolation checks.
- CI: pin `nfpm` (last unpinned tool); confirm scanner inputs on first run; triage
  Route 53 SDK modules.
- Documentation backlog (install, config reference, features) — currently empty.

---

## Deferred (not scheduled)

- **TLS runtime health-driven port-flip** — the "spicy half" of the 443 lifeboat
  (hot listener rebind + hysteresis). `todo-tls-antilockout.md` §2; own spec needed.
- **TLS reorder-warning surfacing** — make the chain-reorder warning persistent on
  GET status + FE display (currently PUT-response only). `todo-tls.md` §1.
- **`CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` on DB path** — documented limitation
  (`todo-tls-antilockout.md` §1).
