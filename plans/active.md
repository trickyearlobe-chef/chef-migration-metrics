# Active Plan — Phase 1: admin status endpoint

From `plans/roadmap.md` Phase 1 ("features the code claims but doesn't do at
runtime"). Then the orphan-sweep ticker (separate chunk). SAML customer work is
done/merged — see `plans/saml-customer-fixes.md` + queued follow-ups at the bottom.

## DONE (branch feature/admin-status-endpoint) — frontend status view
Added as a "Status" tab in the existing System Health hub
(`/admin/system-stats?tab=status`) — no new route/nav, avoids "Status" vs "Health"
nav duplication. `AdminStatusPage.tsx` (+test), `fetchAdminStatus` in `api/admin.ts`,
`AdminStatus` types, third tab wired into `AdminSystemHealthPage.tsx`. Reuses
SectionCard/StatusBadge/Feedback/formatDate. tsc + eslint clean; 415 vitest pass.

## DONE (branch feature/admin-status-endpoint) — `GET /api/v1/admin/status`

Implemented per spec: `handle_admin_status.go` + 6 table tests (key-not-configured,
creds/types/orphans, per-org file-vs-db + never_collected, pending-migrations math,
db-down → degraded, 405). Route wired at `router.go:843`. Green under -race/vet/build.
NOTE for review: org/last_run `status` passes through the real `CollectionRun.Status`
values (completed/failed/...) rather than the spec example's "success" literal, plus
`never_collected`/`unknown` for no-run/error orgs — flag to confirm spec example wording.

## Chunk — implement `GET /api/v1/admin/status` (currently 501)

Replace `handleNotImplemented` at `router.go:843`. Spec is complete:
`specifications/web-api-admin.md` §`GET /api/v1/admin/status`. Admin-only; always
HTTP 200 (health reflected in the `status` field). New file
`internal/webapi/handle_admin_status.go` + `handle_admin_status_test.go`.

Payload + data sources (all reachable from Router today — no new plumbing):
- `version` ← `r.version`.
- `datastore.status` ← `r.db.Ping(ctx)` ("connected" / "error").
- `datastore.pending_migrations` ← count `*.up.sql` in `migrations.FS()` minus
  `len(r.db.ListAppliedMigrations(ctx))`, floored at 0.
- `credential_storage`: if `r.credentialStore == nil` → `encryption_key_configured:false`
  + zeros. Else `List(ctx)` → `total_credentials` + `credential_types` map;
  `orphaned_credentials` = count where `ReferencedBy(ctx,name)` is empty (table is
  small — N+1 acceptable, matches existing handler comment).
- `collection.next_run_at` ← `collector.ParseSchedule(liveConfig().Collection.Schedule).Next(now)`
  (webapi already imports collector; no cycle). `last_run_at`/`last_run_status` ←
  most-recent run across orgs (max `StartedAt`).
- `organisations[]` ← `ListOrganisations`; per org `credential_source` =
  `ClientKeyCredentialName==""` ? "file" : "database"; `GetLatestCollectionRun` →
  `status`, `last_collected_at`=CompletedAt, `node_count`=TotalNodes (ErrNotFound →
  zero-value/"never collected").
- top-level `status` = "healthy" when datastore connected AND pending==0, else
  "degraded" (missing encryption key alone is NOT degraded — file creds are valid).

TDD (table tests, stub DataStore + stub/ nil credentialStore):
- 501→200; admin-only enforced.
- key-not-configured → `encryption_key_configured:false`, zeros.
- creds present → totals, type breakdown, orphan count.
- per-org file vs database source; no-runs org handled.
- pending_migrations math; degraded when Ping fails / pending>0.

Acceptance: `go test ./internal/webapi/`, `golangci-lint`, `go vet` green; payload
matches the spec example shape.

## Next chunk — orphan-sweep ticker wiring

`StartSweepTicker` (`internal/hypervisor/sweep_ticker.go`) is implemented but never
called from `cmd/.../main.go` → scheduled sweep silently never runs. Wire at
startup, gated on TK enabled + hypervisor configured. NB customer is **vSphere**
(folder scoping is the relevant path) — see [[lab-vs-customer-hypervisor]].

## Queued — SAML config follow-ups (lower priority)
- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
