# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

## Branch map (2026-08-03)

`main` — **v2.18.13** tagged and pushed. Collection is hourly. A few commits sit unpushed on
top of the tag: the git repo owner filter, and the snagging fixes made after it was cut.

`feature/owner-ingest-discovery` — merged into `main` 2026-08-02. **Gate 2 was overridden
deliberately by the product owner**, ahead of the MVP being complete, to get a build deployable
at customer scale and take the measurements below. Later chunks branch fresh from `main`.

`feature/ownership-list-filters` — **merged into `main` 2026-08-03** after sign-off. The
ownership filter on the git repo and cookbook lists (savable as a named cohort, enforced on the
export path), the snagging fixes and migration 0063 found while using it, and the switch that
stops Test Kitchen feeding blocking. 0063 is applied to the dev DB.

**Turn `tk_blocks_readiness` off at the customer site** while vSphere access is gone. It ships
on, so nothing changes until somebody does.

**If a rollback is ever needed:** there is **no down-migration runner** — only `MigrateUp` — so a
schema rollback is `psql` by hand or a restore. Take a `pg_dump` before any deploy. Two
migrations leave a residue the old binary reads:

- **0059** — `owner_aliases` back-filled from `owners.contact_email`; its down script removes
  exactly those rows.
- **0063** — git repo assignments re-keyed from the git URL to the repo name. An older binary
  reads three of those paths by URL, so it would show those repos as unowned again. The down
  script rewrites them back, but it is **not a true inverse**: it cannot tell a row it rewrote
  from one the import always held by name, and the redundant duplicates it removed are gone.

## DEADLINE — the ownership MVP must be complete TODAY, Monday 2026-08-03

Set by the product owner. **Scope is ownership only.** Deploy access is bureaucratic to
arrange, so the work is batched into one release rather than shipped piecemeal.

Two things are named as not done:

- **MSSQL ingest — half done, and the next session picks it up from here.**

  **Done:** `internal/ownershipsql` reads ownership rows from a database as an
  `ownershipimport.RowSource`, so everything above the source abstraction — the row cap, the
  value filter, the distinct-value cap, report truncation — applies to a query result with no
  change. It supports `sqlserver` and `postgres`, registers both drivers itself, verifies the
  connection before running the query (an unreadable source must not read as an empty one),
  and renders NULL as empty. Functional tests run against PostgreSQL, which needs no SQL
  Server to hand; the SQL Server path differs only in driver name and connection string.

  **The dependency is settled — do not re-litigate it.** `github.com/microsoft/go-mssqldb`
  v1.10.0, pinned. It adds **4 modules** to `go.mod` (the driver plus `golang-sql/civil`,
  `golang-sql/sqlexp`, `shopspring/decimal`) and **compiles no Azure or Kerberos code** —
  `go list -deps` shows none. `go.sum` does gain 12 Azure/Kerberos checksum lines, so expect
  scanner advisories about code that never runs; that is the same false-positive pattern as
  the settled Dependabot entries. `govulncheck` is clean. The archived `denisenkom` driver is
  lighter but unmaintained, so it was rejected.

  **What is left, in order:**
  1. **Credentials.** The DSN carries a password, so it belongs in the encrypted config store
     like every other secret — never in `config.yaml`, never returned to a client, never
     logged. `Config.DSN` is already documented that way.
  2. **Endpoints.** The intake already has profile/preview/run over a file; each opens its own
     source, which is why the SQL source is a single-pass cursor. Add the database as an
     alternative source for the same three, rather than a parallel flow.
  3. **UI.** Choose file or database; enter the connection; supply a query; preview what it
     returns; then the existing mapping flow, unchanged.
  4. **A SQL Server to test against.** Run one in docker locally. Do not defer this for want
     of the customer's database — the whole point is that it is testable here.
- **Node ownership — DONE.** The list carries the ownership control, the API already resolved
  it, and the import always accepted `node`. Local data seeded in the dev DB: 8 nodes across 3
  owners, 5 unowned, so both questions show something.

## NOW — the ownership MVP (`plans/ownership-work-attribution.md`)

Work order and journeys live in that plan; per-chunk scope lives in
`plans/todo-ownership.md`. Do not re-plan either.

**Chunks 1–3 are partly built.** Identity and alias management and the failure register are
shipped. **Owner ingest is not: it reads a file, and the SQL source has never been built.**
Recording it as shipped is what hid the gap — the open item was in `todo-ownership.md` all
along. Behaviour lives in `specifications/ownership-intake.md`, `ownership-identity.md` and
`failure-register.md`. Three decisions from those chunks still bind:

- Ingest **creates** unresolved people rather than rejecting the row, and a fuzzy candidate does
  not reject it either. Correction is deferred to the point of use, by design.
- **Read `specifications/ownership-identity.md` § Proposed before extending aliases.**
  `alias_type` conflates what shape an identifier is with where it came from, and uniqueness
  includes the provenance, so one address can belong to two owners. Recorded, not fixed.
- A human verdict in the failure register **outranks CookStyle and Test Kitchen** and joins the
  existing per-source verdicts on `node_readiness.blocking_cookbooks` rather than sitting beside
  them.

**Node and git repo matching are probably dead.** The measurement they were waiting on has been
taken against the real estate: **92% of repos carry an owner, and 126 are blocking and unowned**.
Both chunks were scoped assuming ownership was largely absent. 126 is a hand-workable list. Full
numbers and the reasoning are in `plans/todo-ownership.md`; do not start either chunk without
revisiting them.

**But the blocking signal itself is the real problem, and it is worse than "untrustworthy".**
Nothing distinguishes a cookbook that fails to converge from a lab that could not authenticate
or hand out an IP. A Test Kitchen failure rolls up to `tk_status = failed`, and readiness treats
any TK failure as incompatible — **overriding a CookStyle pass**. On this estate, where most TK
runs fail on auth or DHCP, lab failures are blocking real nodes. CookStyle offences are
separately reported as badly curated for this work.

**Measured on the estate 2026-08-03: 89% of the Test Kitchen failure signal was never about a
cookbook.** The fix is a config switch that stops Test Kitchen feeding blocking while vSphere
access at the customer site is gone, not a smarter signal. A classifier was built and reverted
the same day as more machinery than the situation needs; it is in git history if Test Kitchen
comes back. Detail: `plans/todo-bulk-kitchen-scanning.md`.

So the 126 **bounds** the unowned work rather than describing it, and the top open question is
no longer ownership — it is whether the blocking list is true at all. Detail and the shape of a
fix: `plans/todo-bulk-kitchen-scanning.md`. The failure register is the only instrument that can
correct it today, one repo at a time, and every entry is also a measurement of how wrong the
signal is.

## NEXT — ownership filtering in the list views

Scope and the decisions that bind: `plans/todo-ownership.md` § Ownership filtering in the list
views.

The git repo and cookbook lists carry the control. Ownership is savable as a named cohort, and
the export path enforces the same rule as the list views. **The node list is in scope for
today** — see the deadline section.

## Snagging (`plans/todo-snagging.md`)

Defects found by the product owner using the shipped app. Faults in what is built, so they come
before new work. Seven found and fixed on 2026-08-02, six of them while importing real data —
none would have come from a code review. Reproduce, write the failing test, then fix.

**Next free migration number: 0064.**

## QUEUED behind the ownership MVP

CC19 target-version preset (`plans/todo-event-ingest.md`) and the general audit log
(`plans/todo-audit.md`). Scope lives there; do not restate it here.

## Dependabot — settled 2026-08-02, do not re-triage

All 12 open alerts were dismissed as **inaccurate**: every one named a version outside its own
advisory's vulnerable range. **The local gates were right and GitHub was wrong**, which is the
reverse of the usual assumption — Trivy scans the lockfile that ships, Dependabot was resolving
the manifest. Do not "fix" this by unpinning `overrides` in `frontend/package.json`; the parent
ranges already permit the patched versions, and several pins exist because the Harness registry
quarantines recent versions.

## Release preconditions (the bump target runs no tests)

1. `make ci` and `make vuln-go` must pass first — `bump-patch-push` does not depend on `ci`.
2. Local `make ci` needs `TRIVY_SKIP_DB_UPDATE=true`: the 1.2GB Trivy DB pull from
   `mirror.gcr.io` stalls mid-transfer (registry handshake is fine; the blob does not
   move). GitHub CI pulls it fresh, so the gap closes on push.
3. The push is a human step — CLAUDE.md forbids remotes. An assistant may bump and tag
   locally, and may push only with explicit, per-action authorisation.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes,
  conflicts with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at`
  stamp that rides with it — complex and risky for a small gain; the duration question is
  answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`).
- **SAML config follow-ups** — empty `username_attr` warning; local-user username
  collision returning an opaque 500.
