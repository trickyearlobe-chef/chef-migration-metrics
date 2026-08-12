# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — the assistant-facing surface (chunk 3)

**Read first:** [asking my assistant why this is failing](../journeys/agent-access.md), then run
`make journey`. One red is left in `internal/webapi/agent_access_journey_test.go`:
`TheAssistantSurfaceIsBuiltIn`. The service hosts nothing an editor assistant can connect to, so
using it would mean deploying a second thing beside it — which cannot happen inside the customer
estate. It has to ship and upgrade with everything else.

The description and the credential it would be built on are both there now, so this chunk is the
protocol endpoint and nothing else.

**Two things it must not re-litigate.** The credential is the account's own, at the account's own
level, and read-only unless its maker chose otherwise. How a caller got in is settled at sign-in
and lives on the session — the surface reads it, never sets it.

**One rule that cost a near-miss, and applies to anything else being tightened:** check whether an
ordinary user can see a control wired to an endpoint *before* hardening it, rather than reasoning
from whether neighbouring endpoints look consistent. Reasoning from neighbours would have broken
the Retry button on the git repo page, which viewers use to re-run a test that failed on DHCP or
auth. Detail: `plans/todo-tech-debt.md`.

**Still open, nobody has decided:** a feature switched off at runtime is still described, so an
assistant asks for it and is told it does not exist. Detail: `plans/todo-documentation.md`.

## QUEUED NEXT — the description as a page an operator can read

Nothing renders it today: the document is served at `GET /api/v1/openapi.json` and there is no
page and no renderer dependency. Owner's spec, 2026-08-12: **a side tab, visible to operators and
admins only.** Bundled, not a CDN; lazy-loaded; supply-chain checked before anything is added.

**Try-it-out stays off** — the page is served to a signed-in person, so a "try" button fires real
calls as them, and the destructive ones are one click from a document people open to browse.

**Decide first, because it changes the work:** the document itself is `r.protect` — any
authenticated session, viewers included. An operator-only tab over a viewer-readable document
hides clutter but does not stop a viewer enumerating the surface. Either is defensible; leaving
the document open matters if an assistant holding a viewer's credential is meant to work at all.
Ask the owner rather than inferring from the tab's role.

Sized as a small chunk, and it does not block the surface above.

## The top open question is not ownership — it is whether the blocking list is true

Nothing distinguishes a cookbook that fails to converge from a lab that could not authenticate or
hand out an IP, and readiness treats any Test Kitchen failure as incompatible, **overriding a
CookStyle pass**. Measured on the estate 2026-08-03: **89% of the Test Kitchen failure signal was
never about a cookbook.**

**Standing action: turn `tk_blocks_readiness` off at the customer site** while vSphere access is
gone. It ships on, so nothing changes until somebody does it.

The failure register is the only instrument that corrects this today, one repo at a time, and
every entry is also a measurement of how wrong the signal is. Detail and the shape of a fix:
`plans/todo-bulk-kitchen-scanning.md`.

## QUEUED

- Ownership: `plans/todo-ownership.md` — identity matching for "my stuff" is the live one. The
  92% repo-ownership figure is **inflated** (about half are assigned to one person as a stand-in
  for unknown); genuine coverage is nearer 45%. Do not plan against 92% until re-measured.
- Snagging: `plans/todo-snagging.md` — defects in the shipped app, ahead of new work. Reproduce,
  write the failing test, then fix.
- CC19 target-version preset: `plans/todo-event-ingest.md`.
- General audit log: `plans/todo-audit.md`. Note this now overlaps chunk 2 — recording *how*
  something got in is decided; recording *what it did* is not.

## Operational facts that bite

**Next free migration number: 0069.**

**There is no down-migration runner** — only `MigrateUp` — so a schema rollback is `psql` by hand
or a restore. Take a `pg_dump` before any deploy. Two migrations leave a residue an older binary
reads: **0059** (`owner_aliases` back-filled from `owners.contact_email`) and **0063** (git repo
assignments re-keyed from URL to repo name). 0063's down script is **not a true inverse** — it
cannot tell a row it rewrote from one always held by name.

**Release preconditions — the bump target runs no tests.** `make ci` and `make vuln-go` must pass
first; `bump-patch-push` does not depend on `ci`. **Do not set `TRIVY_SKIP_DB_UPDATE=true` for
`make ci`** — it makes the run fail. Trivy rejects that flag alongside `--download-db-only`, which
is what the DB-refresh step uses, so the refresh dies before any scan happens. Plain `make ci`
works; the refresh is already bounded and retried. The push is a human step —
CLAUDE.md forbids remotes, so an assistant may bump and tag locally and push only with explicit
per-action authorisation.

**Dependabot — settled 2026-08-02, do not re-triage.** All 12 alerts were dismissed as inaccurate:
every one named a version outside its own advisory's vulnerable range. The local gates were right
and GitHub was wrong. Do not "fix" it by unpinning `overrides` in `frontend/package.json` — the
parent ranges already permit the patched versions, and several pins exist because the Harness
registry quarantines recent versions.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes, conflicts
  with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at` stamp that
  rides with it — the duration question is answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`).
- **SAML config follow-ups** — empty `username_attr` warning; local-user username collision
  returning an opaque 500.
