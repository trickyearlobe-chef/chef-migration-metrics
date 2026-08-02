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

**Backout, should it be needed.** Redeploy the previous package; the code side is that
simple. There is **no down-migration runner** — only `MigrateUp` exists — so a schema
rollback is `psql` by hand or a restore, and a `pg_dump` before deploying is the real
control. Most of it needs no rollback at all: 0056, 0057, 0060 and 0061 are new tables the
old binary ignores, and 0058 only loosened constraints. The one residue is 0059, which
back-fills `owner_aliases` from `owners.contact_email` — old code reads that table, so those
rows would persist. Its down script removes exactly them
(`DELETE FROM owner_aliases WHERE source = 'contact_email'`), leaving hand-recorded ones.

## NOW — the ownership MVP (`plans/ownership-work-attribution.md`)

Work order and journeys live in that plan; per-chunk scope lives in
`plans/todo-ownership.md`. Do not re-plan either.

**Chunk 1, owner ingest — done and reviewed by the product owner, 2026-08-02.** Behaviour is
`specifications/ownership-intake.md`. Two decisions departed from the written plan and were
confirmed in review: unresolved people are created rather than rejected, and a fuzzy
candidate no longer rejects the row. Both stay.

**Chunk 2, identity and alias management — done and reviewed by the product owner, 2026-08-02.**
Aliases editable on the owner's own page; a possible-duplicate-owners view at
`/ownership/duplicates`; and a merge folding one owner into another, moving the aliases and
keeping the folded-away name reachable so a correction survives a re-ingest. Behaviour is
`specifications/ownership-identity.md`.

Reviewed against real data, which found two defects since fixed: the scan had to compare display
names (the only signal that survives a hosting platform rewriting a commit address), and the
committer path was dropping every commit address after a person's first.

**Read `specifications/ownership-identity.md` § Proposed before extending any of this.** A long
design session established that the alias model is wrong in a way that is recorded but not fixed —
`alias_type` conflates what shape an identifier is with where it came from, and uniqueness
includes the provenance, so one address can belong to two owners. The rough edges are all in that
file and in `plans/todo-ownership.md`; none is a blocker for what shipped.

**Chunk 3, the failure register** (`specifications/failure-register.md`) — **built and shipped
(v2.18.13), reviewed in use rather than in a sitting.** Moved ahead of both matching chunks on 2026-08-02 because both automated
blocker signals are currently untrustworthy — CookStyle marks cookbooks blocked that run fine, and
Test Kitchen reports environment failures as cookbook failures. A person's verdict outranks both.
Journeys 4 and 6, declared in scope from the start and previously missing from the work order.

The load-bearing assumption held: `node_readiness.blocking_cookbooks` already carried a per-source
verdicts array, so a human verdict joins it as a fourth source rather than becoming a parallel list.
Seed it with the ten real cookbooks before reviewing — the register is only as good as what is in it.

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

All 12 open alerts were dismissed as **inaccurate**, with the evidence on each alert.
Every one named a version outside its own advisory's vulnerable range: undici 7.28.0
against `< 7.28.0`, brace-expansion 5.0.7 against `< 5.0.7`, postcss 8.5.18 against
`<= 8.5.17`, react-router 7.18.0 against `< 7.18.0`, and react-router-dom 7.18.0 against
an advisory covering only `6.30.2 - 6.30.4`.

**The local gates were right and GitHub was wrong**, which is the reverse of the usual
assumption and the reason this took a while to see. Trivy scans
`frontend/package-lock.json` — the resolved tree that actually ships — and passes;
govulncheck covers reachable Go code and is clean. Dependabot was reporting against
manifest resolution that did not match the lockfile.

**Do not "fix" this by unpinning `overrides` in `frontend/package.json`.** The ranges
their parents ask for (`jsdom` wants `undici ^7.25.0`, `minimatch` wants
`brace-expansion ^5.0.5`) already permit the patched versions, so unpinning changes the
tree not at all — and several other pins there exist because the Harness registry
quarantines very recent versions, so removing them trades a working build for nothing.

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
