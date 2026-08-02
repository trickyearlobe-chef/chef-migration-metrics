# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

## Branch map (2026-08-02)

`main` — **v2.18.13** tagged and pushed. Collection is hourly. A few commits sit unpushed on
top of the tag: the git repo owner filter, and the snagging fixes made after it was cut.

`feature/owner-ingest-discovery` — merged into `main` 2026-08-02 and kept locally. **Gate 2
was overridden deliberately by the product owner**, ahead of the MVP being complete, to get
a build deployable at customer scale and take the measurements that decide whether node and
repo matching are worth building at all. That decision bought the measurement at the cost of
having work on `main` that has not been through a full MVP sign-off — if it has to come out,
see the backout note below. Later chunks should branch fresh from `main`.

**If a rollback is ever needed:** redeploy the previous package, and note there is **no
down-migration runner** — only `MigrateUp` exists, so a schema rollback is `psql` by hand or a
restore. Take a `pg_dump` before any deploy. Only migration 0059 leaves a residue the old binary
reads (`owner_aliases` back-filled from `owners.contact_email`); its down script removes exactly
those rows.

## NOW — the ownership MVP (`plans/ownership-work-attribution.md`)

Work order and journeys live in that plan; per-chunk scope lives in
`plans/todo-ownership.md`. Do not re-plan either.

**Chunks 1–3 are built and shipped** (owner ingest, identity and alias management, the failure
register). Behaviour lives in `specifications/ownership-intake.md`, `ownership-identity.md` and
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

**The 126 are mostly CookStyle-blocked with Test Kitchen untested**, so they are unverified
claims rather than repos needing owners — which is what the failure register is for. Verifying
the highest-impact few comes before hunting any owners.

## NEXT — ownership filtering in the list views

The work in flight, and where a new thread starts. Scope and findings:
`plans/todo-ownership.md` § Ownership filtering in the list views.

Backend is done for git repos, cookbooks and nodes. **Every one of them is missing the UI
control**, which is the entire remaining gap. Order: git repos, then cookbooks (UI only), then
nodes (UI only, deferred — no node ownership data exists yet).

## Snagging (`plans/todo-snagging.md`)

Defects found by the product owner using the shipped app. Faults in what is built, so they come
before new work. Seven found and fixed on 2026-08-02, six of them while importing real data —
none would have come from a code review. Reproduce, write the failing test, then fix.

**Next free migration number: 0063.**

## QUEUED behind the ownership MVP

- **CC19 target-version failing-nodes preset** (`plans/todo-event-ingest.md`) — wiring
  `useTargetChefVersion` into `RunEventsPage`; copy an existing call site, not new behaviour.
- **General audit log** (`plans/todo-audit.md`, spec `specifications/audit-log.md`) — who
  changed config, who triggered a rescan. Proposed, not started.

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
